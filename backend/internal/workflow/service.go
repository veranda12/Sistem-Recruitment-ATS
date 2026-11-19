package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"ats/db/sqlc"
	"ats/internal/platform"

	"github.com/google/uuid"
)

// Publish memvalidasi draft lalu menulis versi immutable, memateri stage, dan mengaktifkannya untuk lamaran baru.
// Dipanggil di dalam transaksi; lock workflow mencegah dua publish bersamaan menghasilkan nomor versi ganda.
func Publish(ctx context.Context, q *sqlc.Queries, workflowID uuid.UUID, expectedRevision int32, actor *uuid.UUID, notes, requestID string) (sqlc.WorkflowVersion, error) {
	w, err := q.LockWorkflow(ctx, workflowID)
	if platform.IsNoRows(err) {
		return sqlc.WorkflowVersion{}, platform.ErrNotFound("Workflow tidak ditemukan.")
	}
	if err != nil {
		return sqlc.WorkflowVersion{}, err
	}
	if expectedRevision != 0 && w.DraftRevision != expectedRevision {
		return sqlc.WorkflowVersion{}, platform.ErrConflict("stale_version", "Draft sudah diubah orang lain. Muat ulang sebelum menerbitkan.")
	}
	var def Definition
	if err := json.Unmarshal(w.DraftDefinition, &def); err != nil {
		return sqlc.WorkflowVersion{}, platform.ErrBadRequest("Draft workflow tidak dapat dibaca.")
	}
	if issues := Validate(def); len(issues) > 0 {
		e := platform.ErrConflict("workflow_invalid", "Workflow belum valid. Perbaiki temuan sebelum menerbitkan.")
		e.Details = issues
		return sqlc.WorkflowVersion{}, e
	}
	canonical, _ := json.Marshal(def)
	sum := sha256.Sum256(canonical)
	no, err := q.NextWorkflowVersionNo(ctx, w.ID)
	if err != nil {
		return sqlc.WorkflowVersion{}, err
	}
	v, err := q.InsertWorkflowVersion(ctx, sqlc.InsertWorkflowVersionParams{OrgID: w.OrgID, WorkflowID: w.ID, VersionNo: no,
		Definition: canonical, Checksum: sum[:], Notes: notes, PublishedBy: actor})
	if err != nil {
		return sqlc.WorkflowVersion{}, err
	}
	for i, s := range def.Stages {
		var outcome *string
		if s.Outcome != "" {
			o := s.Outcome
			outcome = &o
		}
		if err := q.InsertWorkflowStage(ctx, sqlc.InsertWorkflowStageParams{WorkflowVersionID: v.ID, Key: s.Key, Name: s.Name,
			Type: s.Type, TerminalOutcome: outcome, Position: int32(i)}); err != nil {
			return sqlc.WorkflowVersion{}, err
		}
	}
	if err := q.SetActiveWorkflowVersion(ctx, sqlc.SetActiveWorkflowVersionParams{ID: w.ID, ActiveVersionID: &v.ID}); err != nil {
		return sqlc.WorkflowVersion{}, err
	}
	if err := q.InsertWorkflowActivation(ctx, sqlc.InsertWorkflowActivationParams{WorkflowID: w.ID, VersionID: v.ID, Action: "publish", ActorUserID: actor, Reason: notes}); err != nil {
		return sqlc.WorkflowVersion{}, err
	}
	kind := "staff"
	if actor == nil {
		kind = "system"
	}
	err = platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: w.OrgID, ActorID: actor, ActorKind: kind, Action: "workflow.publish",
		TargetType: "workflow", TargetID: w.ID.String(), RequestID: requestID,
		Detail: map[string]any{"version": no, "checksum": fmt.Sprintf("%x", sum[:8])}})
	return v, err
}

// Rollback mengaktifkan versi published sebelumnya untuk lamaran baru. Lamaran berjalan tetap pada versinya.
func Rollback(ctx context.Context, q *sqlc.Queries, workflowID, versionID uuid.UUID, actor *uuid.UUID, reason, requestID string) error {
	w, err := q.LockWorkflow(ctx, workflowID)
	if err != nil {
		return err
	}
	v, err := q.GetWorkflowVersion(ctx, versionID)
	if platform.IsNoRows(err) || (err == nil && v.WorkflowID != w.ID) {
		return platform.ErrNotFound("Versi workflow tidak ditemukan.")
	}
	if err != nil {
		return err
	}
	if w.ActiveVersionID != nil && *w.ActiveVersionID == v.ID {
		return platform.ErrConflict("already_active", "Versi tersebut sudah aktif.")
	}
	if err := q.SetActiveWorkflowVersion(ctx, sqlc.SetActiveWorkflowVersionParams{ID: w.ID, ActiveVersionID: &v.ID}); err != nil {
		return err
	}
	if err := q.InsertWorkflowActivation(ctx, sqlc.InsertWorkflowActivationParams{WorkflowID: w.ID, VersionID: v.ID, Action: "rollback", ActorUserID: actor, Reason: reason}); err != nil {
		return err
	}
	return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: w.OrgID, ActorID: actor, ActorKind: "staff", Action: "workflow.rollback",
		TargetType: "workflow", TargetID: w.ID.String(), Reason: reason, RequestID: requestID, Detail: map[string]any{"version": v.VersionNo}})
}

// SimStep adalah satu langkah simulasi. Simulasi memakai evaluator yang sama tanpa side effect.
type SimStep struct {
	Kind  string `json:"kind"` // manual | event | approval | reject
	To    string `json:"to,omitempty"`
	Event string `json:"event,omitempty"`
	Facts Facts  `json:"facts,omitempty"`
}

type SimResult struct {
	Stage   string `json:"stage"`
	EdgeID  string `json:"edgeId,omitempty"`
	Matched string `json:"matched"`
	OK      bool   `json:"ok"`
}

func Simulate(def Definition, steps []SimStep) []SimResult {
	cur := def.Start
	out := []SimResult{{Stage: cur, Matched: "start", OK: true}}
	for _, s := range steps {
		if st, _ := def.Stage(cur); st.Type == "terminal" {
			out = append(out, SimResult{Stage: cur, Matched: "lamaran sudah di tahap akhir", OK: false})
			break
		}
		switch s.Kind {
		case "reject":
			rs, _ := def.RejectStage()
			cur = rs.Key
			out = append(out, SimResult{Stage: cur, Matched: "aksi tolak", OK: true})
		case "manual":
			var found *Edge
			for _, e := range def.OutEdges(cur, "manual") {
				if e.To == s.To {
					e := e
					found = &e
				}
			}
			if found == nil || (found.When != nil && !Eval(*found.When, s.Facts)) {
				out = append(out, SimResult{Stage: cur, Matched: "transisi manual tidak tersedia atau syarat gagal", OK: false})
				continue
			}
			cur = found.To
			out = append(out, SimResult{Stage: cur, EdgeID: found.ID, Matched: "manual", OK: true})
		case "event", "approval":
			trig := s.Kind
			e, why := Decide(&def, cur, trig, s.Event, s.Facts)
			if e == nil {
				out = append(out, SimResult{Stage: cur, Matched: why, OK: false})
				continue
			}
			cur = e.To
			out = append(out, SimResult{Stage: cur, EdgeID: e.ID, Matched: why, OK: true})
		default:
			out = append(out, SimResult{Stage: cur, Matched: "langkah tidak dikenal", OK: false})
		}
	}
	return out
}

// DefaultDefinition: Screening -> Ujian -> Interview -> Persetujuan -> Diterima, dengan cabang review manual dan ditolak.
func DefaultDefinition() Definition {
	passed := &Rule{All: []Rule{
		{Field: "assessment.status", Op: "eq", Value: "released"},
		{Field: "assessment.score", Op: "gte", Value: float64(75)},
	}}
	manual := &Rule{Field: "assessment.pending_manual", Op: "eq", Value: true}
	return Definition{
		Start: "screening",
		Stages: []Stage{
			{Key: "screening", Name: "Screening", Type: "screening"},
			{Key: "ujian", Name: "Ujian", Type: "assessment", AssessmentKey: "tes-dasar"},
			{Key: "review", Name: "Review Penilaian", Type: "screening"},
			{Key: "interview", Name: "Interview", Type: "interview"},
			{Key: "persetujuan", Name: "Persetujuan", Type: "approval", Approval: &ApprovalPolicy{Roles: []string{"hiring_manager"}, Quorum: 1}},
			{Key: "diterima", Name: "Diterima", Type: "terminal", Outcome: "hired"},
			{Key: "ditolak", Name: "Ditolak", Type: "terminal", Outcome: "rejected"},
		},
		Edges: []Edge{
			{ID: "e1", From: "screening", To: "ujian", Order: 1, Trigger: "manual", Label: "Lolos screening"},
			{ID: "e2", From: "ujian", To: "interview", Order: 1, Trigger: "event", Event: "assessment.released", When: passed, Label: "Nilai memenuhi ambang"},
			{ID: "e3", From: "ujian", To: "review", Order: 2, Trigger: "event", Event: "assessment.released", When: manual, Label: "Perlu penilaian manual"},
			{ID: "e4", From: "ujian", To: "ditolak", Order: 3, Trigger: "event", Event: "assessment.released", Default: true, Label: "Nilai di bawah ambang"},
			{ID: "e5", From: "review", To: "interview", Order: 1, Trigger: "manual", Label: "Lanjut interview"},
			{ID: "e6", From: "interview", To: "persetujuan", Order: 1, Trigger: "manual", Label: "Ajukan persetujuan"},
			{ID: "e7", From: "persetujuan", To: "diterima", Order: 1, Trigger: "approval", When: &Rule{Field: "approval.outcome", Op: "eq", Value: "approved"}, Label: "Disetujui"},
			{ID: "e8", From: "persetujuan", To: "ditolak", Order: 2, Trigger: "approval", Default: true, Label: "Tidak disetujui"},
		},
	}
}
