// Package reporting: dashboard rekrutmen per kohort, ekspor CSV asinkron, tautan unduh berumur pendek.
package reporting

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // zona waktu tersedia di Windows tanpa database tz sistem

	"ats/db/sqlc"
	"ats/internal/auth"
	"ats/internal/platform"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	TopicExport = "report.export"
	ExportTTL   = 24 * time.Hour
	LinkTTL     = 60 * time.Second
)

// Range: tanggal YYYY-MM-DD ditafsirkan di zona organisasi; to inklusif (diubah ke [from, to+1hari)).
type Range struct {
	From, To string
	FromAt   time.Time
	ToAt     time.Time
	TimeZone string
}

func ParseRange(from, to, tz string) (Range, error) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return Range{}, err
	}
	now := time.Now().In(loc)
	if to == "" {
		to = now.Format("2006-01-02")
	}
	if from == "" {
		from = now.AddDate(0, 0, -89).Format("2006-01-02")
	}
	f, err1 := time.ParseInLocation("2006-01-02", from, loc)
	t, err2 := time.ParseInLocation("2006-01-02", to, loc)
	if err1 != nil || err2 != nil || t.Before(f) || t.Sub(f) > 731*24*time.Hour {
		return Range{}, platform.ErrValidation(map[string]string{"from": "Rentang tanggal tidak valid (maksimal 2 tahun)."})
	}
	return Range{From: from, To: to, FromAt: f, ToAt: t.AddDate(0, 0, 1), TimeZone: tz}, nil
}

type JobRow struct {
	JobID        uuid.UUID        `json:"jobId"`
	JobTitle     string           `json:"jobTitle"`
	Applications int64            `json:"applications"`
	Candidates   int64            `json:"candidates"`
	Active       int64            `json:"active"`
	Withdrawn    int64            `json:"withdrawn"`
	Rejected     int64            `json:"rejected"`
	Hired        int64            `json:"hired"`
	Reached      map[string]int64 `json:"reached"`
}

type Report struct {
	Range       Range                    `json:"range"`
	Definitions map[string]string        `json:"definitions"`
	Totals      JobRow                   `json:"totals"`
	Jobs        []JobRow                 `json:"jobs"`
	Durations   []sqlc.StageDurationsRow `json:"durations"`
}

// Build menghitung laporan dengan scope s. Dipakai dashboard dan ekspor sehingga angka identik untuk filter sama.
func Build(ctx context.Context, q *sqlc.Queries, org uuid.UUID, s auth.Scope, r Range, job *uuid.UUID) (Report, error) {
	rep := Report{Range: r, Jobs: []JobRow{}, Definitions: map[string]string{
		"kohort":       "Lamaran dengan tanggal melamar (zona " + r.TimeZone + ") dalam rentang, inklusif.",
		"applications": "Penyebut: jumlah lamaran, bukan jumlah orang. Kandidat dengan dua lamaran dihitung dua kali.",
		"candidates":   "Jumlah orang unik di antara lamaran tersebut.",
		"reached":      "Lamaran yang pernah mencapai tahap itu (dari histori), termasuk yang kemudian ditarik atau ditolak.",
		"withdrawn":    "Status saat ini: ditarik oleh kandidat.",
	}}
	st, err := q.StatusByJob(ctx, sqlc.StatusByJobParams{OrgID: org, OrgWide: s.OrgWide, UnitIds: s.Units, JobIds: s.Jobs, FromAt: r.FromAt, ToAt: r.ToAt, JobID: job})
	if err != nil {
		return rep, err
	}
	fn, err := q.FunnelByStage(ctx, sqlc.FunnelByStageParams{OrgID: org, OrgWide: s.OrgWide, UnitIds: s.Units, JobIds: s.Jobs, FromAt: r.FromAt, ToAt: r.ToAt, JobID: job})
	if err != nil {
		return rep, err
	}
	reached := map[uuid.UUID]map[string]int64{}
	for _, f := range fn {
		if reached[f.JobID] == nil {
			reached[f.JobID] = map[string]int64{}
		}
		reached[f.JobID][f.StageKey] = f.Reached
	}
	rep.Totals.Reached = map[string]int64{}
	for _, x := range st {
		row := JobRow{JobID: x.JobID, JobTitle: x.JobTitle, Applications: x.Applications, Candidates: x.Candidates, Active: x.Active,
			Withdrawn: x.Withdrawn, Rejected: x.Rejected, Hired: x.Hired, Reached: reached[x.JobID]}
		if row.Reached == nil {
			row.Reached = map[string]int64{}
		}
		rep.Jobs = append(rep.Jobs, row)
		rep.Totals.Applications += x.Applications
		rep.Totals.Active += x.Active
		rep.Totals.Withdrawn += x.Withdrawn
		rep.Totals.Rejected += x.Rejected
		rep.Totals.Hired += x.Hired
		for k, v := range row.Reached {
			rep.Totals.Reached[k] += v
		}
	}
	// Kandidat unik lintas lowongan tidak bisa dijumlahkan per lowongan; hitung dari daftar ID.
	if len(st) > 0 {
		n, err := q.CandidatesInCohort(ctx, sqlc.CandidatesInCohortParams{OrgID: org, OrgWide: s.OrgWide, UnitIds: s.Units, JobIds: s.Jobs, FromAt: r.FromAt, ToAt: r.ToAt, JobID: job})
		if err != nil {
			return rep, err
		}
		rep.Totals.Candidates = n
	}
	rep.Durations, err = q.StageDurations(ctx, sqlc.StageDurationsParams{OrgID: org, OrgWide: s.OrgWide, UnitIds: s.Units, JobIds: s.Jobs, FromAt: r.FromAt, ToAt: r.ToAt, JobID: job})
	return rep, err
}

// SafeCell menetralkan spreadsheet formula injection (=, +, -, @, tab, CR di awal sel).
func SafeCell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// ---- HTTP ----

type Handler struct {
	Pool       *pgxpool.Pool
	Q          *sqlc.Queries
	OrgID      uuid.UUID
	ObjectDir  string
	SigningKey []byte
	Limiter    *platform.Limiter
	Strict     bool
}

func (h *Handler) orgTZ(ctx context.Context) string {
	if o, err := h.Q.GetOrganization(ctx, h.OrgID); err == nil {
		return o.Timezone
	}
	return "Asia/Jakarta"
}

func jobParam(c *gin.Context) (*uuid.UUID, error) {
	if v := c.Query("job"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return nil, platform.ErrValidation(map[string]string{"job": "ID lowongan tidak valid."})
		}
		return &id, nil
	}
	return nil, nil
}

// GET /reports/recruitment?from=&to=&job=
func (h *Handler) Recruitment(c *gin.Context) {
	p := auth.Current(c)
	ctx := c.Request.Context()
	r, err := ParseRange(c.Query("from"), c.Query("to"), h.orgTZ(ctx))
	if err != nil {
		platform.Fail(c, err)
		return
	}
	job, err := jobParam(c)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	rep, err := Build(ctx, h.Q, h.OrgID, p.Scope(auth.PermReportsRead), r, job)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, rep)
}

type exportReq struct {
	Kind string `json:"kind"`
	From string `json:"from"`
	To   string `json:"to"`
}

// POST /reports/exports: scope diperiksa saat request; pekerjaan dijalankan worker.
// Ekspor data pribadi (kind=applications) butuh pii.export dan limiter global (ditolak 503 saat Redis tidak tersedia).
func (h *Handler) CreateExport(c *gin.Context) {
	p := auth.Current(c)
	var req exportReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	if _, err := ParseRange(req.From, req.To, h.orgTZ(ctx)); err != nil {
		platform.Fail(c, err)
		return
	}
	switch req.Kind {
	case "funnel":
		if !p.CanAny(auth.PermReportsExport) {
			platform.Fail(c, platform.ErrForbidden())
			return
		}
	case "applications":
		if !p.CanAny(auth.PermPIIExport) {
			platform.Fail(c, platform.ErrForbidden())
			return
		}
		if h.Strict && !h.Limiter.Global() {
			platform.Fail(c, platform.ErrUnavailable("Ekspor data pribadi sementara tidak tersedia karena layanan pembatas lalu lintas tidak aktif.", 60))
			return
		}
	default:
		platform.Fail(c, platform.ErrValidation(map[string]string{"kind": "Jenis ekspor tidak dikenal."}))
		return
	}
	params, _ := json.Marshal(req)
	var ex sqlc.Export
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		var err error
		ex, err = q.CreateExport(ctx, sqlc.CreateExportParams{OrgID: h.OrgID, RequestedBy: p.UserID, Kind: req.Kind, Params: params})
		if err != nil {
			return err
		}
		if _, err := platform.Enqueue(ctx, q, h.OrgID, TopicExport, "export", ex.ID.String(), gin.H{"exportId": ex.ID}); err != nil {
			return err
		}
		return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff", Action: "report.export_request",
			TargetType: "export", TargetID: ex.ID.String(), RequestID: platform.RequestID(c), Detail: map[string]any{"kind": req.Kind, "from": req.From, "to": req.To}})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, exportDTO(ex))
}

func exportDTO(e sqlc.Export) gin.H {
	return gin.H{"id": e.ID, "kind": e.Kind, "status": e.Status, "rowCount": e.RowCount, "error": e.Error, "createdAt": e.CreatedAt,
		"finishedAt": e.FinishedAt, "expiresAt": e.ExpiresAt, "params": json.RawMessage(e.Params)}
}

// GET /reports/exports
func (h *Handler) MyExports(c *gin.Context) {
	rows, err := h.Q.ListMyExports(c.Request.Context(), auth.Current(c).UserID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	out := []gin.H{}
	for _, r := range rows {
		out = append(out, exportDTO(r))
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func (h *Handler) ownExport(c *gin.Context, id uuid.UUID) (sqlc.Export, bool) {
	e, err := h.Q.GetExport(c.Request.Context(), id)
	if platform.IsNoRows(err) || (err == nil && e.RequestedBy != auth.Current(c).UserID) {
		platform.Fail(c, platform.ErrNotFound("Ekspor tidak ditemukan."))
		return e, false
	}
	if err != nil {
		platform.Fail(c, err)
		return e, false
	}
	return e, true
}

func (h *Handler) permFor(kind string) string {
	if kind == "applications" {
		return auth.PermPIIExport
	}
	return auth.PermReportsExport
}

func (h *Handler) sign(exportID, userID uuid.UUID, exp int64) string {
	m := hmac.New(sha256.New, h.SigningKey)
	fmt.Fprintf(m, "export:%s:%s:%d", exportID, userID, exp)
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// POST /reports/exports/:id/link: tautan unduh 60 detik, terikat ekspor + pengguna.
func (h *Handler) Link(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		platform.Fail(c, platform.ErrNotFound(""))
		return
	}
	e, ok := h.ownExport(c, id)
	if !ok {
		return
	}
	if e.Status != "ready" {
		platform.Fail(c, platform.ErrConflict("export_not_ready", "Ekspor belum siap atau sudah kedaluwarsa."))
		return
	}
	p := auth.Current(c)
	exp := time.Now().Add(LinkTTL).Unix()
	url := fmt.Sprintf("/api/v1/reports/download?e=%s&exp=%d&sig=%s", id, exp, h.sign(id, p.UserID, exp))
	c.JSON(http.StatusOK, gin.H{"url": url, "expiresAt": time.Unix(exp, 0).UTC()})
}

// GET /reports/download: verifikasi tanda tangan, umur, pemilik, izin saat ini, dan status ekspor.
func (h *Handler) Download(c *gin.Context) {
	p := auth.Current(c)
	id, err := uuid.Parse(c.Query("e"))
	exp, err2 := strconv.ParseInt(c.Query("exp"), 10, 64)
	if err != nil || err2 != nil || time.Now().Unix() > exp || !hmac.Equal([]byte(c.Query("sig")), []byte(h.sign(id, p.UserID, exp))) {
		platform.Fail(c, &platform.APIError{Status: http.StatusForbidden, Code: "link_invalid", Message: "Tautan unduh tidak valid atau sudah kedaluwarsa."})
		return
	}
	e, ok := h.ownExport(c, id)
	if !ok {
		return
	}
	if !p.CanAny(h.permFor(e.Kind)) {
		platform.Fail(c, platform.ErrForbidden())
		return
	}
	if e.Status != "ready" || e.FileKey == nil || (e.ExpiresAt != nil && e.ExpiresAt.Before(time.Now())) {
		platform.Fail(c, platform.ErrConflict("export_not_ready", "Ekspor belum siap atau sudah kedaluwarsa."))
		return
	}
	_ = platform.WriteAudit(c.Request.Context(), h.Q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff", Action: "report.export_download",
		TargetType: "export", TargetID: id.String(), RequestID: platform.RequestID(c)})
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="laporan-%s-%s.csv"`, e.Kind, e.CreatedAt.Format("20060102")))
	c.Header("Cache-Control", "private, no-store")
	c.File(filepath.Join(h.ObjectDir, *e.FileKey))
}

// ---- Worker ----

// Generate dijalankan worker. Scope diperiksa ulang dari hak akses pemohon saat ini.
func Generate(ctx context.Context, pool *pgxpool.Pool, objectDir string, payload []byte) error {
	var p struct {
		ExportID uuid.UUID `json:"exportId"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return err
	}
	q := sqlc.New(pool)
	n, err := q.MarkExportRunning(ctx, p.ExportID)
	if err != nil || n == 0 {
		return err // sudah diproses (delivery minimal sekali)
	}
	e, err := q.GetExport(ctx, p.ExportID)
	if err != nil {
		return err
	}
	fail := func(msg string) error {
		return q.FinishExport(ctx, sqlc.FinishExportParams{ID: e.ID, Status: "failed", Error: &msg})
	}
	grants, err := q.ListUserGrants(ctx, e.RequestedBy)
	if err != nil {
		return err
	}
	pr := &auth.Principal{}
	for _, g := range grants {
		pr.Grants = append(pr.Grants, auth.Grant{Perm: g.PermissionKey, Unit: g.UnitID, Job: g.JobID, Role: g.RoleKey})
	}
	var req exportReq
	_ = json.Unmarshal(e.Params, &req)
	org, err := q.GetOrganization(ctx, e.OrgID)
	if err != nil {
		return err
	}
	r, err := ParseRange(req.From, req.To, org.Timezone)
	if err != nil {
		return fail("rentang tanggal tidak valid")
	}
	rows := [][]string{}
	switch e.Kind {
	case "funnel":
		if !pr.CanAny(auth.PermReportsExport) {
			return fail("izin ekspor sudah dicabut")
		}
		rep, err := Build(ctx, q, e.OrgID, pr.Scope(auth.PermReportsRead), r, nil)
		if err != nil {
			return err
		}
		stages := map[string]bool{}
		for _, j := range rep.Jobs {
			for k := range j.Reached {
				stages[k] = true
			}
		}
		keys := []string{}
		for k := range stages {
			keys = append(keys, k)
		}
		sortStrings(keys)
		head := []string{"lowongan", "lamaran", "kandidat_unik", "aktif", "ditarik", "ditolak", "diterima"}
		for _, k := range keys {
			head = append(head, "mencapai_"+k)
		}
		rows = append(rows, head)
		for _, j := range rep.Jobs {
			row := []string{SafeCell(j.JobTitle), i64(j.Applications), i64(j.Candidates), i64(j.Active), i64(j.Withdrawn), i64(j.Rejected), i64(j.Hired)}
			for _, k := range keys {
				row = append(row, i64(j.Reached[k]))
			}
			rows = append(rows, row)
		}
	case "applications":
		if !pr.CanAny(auth.PermPIIExport) {
			return fail("izin ekspor data pribadi sudah dicabut")
		}
		s := pr.Scope(auth.PermPIIExport)
		apps, err := q.ApplicationsForExport(ctx, sqlc.ApplicationsForExportParams{OrgID: e.OrgID, OrgWide: s.OrgWide, UnitIds: s.Units, JobIds: s.Jobs, FromAt: r.FromAt, ToAt: r.ToAt})
		if err != nil {
			return err
		}
		loc, _ := time.LoadLocation(org.Timezone)
		rows = append(rows, []string{"id_lamaran", "lowongan", "unit", "nama", "email", "tahap", "status", "tanggal_melamar"})
		for _, a := range apps {
			rows = append(rows, []string{a.ID.String(), SafeCell(a.JobTitle), SafeCell(a.UnitName), SafeCell(a.CandidateName), SafeCell(a.CandidateEmail),
				a.StageKey, a.Status, a.AppliedAt.In(loc).Format("2006-01-02 15:04")})
		}
	}
	rnd := make([]byte, 16)
	_, _ = rand.Read(rnd)
	key := filepath.Join("exports", hex.EncodeToString(rnd)+".csv")
	path := filepath.Join(objectDir, key)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, _ = f.Write([]byte{0xEF, 0xBB, 0xBF}) // BOM agar Excel membaca UTF-8
	w := csv.NewWriter(f)
	w.Comma = ';'
	if err := w.WriteAll(rows); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	exp := time.Now().Add(ExportTTL)
	cnt := int32(len(rows) - 1)
	return q.FinishExport(ctx, sqlc.FinishExportParams{ID: e.ID, Status: "ready", FileKey: &key, RowCount: &cnt, ExpiresAt: &exp})
}

// CleanupExpired menghapus berkas ekspor kedaluwarsa.
func CleanupExpired(ctx context.Context, pool *pgxpool.Pool, objectDir string) int {
	q := sqlc.New(pool)
	rows, err := q.ExpiredExports(ctx)
	if err != nil {
		return 0
	}
	for _, r := range rows {
		if r.FileKey != nil {
			_ = os.Remove(filepath.Join(objectDir, *r.FileKey))
		}
		_ = q.MarkExportExpired(ctx, r.ID)
	}
	return len(rows)
}

func i64(n int64) string { return strconv.FormatInt(n, 10) }

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
