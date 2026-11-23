package recruitment

import (
	"net/http"
	"time"

	"ats/db/sqlc"
	"ats/internal/auth"
	"ats/internal/platform"
	"ats/internal/workflow"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const boardLimit = 500

type boardStage struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// GET /staff/jobs/:id/board: lamaran satu lowongan dikelompokkan per tahap workflow aktif (applications.read).
// Lamaran pada versi workflow lama yang tahapnya sudah tidak ada tetap tampil di kolom tambahan di akhir.
func (h *Handler) JobBoard(c *gin.Context) {
	p := auth.Current(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		platform.Fail(c, platform.ErrNotFound("Lowongan tidak ditemukan."))
		return
	}
	ctx := c.Request.Context()
	job, err := h.Q.GetJob(ctx, id)
	if platform.IsNoRows(err) || (err == nil && (job.OrgID != h.OrgID || !p.Can(auth.PermApplicationsRead, auth.Resource{Unit: job.UnitID, Job: &job.ID}))) {
		platform.Fail(c, platform.ErrNotFound("Lowongan tidak ditemukan."))
		return
	}
	if err != nil {
		platform.Fail(c, err)
		return
	}
	stages := []boardStage{}
	known := map[string]bool{}
	wf, err := h.Q.GetWorkflow(ctx, job.WorkflowID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	if wf.ActiveVersionID != nil {
		def, err := workflow.LoadVersion(ctx, h.Q, *wf.ActiveVersionID)
		if err != nil {
			platform.Fail(c, err)
			return
		}
		for _, s := range def.Stages {
			stages = append(stages, boardStage{s.Key, s.Name, s.Type})
			known[s.Key] = true
		}
	}
	s := p.Scope(auth.PermApplicationsRead)
	rows, err := h.Q.ListStaffApplications(ctx, sqlc.ListStaffApplicationsParams{OrgID: h.OrgID, OrgWide: s.OrgWide, UnitIds: s.Units, JobIds: s.Jobs,
		JobID: &job.ID, PageSize: boardLimit + 1})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	truncated := len(rows) > boardLimit
	if truncated {
		rows = rows[:boardLimit]
	}
	for _, r := range rows {
		if !known[r.StageKey] {
			stages = append(stages, boardStage{r.StageKey, r.StageName, r.StageType})
			known[r.StageKey] = true
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"job":    gin.H{"id": job.ID, "title": job.Title, "unitName": job.UnitName},
		"stages": stages, "items": rows, "truncated": truncated,
	})
}

// GET /interviews?from=&to=&mine=true: kalender interview staf. Rentang maksimal 62 hari.
func (h *Handler) InterviewCalendar(c *gin.Context) {
	p := auth.Current(c)
	from, err1 := time.Parse(time.RFC3339, c.Query("from"))
	to, err2 := time.Parse(time.RFC3339, c.Query("to"))
	if err1 != nil || err2 != nil || !to.After(from) || to.Sub(from) > 62*24*time.Hour {
		platform.Fail(c, platform.ErrValidation(map[string]string{"from": "Isi from dan to (RFC 3339) dengan rentang 1 sampai 62 hari."}))
		return
	}
	s := p.Scope(auth.PermApplicationsRead)
	rows, err := h.Q.ListInterviewsInRange(c.Request.Context(), sqlc.ListInterviewsInRangeParams{OrgID: h.OrgID, FromAt: from, ToAt: to,
		OrgWide: s.OrgWide, UnitIds: s.Units, JobIds: s.Jobs, UserID: p.UserID, OnlyMine: c.Query("mine") == "true"})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}
