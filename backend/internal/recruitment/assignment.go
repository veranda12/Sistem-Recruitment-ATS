package recruitment

import (
	"net/http"
	"slices"

	"ats/db/sqlc"
	"ats/internal/auth"
	"ats/internal/platform"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type shortlistReq struct {
	Shortlisted     bool  `json:"shortlisted"`
	ExpectedVersion int32 `json:"expectedVersion"`
}

// POST /applications/:id/shortlist: tandai atau lepas shortlist (applications.transition dalam scope).
func (h *Handler) Shortlist(c *gin.Context) {
	app, p, ok := h.loadScopedApplication(c, auth.PermApplicationsMove)
	if !ok {
		return
	}
	var req shortlistReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	var out sqlc.JobApplication
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		row, err := q.SetApplicationShortlist(ctx, sqlc.SetApplicationShortlistParams{ID: app.ID, Shortlisted: req.Shortlisted, Version: req.ExpectedVersion})
		if platform.IsNoRows(err) {
			return platform.ErrConflict("stale_version", "Lamaran sudah diubah orang lain. Muat ulang untuk melihat versi terbaru.")
		}
		if err != nil {
			return err
		}
		out = row
		return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff", Action: "application.shortlist",
			TargetType: "application", TargetID: app.ID.String(), ScopeUnit: &app.UnitID, RequestID: platform.RequestID(c),
			Detail: map[string]any{"shortlisted": req.Shortlisted}})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"shortlisted": out.Shortlisted, "version": out.Version})
}

type recruiterReq struct {
	RecruiterUserID *uuid.UUID `json:"recruiterUserId"` // null = lepas penugasan
	ExpectedVersion int32      `json:"expectedVersion"`
}

// GET /applications/:id/assignable-recruiters: staf yang boleh ditugaskan (applications.assign dalam scope).
func (h *Handler) AssignableRecruiters(c *gin.Context) {
	app, _, ok := h.loadScopedApplication(c, auth.PermApplicationsAssign)
	if !ok {
		return
	}
	rows, err := h.Q.ListAssignableRecruiters(c.Request.Context(), sqlc.ListAssignableRecruitersParams{OrgID: h.OrgID, UnitID: app.UnitID, JobID: app.JobID})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}

// POST /applications/:id/recruiter: tugaskan atau lepas recruiter penanggung jawab lamaran.
func (h *Handler) AssignApplicationRecruiter(c *gin.Context) {
	app, p, ok := h.loadScopedApplication(c, auth.PermApplicationsAssign)
	if !ok {
		return
	}
	var req recruiterReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	if req.RecruiterUserID != nil {
		rows, err := h.Q.ListAssignableRecruiters(ctx, sqlc.ListAssignableRecruitersParams{OrgID: h.OrgID, UnitID: app.UnitID, JobID: app.JobID})
		if err != nil {
			platform.Fail(c, err)
			return
		}
		if !slices.ContainsFunc(rows, func(r sqlc.ListAssignableRecruitersRow) bool { return r.ID == *req.RecruiterUserID }) {
			platform.Fail(c, platform.ErrValidation(map[string]string{"recruiterUserId": "Staf ini tidak memiliki akses untuk menangani lamaran ini."}))
			return
		}
	}
	var out sqlc.JobApplication
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		row, err := q.AssignRecruiter(ctx, sqlc.AssignRecruiterParams{ID: app.ID, RecruiterUserID: req.RecruiterUserID, Version: req.ExpectedVersion})
		if platform.IsNoRows(err) {
			return platform.ErrConflict("stale_version", "Lamaran sudah diubah orang lain. Muat ulang untuk melihat versi terbaru.")
		}
		if err != nil {
			return err
		}
		out = row
		detail := map[string]any{"recruiterUserId": req.RecruiterUserID}
		if app.RecruiterUserID != nil {
			detail["previousRecruiterUserId"] = *app.RecruiterUserID
		}
		return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff", Action: "application.assign_recruiter",
			TargetType: "application", TargetID: app.ID.String(), ScopeUnit: &app.UnitID, RequestID: platform.RequestID(c), Detail: detail})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"recruiterUserId": out.RecruiterUserID, "version": out.Version})
}
