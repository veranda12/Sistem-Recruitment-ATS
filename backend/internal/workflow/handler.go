package workflow

import (
	"encoding/json"
	"net/http"
	"strings"

	"ats/db/sqlc"
	"ats/internal/auth"
	"ats/internal/platform"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	Pool  *pgxpool.Pool
	Q     *sqlc.Queries
	OrgID uuid.UUID
}

func (h *Handler) load(c *gin.Context) (sqlc.Workflow, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		platform.Fail(c, platform.ErrNotFound("Workflow tidak ditemukan."))
		return sqlc.Workflow{}, false
	}
	w, err := h.Q.GetWorkflow(c.Request.Context(), id)
	if platform.IsNoRows(err) || (err == nil && w.OrgID != h.OrgID) {
		platform.Fail(c, platform.ErrNotFound("Workflow tidak ditemukan."))
		return w, false
	}
	if err != nil {
		platform.Fail(c, err)
		return w, false
	}
	return w, true
}

// GET /workflows
func (h *Handler) List(c *gin.Context) {
	rows, err := h.Q.ListWorkflows(c.Request.Context(), h.OrgID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	type item struct {
		ID              uuid.UUID `json:"id"`
		Key             string    `json:"key"`
		Name            string    `json:"name"`
		DraftRevision   int32     `json:"draftRevision"`
		ActiveVersionNo *int32    `json:"activeVersionNo"`
	}
	out := []item{}
	for _, r := range rows {
		out = append(out, item{r.ID, r.Key, r.Name, r.DraftRevision, r.ActiveVersionNo})
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

// GET /workflows/:id: draft + temuan validasi + daftar versi + riwayat aktivasi.
func (h *Handler) Get(c *gin.Context) {
	w, ok := h.load(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	var def Definition
	_ = json.Unmarshal(w.DraftDefinition, &def)
	versions, err := h.Q.ListWorkflowVersions(ctx, w.ID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	acts, err := h.Q.ListWorkflowActivations(ctx, w.ID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	issues := Validate(def)
	if issues == nil {
		issues = []Issue{}
	}
	c.JSON(http.StatusOK, gin.H{"id": w.ID, "key": w.Key, "name": w.Name, "draft": def, "draftRevision": w.DraftRevision,
		"activeVersionId": w.ActiveVersionID, "issues": issues, "versions": versions, "activations": acts,
		"catalog": gin.H{"stageTypes": StageTypes, "triggers": Triggers, "events": Events, "fields": Fields, "ops": opsByType}})
}

type draftReq struct {
	Name          string     `json:"name"`
	Definition    Definition `json:"definition"`
	DraftRevision int32      `json:"draftRevision"`
}

// PUT /workflows/:id/draft: draft boleh belum valid (disimpan dengan temuan), publish yang mewajibkan valid.
func (h *Handler) SaveDraft(c *gin.Context) {
	w, ok := h.load(c)
	if !ok {
		return
	}
	p := auth.Current(c)
	var req draftReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if len(req.Name) < 2 || len(req.Name) > 200 {
		platform.Fail(c, platform.ErrValidation(map[string]string{"name": "Nama workflow 2 sampai 200 karakter."}))
		return
	}
	if len(req.Definition.Stages) > MaxStages || len(req.Definition.Edges) > MaxEdges {
		platform.Fail(c, platform.ErrValidation(map[string]string{"definition": "Workflow melebihi batas ukuran."}))
		return
	}
	ctx := c.Request.Context()
	b, _ := json.Marshal(req.Definition)
	var out sqlc.Workflow
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		var err error
		out, err = q.UpdateWorkflowDraft(ctx, sqlc.UpdateWorkflowDraftParams{ID: w.ID, DraftRevision: req.DraftRevision,
			DraftDefinition: b, Name: req.Name, UpdatedBy: &p.UserID})
		if platform.IsNoRows(err) {
			return platform.ErrConflict("stale_version", "Draft sudah diubah orang lain. Muat ulang sebelum menyimpan.")
		}
		if err != nil {
			return err
		}
		return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff",
			Action: "workflow.draft_save", TargetType: "workflow", TargetID: w.ID.String(), RequestID: platform.RequestID(c),
			Detail: map[string]any{"revision": out.DraftRevision}})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	issues := Validate(req.Definition)
	if issues == nil {
		issues = []Issue{}
	}
	c.JSON(http.StatusOK, gin.H{"draftRevision": out.DraftRevision, "issues": issues})
}

// POST /workflows/:id/validate: memvalidasi definisi kiriman (atau draft tersimpan bila kosong).
func (h *Handler) ValidateDraft(c *gin.Context) {
	w, ok := h.load(c)
	if !ok {
		return
	}
	var req struct {
		Definition *Definition `json:"definition"`
	}
	_ = c.ShouldBindJSON(&req)
	def := req.Definition
	if def == nil {
		def = &Definition{}
		_ = json.Unmarshal(w.DraftDefinition, def)
	}
	issues := Validate(*def)
	if issues == nil {
		issues = []Issue{}
	}
	c.JSON(http.StatusOK, gin.H{"valid": len(issues) == 0, "issues": issues})
}

// POST /workflows/:id/simulate: evaluator yang sama tanpa side effect.
func (h *Handler) SimulateDraft(c *gin.Context) {
	w, ok := h.load(c)
	if !ok {
		return
	}
	var req struct {
		Definition *Definition `json:"definition"`
		Steps      []SimStep   `json:"steps"`
	}
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	if len(req.Steps) > 50 {
		platform.Fail(c, platform.ErrValidation(map[string]string{"steps": "Maksimal 50 langkah simulasi."}))
		return
	}
	def := req.Definition
	if def == nil {
		def = &Definition{}
		_ = json.Unmarshal(w.DraftDefinition, def)
	}
	if issues := Validate(*def); len(issues) > 0 {
		e := platform.ErrConflict("workflow_invalid", "Perbaiki temuan validasi sebelum simulasi.")
		e.Details = issues
		platform.Fail(c, e)
		return
	}
	c.JSON(http.StatusOK, gin.H{"steps": Simulate(*def, req.Steps)})
}

// POST /workflows/:id/publish
func (h *Handler) PublishDraft(c *gin.Context) {
	w, ok := h.load(c)
	if !ok {
		return
	}
	p := auth.Current(c)
	var req struct {
		DraftRevision int32  `json:"draftRevision"`
		Notes         string `json:"notes"`
	}
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	if req.DraftRevision == 0 {
		platform.Fail(c, platform.ErrValidation(map[string]string{"draftRevision": "Revisi draft wajib dikirim."}))
		return
	}
	ctx := c.Request.Context()
	var v sqlc.WorkflowVersion
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var err error
		v, err = Publish(ctx, h.Q.WithTx(tx), w.ID, req.DraftRevision, &p.UserID, strings.TrimSpace(req.Notes), platform.RequestID(c))
		return err
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"versionId": v.ID, "versionNo": v.VersionNo})
}

// GET /workflows/:id/versions
func (h *Handler) Versions(c *gin.Context) {
	w, ok := h.load(c)
	if !ok {
		return
	}
	rows, err := h.Q.ListWorkflowVersions(c.Request.Context(), w.ID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}

// POST /workflows/:id/rollback
func (h *Handler) RollbackTo(c *gin.Context) {
	w, ok := h.load(c)
	if !ok {
		return
	}
	p := auth.Current(c)
	var req struct {
		VersionID uuid.UUID `json:"versionId"`
		Reason    string    `json:"reason"`
	}
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	if len(strings.TrimSpace(req.Reason)) < 5 {
		platform.Fail(c, platform.ErrValidation(map[string]string{"reason": "Alasan rollback wajib diisi."}))
		return
	}
	ctx := c.Request.Context()
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		return Rollback(ctx, h.Q.WithTx(tx), w.ID, req.VersionID, &p.UserID, strings.TrimSpace(req.Reason), platform.RequestID(c))
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"activeVersionId": req.VersionID})
}
