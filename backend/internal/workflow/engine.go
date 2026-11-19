package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"

	"ats/db/sqlc"
	"ats/internal/platform"

	"github.com/google/uuid"
)

const TopicTransitioned = "application.transitioned"

type Actor struct {
	UserID *uuid.UUID
	Kind   string // staff | candidate | system
	Roles  []string
}

// Command adalah satu perintah atau event untuk satu lamaran. ExecutionID unik membuat event duplikat menjadi no-op.
type Command struct {
	ExecutionID     uuid.UUID
	ApplicationID   uuid.UUID
	Kind            string // manual | reject | event | approval | withdraw
	ToStage         string
	ExpectedVersion *int32
	Event           string
	AssessmentKey   string
	Facts           Facts
	Decision        string // approve | reject
	Actor           Actor
	Reason          string
	RequestID       string
}

type Result struct {
	Duplicate bool   `json:"duplicate"`
	Changed   bool   `json:"changed"`
	FromStage string `json:"fromStage"`
	ToStage   string `json:"toStage"`
	Status    string `json:"status"`
	Version   int32  `json:"version"`
	Note      string `json:"note"`
}

func conflict(code, msg string) error { return platform.ErrConflict(code, msg) }

func LoadVersion(ctx context.Context, q *sqlc.Queries, id uuid.UUID) (Definition, error) {
	v, err := q.GetWorkflowVersion(ctx, id)
	if err != nil {
		return Definition{}, err
	}
	var d Definition
	if err := json.Unmarshal(v.Definition, &d); err != nil {
		return Definition{}, fmt.Errorf("definisi workflow versi %s rusak: %w", id, err)
	}
	return d, nil
}

// Execute dijalankan di dalam transaksi caller: lock lamaran, periksa state asal + versi, evaluasi guard,
// tulis approval/transisi/history/outbox secara atomik.
func Execute(ctx context.Context, q *sqlc.Queries, cmd Command) (Result, error) {
	if prev, err := q.GetTransitionByExecution(ctx, cmd.ExecutionID); err == nil {
		return Result{Duplicate: true, FromStage: deref(prev.FromStage), ToStage: prev.ToStage, Status: prev.ToStatus, Note: "eksekusi sudah diproses"}, nil
	} else if !platform.IsNoRows(err) {
		return Result{}, err
	}
	app, err := q.LockApplication(ctx, cmd.ApplicationID)
	if platform.IsNoRows(err) {
		return Result{}, platform.ErrNotFound("Lamaran tidak ditemukan.")
	}
	if err != nil {
		return Result{}, err
	}
	res := Result{FromStage: app.StageKey, ToStage: app.StageKey, Status: app.Status, Version: app.Version}
	if cmd.ExpectedVersion != nil && *cmd.ExpectedVersion != app.Version {
		e := platform.ErrConflict("stale_version", "Lamaran sudah diubah orang lain. Muat ulang untuk melihat data terbaru.")
		e.Details = map[string]any{"currentVersion": app.Version, "stage": app.StageKey, "status": app.Status}
		return res, e
	}
	if app.Status != "active" {
		if cmd.Kind == "event" || cmd.Kind == "approval" {
			res.Note = "lamaran tidak aktif; event diabaikan"
			if cmd.Kind == "approval" {
				return res, conflict("application_inactive", "Lamaran sudah tidak aktif sehingga persetujuan tidak berlaku.")
			}
			return res, nil
		}
		return res, conflict("application_inactive", "Lamaran sudah tidak aktif.")
	}
	def, err := LoadVersion(ctx, q, app.WorkflowVersionID)
	if err != nil {
		return res, err
	}
	cur, _ := def.Stage(app.StageKey)

	var (
		target  Stage
		edge    *Edge
		ruleRef map[string]any
		trigger = cmd.Kind
		status  = app.Status
	)
	switch cmd.Kind {
	case "withdraw":
		target = cur
		status = "withdrawn"
		if err := q.InvalidateApprovals(ctx, app.ID); err != nil {
			return res, err
		}
	case "reject":
		rs, ok := def.RejectStage()
		if !ok {
			return res, conflict("transition_not_allowed", "Workflow tidak memiliki tahap penolakan.")
		}
		target = rs
	case "manual":
		for _, e := range def.OutEdges(app.StageKey, "manual") {
			if e.To == cmd.ToStage {
				e := e
				edge = &e
				break
			}
		}
		if edge == nil {
			return res, conflict("transition_not_allowed", "Perpindahan ke tahap tersebut tidak tersedia dari tahap saat ini.")
		}
		if edge.When != nil && !Eval(*edge.When, cmd.Facts) {
			return res, conflict("guard_failed", "Syarat perpindahan tahap belum terpenuhi.")
		}
		target, _ = def.Stage(edge.To)
	case "event":
		// Hasil ujian hanya memicu transisi pada lamaran yang sedang di tahap ujian dengan kunci paket yang sama.
		if cur.Type != "assessment" || cur.AssessmentKey != cmd.AssessmentKey {
			res.Note = "lamaran tidak berada pada tahap ujian yang sesuai; event diabaikan"
			return res, nil
		}
		var why string
		edge, why = Decide(&def, app.StageKey, "event", cmd.Event, cmd.Facts)
		if edge == nil {
			res.Note = why
			return res, nil
		}
		target, _ = def.Stage(edge.To)
		ruleRef = map[string]any{"edgeId": edge.ID, "matchedBy": why, "rule": edge.When}
	case "approval":
		if cur.Type != "approval" || cur.Approval == nil {
			return res, conflict("not_in_approval", "Lamaran tidak sedang menunggu persetujuan.")
		}
		if cmd.Decision != "approve" && cmd.Decision != "reject" {
			return res, platform.ErrValidation(map[string]string{"decision": "Pilih setujui atau tolak."})
		}
		if cmd.Actor.UserID == nil || !slices.ContainsFunc(cmd.Actor.Roles, func(r string) bool { return slices.Contains(cur.Approval.Roles, r) }) {
			return res, &platform.APIError{Status: http.StatusForbidden, Code: "approval_not_permitted", Message: "Peran Anda tidak termasuk penyetuju tahap ini."}
		}
		seq, err := q.LastStageEntrySeq(ctx, sqlc.LastStageEntrySeqParams{ApplicationID: app.ID, ToStage: app.StageKey})
		if err != nil {
			return res, err
		}
		if !cur.Approval.AllowSelfApproval {
			entry, err := q.ListTransitions(ctx, app.ID)
			if err != nil {
				return res, err
			}
			for _, t := range entry {
				if t.ID == seq && t.ActorUserID != nil && *t.ActorUserID == *cmd.Actor.UserID {
					return res, conflict("self_approval", "Anda memindahkan lamaran ke tahap ini sehingga tidak dapat menyetujuinya sendiri.")
				}
			}
		}
		id, err := q.InsertApproval(ctx, sqlc.InsertApprovalParams{OrgID: app.OrgID, ApplicationID: app.ID, StageKey: app.StageKey,
			StageEntrySeq: seq, ApproverUserID: *cmd.Actor.UserID, Decision: cmd.Decision, Comment: cmd.Reason})
		if platform.IsNoRows(err) || id == uuid.Nil {
			return res, conflict("already_decided", "Anda sudah memberikan keputusan pada tahap ini.")
		}
		if err != nil {
			return res, err
		}
		counts, err := q.CountApprovals(ctx, sqlc.CountApprovalsParams{ApplicationID: app.ID, StageKey: app.StageKey, StageEntrySeq: seq})
		if err != nil {
			return res, err
		}
		outcome := ""
		if counts.Rejects > 0 {
			outcome = "rejected"
		} else if int(counts.Approves) >= cur.Approval.Quorum {
			outcome = "approved"
		}
		if outcome == "" {
			res.Note = fmt.Sprintf("persetujuan tercatat (%d/%d)", counts.Approves, cur.Approval.Quorum)
			res.Changed = false
			return res, nil
		}
		facts := Facts{"approval.outcome": outcome}
		var why string
		edge, why = Decide(&def, app.StageKey, "approval", "", facts)
		if edge == nil {
			return res, conflict("guard_failed", "Tidak ada transisi untuk hasil persetujuan: "+why)
		}
		target, _ = def.Stage(edge.To)
		cmd.Facts = facts
		ruleRef = map[string]any{"edgeId": edge.ID, "matchedBy": why, "quorum": cur.Approval.Quorum, "approves": counts.Approves, "rejects": counts.Rejects}
	default:
		return res, platform.ErrBadRequest("Jenis perintah tidak dikenal.")
	}

	if target.Type == "terminal" {
		status = target.Outcome
	}
	if edge != nil && ruleRef == nil {
		ruleRef = map[string]any{"edgeId": edge.ID}
	}
	updated, err := q.UpdateApplicationState(ctx, sqlc.UpdateApplicationStateParams{ID: app.ID, StageKey: target.Key, Status: status})
	if err != nil {
		return res, err
	}
	var rr []byte
	if ruleRef != nil {
		ruleRef["workflowVersionId"] = app.WorkflowVersionID
		rr, _ = json.Marshal(ruleRef)
	}
	summary, _ := json.Marshal(summarize(cmd.Facts))
	fromStatus := app.Status
	if _, err := q.InsertTransition(ctx, sqlc.InsertTransitionParams{
		OrgID: app.OrgID, ApplicationID: app.ID, ExecutionID: cmd.ExecutionID, WorkflowVersionID: app.WorkflowVersionID,
		FromStage: &app.StageKey, ToStage: target.Key, FromStatus: &fromStatus, ToStatus: status, Trigger: trigger,
		ActorUserID: cmd.Actor.UserID, ActorKind: cmd.Actor.Kind, Reason: cmd.Reason, RuleRef: rr, InputSummary: summary,
		RequestID: strOrNil(cmd.RequestID),
	}); err != nil {
		return res, err
	}
	if _, err := platform.Enqueue(ctx, q, app.OrgID, TopicTransitioned, "application", app.ID.String(), map[string]any{
		"applicationId": app.ID, "executionId": cmd.ExecutionID, "from": app.StageKey, "to": target.Key,
		"fromStatus": fromStatus, "status": status, "candidateId": app.CandidateUserID,
	}); err != nil {
		return res, err
	}
	return Result{Changed: true, FromStage: app.StageKey, ToStage: target.Key, Status: status, Version: updated.Version}, nil
}

// summarize menyimpan ringkasan input tanpa PII: hanya field fakta allowlist.
func summarize(f Facts) map[string]any {
	out := map[string]any{}
	for k, v := range f {
		if _, ok := Fields[k]; ok {
			out[k] = v
		}
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func strOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
