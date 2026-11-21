// Package recruitment: lowongan, akun dan profil kandidat, lamaran, snapshot, dan tindakan staf atas lamaran.
package recruitment

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"ats/db/sqlc"
	"ats/internal/auth"
	"ats/internal/identity"
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
	Keys  identity.Keys
	Cache *PublicCache
	// Docs memuat daftar dokumen kandidat (diisi composition root agar recruitment tidak bergantung ke documents).
	Docs func(context.Context, uuid.UUID) ([]sqlc.ListOwnerDocumentsRow, error)
}

var employmentTypes = []string{"full_time", "contract", "internship", "part_time"}

type publicJobDTO struct {
	ID             uuid.UUID  `json:"id"`
	Slug           string     `json:"slug"`
	Title          string     `json:"title"`
	EmploymentType string     `json:"employmentType"`
	PublishedAt    *time.Time `json:"publishedAt"`
	ClosesAt       *time.Time `json:"closesAt"`
	Unit           codeName   `json:"unit"`
	Location       location   `json:"location"`
}

type codeName struct {
	Code string `json:"code"`
	Name string `json:"name"`
}
type location struct {
	Code string `json:"code"`
	Name string `json:"name"`
	City string `json:"city"`
}

// GET /org: profil perusahaan, brand, master filter untuk portal publik.
func (h *Handler) PublicOrg(c *gin.Context) {
	ctx := c.Request.Context()
	if b, ok := h.Cache.Get(ctx, "org"); ok {
		c.Header("Cache-Control", "public, max-age=30")
		c.Data(http.StatusOK, "application/json; charset=utf-8", b)
		return
	}
	org, err := h.Q.GetOrganization(ctx, h.OrgID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	units, err := h.Q.ListUnits(ctx, h.OrgID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	locs, err := h.Q.ListLocations(ctx, h.OrgID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	us := []codeName{}
	for _, u := range units {
		if u.Active {
			us = append(us, codeName{u.Code, u.Name})
		}
	}
	ls := []location{}
	for _, l := range locs {
		if l.Active {
			ls = append(ls, location{l.Code, l.Name, l.City})
		}
	}
	body, _ := json.Marshal(gin.H{"name": org.Name, "timezone": org.Timezone, "profile": json.RawMessage(org.Profile),
		"brand": json.RawMessage(org.Brand), "units": us, "locations": ls})
	h.Cache.Set(ctx, "org", body)
	c.Header("Cache-Control", "public, max-age=30")
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

type cursor struct {
	T  time.Time `json:"t"`
	ID uuid.UUID `json:"i"`
}

func encodeCursor(t time.Time, id uuid.UUID) string {
	b, _ := json.Marshal(cursor{t, id})
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(s string) (*cursor, error) {
	if s == "" {
		return nil, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, platform.ErrBadRequest("Parameter halaman tidak valid.")
	}
	var c cursor
	if err := json.Unmarshal(b, &c); err != nil || c.ID == uuid.Nil {
		return nil, platform.ErrBadRequest("Parameter halaman tidak valid.")
	}
	return &c, nil
}

func pageSize(c *gin.Context, def, max int) (int32, error) {
	s := c.Query("limit")
	if s == "" {
		return int32(def), nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > max {
		return 0, platform.ErrValidation(map[string]string{"limit": "Jumlah per halaman 1 sampai " + strconv.Itoa(max) + "."})
	}
	return int32(n), nil
}

func optStr(c *gin.Context, k string) *string {
	v := strings.TrimSpace(c.Query(k))
	if v == "" {
		return nil
	}
	return &v
}

// GET /jobs: hanya status published. Filter allowlist: unit, location, type, q. Keyset pagination.
func (h *Handler) ListPublicJobs(c *gin.Context) {
	ctx := c.Request.Context()
	size, err := pageSize(c, 20, 50)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	cur, err := decodeCursor(c.Query("cursor"))
	if err != nil {
		platform.Fail(c, err)
		return
	}
	et := optStr(c, "type")
	if et != nil && !slices.Contains(employmentTypes, *et) {
		platform.Fail(c, platform.ErrValidation(map[string]string{"type": "Jenis kerja tidak dikenal."}))
		return
	}
	q := optStr(c, "q")
	if q != nil && len(*q) > 100 {
		platform.Fail(c, platform.ErrValidation(map[string]string{"q": "Kata kunci maksimal 100 karakter."}))
		return
	}
	cacheKey := "jobs:" + c.Request.URL.RawQuery
	if b, ok := h.Cache.Get(ctx, cacheKey); ok {
		c.Data(http.StatusOK, "application/json; charset=utf-8", b)
		return
	}
	p := sqlc.ListPublicJobsParams{OrgID: h.OrgID, UnitCode: optStr(c, "unit"), LocationCode: optStr(c, "location"),
		EmploymentType: et, Q: q, PageSize: size + 1}
	if cur != nil {
		p.CursorPublishedAt, p.CursorID = &cur.T, &cur.ID
	}
	rows, err := h.Q.ListPublicJobs(ctx, p)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	items := make([]publicJobDTO, 0, len(rows))
	var next *string
	for i, r := range rows {
		if i == int(size) {
			last := rows[i-1]
			s := encodeCursor(*last.PublishedAt, last.ID)
			next = &s
			break
		}
		items = append(items, publicJobDTO{ID: r.ID, Slug: r.Slug, Title: r.Title, EmploymentType: r.EmploymentType,
			PublishedAt: r.PublishedAt, ClosesAt: r.ClosesAt, Unit: codeName{r.UnitCode, r.UnitName},
			Location: location{r.LocationCode, r.LocationName, r.City}})
	}
	body, _ := json.Marshal(gin.H{"items": items, "nextCursor": next})
	h.Cache.Set(ctx, cacheKey, body)
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

// GET /jobs/:slug. Draft = 404. Lowongan tutup tetap tampil dengan status jelas agar tautan lama tidak membingungkan.
func (h *Handler) GetPublicJob(c *gin.Context) {
	j, err := h.Q.GetJobBySlug(c.Request.Context(), sqlc.GetJobBySlugParams{OrgID: h.OrgID, Slug: c.Param("slug")})
	if platform.IsNoRows(err) || (err == nil && j.Status == "draft") {
		platform.Fail(c, platform.ErrNotFound("Lowongan tidak ditemukan."))
		return
	}
	if err != nil {
		platform.Fail(c, err)
		return
	}
	open := j.Status == "published" && (j.ClosesAt == nil || j.ClosesAt.After(time.Now()))
	c.JSON(http.StatusOK, gin.H{
		"id": j.ID, "slug": j.Slug, "title": j.Title, "employmentType": j.EmploymentType,
		"description": j.Description, "requirements": j.Requirements, "publishedAt": j.PublishedAt, "closesAt": j.ClosesAt,
		"status": j.Status, "acceptingApplications": open,
		"unit": codeName{j.UnitCode, j.UnitName}, "location": location{j.LocationCode, j.LocationName, j.City},
	})
}

// ---- Staf: lowongan ----

func (h *Handler) ListStaffJobs(c *gin.Context) {
	p := auth.Current(c)
	s := p.Scope(auth.PermJobsRead)
	rows, err := h.Q.ListStaffJobs(c.Request.Context(), sqlc.ListStaffJobsParams{OrgID: h.OrgID, OrgWide: s.OrgWide, UnitIds: s.Units, JobIds: s.Jobs})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}

type jobReq struct {
	UnitCode       string     `json:"unitCode"`
	LocationCode   string     `json:"locationCode"`
	WorkflowKey    string     `json:"workflowKey"`
	Title          string     `json:"title"`
	Slug           string     `json:"slug"`
	EmploymentType string     `json:"employmentType"`
	Description    string     `json:"description"`
	Requirements   string     `json:"requirements"`
	ClosesAt       *time.Time `json:"closesAt"`
	Version        int32      `json:"version"`
}

var slugRe = regexp.MustCompile(`^[a-z0-9-]{3,120}$`)

func (r jobReq) validate(create bool) map[string]string {
	f := map[string]string{}
	if l := len(strings.TrimSpace(r.Title)); l < 3 || l > 200 {
		f["title"] = "Judul wajib 3 sampai 200 karakter."
	}
	if create && !slugRe.MatchString(r.Slug) {
		f["slug"] = "Slug hanya huruf kecil, angka, dan tanda hubung (3 sampai 120)."
	}
	if !slices.Contains(employmentTypes, r.EmploymentType) {
		f["employmentType"] = "Pilih jenis kerja."
	}
	if len(r.Description) > 20000 || len(r.Requirements) > 20000 {
		f["description"] = "Deskripsi terlalu panjang."
	}
	if r.ClosesAt != nil && r.ClosesAt.Before(time.Now()) {
		f["closesAt"] = "Batas lamaran harus di masa depan."
	}
	return f
}

func (h *Handler) unitByCode(c *gin.Context, code string) (*sqlc.Unit, error) {
	units, err := h.Q.ListUnits(c.Request.Context(), h.OrgID)
	if err != nil {
		return nil, err
	}
	for _, u := range units {
		if u.Code == code && u.Active {
			return &u, nil
		}
	}
	return nil, nil
}

func (h *Handler) locationByCode(c *gin.Context, code string) (*sqlc.Location, error) {
	locs, err := h.Q.ListLocations(c.Request.Context(), h.OrgID)
	if err != nil {
		return nil, err
	}
	for _, l := range locs {
		if l.Code == code && l.Active {
			return &l, nil
		}
	}
	return nil, nil
}

// POST /staff/jobs
func (h *Handler) CreateJob(c *gin.Context) {
	p := auth.Current(c)
	var req jobReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	f := req.validate(true)
	unit, err := h.unitByCode(c, req.UnitCode)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	if unit == nil {
		f["unitCode"] = "Pilih unit."
	}
	loc, err := h.locationByCode(c, req.LocationCode)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	if loc == nil {
		f["locationCode"] = "Pilih lokasi."
	}
	ctx := c.Request.Context()
	wf, err := h.Q.GetWorkflowByKey(ctx, sqlc.GetWorkflowByKeyParams{OrgID: h.OrgID, Key: req.WorkflowKey})
	if platform.IsNoRows(err) || (err == nil && wf.ActiveVersionID == nil) {
		f["workflowKey"] = "Pilih workflow yang sudah diterbitkan."
	} else if err != nil {
		platform.Fail(c, err)
		return
	}
	if len(f) > 0 {
		platform.Fail(c, platform.ErrValidation(f))
		return
	}
	if !p.Can(auth.PermJobsManage, auth.Resource{Unit: unit.ID}) {
		platform.Fail(c, platform.ErrForbidden())
		return
	}
	var job sqlc.JobOpening
	err = platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		var err error
		job, err = q.CreateJob(ctx, sqlc.CreateJobParams{OrgID: h.OrgID, UnitID: unit.ID, LocationID: loc.ID, WorkflowID: wf.ID,
			Slug: req.Slug, Title: strings.TrimSpace(req.Title), EmploymentType: req.EmploymentType, Description: req.Description,
			Requirements: req.Requirements, ClosesAt: req.ClosesAt, CreatedBy: p.UserID})
		if platform.IsUniqueViolation(err, "job_openings_org_id_slug_key") {
			return platform.ErrValidation(map[string]string{"slug": "Slug sudah dipakai lowongan lain."})
		}
		if err != nil {
			return err
		}
		return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff",
			Action: "job.create", TargetType: "job", TargetID: job.ID.String(), ScopeUnit: &unit.ID, RequestID: platform.RequestID(c)})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, job)
}

type statusReq struct {
	Version int32  `json:"version"`
	Reason  string `json:"reason"`
}

// POST /staff/jobs/:id/publish dan /close. FOR UPDATE membuat close menunggu apply yang sedang memegang FOR SHARE.
func (h *Handler) SetJobStatus(status string) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := auth.Current(c)
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			platform.Fail(c, platform.ErrNotFound(""))
			return
		}
		var req statusReq
		if err := platform.BindJSON(c, &req); err != nil {
			platform.Fail(c, err)
			return
		}
		ctx := c.Request.Context()
		var out sqlc.JobOpening
		err = platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
			q := h.Q.WithTx(tx)
			j, err := q.LockJobForUpdate(ctx, id)
			if platform.IsNoRows(err) {
				return platform.ErrNotFound("Lowongan tidak ditemukan.")
			}
			if err != nil {
				return err
			}
			if !p.Can(auth.PermJobsPublish, auth.Resource{Unit: j.UnitID, Job: &j.ID}) {
				return platform.ErrForbidden()
			}
			if (status == "published" && j.Status == "closed") || j.Status == status {
				return platform.ErrConflict("invalid_status", "Status lowongan tidak dapat diubah ke status tersebut.")
			}
			out, err = q.SetJobStatus(ctx, sqlc.SetJobStatusParams{ID: id, Status: status, ExpectedVersion: req.Version})
			if platform.IsNoRows(err) {
				return platform.ErrConflict("stale_version", "Lowongan sudah diubah orang lain. Muat ulang.")
			}
			if err != nil {
				return err
			}
			return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff",
				Action: "job." + status, TargetType: "job", TargetID: id.String(), ScopeUnit: &j.UnitID, Reason: req.Reason, RequestID: platform.RequestID(c)})
		})
		if err != nil {
			platform.Fail(c, err)
			return
		}
		h.Cache.Invalidate(ctx)
		c.JSON(http.StatusOK, out)
	}
}
