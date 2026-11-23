package exam

import (
	"net/http"
	"slices"
	"time"

	"ats/db/sqlc"
	"ats/internal/auth"
	"ats/internal/platform"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Log perilaku peserta (tanpa kamera). Bukan bukti kecurangan dengan sendirinya: hanya catatan untuk panitia.
var activityKinds = []string{"tab_hidden", "tab_visible", "window_blur", "copy", "paste", "offline", "online"}

const (
	maxActivityPerAttempt = 1000
	maxActivityPerRequest = 50
)

type activityEvent struct {
	Seq        int32     `json:"seq"`
	Kind       string    `json:"kind"`
	At         time.Time `json:"at"`
	DurationMs *int32    `json:"durationMs"`
}

// POST /exam-attempts/:id/activity: kandidat pemilik percobaan; diterima selama percobaan berjalan.
// Kiriman ulang (seq sama) diabaikan; lebih dari batas per percobaan diabaikan tanpa error agar ujian tidak terganggu.
func (h *Handler) RecordActivity(c *gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	var req struct {
		Events []activityEvent `json:"events"`
	}
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	if len(req.Events) > maxActivityPerRequest {
		platform.Fail(c, platform.ErrValidation(map[string]string{"events": "Maksimal 50 catatan per kiriman."}))
		return
	}
	ctx := c.Request.Context()
	a, err := h.Q.GetAttempt(ctx, id)
	if platform.IsNoRows(err) || (err == nil && a.CandidateUserID != auth.Current(c).UserID) {
		platform.Fail(c, platform.ErrNotFound("Ujian tidak ditemukan."))
		return
	}
	if err != nil {
		platform.Fail(c, err)
		return
	}
	if a.Status != "in_progress" {
		c.JSON(http.StatusOK, gin.H{"accepted": 0})
		return
	}
	n, err := h.Q.CountAttemptEvents(ctx, id)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	accepted := 0
	now := time.Now()
	for _, e := range req.Events {
		if n >= maxActivityPerAttempt {
			break
		}
		if e.Seq <= 0 || !slices.Contains(activityKinds, e.Kind) {
			continue
		}
		// Jam perangkat bisa salah; dibatasi ke rentang percobaan agar urutan di laporan masuk akal.
		at := e.At
		if at.Before(a.StartedAt) || at.After(now.Add(time.Minute)) {
			at = now
		}
		if e.DurationMs != nil && *e.DurationMs < 0 {
			e.DurationMs = nil
		}
		rows, err := h.Q.InsertAttemptEvent(ctx, sqlc.InsertAttemptEventParams{AttemptID: id, ClientSeq: e.Seq, Kind: e.Kind, OccurredAt: at, DurationMs: e.DurationMs})
		if err != nil {
			platform.Fail(c, err)
			return
		}
		accepted += int(rows)
		n += rows
	}
	c.JSON(http.StatusOK, gin.H{"accepted": accepted})
}

// GET /exam-attempts/:id/activity (exams.manage): catatan perilaku satu percobaan.
func (h *Handler) AttemptActivity(c *gin.Context) {
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
	rows, err := h.Q.ListAttemptEvents(ctx, id)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows, "startedAt": a.StartedAt, "deadlineAt": a.DeadlineAt})
}

type activitySummary map[string]struct {
	Count   int64 `json:"count"`
	TotalMs int64 `json:"totalMs"`
}

func (h *Handler) activityByAttempt(c *gin.Context, scheduleID uuid.UUID) (map[uuid.UUID]activitySummary, error) {
	rows, err := h.Q.ScheduleActivitySummary(c.Request.Context(), scheduleID)
	if err != nil {
		return nil, err
	}
	out := map[uuid.UUID]activitySummary{}
	for _, r := range rows {
		if out[r.AttemptID] == nil {
			out[r.AttemptID] = activitySummary{}
		}
		out[r.AttemptID][r.Kind] = struct {
			Count   int64 `json:"count"`
			TotalMs int64 `json:"totalMs"`
		}{r.N, r.TotalMs}
	}
	return out, nil
}
