// Package app adalah composition root: merangkai modul menjadi router HTTP. Tidak berisi aturan bisnis.
package app

import (
	"context"
	"net/http"
	"time"

	"ats/db/sqlc"
	"ats/internal/auth"
	"ats/internal/documents"
	"ats/internal/exam"
	"ats/internal/identity"
	"ats/internal/notify"
	"ats/internal/platform"
	"ats/internal/recruitment"
	"ats/internal/reporting"
	"ats/internal/settings"
	"ats/internal/workflow"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Deps struct {
	Cfg     platform.Config
	Pool    *pgxpool.Pool
	OrgID   uuid.UUID
	Limiter *platform.Limiter
}

// ResolveOrg memuat organisasi instalasi (satu org per instalasi).
func ResolveOrg(ctx context.Context, pool *pgxpool.Pool, slug string) (uuid.UUID, error) {
	o, err := sqlc.New(pool).GetOrganizationBySlug(ctx, slug)
	return o.ID, err
}

func NewRouter(d Deps) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	_ = r.SetTrustedProxies(d.Cfg.TrustedProxies)
	r.Use(platform.RequestIDMiddleware(), platform.RecoverMiddleware(), platform.MetricsMiddleware(), platform.AccessLog(), platform.SecurityHeaders(d.Cfg.IsProduction()))

	q := sqlc.New(d.Pool)
	sessions := auth.NewSessions(d.Pool, d.Cfg)
	keys := identity.Keys{EncKey: d.Cfg.NIKEncKey, EncVersion: d.Cfg.NIKEncKeyVer, IndexKey: d.Cfg.NIKIndexKey, IndexVersion: d.Cfg.NIKIndexKeyVer}
	authH := &auth.Handler{S: sessions, OrgID: d.OrgID}
	rec := &recruitment.Handler{Pool: d.Pool, Q: q, OrgID: d.OrgID, Keys: keys,
		Cache: recruitment.NewPublicCache(d.Cfg.RedisURL, d.Limiter.Global)}
	wf := &workflow.Handler{Pool: d.Pool, Q: q, OrgID: d.OrgID}
	inbox := &notify.InboxHandler{Q: q}
	ex := &exam.Handler{Pool: d.Pool, Q: q, OrgID: d.OrgID}
	docs := &documents.Handler{Pool: d.Pool, Q: q, OrgID: d.OrgID, ObjectDir: d.Cfg.ObjectDir, SigningKey: d.Cfg.SessionKey}
	rec.Docs = docs.ListForCandidate
	adm := &settings.Handler{Pool: d.Pool, Q: q, OrgID: d.OrgID, InvalidatePublic: rec.Cache.Invalidate}
	rep := &reporting.Handler{Pool: d.Pool, Q: q, OrgID: d.OrgID, ObjectDir: d.Cfg.ObjectDir, SigningKey: d.Cfg.SessionKey,
		Limiter: d.Limiter, Strict: d.Cfg.StrictSensitiveLimits}

	health := func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) }
	ready := func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		deps := gin.H{"redis": d.Limiter.RedisStatus()}
		if err := d.Pool.Ping(ctx); err != nil {
			deps["database"] = "down"
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable", "dependencies": deps})
			return
		}
		deps["database"] = "ok"
		status := "ok"
		if deps["redis"] == "degraded" {
			status = "degraded"
		}
		c.JSON(http.StatusOK, gin.H{"status": status, "dependencies": deps})
	}
	r.GET("/healthz", health)
	r.GET("/readyz", ready)

	ip := func(c *gin.Context) string { return c.ClientIP() }
	scale := max(d.Cfg.RateLimitScale, 1)
	rl := func(name string, perMin int, key func(*gin.Context) string) gin.HandlerFunc {
		return d.Limiter.RateLimit(name, perMin*scale, time.Minute, key)
	}
	api := r.Group("/api/v1", platform.BodyLimitByRoute(1<<20, map[string]int64{"/api/v1/me/documents": documents.MaxBytes + 64<<10}), sessions.Load(), sessions.CSRF(),
		rl("api", 600, ip))
	interactive := platform.Deadline(10 * time.Second)
	critical := platform.Deadline(5 * time.Second)

	api.GET("/healthz", health)
	api.GET("/readyz", ready)
	api.GET("/org", interactive, rec.PublicOrg)
	api.GET("/jobs", interactive, rec.ListPublicJobs)
	api.GET("/jobs/:slug", interactive, rec.GetPublicJob)

	a := api.Group("/auth", interactive)
	a.GET("/session", authH.Session)
	a.POST("/login", rl("login", 20, ip), authH.Login)
	a.POST("/logout", authH.Logout)
	a.POST("/register", rl("register", 10, ip), rec.Register)
	a.POST("/verify-email", rl("verify", 20, ip), authH.VerifyEmail)
	a.POST("/resend-verification", auth.RequireAuth(), rl("resend", 3, ip), authH.ResendVerification)
	a.POST("/forgot-password", rl("forgot", 5, ip), authH.ForgotPassword)
	a.POST("/reset-password", rl("reset", 10, ip), authH.ResetPassword)

	me := api.Group("/me", auth.RequireKind("candidate"), interactive)
	me.GET("/profile", rec.GetProfile)
	me.PUT("/profile", rec.UpdateProfile)
	me.GET("/applications", rec.MyApplications)
	me.GET("/applications/:id/timeline", rec.MyApplicationTimeline)
	me.POST("/applications/:id/withdraw", rec.Withdraw)
	me.GET("/notifications", inbox.List)
	me.POST("/notifications/:id/read", inbox.MarkRead)
	me.GET("/documents", docs.Mine)
	me.POST("/documents", d.Limiter.RateLimit("upload", 20*max(d.Cfg.RateLimitScale, 1), time.Minute, ip), docs.Upload)
	me.DELETE("/documents/:id", docs.Delete)
	api.POST("/documents/:id/link", auth.RequireAuth(), interactive, docs.Link)
	api.GET("/documents/download", auth.RequireAuth(), docs.Download)
	api.POST("/jobs/:slug/apply", auth.RequireKind("candidate"), critical, rec.Apply)

	// Ujian: start/save/submit diprioritaskan (deadline critical, batch dibatasi); heartbeat best-effort.
	cand := api.Group("", auth.RequireKind("candidate"))
	cand.GET("/me/exam-invitations", interactive, ex.MyInvitations)
	cand.POST("/exam-invitations/:id/start", critical, ex.Start)
	cand.GET("/exam-attempts/:id", interactive, ex.Resume)
	cand.PUT("/exam-attempts/:id/answers", platform.BodyLimit(256<<10), critical, ex.Save)
	cand.POST("/exam-attempts/:id/submit", platform.BodyLimit(256<<10), critical, ex.Submit)
	cand.POST("/exam-attempts/:id/heartbeat", rl("heartbeat", 10, func(c *gin.Context) string { return c.Param("id") }), ex.Heartbeat)

	staff := api.Group("", auth.RequireKind("staff"), interactive)
	staff.GET("/staff/jobs", auth.RequireAnyPerm(auth.PermJobsRead), rec.ListStaffJobs)
	staff.POST("/staff/jobs", auth.RequireAnyPerm(auth.PermJobsManage), rec.CreateJob)
	staff.POST("/staff/jobs/:id/publish", auth.RequireAnyPerm(auth.PermJobsPublish), rec.SetJobStatus("published"))
	staff.POST("/staff/jobs/:id/close", auth.RequireAnyPerm(auth.PermJobsPublish), rec.SetJobStatus("closed"))
	staff.GET("/applications", auth.RequireAnyPerm(auth.PermApplicationsRead), rec.ListApplications)
	staff.GET("/applications/:id", auth.RequireAnyPerm(auth.PermApplicationsRead), rec.GetApplication)
	staff.POST("/applications/:id/transitions", auth.RequireAnyPerm(auth.PermApplicationsMove), rec.Transition)
	staff.POST("/applications/:id/approvals", auth.RequireAnyPerm(auth.PermApprovalsDecide), rec.Approve)
	staff.POST("/applications/:id/reveal-identity", auth.RequireAnyPerm(auth.PermIdentityReveal), rec.RevealIdentity)
	staff.POST("/applications/:id/interviews", auth.RequireAnyPerm(auth.PermApplicationsMove), rec.ScheduleInterview)
	staff.POST("/applications/:id/decision-notice", auth.RequireAnyPerm(auth.PermApplicationsMove), rec.DecisionNotice)
	staff.POST("/interviews/:id/reschedule", auth.RequireAnyPerm(auth.PermApplicationsMove), rec.RescheduleInterview)
	staff.POST("/interviews/:id/cancel", auth.RequireAnyPerm(auth.PermApplicationsMove), rec.CancelInterview)
	staff.POST("/interviews/:id/evaluations", rec.Evaluate)
	staff.GET("/staff/interviews", rec.MyInterviews)
	staff.GET("/staff/interviewers", auth.RequireAnyPerm(auth.PermApplicationsMove), rec.Interviewers)
	staff.GET("/requisitions", rec.ListRequisitions)
	staff.POST("/requisitions", auth.RequireAnyPerm(auth.PermRequisitionsManage), rec.CreateRequisition)
	staff.POST("/requisitions/:id/submit", rec.RequisitionAction("submit"))
	staff.POST("/requisitions/:id/cancel", rec.RequisitionAction("cancel"))
	staff.POST("/requisitions/:id/approve", auth.RequireAnyPerm(auth.PermRequisitionsApprove), rec.RequisitionAction("approve"))
	staff.POST("/requisitions/:id/reject", auth.RequireAnyPerm(auth.PermRequisitionsApprove), rec.RequisitionAction("reject"))

	staff.GET("/workflows", auth.RequireAnyPerm(auth.PermWorkflowsRead), wf.List)
	staff.GET("/workflows/:id", auth.RequireAnyPerm(auth.PermWorkflowsRead), wf.Get)
	staff.PUT("/workflows/:id/draft", auth.RequireAnyPerm(auth.PermWorkflowsManage), wf.SaveDraft)
	staff.POST("/workflows/:id/validate", auth.RequireAnyPerm(auth.PermWorkflowsRead), wf.ValidateDraft)
	staff.POST("/workflows/:id/simulate", auth.RequireAnyPerm(auth.PermWorkflowsRead), wf.SimulateDraft)
	staff.POST("/workflows/:id/publish", auth.RequireAnyPerm(auth.PermWorkflowsPublish), wf.PublishDraft)
	staff.GET("/workflows/:id/versions", auth.RequireAnyPerm(auth.PermWorkflowsRead), wf.Versions)
	staff.POST("/workflows/:id/rollback", auth.RequireAnyPerm(auth.PermWorkflowsPublish), wf.RollbackTo)

	staff.GET("/exam-schedules", auth.RequireAnyPerm(auth.PermExamsManage), ex.ListSchedules)
	staff.POST("/exam-schedules", auth.RequireAnyPerm(auth.PermExamsManage), ex.CreateSchedule)
	staff.POST("/exam-schedules/:id/invitations", ex.Invite)
	staff.GET("/exam-schedules/:id/monitor", auth.RequireAnyPerm(auth.PermExamsManage), ex.Monitor)
	staff.POST("/exam-schedules/:id/release", auth.RequireAnyPerm(auth.PermResultsRelease), ex.Release)
	staff.GET("/exam-attempts/:id/review", auth.RequireAnyPerm(auth.PermExamsGrade), ex.Review)
	staff.POST("/exam-attempts/:id/essay-grades", auth.RequireAnyPerm(auth.PermExamsGrade), ex.Grade)

	// Pelaporan: deadline lebih panjang (background-read).
	reports := api.Group("/reports", auth.RequireKind("staff"), platform.Deadline(30*time.Second))
	reports.GET("/recruitment", auth.RequireAnyPerm(auth.PermReportsRead), rep.Recruitment)
	reports.POST("/exports", rep.CreateExport)
	reports.GET("/exports", rep.MyExports)
	reports.POST("/exports/:id/link", rep.Link)
	reports.GET("/download", rep.Download)

	// Administrasi akses, audit, pengaturan.
	admin := api.Group("/admin", auth.RequireKind("staff"), interactive)
	admin.GET("/users", auth.RequireAnyPerm(auth.PermUsersManage), adm.ListUsers)
	admin.POST("/users", auth.RequireAnyPerm(auth.PermUsersManage), adm.CreateUser)
	admin.POST("/users/:id/assignments", auth.RequireAnyPerm(auth.PermUsersManage), adm.Assign)
	admin.POST("/users/:id/status", auth.RequireAnyPerm(auth.PermUsersManage), adm.SetStatus)
	admin.POST("/assignments/:id/revoke", auth.RequireAnyPerm(auth.PermUsersManage), adm.Revoke)
	admin.GET("/roles", auth.RequireAnyPerm(auth.PermUsersManage), adm.ListRoles)
	admin.GET("/audit", auth.RequireAnyPerm(auth.PermAuditRead), adm.Audit)
	admin.GET("/settings", auth.RequireAnyPerm(auth.PermSettingsManage), adm.ListSettings)
	admin.PUT("/settings/:key", auth.RequireAnyPerm(auth.PermSettingsManage), adm.PutSetting)
	admin.GET("/settings/:key/history", auth.RequireAnyPerm(auth.PermSettingsManage), adm.SettingHistory)
	admin.GET("/organization", auth.RequireAnyPerm(auth.PermSettingsManage), adm.GetOrganization)
	admin.PUT("/organization", auth.RequireAnyPerm(auth.PermSettingsManage), adm.PutOrganization)
	admin.POST("/units/:id/active", auth.RequireAnyPerm(auth.PermSettingsManage), adm.SetMasterActive("unit"))
	admin.POST("/locations/:id/active", auth.RequireAnyPerm(auth.PermSettingsManage), adm.SetMasterActive("location"))
	admin.GET("/deliveries", auth.RequireAnyPerm(auth.PermSettingsManage), adm.Deliveries)
	admin.POST("/outbox/:id/replay", auth.RequireAnyPerm(auth.PermSettingsManage), adm.Replay)

	r.NoRoute(func(c *gin.Context) { platform.Fail(c, platform.ErrNotFound("Alamat tidak ditemukan.")) })
	return r
}
