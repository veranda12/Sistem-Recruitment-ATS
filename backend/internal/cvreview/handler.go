package cvreview

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ats/db/sqlc"
	"ats/internal/auth"
	"ats/internal/cvparse"
	"ats/internal/platform"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	Pool      *pgxpool.Pool
	Q         *sqlc.Queries
	OrgID     uuid.UUID
	ObjectDir string
	Limits    cvparse.Limits
	Enq       *Enqueuer
}

// ---- DTO ----

type pageDTO struct {
	No       int32    `json:"no"`
	Method   string   `json:"method"`
	Status   string   `json:"status"`
	Failure  *string  `json:"failure,omitempty"`
	Message  string   `json:"message,omitempty"`
	HasImage bool     `json:"hasImage"`
	Width    *float32 `json:"width,omitempty"`
	Height   *float32 `json:"height,omitempty"`
	Scale    *float32 `json:"scale,omitempty"`
}

type evidenceDTO struct {
	ID      uuid.UUID `json:"id"`
	Page    int32     `json:"page"`
	BBox    []float32 `json:"bbox,omitempty"`
	Snippet string    `json:"snippet"`
	Source  string    `json:"source"`
	Locator string    `json:"locator,omitempty"`
}

type suggestionDTO struct {
	ID          uuid.UUID       `json:"id"`
	Path        string          `json:"path"`
	Section     string          `json:"section"`
	EntryKey    string          `json:"entryKey"`
	Field       string          `json:"field"`
	Raw         string          `json:"raw"`
	Normalized  json.RawMessage `json:"normalized"`
	EditedValue json.RawMessage `json:"editedValue,omitempty"`
	Status      string          `json:"status"`
	ReadDoubts  []string        `json:"readDoubts"`
	AssocDoubts []string        `json:"assocDoubts"`
	EvidenceIDs []uuid.UUID     `json:"evidenceIds"`
}

type runDTO struct {
	ID            uuid.UUID       `json:"id"`
	DocumentID    uuid.UUID       `json:"documentId"`
	Status        string          `json:"status"`
	Stage         string          `json:"stage"`
	PagesTotal    *int32          `json:"pagesTotal"`
	PagesDone     int32           `json:"pagesDone"`
	Failure       *failureDTO     `json:"failure,omitempty"`
	ReviewVersion int32           `json:"reviewVersion"`
	CreatedAt     time.Time       `json:"createdAt"`
	FinishedAt    *time.Time      `json:"finishedAt"`
	Pages         []pageDTO       `json:"pages"`
	Suggestions   []suggestionDTO `json:"suggestions"`
	Evidence      []evidenceDTO   `json:"evidence"`
	ApplyResult   json.RawMessage `json:"applyResult,omitempty"`
	StatusURL     string          `json:"statusUrl"`
}

type failureDTO struct {
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	Transient bool   `json:"transient"`
}

func runURL(id uuid.UUID) string { return "/api/v1/me/cv-parses/" + id.String() }

func (h *Handler) buildRun(ctx context.Context, q *sqlc.Queries, r sqlc.CvParseRun, detail bool) (runDTO, error) {
	out := runDTO{ID: r.ID, DocumentID: r.DocumentID, Status: r.Status, Stage: r.Stage, PagesTotal: r.PagesTotal, PagesDone: r.PagesDone,
		ReviewVersion: r.ReviewVersion, CreatedAt: r.CreatedAt, FinishedAt: r.FinishedAt, StatusURL: runURL(r.ID),
		Pages: []pageDTO{}, Suggestions: []suggestionDTO{}, Evidence: []evidenceDTO{}}
	if r.FailureReason != nil {
		out.Failure = &failureDTO{Reason: *r.FailureReason, Message: failureMessage(*r.FailureReason), Transient: r.FailureTransient != nil && *r.FailureTransient}
	}
	if !detail {
		return out, nil
	}
	pages, err := q.ListCVParsePages(ctx, r.ID)
	if err != nil {
		return out, err
	}
	for _, p := range pages {
		pd := pageDTO{No: p.PageNo, Method: p.Method, Status: p.Status, Failure: p.FailureReason, HasImage: p.ImageName != nil && r.Status != "cancelled",
			Width: p.Width, Height: p.Height, Scale: p.Scale}
		if p.FailureReason != nil {
			pd.Message = failureMessage(*p.FailureReason)
		}
		out.Pages = append(out.Pages, pd)
	}
	if r.Status == "cancelled" {
		return out, nil
	}
	sugs, err := q.ListCVSuggestions(ctx, r.ID)
	if err != nil {
		return out, err
	}
	for _, s := range sugs {
		out.Suggestions = append(out.Suggestions, suggestionDTO{ID: s.ID, Path: s.Path, Section: s.Section, EntryKey: s.EntryKey, Field: s.Field, Raw: s.RawValue,
			Normalized: s.Normalized, EditedValue: s.EditedValue, Status: s.Status, ReadDoubts: s.ReadDoubts, AssocDoubts: s.AssocDoubts, EvidenceIDs: s.EvidenceIds})
	}
	evs, err := q.ListCVEvidence(ctx, r.ID)
	if err != nil {
		return out, err
	}
	for _, e := range evs {
		out.Evidence = append(out.Evidence, evidenceDTO{ID: e.ID, Page: e.PageNo, BBox: e.Bbox, Snippet: e.Snippet, Source: e.Source, Locator: e.Locator})
	}
	if r.Status == "applied" {
		if d, err := q.LastCVApplyEvent(ctx, r.ID); err == nil {
			out.ApplyResult = d
		}
	}
	return out, nil
}

// failureMessage: pesan kandidat untuk kode alasan (termasuk alasan operasional di luar parser).
func failureMessage(reason string) string {
	switch reason {
	case "scan_infected":
		return "Berkas ditolak pemindai keamanan. Unggah berkas lain atau isi profil secara manual."
	case "scan_error":
		return "Pemindaian keamanan belum dapat dilakukan. Coba lagi nanti atau isi profil secara manual."
	case "attempts_exhausted", "internal_error":
		return "Pembacaan CV terhenti berulang kali. Coba unggah ulang atau isi profil secara manual."
	}
	return cvparse.UserMessage(cvparse.Reason(reason))
}

// ---- Kandidat ----

// POST /me/documents/:id/parse — membuat atau mengambil run idempoten; 202 + URL status.
func (h *Handler) RequestParse(c *gin.Context) {
	p := auth.Current(c)
	docID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		platform.Fail(c, platform.ErrNotFound("Dokumen tidak ditemukan."))
		return
	}
	ctx := c.Request.Context()
	d, err := h.Q.GetDocument(ctx, docID)
	// Dokumen orang lain atau terhapus: 404 yang sama (tidak mengungkap keberadaan).
	if platform.IsNoRows(err) || (err == nil && (d.OwnerUserID != p.UserID || d.DeletedAt != nil)) {
		platform.Fail(c, platform.ErrNotFound("Dokumen tidak ditemukan."))
		return
	}
	if err != nil {
		platform.Fail(c, err)
		return
	}
	if d.Kind != "cv" {
		platform.Fail(c, platform.ErrValidation(map[string]string{"document": "Pilih dokumen berjenis CV."}))
		return
	}
	initial := "queued"
	switch d.ScanStatus {
	case "pending":
		initial = "waiting_scan"
	case "infected":
		platform.Fail(c, platform.ErrConflict("scan_infected", failureMessage("scan_infected")))
		return
	}
	var run sqlc.CvParseRun
	err = platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		var err error
		run, err = q.CreateCVParseRun(ctx, sqlc.CreateCVParseRunParams{OrgID: h.OrgID, CandidateUserID: p.UserID, DocumentID: d.ID,
			ParserConfigVersion: ConfigVersion(h.Limits), Status: initial})
		if err != nil {
			return err
		}
		// Coba lagi atas permintaan kandidat untuk run yang gagal (bukan dead-letter operasional).
		if run.Status == "failed" && c.Query("retry") == "1" && run.DeadLetteredAt == nil {
			if _, err := q.ReplayCVParseRun(ctx, run.ID); err != nil {
				return err
			}
			run, err = q.GetCVParseRun(ctx, run.ID)
			if err != nil {
				return err
			}
		}
		return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "candidate", Action: "cv_parse.request",
			TargetType: "cv_parse_run", TargetID: run.ID.String(), RequestID: platform.RequestID(c)})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	if run.Status == "queued" {
		_ = h.Enq.Signal(ctx, run.ID, run.Fence) // gagal = dispatcher mengambil dari PostgreSQL
	}
	out, err := h.buildRun(ctx, h.Q, run, false)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.Header("Location", out.StatusURL)
	c.JSON(http.StatusAccepted, out)
}

// GET /me/cv-parses — run terbaru milik kandidat (untuk melanjutkan setelah refresh).
func (h *Handler) MyRuns(c *gin.Context) {
	rows, err := h.Q.ListCVParseRunsForCandidate(c.Request.Context(), auth.Current(c).UserID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	type item struct {
		runDTO
		DocumentName string `json:"documentName"`
	}
	items := []item{}
	for _, r := range rows {
		run := sqlc.CvParseRun{ID: r.ID, DocumentID: r.DocumentID, Status: r.Status, Stage: r.Stage, PagesTotal: r.PagesTotal, PagesDone: r.PagesDone,
			ReviewVersion: r.ReviewVersion, CreatedAt: r.CreatedAt, FinishedAt: r.FinishedAt, FailureReason: r.FailureReason, FailureTransient: r.FailureTransient}
		d, _ := h.buildRun(c.Request.Context(), h.Q, run, false)
		items = append(items, item{d, r.DocumentName})
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// loadOwn: run milik kandidat; selain itu 404.
func (h *Handler) loadOwn(c *gin.Context) (sqlc.CvParseRun, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		platform.Fail(c, platform.ErrNotFound("Data pembacaan CV tidak ditemukan."))
		return sqlc.CvParseRun{}, false
	}
	r, err := h.Q.GetCVParseRun(c.Request.Context(), id)
	if platform.IsNoRows(err) || (err == nil && r.CandidateUserID != auth.Current(c).UserID) {
		platform.Fail(c, platform.ErrNotFound("Data pembacaan CV tidak ditemukan."))
		return r, false
	}
	if err != nil {
		platform.Fail(c, err)
		return r, false
	}
	return r, true
}

// GET /me/cv-parses/:id
func (h *Handler) GetRun(c *gin.Context) {
	r, ok := h.loadOwn(c)
	if !ok {
		return
	}
	out, err := h.buildRun(c.Request.Context(), h.Q, r, true)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, out)
}

// GET /me/cv-parses/:id/pages/:n/image — preview halaman privat untuk panel bukti.
func (h *Handler) PageImage(c *gin.Context) {
	r, ok := h.loadOwn(c)
	if !ok {
		return
	}
	h.servePage(c, r)
}

func (h *Handler) servePage(c *gin.Context, r sqlc.CvParseRun) {
	n, err := strconv.Atoi(c.Param("n"))
	if err != nil || n < 1 || r.Status == "cancelled" {
		platform.Fail(c, platform.ErrNotFound("Halaman tidak ditemukan."))
		return
	}
	ctx := c.Request.Context()
	d, err := h.Q.GetDocument(ctx, r.DocumentID)
	if err != nil || d.DeletedAt != nil {
		platform.Fail(c, platform.ErrNotFound("Halaman tidak ditemukan."))
		return
	}
	pg, err := h.Q.GetCVParsePage(ctx, sqlc.GetCVParsePageParams{RunID: r.ID, PageNo: int32(n)})
	if err != nil || pg.ImageName == nil || strings.Contains(*pg.ImageName, "..") {
		platform.Fail(c, platform.ErrNotFound("Halaman tidak ditemukan."))
		return
	}
	path := filepath.Join(h.ObjectDir, "cvparse", r.ID.String(), filepath.FromSlash(*pg.ImageName))
	if _, err := os.Stat(path); err != nil {
		platform.Fail(c, platform.ErrNotFound("Halaman tidak ditemukan."))
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Type", "image/jpeg")
	c.Header("Content-Disposition", "inline")
	c.File(path)
}

// DELETE /me/cv-parses/:id — kandidat mengabaikan hasil (profil tidak berubah).
func (h *Handler) Dismiss(c *gin.Context) {
	r, ok := h.loadOwn(c)
	if !ok {
		return
	}
	p := auth.Current(c)
	ctx := c.Request.Context()
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		if _, err := q.MarkCVRunDismissed(ctx, r.ID); err != nil {
			return err
		}
		return q.InsertCVReviewEvent(ctx, sqlc.InsertCVReviewEventParams{RunID: r.ID, ActorUserID: &p.UserID, Action: "dismiss", Detail: []byte("{}")})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ---- Staf ----

// GET /cv-parses/:id — staf dengan applications.read pada salah satu lamaran kandidat (role + scope).
func (h *Handler) StaffGetRun(c *gin.Context) {
	r, ok := h.loadStaff(c)
	if !ok {
		return
	}
	out, err := h.buildRun(c.Request.Context(), h.Q, r, true)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	p := auth.Current(c)
	_ = platform.WriteAudit(c.Request.Context(), h.Q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff", Action: "cv_parse.view",
		TargetType: "cv_parse_run", TargetID: r.ID.String(), RequestID: platform.RequestID(c)})
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, out)
}

// GET /cv-parses/:id/pages/:n/image (staf, scope sama)
func (h *Handler) StaffPageImage(c *gin.Context) {
	r, ok := h.loadStaff(c)
	if !ok {
		return
	}
	h.servePage(c, r)
}

func (h *Handler) loadStaff(c *gin.Context) (sqlc.CvParseRun, bool) {
	p := auth.Current(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		platform.Fail(c, platform.ErrNotFound(""))
		return sqlc.CvParseRun{}, false
	}
	ctx := c.Request.Context()
	r, err := h.Q.GetCVParseRun(ctx, id)
	if platform.IsNoRows(err) {
		platform.Fail(c, platform.ErrNotFound(""))
		return r, false
	}
	if err != nil {
		platform.Fail(c, err)
		return r, false
	}
	scopes, err := h.Q.ApplicationScopesOfCandidate(ctx, r.CandidateUserID)
	if err != nil {
		platform.Fail(c, err)
		return r, false
	}
	for _, s := range scopes {
		job := s.JobID
		if p.Can(auth.PermApplicationsRead, auth.Resource{Unit: s.UnitID, Job: &job}) {
			return r, true
		}
	}
	platform.Fail(c, platform.ErrNotFound(""))
	return r, false
}

// POST /admin/cv-parses/:id/replay — replay dead-letter oleh pemegang cv_parses.replay.
func (h *Handler) Replay(c *gin.Context) {
	p := auth.Current(c)
	if !p.CanAny(auth.PermCVReplay) {
		platform.Fail(c, platform.ErrNotFound(""))
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		platform.Fail(c, platform.ErrNotFound(""))
		return
	}
	ctx := c.Request.Context()
	var n int64
	err = platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		var err error
		if n, err = q.ReplayCVParseRun(ctx, id); err != nil || n == 0 {
			return err
		}
		if err := q.InsertCVReviewEvent(ctx, sqlc.InsertCVReviewEventParams{RunID: id, ActorUserID: &p.UserID, Action: "replay", Detail: []byte("{}")}); err != nil {
			return err
		}
		return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff", Action: "cv_parse.replay",
			TargetType: "cv_parse_run", TargetID: id.String(), RequestID: platform.RequestID(c)})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	if n == 0 {
		platform.Fail(c, platform.ErrConflict("not_failed", "Hanya run yang gagal yang dapat dijalankan ulang."))
		return
	}
	_ = h.Enq.Signal(ctx, id, -1)
	c.Status(http.StatusAccepted)
}

// GET /admin/cv-parses/stats — kesehatan antrean (tanpa data pribadi).
func (h *Handler) Stats(c *gin.Context) {
	p := auth.Current(c)
	if !p.CanAny(auth.PermCVReplay) {
		platform.Fail(c, platform.ErrNotFound(""))
		return
	}
	st, err := h.Q.CVQueueStats(c.Request.Context())
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, st)
}
