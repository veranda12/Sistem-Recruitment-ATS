// Package app adalah composition root: merangkai modul menjadi router HTTP. Tidak berisi aturan bisnis.
package app

import (
	"context"
	"net/http"
	"time"

	"ats/db/sqlc"
	"ats/internal/auth"
	"ats/internal/identity"
	"ats/internal/notify"
	"ats/internal/platform"
	"ats/internal/recruitment"
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
	r.Use(platform.RequestIDMiddleware(), platform.RecoverMiddleware(), platform.AccessLog(), platform.SecurityHeaders(d.Cfg.IsProduction()))

	q := sqlc.New(d.Pool)
	sessions := auth.NewSessions(d.Pool, d.Cfg)
	keys := identity.Keys{EncKey: d.Cfg.NIKEncKey, EncVersion: d.Cfg.NIKEncKeyVer, IndexKey: d.Cfg.NIKIndexKey, IndexVersion: d.Cfg.NIKIndexKeyVer}
	authH := &auth.Handler{S: sessions, OrgID: d.OrgID}
	rec := &recruitment.Handler{Pool: d.Pool, Q: q, OrgID: d.OrgID, Keys: keys,
		Cache: recruitment.NewPublicCache(d.Cfg.RedisURL, d.Limiter.Global)}
	wf := &workflow.Handler{Pool: d.Pool, Q: q, OrgID: d.OrgID}
	inbox := &notify.InboxHandler{Q: q}

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
	api := r.Group("/api/v1", platform.BodyLimit(1<<20), sessions.Load(), sessions.CSRF(),
		d.Limiter.RateLimit("api", 600, time.Minute, ip))
	interactive := platform.Deadline(10 * time.Second)
	critical := platform.Deadline(5 * time.Second)

	api.GET("/healthz", health)
	api.GET("/readyz", ready)
	api.GET("/org", interactive, rec.PublicOrg)
	api.GET("/jobs", interactive, rec.ListPublicJobs)
	api.GET("/jobs/:slug", interactive, rec.GetPublicJob)

	a := api.Group("/auth", interactive)
	a.GET("/session", authH.Session)
	a.POST("/login", d.Limiter.RateLimit("login", 20, time.Minute, ip), authH.Login)
	a.POST("/logout", authH.Logout)
	a.POST("/register", d.Limiter.RateLimit("register", 10, time.Minute, ip), rec.Register)
	a.POST("/verify-email", d.Limiter.RateLimit("verify", 20, time.Minute, ip), authH.VerifyEmail)
	a.POST("/resend-verification", auth.RequireAuth(), d.Limiter.RateLimit("resend", 3, time.Minute, ip), authH.ResendVerification)
	a.POST("/forgot-password", d.Limiter.RateLimit("forgot", 5, time.Minute, ip), authH.ForgotPassword)
	a.POST("/reset-password", d.Limiter.RateLimit("reset", 10, time.Minute, ip), authH.ResetPassword)

	me := api.Group("/me", auth.RequireKind("candidate"), interactive)
	me.GET("/profile", rec.GetProfile)
	me.PUT("/profile", rec.UpdateProfile)
	me.GET("/applications", rec.MyApplications)
	me.GET("/applications/:id/timeline", rec.MyApplicationTimeline)
	me.POST("/applications/:id/withdraw", rec.Withdraw)
	me.GET("/notifications", inbox.List)
	me.POST("/notifications/:id/read", inbox.MarkRead)
	api.POST("/jobs/:slug/apply", auth.RequireKind("candidate"), critical, rec.Apply)

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

	staff.GET("/workflows", auth.RequireAnyPerm(auth.PermWorkflowsRead), wf.List)
	staff.GET("/workflows/:id", auth.RequireAnyPerm(auth.PermWorkflowsRead), wf.Get)
	staff.PUT("/workflows/:id/draft", auth.RequireAnyPerm(auth.PermWorkflowsManage), wf.SaveDraft)
	staff.POST("/workflows/:id/validate", auth.RequireAnyPerm(auth.PermWorkflowsRead), wf.ValidateDraft)
	staff.POST("/workflows/:id/simulate", auth.RequireAnyPerm(auth.PermWorkflowsRead), wf.SimulateDraft)
	staff.POST("/workflows/:id/publish", auth.RequireAnyPerm(auth.PermWorkflowsPublish), wf.PublishDraft)
	staff.GET("/workflows/:id/versions", auth.RequireAnyPerm(auth.PermWorkflowsRead), wf.Versions)
	staff.POST("/workflows/:id/rollback", auth.RequireAnyPerm(auth.PermWorkflowsPublish), wf.RollbackTo)

	r.NoRoute(func(c *gin.Context) { platform.Fail(c, platform.ErrNotFound("Alamat tidak ditemukan.")) })
	return r
}
