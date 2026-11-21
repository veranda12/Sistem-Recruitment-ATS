package recruitment

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"time"

	"ats/db/sqlc"
	"ats/internal/auth"
	"ats/internal/platform"
	"ats/internal/workflow"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const TopicApplicationSubmitted = "application.submitted"

type applyResp struct {
	ApplicationID uuid.UUID `json:"applicationId"`
	Stage         string    `json:"stage"`
	AppliedAt     time.Time `json:"appliedAt"`
}

// POST /jobs/:slug/apply. Status lowongan diperiksa langsung di transaksi (bukan dari cache).
// FOR SHARE pada lowongan membuat apply dan close berurutan sesuai transaksi yang lebih dulu memegang lock.
func (h *Handler) Apply(c *gin.Context) {
	p := auth.Current(c)
	ctx := c.Request.Context()
	job, err := h.Q.GetJobBySlug(ctx, sqlc.GetJobBySlugParams{OrgID: h.OrgID, Slug: c.Param("slug")})
	if platform.IsNoRows(err) || (err == nil && job.Status == "draft") {
		platform.Fail(c, platform.ErrNotFound("Lowongan tidak ditemukan."))
		return
	}
	if err != nil {
		platform.Fail(c, err)
		return
	}
	idem, err := platform.IdemFromRequest(c, p.UserID, "apply", job.ID.String(), gin.H{"job": job.ID})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	var resp applyResp
	var replay json.RawMessage
	var replayStatus int
	err = platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		if st, body, found, err := platform.IdemLookup(ctx, q, idem); err != nil || found {
			replay, replayStatus = body, st
			return err
		}
		if !p.EmailVerified {
			return platform.ErrConflict("email_unverified", "Verifikasi email Anda sebelum melamar.")
		}
		lj, err := q.LockJobForApply(ctx, job.ID)
		if err != nil {
			return err
		}
		if lj.Status != "published" || (lj.ClosesAt != nil && !lj.ClosesAt.After(lj.DbNow)) {
			return platform.ErrConflict("job_closed", "Lowongan ini sudah ditutup dan tidak menerima lamaran.")
		}
		if _, err := q.GetCandidateIdentity(ctx, p.UserID); platform.IsNoRows(err) {
			return platform.ErrConflict("identity_pending", "Identitas Anda sedang diverifikasi manual. Hubungi tim rekrutmen.")
		} else if err != nil {
			return err
		}
		prof, err := h.loadProfile(c, q, p)
		if err != nil {
			return err
		}
		if !prof.Complete {
			e := platform.ErrConflict("profile_incomplete", "Lengkapi profil Anda sebelum melamar.")
			e.Details = map[string]any{"missing": prof.Missing}
			return e
		}
		wf, err := q.GetWorkflow(ctx, lj.WorkflowID)
		if err != nil {
			return err
		}
		if wf.ActiveVersionID == nil {
			return platform.ErrUnavailable("Lowongan belum siap menerima lamaran.", 0)
		}
		def, err := workflow.LoadVersion(ctx, q, *wf.ActiveVersionID)
		if err != nil {
			return err
		}
		app, err := q.InsertApplication(ctx, sqlc.InsertApplicationParams{OrgID: h.OrgID, JobID: lj.ID, CandidateUserID: p.UserID,
			UnitID: lj.UnitID, WorkflowVersionID: *wf.ActiveVersionID, StageKey: def.Start})
		if platform.IsNoRows(err) {
			existing, gerr := q.GetApplicationByJobCandidate(ctx, sqlc.GetApplicationByJobCandidateParams{JobID: lj.ID, CandidateUserID: p.UserID})
			e := platform.ErrConflict("already_applied", "Anda sudah melamar lowongan ini.")
			if gerr == nil {
				e.Details = map[string]any{"applicationId": existing.ID}
			}
			return e
		}
		if err != nil {
			return err
		}
		// Snapshot profil saat melamar. Perubahan profil berikutnya tidak mengubah snapshot ini (trigger immutable).
		snap := map[string]any{"email": p.Email, "profile": prof.profileDTO, "nikMasked": prof.Identity.NIKMasked,
			"privacyNoticeVersion": PrivacyNoticeVersion}
		sb, _ := json.Marshal(snap)
		if err := q.InsertApplicationSnapshot(ctx, sqlc.InsertApplicationSnapshotParams{ApplicationID: app.ID, Profile: sb}); err != nil {
			return err
		}
		status := "active"
		if _, err := q.InsertTransition(ctx, sqlc.InsertTransitionParams{OrgID: h.OrgID, ApplicationID: app.ID, ExecutionID: uuid.New(),
			WorkflowVersionID: app.WorkflowVersionID, ToStage: def.Start, ToStatus: status, Trigger: "apply",
			ActorUserID: &p.UserID, ActorKind: "candidate", InputSummary: []byte("{}"), RequestID: strp(platform.RequestID(c))}); err != nil {
			return err
		}
		if _, err := platform.Enqueue(ctx, q, h.OrgID, TopicApplicationSubmitted, "application", app.ID.String(),
			gin.H{"applicationId": app.ID, "candidateId": p.UserID, "jobId": lj.ID}); err != nil {
			return err
		}
		if err := platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "candidate",
			Action: "application.submit", TargetType: "application", TargetID: app.ID.String(), ScopeUnit: &lj.UnitID,
			RequestID: platform.RequestID(c), Detail: map[string]any{"job_id": lj.ID, "workflow_version_id": app.WorkflowVersionID}}); err != nil {
			return err
		}
		resp = applyResp{ApplicationID: app.ID, Stage: def.Start, AppliedAt: app.AppliedAt}
		return platform.IdemStore(ctx, q, idem, http.StatusCreated, resp)
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	if replay != nil {
		platform.WriteRaw(c, replayStatus, replay)
		return
	}
	c.JSON(http.StatusCreated, resp)
}

// GET /me/applications: hanya milik kandidat yang login.
func (h *Handler) MyApplications(c *gin.Context) {
	rows, err := h.Q.ListCandidateApplications(c.Request.Context(), auth.Current(c).UserID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}

// GET /me/applications/:id/timeline: histori tahap kandidat tanpa alasan internal staf.
func (h *Handler) MyApplicationTimeline(c *gin.Context) {
	p := auth.Current(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		platform.Fail(c, platform.ErrNotFound(""))
		return
	}
	ctx := c.Request.Context()
	app, err := h.Q.GetApplication(ctx, id)
	if platform.IsNoRows(err) || (err == nil && app.CandidateUserID != p.UserID) {
		platform.Fail(c, platform.ErrNotFound("Lamaran tidak ditemukan."))
		return
	}
	if err != nil {
		platform.Fail(c, err)
		return
	}
	ts, err := h.Q.ListTransitions(ctx, id)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	def, err := workflow.LoadVersion(ctx, h.Q, app.WorkflowVersionID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	type item struct {
		Stage     string    `json:"stage"`
		StageName string    `json:"stageName"`
		Status    string    `json:"status"`
		At        time.Time `json:"at"`
	}
	items := []item{}
	for _, t := range ts {
		st, _ := def.Stage(t.ToStage)
		items = append(items, item{t.ToStage, st.Name, t.ToStatus, t.CreatedAt})
	}
	c.JSON(http.StatusOK, gin.H{"jobTitle": app.JobTitle, "status": app.Status, "stageName": app.StageName, "items": items, "version": app.Version})
}

type withdrawReq struct {
	Reason  string `json:"reason"`
	Version int32  `json:"version"`
}

// POST /me/applications/:id/withdraw. Approval yang tertunda menjadi tidak berlaku.
func (h *Handler) Withdraw(c *gin.Context) {
	p := auth.Current(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		platform.Fail(c, platform.ErrNotFound(""))
		return
	}
	var req withdrawReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	if len(req.Reason) > 500 {
		platform.Fail(c, platform.ErrValidation(map[string]string{"reason": "Alasan maksimal 500 karakter."}))
		return
	}
	ctx := c.Request.Context()
	var res workflow.Result
	err = platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		app, err := q.LockApplication(ctx, id)
		if platform.IsNoRows(err) || (err == nil && app.CandidateUserID != p.UserID) {
			return platform.ErrNotFound("Lamaran tidak ditemukan.")
		}
		if err != nil {
			return err
		}
		res, err = workflow.Execute(ctx, q, workflow.Command{ExecutionID: uuid.New(), ApplicationID: id, Kind: "withdraw",
			ExpectedVersion: &req.Version, Actor: workflow.Actor{UserID: &p.UserID, Kind: "candidate"}, Reason: req.Reason,
			RequestID: platform.RequestID(c)})
		if err != nil {
			return err
		}
		if err := cancelOpenInterviews(c, q, h.OrgID, p.UserID, id, "Lamaran ditarik kandidat"); err != nil {
			return err
		}
		return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "candidate",
			Action: "application.withdraw", TargetType: "application", TargetID: id.String(), ScopeUnit: &app.UnitID,
			Reason: req.Reason, RequestID: platform.RequestID(c)})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// ---- Staf ----

// GET /applications: scope diterapkan di query (deny-by-default).
func (h *Handler) ListApplications(c *gin.Context) {
	p := auth.Current(c)
	s := p.Scope(auth.PermApplicationsRead)
	size, err := pageSize(c, 25, 100)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	cur, err := decodeCursor(c.Query("cursor"))
	if err != nil {
		platform.Fail(c, err)
		return
	}
	params := sqlc.ListStaffApplicationsParams{OrgID: h.OrgID, OrgWide: s.OrgWide, UnitIds: s.Units, JobIds: s.Jobs,
		Status: optStr(c, "status"), StageKey: optStr(c, "stage"), PageSize: size + 1}
	if st := params.Status; st != nil && !slices.Contains([]string{"active", "withdrawn", "rejected", "hired"}, *st) {
		platform.Fail(c, platform.ErrValidation(map[string]string{"status": "Status tidak dikenal."}))
		return
	}
	if j := c.Query("job"); j != "" {
		jid, err := uuid.Parse(j)
		if err != nil {
			platform.Fail(c, platform.ErrValidation(map[string]string{"job": "ID lowongan tidak valid."}))
			return
		}
		params.JobID = &jid
	}
	if cur != nil {
		params.CursorAppliedAt, params.CursorID = &cur.T, &cur.ID
	}
	rows, err := h.Q.ListStaffApplications(c.Request.Context(), params)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	var next *string
	if len(rows) > int(size) {
		last := rows[size-1]
		s := encodeCursor(last.AppliedAt, last.ID)
		next = &s
		rows = rows[:size]
	}
	c.JSON(http.StatusOK, gin.H{"items": rows, "nextCursor": next})
}

// loadScopedApplication memuat lamaran dan memeriksa scope; di luar scope dijawab 404 agar ID tidak bisa ditebak.
func (h *Handler) loadScopedApplication(c *gin.Context, perm string) (sqlc.GetApplicationRow, *auth.Principal, bool) {
	p := auth.Current(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		platform.Fail(c, platform.ErrNotFound("Lamaran tidak ditemukan."))
		return sqlc.GetApplicationRow{}, p, false
	}
	app, err := h.Q.GetApplication(c.Request.Context(), id)
	if platform.IsNoRows(err) || (err == nil && !p.Can(auth.PermApplicationsRead, auth.Resource{Unit: app.UnitID, Job: &app.JobID})) {
		platform.Fail(c, platform.ErrNotFound("Lamaran tidak ditemukan."))
		return app, p, false
	}
	if err != nil {
		platform.Fail(c, err)
		return app, p, false
	}
	if perm != auth.PermApplicationsRead && !p.Can(perm, auth.Resource{Unit: app.UnitID, Job: &app.JobID}) {
		platform.Fail(c, platform.ErrForbidden())
		return app, p, false
	}
	return app, p, true
}

type actionDTO struct {
	Kind  string `json:"kind"`
	To    string `json:"to,omitempty"`
	Label string `json:"label"`
}

// GET /applications/:id
func (h *Handler) GetApplication(c *gin.Context) {
	app, p, ok := h.loadScopedApplication(c, auth.PermApplicationsRead)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	snap, err := h.Q.GetApplicationSnapshot(ctx, app.ID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	ts, err := h.Q.ListTransitions(ctx, app.ID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	approvals, err := h.Q.ListApprovals(ctx, app.ID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	others, err := h.Q.ListOtherApplicationsOfCandidate(ctx, sqlc.ListOtherApplicationsOfCandidateParams{CandidateUserID: app.CandidateUserID, ID: app.ID})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	// Lamaran lain kandidat hanya ditampilkan bila dalam scope staf; sisanya dihitung tanpa detail.
	visible := []sqlc.ListOtherApplicationsOfCandidateRow{}
	hidden := 0
	for _, o := range others {
		if p.Can(auth.PermApplicationsRead, auth.Resource{Unit: o.UnitID, Job: &o.JobID}) {
			visible = append(visible, o)
		} else {
			hidden++
		}
	}
	def, err := workflow.LoadVersion(ctx, h.Q, app.WorkflowVersionID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	docs := []sqlc.ListOwnerDocumentsRow{}
	if h.Docs != nil {
		if docs, err = h.Docs(ctx, app.CandidateUserID); err != nil {
			platform.Fail(c, err)
			return
		}
	}
	ivs, err := h.Q.ListInterviewsForApplication(ctx, app.ID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	evals, err := h.Q.ListEvaluationsForApplication(ctx, app.ID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	actions := []actionDTO{}
	res := auth.Resource{Unit: app.UnitID, Job: &app.JobID}
	if app.Status == "active" {
		cur, _ := def.Stage(app.StageKey)
		if p.Can(auth.PermApplicationsMove, res) {
			for _, e := range def.OutEdges(app.StageKey, "manual") {
				actions = append(actions, actionDTO{Kind: "manual", To: e.To, Label: e.Label})
			}
			actions = append(actions, actionDTO{Kind: "reject", Label: "Tolak"})
		}
		if cur.Type == "approval" && cur.Approval != nil && p.Can(auth.PermApprovalsDecide, res) {
			roles := p.RolesFor(auth.PermApprovalsDecide, res)
			if slices.ContainsFunc(roles, func(r string) bool { return slices.Contains(cur.Approval.Roles, r) }) {
				actions = append(actions, actionDTO{Kind: "approve", Label: "Setujui"}, actionDTO{Kind: "decline", Label: "Tidak setujui"})
			}
		}
	}
	type stageDTO struct {
		Key  string `json:"key"`
		Name string `json:"name"`
		Type string `json:"type"`
	}
	stages := []stageDTO{}
	for _, s := range def.Stages {
		stages = append(stages, stageDTO{s.Key, s.Name, s.Type})
	}
	c.JSON(http.StatusOK, gin.H{
		"id": app.ID, "jobId": app.JobID, "jobTitle": app.JobTitle, "unitName": app.UnitName, "status": app.Status,
		"stage": gin.H{"key": app.StageKey, "name": app.StageName, "type": app.StageType}, "appliedAt": app.AppliedAt,
		"stageEnteredAt": app.StageEnteredAt, "version": app.Version, "shortlisted": app.Shortlisted,
		"candidate": gin.H{"id": app.CandidateUserID, "name": app.CandidateName, "email": app.CandidateEmail},
		"snapshot":  json.RawMessage(snap.Profile), "snapshotAt": snap.CapturedAt,
		"history": ts, "approvals": approvals, "otherApplications": visible, "hiddenOtherApplications": hidden,
		"actions": actions, "stages": stages, "canRevealIdentity": p.Can(auth.PermIdentityReveal, res), "documents": docs,
		"interviews": ivs, "evaluations": evals, "canSchedule": app.Status == "active" && p.Can(auth.PermApplicationsMove, res),
		"decisionNotifiedAt": app.DecisionNotifiedAt,
	})
}

type transitionReq struct {
	Kind            string `json:"kind"` // manual | reject
	To              string `json:"to"`
	Reason          string `json:"reason"`
	ExpectedVersion int32  `json:"expectedVersion"`
}

// POST /applications/:id/transitions: izin + scope, expected version, guard; atomik dengan history dan outbox.
func (h *Handler) Transition(c *gin.Context) {
	app, p, ok := h.loadScopedApplication(c, auth.PermApplicationsMove)
	if !ok {
		return
	}
	var req transitionReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	f := map[string]string{}
	if req.Kind != "manual" && req.Kind != "reject" {
		f["kind"] = "Pilih tindakan."
	}
	if req.Kind == "reject" && len(req.Reason) < 5 {
		f["reason"] = "Alasan penolakan wajib diisi minimal 5 karakter."
	}
	if len(req.Reason) > 1000 {
		f["reason"] = "Alasan maksimal 1000 karakter."
	}
	if len(f) > 0 {
		platform.Fail(c, platform.ErrValidation(f))
		return
	}
	h.execute(c, p, app, workflow.Command{Kind: req.Kind, ToStage: req.To, ExpectedVersion: &req.ExpectedVersion, Reason: req.Reason}, "application.transition")
}

type approvalReq struct {
	Decision        string `json:"decision"` // approve | reject
	Comment         string `json:"comment"`
	ExpectedVersion int32  `json:"expectedVersion"`
}

// POST /applications/:id/approvals
func (h *Handler) Approve(c *gin.Context) {
	app, p, ok := h.loadScopedApplication(c, auth.PermApprovalsDecide)
	if !ok {
		return
	}
	var req approvalReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	if len(req.Comment) > 2000 {
		platform.Fail(c, platform.ErrValidation(map[string]string{"comment": "Komentar maksimal 2000 karakter."}))
		return
	}
	roles := p.RolesFor(auth.PermApprovalsDecide, auth.Resource{Unit: app.UnitID, Job: &app.JobID})
	h.execute(c, p, app, workflow.Command{Kind: "approval", Decision: req.Decision, ExpectedVersion: &req.ExpectedVersion,
		Reason: strings.TrimSpace(req.Comment), Actor: workflow.Actor{Roles: roles}}, "application.approval")
}

func (h *Handler) execute(c *gin.Context, p *auth.Principal, app sqlc.GetApplicationRow, cmd workflow.Command, action string) {
	ctx := c.Request.Context()
	cmd.ExecutionID = uuid.New()
	cmd.ApplicationID = app.ID
	cmd.Actor.UserID, cmd.Actor.Kind = &p.UserID, "staff"
	cmd.RequestID = platform.RequestID(c)
	var res workflow.Result
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		var err error
		res, err = workflow.Execute(ctx, q, cmd)
		if err != nil {
			return err
		}
		if res.Changed && res.Status != "active" {
			if err := cancelOpenInterviews(c, q, h.OrgID, p.UserID, app.ID, "Lamaran "+res.Status); err != nil {
				return err
			}
		}
		return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff",
			Action: action, TargetType: "application", TargetID: app.ID.String(), ScopeUnit: &app.UnitID, Reason: cmd.Reason,
			RequestID: cmd.RequestID, Detail: map[string]any{"from": res.FromStage, "to": res.ToStage, "status": res.Status, "kind": cmd.Kind, "decision": cmd.Decision}})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

type revealReq struct {
	Reason string `json:"reason"`
}

// POST /applications/:id/reveal-identity: butuh izin identity.reveal, alasan, dan tercatat di audit.
func (h *Handler) RevealIdentity(c *gin.Context) {
	app, p, ok := h.loadScopedApplication(c, auth.PermIdentityReveal)
	if !ok {
		return
	}
	var req revealReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	if len(strings.TrimSpace(req.Reason)) < 10 {
		platform.Fail(c, platform.ErrValidation(map[string]string{"reason": "Tuliskan alasan minimal 10 karakter."}))
		return
	}
	ctx := c.Request.Context()
	var nik string
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		id, err := q.GetCandidateIdentity(ctx, app.CandidateUserID)
		if platform.IsNoRows(err) {
			return platform.ErrNotFound("Identitas kandidat belum tersedia.")
		}
		if err != nil {
			return err
		}
		nik, err = h.Keys.Open(id.NikCiphertext, id.NikNonce, int(id.KeyVersion), h.OrgID[:])
		if err != nil {
			return err
		}
		return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff",
			Action: "identity.reveal", TargetType: "candidate", TargetID: app.CandidateUserID.String(), ScopeUnit: &app.UnitID,
			Reason: strings.TrimSpace(req.Reason), RequestID: platform.RequestID(c), Detail: map[string]any{"application_id": app.ID}})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"nik": nik})
}

func strp(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
