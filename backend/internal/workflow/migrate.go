package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"ats/db/sqlc"
	"ats/internal/auth"
	"ats/internal/platform"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Pemindahan lamaran aktif dari versi workflow lama ke versi aktif. Lamaran berjalan tetap pada versinya sampai
// dipindahkan secara eksplisit lewat alur ini: pratinjau (dry-run) lalu eksekusi dalam satu transaksi.

type migrateReq struct {
	FromVersionID           uuid.UUID         `json:"fromVersionId"`
	Mapping                 map[string]string `json:"mapping"` // kunci tahap lama -> kunci tahap baru; kosong = kunci yang sama
	ExpectedActiveVersionID uuid.UUID         `json:"expectedActiveVersionId"`
	Reason                  string            `json:"reason"`
}

type stagePlan struct {
	FromKey  string   `json:"fromKey"`
	FromName string   `json:"fromName"`
	Count    int64    `json:"count"`
	ToKey    string   `json:"toKey"`
	ToName   string   `json:"toName"`
	Problem  string   `json:"problem,omitempty"`
	Warnings []string `json:"warnings"`
}

type migrationPlan struct {
	From   versionRef  `json:"from"`
	To     versionRef  `json:"to"`
	Stages []stagePlan `json:"stages"`
	Total  int64       `json:"total"`
	Ready  bool        `json:"ready"`
	// Tahap versi baru yang boleh menjadi tujuan (bukan keputusan akhir), untuk pilihan pemetaan.
	Targets []stageRef `json:"targets"`
}

type versionRef struct {
	ID        uuid.UUID `json:"id"`
	VersionNo int32     `json:"versionNo"`
}

type stageRef struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

func (h *Handler) plan(ctx context.Context, q *sqlc.Queries, wf sqlc.Workflow, req migrateReq) (migrationPlan, Definition, error) {
	var out migrationPlan
	if wf.ActiveVersionID == nil {
		return out, Definition{}, platform.ErrConflict("no_active_version", "Workflow belum memiliki versi aktif.")
	}
	if req.FromVersionID == *wf.ActiveVersionID {
		return out, Definition{}, platform.ErrValidation(map[string]string{"fromVersionId": "Lamaran pada versi ini sudah memakai versi aktif."})
	}
	fromV, err := q.GetWorkflowVersion(ctx, req.FromVersionID)
	if platform.IsNoRows(err) || (err == nil && fromV.WorkflowID != wf.ID) {
		return out, Definition{}, platform.ErrValidation(map[string]string{"fromVersionId": "Versi tidak ditemukan pada workflow ini."})
	}
	if err != nil {
		return out, Definition{}, err
	}
	toV, err := q.GetWorkflowVersion(ctx, *wf.ActiveVersionID)
	if err != nil {
		return out, Definition{}, err
	}
	fromDef, err := LoadVersion(ctx, q, fromV.ID)
	if err != nil {
		return out, Definition{}, err
	}
	toDef, err := LoadVersion(ctx, q, toV.ID)
	if err != nil {
		return out, Definition{}, err
	}
	out.From, out.To = versionRef{fromV.ID, fromV.VersionNo}, versionRef{toV.ID, toV.VersionNo}
	out.Stages, out.Targets = []stagePlan{}, []stageRef{}
	for _, s := range toDef.Stages {
		if s.Type != "terminal" {
			out.Targets = append(out.Targets, stageRef{s.Key, s.Name})
		}
	}
	counts, err := q.ActiveApplicationsByVersionStage(ctx, wf.ID)
	if err != nil {
		return out, Definition{}, err
	}
	out.Ready = true
	for _, c := range counts {
		if c.WorkflowVersionID != fromV.ID {
			continue
		}
		old, _ := fromDef.Stage(c.StageKey)
		p := stagePlan{FromKey: c.StageKey, FromName: old.Name, Count: c.N, Warnings: []string{}}
		if p.FromName == "" {
			p.FromName = c.StageKey
		}
		target := req.Mapping[c.StageKey]
		if target == "" {
			target = c.StageKey
		}
		if dst, ok := toDef.Stage(target); !ok {
			p.Problem = "Tahap ini tidak ada di versi baru. Pilih tahap tujuan."
		} else if dst.Type == "terminal" {
			p.Problem = "Tahap keputusan akhir tidak dapat menjadi tujuan pemindahan."
		} else {
			p.ToKey, p.ToName = dst.Key, dst.Name
			if target != c.StageKey && old.Type == "approval" {
				p.Warnings = append(p.Warnings, "Persetujuan yang sudah diberikan pada tahap ini dibatalkan.")
			}
			if target != c.StageKey && old.Type == "assessment" {
				p.Warnings = append(p.Warnings, "Undangan dan hasil ujian tetap ada, tetapi tidak lagi memicu perpindahan tahap otomatis.")
			}
			if dst.Type == "assessment" && target != c.StageKey {
				p.Warnings = append(p.Warnings, "Kandidat perlu diundang ke jadwal ujian secara manual.")
			}
		}
		if p.Problem != "" {
			out.Ready = false
		}
		out.Total += c.N
		out.Stages = append(out.Stages, p)
	}
	if out.Total == 0 {
		out.Ready = false
	}
	return out, toDef, nil
}

// GET /workflows/:id/migrations: lamaran aktif per versi, untuk memilih versi asal.
func (h *Handler) MigrationOverview(c *gin.Context) {
	wf, ok := h.load(c)
	if !ok {
		return
	}
	rows, err := h.Q.ActiveApplicationsByVersionStage(c.Request.Context(), wf.ID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	type ver struct {
		ID        uuid.UUID `json:"id"`
		VersionNo int32     `json:"versionNo"`
		Active    int64     `json:"activeApplications"`
		Current   bool      `json:"current"`
	}
	byID := map[uuid.UUID]*ver{}
	order := []uuid.UUID{}
	for _, r := range rows {
		v := byID[r.WorkflowVersionID]
		if v == nil {
			v = &ver{ID: r.WorkflowVersionID, VersionNo: r.VersionNo, Current: wf.ActiveVersionID != nil && *wf.ActiveVersionID == r.WorkflowVersionID}
			byID[r.WorkflowVersionID] = v
			order = append(order, r.WorkflowVersionID)
		}
		v.Active += r.N
	}
	out := []ver{}
	for _, id := range order {
		out = append(out, *byID[id])
	}
	c.JSON(http.StatusOK, gin.H{"versions": out, "activeVersionId": wf.ActiveVersionID})
}

// POST /workflows/:id/migrations/preview: dry-run tanpa mengubah data.
func (h *Handler) MigrationPreview(c *gin.Context) {
	wf, ok := h.load(c)
	if !ok {
		return
	}
	var req migrateReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	p, _, err := h.plan(c.Request.Context(), h.Q, wf, req)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, p)
}

// POST /workflows/:id/migrations: pindahkan semua lamaran aktif versi asal ke versi aktif dalam satu transaksi.
func (h *Handler) MigrationRun(c *gin.Context) {
	wf, ok := h.load(c)
	if !ok {
		return
	}
	var req migrateReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if len(req.Reason) < 5 {
		platform.Fail(c, platform.ErrValidation(map[string]string{"reason": "Tuliskan alasan pemindahan minimal 5 karakter."}))
		return
	}
	p := auth.Current(c)
	ctx := c.Request.Context()
	var moved int
	var plan migrationPlan
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		cur, err := q.LockWorkflow(ctx, wf.ID)
		if err != nil {
			return err
		}
		if cur.ActiveVersionID == nil || *cur.ActiveVersionID != req.ExpectedActiveVersionID {
			return platform.ErrConflict("stale_version", "Versi aktif workflow berubah. Muat ulang pratinjau.")
		}
		apps, err := q.LockActiveApplicationsOnVersion(ctx, req.FromVersionID)
		if err != nil {
			return err
		}
		var toDef Definition
		plan, toDef, err = h.plan(ctx, q, cur, req)
		if err != nil {
			return err
		}
		if !plan.Ready {
			return platform.ErrConflict("migration_not_ready", "Masih ada tahap yang belum dipetakan. Periksa pratinjau.")
		}
		fromDef, err := LoadVersion(ctx, q, req.FromVersionID)
		if err != nil {
			return err
		}
		summary, _ := json.Marshal(map[string]any{"fromVersion": plan.From.VersionNo, "toVersion": plan.To.VersionNo})
		active := "active"
		rid := platform.RequestID(c)
		for _, a := range apps {
			target := req.Mapping[a.StageKey]
			if target == "" {
				target = a.StageKey
			}
			if _, ok := toDef.Stage(target); !ok {
				return platform.ErrConflict("migration_not_ready", fmt.Sprintf("Tahap %s belum dipetakan.", a.StageKey))
			}
			if err := q.MigrateApplicationVersion(ctx, sqlc.MigrateApplicationVersionParams{ID: a.ID, WorkflowVersionID: plan.To.ID, StageKey: target}); err != nil {
				return err
			}
			if old, _ := fromDef.Stage(a.StageKey); target != a.StageKey && old.Type == "approval" {
				if err := q.InvalidateApprovals(ctx, a.ID); err != nil {
					return err
				}
			}
			from := a.StageKey
			if _, err := q.InsertTransition(ctx, sqlc.InsertTransitionParams{OrgID: a.OrgID, ApplicationID: a.ID, ExecutionID: uuid.New(),
				WorkflowVersionID: plan.To.ID, FromStage: &from, ToStage: target, FromStatus: &active, ToStatus: "active", Trigger: "migration",
				ActorUserID: &p.UserID, ActorKind: "staff", Reason: req.Reason, RuleRef: json.RawMessage("null"), InputSummary: summary, RequestID: &rid}); err != nil {
				return err
			}
			moved++
		}
		return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff", Action: "workflow.migrate_applications",
			TargetType: "workflow", TargetID: wf.ID.String(), Reason: req.Reason, RequestID: rid,
			Detail: map[string]any{"fromVersion": plan.From.VersionNo, "toVersion": plan.To.VersionNo, "moved": moved, "mapping": req.Mapping}})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"moved": moved, "toVersion": plan.To.VersionNo})
}
