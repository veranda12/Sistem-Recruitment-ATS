package exam

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

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

func paramID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		platform.Fail(c, platform.ErrNotFound("Data tidak ditemukan."))
		return id, false
	}
	return id, true
}

func noStore(c *gin.Context) { c.Header("Cache-Control", "private, no-store") }

// ---- Kandidat ----

// GET /me/exam-invitations
func (h *Handler) MyInvitations(c *gin.Context) {
	rows, err := h.Q.ListCandidateInvitations(c.Request.Context(), auth.Current(c).UserID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows, "serverTime": time.Now().UTC()})
}

// POST /exam-invitations/:id/start
func (h *Handler) Start(c *gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	p := auth.Current(c)
	ctx := c.Request.Context()
	var dto AttemptDTO
	created := false
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		a, isNew, err := Start(ctx, q, id, p.UserID)
		if err != nil {
			return err
		}
		created = isNew
		if isNew {
			if err := platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "candidate", Action: "exam.start",
				TargetType: "exam_attempt", TargetID: a.ID.String(), RequestID: platform.RequestID(c)}); err != nil {
				return err
			}
		}
		dto, err = Resume(ctx, q, a)
		return err
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	noStore(c)
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, dto)
}

func (h *Handler) ownAttempt(c *gin.Context) (sqlc.ExamAttempt, bool) {
	id, ok := paramID(c)
	if !ok {
		return sqlc.ExamAttempt{}, false
	}
	a, err := h.Q.GetAttempt(c.Request.Context(), id)
	if platform.IsNoRows(err) || (err == nil && a.CandidateUserID != auth.Current(c).UserID) {
		platform.Fail(c, platform.ErrNotFound("Ujian tidak ditemukan."))
		return a, false
	}
	if err != nil {
		platform.Fail(c, err)
		return a, false
	}
	return a, true
}

// GET /exam-attempts/:id: state, server_time, deadline, urutan soal, dan revisi jawaban committed.
func (h *Handler) Resume(c *gin.Context) {
	a, ok := h.ownAttempt(c)
	if !ok {
		return
	}
	dto, err := Resume(c.Request.Context(), h.Q, a)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	noStore(c)
	c.JSON(http.StatusOK, dto)
}

// PUT /exam-attempts/:id/answers
func (h *Handler) Save(c *gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	var req SaveRequest
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	var body json.RawMessage
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var err error
		body, err = Save(ctx, h.Q.WithTx(tx), id, auth.Current(c).UserID, req)
		return err
	})
	countOp("save", err)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	noStore(c)
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

// POST /exam-attempts/:id/submit (Idempotency-Key wajib)
func (h *Handler) Submit(c *gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	var req SubmitRequest
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	var r ReceiptDTO
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var err error
		r, err = Submit(ctx, h.Q.WithTx(tx), id, auth.Current(c).UserID, c.GetHeader("Idempotency-Key"), req)
		return err
	})
	countOp("submit", err)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	if r.Reason == "expired" {
		e := platform.ErrConflict("deadline_passed", "Waktu ujian sudah habis. Jawaban terakhir yang tersimpan di server sudah difinalisasi.")
		e.Details = map[string]any{"receipt": r}
		platform.Fail(c, e)
		return
	}
	noStore(c)
	c.JSON(http.StatusOK, r)
}

// POST /exam-attempts/:id/heartbeat: best-effort, tidak memperpanjang sesi.
func (h *Handler) Heartbeat(c *gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	if _, err := h.Q.Heartbeat(c.Request.Context(), sqlc.HeartbeatParams{ID: id, CandidateUserID: auth.Current(c).UserID}); err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"serverTime": time.Now().UTC()})
}

// ---- Staf ----

// GET /exam-schedules
func (h *Handler) ListSchedules(c *gin.Context) {
	rows, err := h.Q.ListSchedules(c.Request.Context(), h.OrgID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}

type scheduleReq struct {
	PackageKey  string    `json:"packageKey"`
	Name        string    `json:"name"`
	OpensAt     time.Time `json:"opensAt"`
	ClosesAt    time.Time `json:"closesAt"`
	MaxAttempts int32     `json:"maxAttempts"`
}

// POST /exam-schedules: memakai versi paket terbaru; perubahan bank soal setelahnya tidak memengaruhi jadwal.
func (h *Handler) CreateSchedule(c *gin.Context) {
	p := auth.Current(c)
	var req scheduleReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	f := map[string]string{}
	if l := len(strings.TrimSpace(req.Name)); l < 2 || l > 200 {
		f["name"] = "Nama jadwal 2 sampai 200 karakter."
	}
	if !req.ClosesAt.After(req.OpensAt) {
		f["closesAt"] = "Waktu tutup harus setelah waktu buka."
	}
	if req.MaxAttempts == 0 {
		req.MaxAttempts = 1
	}
	if req.MaxAttempts < 1 || req.MaxAttempts > 5 {
		f["maxAttempts"] = "Kesempatan 1 sampai 5."
	}
	ctx := c.Request.Context()
	pkg, err := h.Q.GetExamPackageByKey(ctx, sqlc.GetExamPackageByKeyParams{OrgID: h.OrgID, Key: req.PackageKey})
	if platform.IsNoRows(err) {
		f["packageKey"] = "Paket ujian tidak ditemukan."
	} else if err != nil {
		platform.Fail(c, err)
		return
	}
	if len(f) > 0 {
		platform.Fail(c, platform.ErrValidation(f))
		return
	}
	var s sqlc.ExamSchedule
	err = platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		v, err := q.LatestPackageVersion(ctx, pkg.ID)
		if err != nil {
			return platform.ErrConflict("package_unpublished", "Paket ujian belum memiliki versi terbit.")
		}
		s, err = q.CreateSchedule(ctx, sqlc.CreateScheduleParams{OrgID: h.OrgID, PackageVersionID: v.ID, Name: strings.TrimSpace(req.Name),
			OpensAt: req.OpensAt, ClosesAt: req.ClosesAt, MaxAttempts: req.MaxAttempts, CreatedBy: &p.UserID})
		if err != nil {
			return err
		}
		return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff", Action: "exam.schedule_create",
			TargetType: "exam_schedule", TargetID: s.ID.String(), RequestID: platform.RequestID(c), Detail: map[string]any{"package_version": v.VersionNo}})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, s)
}

type inviteReq struct {
	ApplicationIDs []uuid.UUID `json:"applicationIds"`
	ExtraMinutes   int32       `json:"extraMinutes"`
	Reason         string      `json:"reason"`
}

type inviteResult struct {
	ApplicationID uuid.UUID `json:"applicationId"`
	Outcome       string    `json:"outcome"` // invited | already_invited | forbidden | not_found | wrong_stage
}

// POST /exam-schedules/:id/invitations: tindakan massal, scope diperiksa per lamaran.
func (h *Handler) Invite(c *gin.Context) {
	sid, ok := paramID(c)
	if !ok {
		return
	}
	p := auth.Current(c)
	var req inviteReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	if len(req.ApplicationIDs) == 0 || len(req.ApplicationIDs) > 500 {
		platform.Fail(c, platform.ErrValidation(map[string]string{"applicationIds": "Pilih 1 sampai 500 lamaran."}))
		return
	}
	if req.ExtraMinutes < 0 || req.ExtraMinutes > 240 || (req.ExtraMinutes > 0 && len(strings.TrimSpace(req.Reason)) < 5) {
		platform.Fail(c, platform.ErrValidation(map[string]string{"extraMinutes": "Akomodasi 0 sampai 240 menit dan wajib beralasan."}))
		return
	}
	ctx := c.Request.Context()
	results := []inviteResult{}
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		s, err := q.GetSchedule(ctx, sid)
		if platform.IsNoRows(err) || (err == nil && s.OrgID != h.OrgID) {
			return platform.ErrNotFound("Jadwal tidak ditemukan.")
		}
		if err != nil {
			return err
		}
		for _, appID := range req.ApplicationIDs {
			app, err := q.GetApplication(ctx, appID)
			if platform.IsNoRows(err) {
				results = append(results, inviteResult{appID, "not_found"})
				continue
			}
			if err != nil {
				return err
			}
			res := auth.Resource{Unit: app.UnitID, Job: &app.JobID}
			if !p.Can(auth.PermApplicationsMove, res) && !p.Can(auth.PermExamsManage, res) {
				results = append(results, inviteResult{appID, "not_found"})
				continue
			}
			if app.Status != "active" || app.StageType != "assessment" {
				results = append(results, inviteResult{appID, "wrong_stage"})
				continue
			}
			ep, err := q.InsertParticipant(ctx, sqlc.InsertParticipantParams{OrgID: h.OrgID, ScheduleID: sid, ApplicationID: appID,
				CandidateUserID: app.CandidateUserID, ExtraMinutes: req.ExtraMinutes, InvitedBy: &p.UserID})
			if platform.IsNoRows(err) {
				results = append(results, inviteResult{appID, "already_invited"})
				continue
			}
			if err != nil {
				return err
			}
			if _, err := platform.Enqueue(ctx, q, h.OrgID, TopicInvited, "exam_participant", ep.ID.String(), gin.H{"participantId": ep.ID,
				"applicationId": appID, "candidateId": app.CandidateUserID}); err != nil {
				return err
			}
			if err := platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff", Action: "exam.invite",
				TargetType: "application", TargetID: appID.String(), ScopeUnit: &app.UnitID, Reason: req.Reason, RequestID: platform.RequestID(c),
				Detail: map[string]any{"schedule_id": sid, "extra_minutes": req.ExtraMinutes}}); err != nil {
				return err
			}
			results = append(results, inviteResult{appID, "invited"})
		}
		return nil
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"results": results})
}

// GET /exam-schedules/:id/monitor: last_seen, progres jawaban committed, dan status submit dibedakan.
func (h *Handler) Monitor(c *gin.Context) {
	sid, ok := paramID(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	s, err := h.Q.GetSchedule(ctx, sid)
	if platform.IsNoRows(err) || (err == nil && s.OrgID != h.OrgID) {
		platform.Fail(c, platform.ErrNotFound("Jadwal tidak ditemukan."))
		return
	}
	if err != nil {
		platform.Fail(c, err)
		return
	}
	rows, err := h.Q.MonitorSchedule(ctx, sid)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	activity, err := h.activityByAttempt(c, sid)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	type participant struct {
		sqlc.MonitorScheduleRow
		Activity activitySummary `json:"activity"`
	}
	out := make([]participant, 0, len(rows))
	for _, r := range rows {
		act := activitySummary{}
		if r.AttemptID != nil && activity[*r.AttemptID] != nil {
			act = activity[*r.AttemptID]
		}
		out = append(out, participant{r, act})
	}
	c.JSON(http.StatusOK, gin.H{"schedule": s, "participants": out, "serverTime": time.Now().UTC()})
}

// GET /exam-attempts/:id/review: tampilan penilai tanpa identitas kandidat (penilaian buta).
func (h *Handler) Review(c *gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	a, err := h.Q.GetAttempt(ctx, id)
	if platform.IsNoRows(err) || (err == nil && a.OrgID != h.OrgID) {
		platform.Fail(c, platform.ErrNotFound("Ujian tidak ditemukan."))
		return
	}
	if err != nil {
		platform.Fail(c, err)
		return
	}
	items, err := h.Q.ListAttemptItems(ctx, id)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	ans, err := h.Q.ListAnswers(ctx, id)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	history, err := h.Q.ListEssayGradeHistory(ctx, id)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	res, _ := h.Q.GetResult(ctx, id)
	byItem := map[uuid.UUID]json.RawMessage{}
	for _, x := range ans {
		byItem[x.ItemID] = json.RawMessage(x.Value)
	}
	type reviewItem struct {
		ItemID uuid.UUID       `json:"itemId"`
		Number int32           `json:"number"`
		Kind   string          `json:"kind"`
		Stem   string          `json:"stem"`
		Weight float64         `json:"weight"`
		Rubric string          `json:"rubric"`
		Answer json.RawMessage `json:"answer"`
	}
	out := []reviewItem{}
	for _, it := range items {
		if it.Kind != "essay" {
			continue
		}
		v := byItem[it.ItemID]
		if len(v) == 0 {
			v = json.RawMessage("null")
		}
		out = append(out, reviewItem{it.ItemID, it.Position, it.Kind, it.Stem, it.Weight, it.Rubric, v})
	}
	noStore(c)
	c.JSON(http.StatusOK, gin.H{"attemptId": id, "participantCode": "P-" + strings.ToUpper(id.String()[:8]), "status": a.Status,
		"items": out, "gradeHistory": history, "result": res})
}

type gradeReq struct {
	ItemID uuid.UUID `json:"itemId"`
	Points float64   `json:"points"`
	Reason string    `json:"reason"`
}

// POST /exam-attempts/:id/essay-grades
func (h *Handler) Grade(c *gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	p := auth.Current(c)
	var req gradeReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	var res sqlc.ExamResult
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var err error
		res, err = GradeEssay(ctx, h.Q.WithTx(tx), id, req.ItemID, p.UserID, req.Points, strings.TrimSpace(req.Reason), platform.RequestID(c))
		return err
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// POST /exam-schedules/:id/release: izin results.release per scope lamaran.
func (h *Handler) Release(c *gin.Context) {
	sid, ok := paramID(c)
	if !ok {
		return
	}
	p := auth.Current(c)
	ctx := c.Request.Context()
	var n int
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var err error
		n, err = ReleaseSchedule(ctx, h.Q.WithTx(tx), sid, p.UserID, func(unit, job uuid.UUID) bool {
			return p.Can(auth.PermResultsRelease, auth.Resource{Unit: unit, Job: &job})
		}, platform.RequestID(c))
		return err
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"released": n})
}

var _ = errForbidden

func countOp(op string, err error) {
	outcome := "commit"
	if err != nil {
		outcome = "error"
		if ae, ok := err.(*platform.APIError); ok {
			outcome = "rejected"
			if ae.Code == "answer_conflict" || ae.Code == "stale_revision" {
				outcome = "conflict"
			}
		}
	}
	platform.ExamOps.WithLabelValues(op, outcome).Inc()
}
