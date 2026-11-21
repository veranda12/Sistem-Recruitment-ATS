package recruitment

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"ats/db/sqlc"
	"ats/internal/auth"
	"ats/internal/platform"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	TopicInterviewScheduled   = "interview.scheduled"
	TopicInterviewRescheduled = "interview.rescheduled"
	TopicInterviewCancelled   = "interview.cancelled"
	TopicDecision             = "application.decision"
)

var interviewModes = []string{"onsite", "video", "phone"}

func isOverlap(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23P01"
}

var errInterviewerConflict = platform.ErrConflict("interviewer_conflict", "Salah satu interviewer sudah memiliki interview lain pada waktu tersebut.")

// whenLabel memformat waktu interview di zona yang dipilih, lengkap dengan singkatan zona, untuk notifikasi.
func whenLabel(t time.Time, tz string) string {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	l := t.In(loc)
	months := []string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}
	zone, _ := l.Zone()
	return l.Format("2") + " " + months[l.Month()-1] + " " + l.Format("2006 15:04") + " " + zone + " (" + tz + ")"
}

type scheduleReq struct {
	StartsAt        time.Time   `json:"startsAt"`
	DurationMinutes int         `json:"durationMinutes"`
	Timezone        string      `json:"timezone"`
	Mode            string      `json:"mode"`
	Location        string      `json:"location"`
	InterviewerIDs  []uuid.UUID `json:"interviewerIds"`
	Reason          string      `json:"reason"`
	Version         int32       `json:"version"`
}

func (r scheduleReq) validate(withPanel bool) map[string]string {
	f := map[string]string{}
	if _, err := time.LoadLocation(r.Timezone); err != nil || r.Timezone == "" {
		f["timezone"] = "Zona waktu tidak dikenal (contoh Asia/Jakarta)."
	}
	if r.StartsAt.Before(time.Now().Add(-5 * time.Minute)) {
		f["startsAt"] = "Waktu interview harus di masa depan."
	}
	if r.DurationMinutes < 15 || r.DurationMinutes > 480 {
		f["durationMinutes"] = "Durasi 15 sampai 480 menit."
	}
	if withPanel {
		if !slices.Contains(interviewModes, r.Mode) {
			f["mode"] = "Pilih jenis interview."
		}
		if len(r.Location) > 300 {
			f["location"] = "Lokasi/tautan maksimal 300 karakter."
		}
		if len(r.InterviewerIDs) == 0 || len(r.InterviewerIDs) > 10 {
			f["interviewerIds"] = "Pilih 1 sampai 10 interviewer."
		}
	}
	return f
}

// POST /applications/:id/interviews
func (h *Handler) ScheduleInterview(c *gin.Context) {
	app, p, ok := h.loadScopedApplication(c, auth.PermApplicationsMove)
	if !ok {
		return
	}
	var req scheduleReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	if f := req.validate(true); len(f) > 0 {
		platform.Fail(c, platform.ErrValidation(f))
		return
	}
	if app.Status != "active" {
		platform.Fail(c, platform.ErrConflict("application_inactive", "Lamaran sudah tidak aktif."))
		return
	}
	ctx := c.Request.Context()
	ends := req.StartsAt.Add(time.Duration(req.DurationMinutes) * time.Minute)
	var iv sqlc.Interview
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		var err error
		iv, err = q.InsertInterview(ctx, sqlc.InsertInterviewParams{OrgID: h.OrgID, ApplicationID: app.ID, StartsAt: req.StartsAt, EndsAt: ends,
			Timezone: req.Timezone, Mode: req.Mode, Location: strings.TrimSpace(req.Location), CreatedBy: p.UserID})
		if err != nil {
			return err
		}
		seen := map[uuid.UUID]bool{}
		for _, uid := range req.InterviewerIDs {
			if seen[uid] {
				continue
			}
			seen[uid] = true
			u, err := q.GetStaffUser(ctx, uid)
			if platform.IsNoRows(err) || (err == nil && (u.OrgID != h.OrgID || u.Status != "active")) {
				return platform.ErrValidation(map[string]string{"interviewerIds": "Interviewer tidak dikenal atau nonaktif."})
			}
			if err != nil {
				return err
			}
			if err := q.InsertPanelist(ctx, sqlc.InsertPanelistParams{InterviewID: iv.ID, InterviewerID: uid, StartsAt: req.StartsAt, EndsAt: ends}); err != nil {
				if isOverlap(err) {
					return errInterviewerConflict
				}
				return err
			}
		}
		if err := q.InsertInterviewChange(ctx, sqlc.InsertInterviewChangeParams{InterviewID: iv.ID, Action: "schedule", NewStartsAt: &req.StartsAt,
			Timezone: req.Timezone, Reason: req.Reason, ActorUserID: p.UserID}); err != nil {
			return err
		}
		if _, err := platform.Enqueue(ctx, q, h.OrgID, TopicInterviewScheduled, "interview", iv.ID.String(), gin.H{"applicationId": app.ID,
			"interviewId": iv.ID, "when": whenLabel(req.StartsAt, req.Timezone), "mode": req.Mode, "location": iv.Location}); err != nil {
			return err
		}
		return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff", Action: "interview.schedule",
			TargetType: "application", TargetID: app.ID.String(), ScopeUnit: &app.UnitID, RequestID: platform.RequestID(c),
			Detail: map[string]any{"interview_id": iv.ID, "starts_at": req.StartsAt, "timezone": req.Timezone}})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, iv)
}

// loadInterview memuat interview dan memeriksa scope lamaran terkait.
func (h *Handler) loadInterview(c *gin.Context, q *sqlc.Queries, perm string) (sqlc.Interview, sqlc.GetApplicationRow, error) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return sqlc.Interview{}, sqlc.GetApplicationRow{}, platform.ErrNotFound("Interview tidak ditemukan.")
	}
	ctx := c.Request.Context()
	iv, err := q.LockInterview(ctx, id)
	if platform.IsNoRows(err) {
		return iv, sqlc.GetApplicationRow{}, platform.ErrNotFound("Interview tidak ditemukan.")
	}
	if err != nil {
		return iv, sqlc.GetApplicationRow{}, err
	}
	app, err := q.GetApplication(ctx, iv.ApplicationID)
	if err != nil {
		return iv, app, err
	}
	p := auth.Current(c)
	res := auth.Resource{Unit: app.UnitID, Job: &app.JobID}
	if !p.Can(auth.PermApplicationsRead, res) {
		return iv, app, platform.ErrNotFound("Interview tidak ditemukan.")
	}
	if perm != "" && !p.Can(perm, res) {
		return iv, app, platform.ErrForbidden()
	}
	return iv, app, nil
}

// POST /interviews/:id/reschedule: zona waktu dicatat; peserta mendapat pembaruan.
func (h *Handler) RescheduleInterview(c *gin.Context) {
	p := auth.Current(c)
	var req scheduleReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	if f := req.validate(false); len(f) > 0 {
		platform.Fail(c, platform.ErrValidation(f))
		return
	}
	if len(strings.TrimSpace(req.Reason)) < 5 {
		platform.Fail(c, platform.ErrValidation(map[string]string{"reason": "Alasan penjadwalan ulang wajib diisi."}))
		return
	}
	ctx := c.Request.Context()
	var iv sqlc.Interview
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		old, app, err := h.loadInterview(c, q, auth.PermApplicationsMove)
		if err != nil {
			return err
		}
		ends := req.StartsAt.Add(time.Duration(req.DurationMinutes) * time.Minute)
		iv, err = q.RescheduleInterview(ctx, sqlc.RescheduleInterviewParams{ID: old.ID, Version: req.Version, StartsAt: req.StartsAt, EndsAt: ends, Timezone: req.Timezone})
		if platform.IsNoRows(err) {
			return platform.ErrConflict("stale_version", "Interview sudah diubah atau dibatalkan. Muat ulang.")
		}
		if err != nil {
			return err
		}
		if err := q.UpdatePanelPeriod(ctx, sqlc.UpdatePanelPeriodParams{InterviewID: iv.ID, StartsAt: req.StartsAt, EndsAt: ends}); err != nil {
			if isOverlap(err) {
				return errInterviewerConflict
			}
			return err
		}
		if err := q.InsertInterviewChange(ctx, sqlc.InsertInterviewChangeParams{InterviewID: iv.ID, Action: "reschedule", OldStartsAt: &old.StartsAt,
			NewStartsAt: &req.StartsAt, Timezone: req.Timezone, Reason: req.Reason, ActorUserID: p.UserID}); err != nil {
			return err
		}
		if _, err := platform.Enqueue(ctx, q, h.OrgID, TopicInterviewRescheduled, "interview", iv.ID.String(), gin.H{"applicationId": app.ID,
			"interviewId": iv.ID, "when": whenLabel(req.StartsAt, req.Timezone), "oldWhen": whenLabel(old.StartsAt, old.Timezone)}); err != nil {
			return err
		}
		return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff", Action: "interview.reschedule",
			TargetType: "application", TargetID: app.ID.String(), ScopeUnit: &app.UnitID, Reason: req.Reason, RequestID: platform.RequestID(c),
			Detail: map[string]any{"interview_id": iv.ID, "old": old.StartsAt, "new": req.StartsAt, "timezone": req.Timezone}})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, iv)
}

// POST /interviews/:id/cancel
func (h *Handler) CancelInterview(c *gin.Context) {
	p := auth.Current(c)
	var req struct {
		Version int32  `json:"version"`
		Reason  string `json:"reason"`
	}
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	if len(strings.TrimSpace(req.Reason)) < 5 {
		platform.Fail(c, platform.ErrValidation(map[string]string{"reason": "Alasan pembatalan wajib diisi."}))
		return
	}
	ctx := c.Request.Context()
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		old, app, err := h.loadInterview(c, q, auth.PermApplicationsMove)
		if err != nil {
			return err
		}
		return cancelInterview(c, q, h.OrgID, p.UserID, old, app, req.Version, req.Reason)
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func cancelInterview(c *gin.Context, q *sqlc.Queries, org, actor uuid.UUID, iv sqlc.Interview, app sqlc.GetApplicationRow, version int32, reason string) error {
	ctx := c.Request.Context()
	if _, err := q.SetInterviewStatus(ctx, sqlc.SetInterviewStatusParams{ID: iv.ID, Version: version, Status: "cancelled"}); err != nil {
		if platform.IsNoRows(err) {
			return platform.ErrConflict("stale_version", "Interview sudah diubah atau dibatalkan. Muat ulang.")
		}
		return err
	}
	if err := q.DeactivatePanel(ctx, iv.ID); err != nil {
		return err
	}
	if err := q.InsertInterviewChange(ctx, sqlc.InsertInterviewChangeParams{InterviewID: iv.ID, Action: "cancel", OldStartsAt: &iv.StartsAt,
		Timezone: iv.Timezone, Reason: reason, ActorUserID: actor}); err != nil {
		return err
	}
	if _, err := platform.Enqueue(ctx, q, org, TopicInterviewCancelled, "interview", iv.ID.String(), gin.H{"applicationId": app.ID,
		"interviewId": iv.ID, "when": whenLabel(iv.StartsAt, iv.Timezone)}); err != nil {
		return err
	}
	return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: org, ActorID: &actor, ActorKind: "staff", Action: "interview.cancel",
		TargetType: "application", TargetID: app.ID.String(), ScopeUnit: &app.UnitID, Reason: reason, RequestID: platform.RequestID(c),
		Detail: map[string]any{"interview_id": iv.ID}})
}

// cancelOpenInterviews dipanggil saat lamaran tidak aktif lagi (ditarik, ditolak, diterima).
func cancelOpenInterviews(c *gin.Context, q *sqlc.Queries, org, actor uuid.UUID, appID uuid.UUID, reason string) error {
	ctx := c.Request.Context()
	ivs, err := q.ListInterviewsForApplication(ctx, appID)
	if err != nil {
		return err
	}
	app, err := q.GetApplication(ctx, appID)
	if err != nil {
		return err
	}
	for _, iv := range ivs {
		if iv.Status != "scheduled" || iv.StartsAt.Before(time.Now()) {
			continue
		}
		locked, err := q.LockInterview(ctx, iv.ID)
		if err != nil {
			return err
		}
		if err := cancelInterview(c, q, org, actor, locked, app, locked.Version, reason); err != nil {
			return err
		}
	}
	return nil
}

type evalReq struct {
	Rating         int32  `json:"rating"`
	Recommendation string `json:"recommendation"`
	Notes          string `json:"notes"`
}

// POST /interviews/:id/evaluations: scorecard hanya oleh interviewer di panel.
func (h *Handler) Evaluate(c *gin.Context) {
	p := auth.Current(c)
	var req evalReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	f := map[string]string{}
	if req.Rating < 1 || req.Rating > 5 {
		f["rating"] = "Nilai 1 sampai 5."
	}
	if !slices.Contains([]string{"lanjut", "pertimbangkan", "tidak_lanjut"}, req.Recommendation) {
		f["recommendation"] = "Pilih rekomendasi."
	}
	if len(req.Notes) > 4000 {
		f["notes"] = "Catatan maksimal 4000 karakter."
	}
	if len(f) > 0 {
		platform.Fail(c, platform.ErrValidation(f))
		return
	}
	ctx := c.Request.Context()
	var ev sqlc.Evaluation
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		iv, app, err := h.loadInterview(c, q, "")
		if err != nil {
			return err
		}
		ok, err := q.IsPanelist(ctx, sqlc.IsPanelistParams{InterviewID: iv.ID, InterviewerID: p.UserID})
		if err != nil {
			return err
		}
		if !ok {
			return &platform.APIError{Status: http.StatusForbidden, Code: "not_panelist", Message: "Hanya interviewer di panel yang dapat mengisi penilaian."}
		}
		if iv.Status == "cancelled" {
			return platform.ErrConflict("interview_cancelled", "Interview dibatalkan.")
		}
		ev, err = q.UpsertEvaluation(ctx, sqlc.UpsertEvaluationParams{InterviewID: iv.ID, EvaluatorID: p.UserID, Rating: req.Rating, Recommendation: req.Recommendation, Notes: req.Notes})
		if err != nil {
			return err
		}
		return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff", Action: "interview.evaluate",
			TargetType: "application", TargetID: app.ID.String(), ScopeUnit: &app.UnitID, RequestID: platform.RequestID(c),
			Detail: map[string]any{"interview_id": iv.ID, "rating": req.Rating, "recommendation": req.Recommendation}})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, ev)
}

// GET /me/interviews?from=&to= : kalender interviewer (staf).
func (h *Handler) MyInterviews(c *gin.Context) {
	from := time.Now().Add(-24 * time.Hour)
	to := from.AddDate(0, 0, 60)
	if v := c.Query("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	if v := c.Query("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}
	rows, err := h.Q.ListInterviewsForInterviewer(c.Request.Context(), sqlc.ListInterviewsForInterviewerParams{UserID: auth.Current(c).UserID, FromAt: from, ToAt: to})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}

// POST /applications/:id/decision-notice: recruiter mengirim pemberitahuan hasil akhir sekali (diterima/ditolak).
func (h *Handler) DecisionNotice(c *gin.Context) {
	app, p, ok := h.loadScopedApplication(c, auth.PermApplicationsMove)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		n, err := q.MarkDecisionNotified(ctx, sqlc.MarkDecisionNotifiedParams{ID: app.ID, DecisionNotifiedBy: &p.UserID})
		if err != nil {
			return err
		}
		if n == 0 {
			return platform.ErrConflict("decision_notice_unavailable", "Pemberitahuan hanya untuk lamaran dengan keputusan akhir dan belum pernah dikirim.")
		}
		cur, err := q.GetApplication(ctx, app.ID)
		if err != nil {
			return err
		}
		if _, err := platform.Enqueue(ctx, q, h.OrgID, TopicDecision, "application", app.ID.String(), gin.H{"applicationId": app.ID, "status": cur.Status}); err != nil {
			return err
		}
		return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff", Action: "application.decision_notice",
			TargetType: "application", TargetID: app.ID.String(), ScopeUnit: &app.UnitID, RequestID: platform.RequestID(c), Detail: map[string]any{"status": cur.Status}})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"notified": true})
}

// GET /staff/interviewers: staf aktif untuk dipilih sebagai interviewer (tanpa data pribadi selain nama).
func (h *Handler) Interviewers(c *gin.Context) {
	rows, err := h.Q.ListActiveStaff(c.Request.Context(), h.OrgID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}
