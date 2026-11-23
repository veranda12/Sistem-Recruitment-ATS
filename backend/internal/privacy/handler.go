package privacy

import (
	"encoding/json"
	"fmt"
	"log/slog"
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
	Pool      *pgxpool.Pool
	Q         *sqlc.Queries
	OrgID     uuid.UUID
	ObjectDir string
}

// Data kandidat lintas unit: hanya pemegang privacy.manage dengan scope seluruh organisasi.
func (h *Handler) allowed(c *gin.Context) bool {
	if !auth.Current(c).Scope(auth.PermPrivacyManage).OrgWide {
		platform.Fail(c, platform.ErrForbidden())
		return false
	}
	return true
}

type candidateDTO struct {
	ID              uuid.UUID  `json:"id"`
	Name            string     `json:"name"`
	Email           string     `json:"email"`
	CreatedAt       time.Time  `json:"createdAt"`
	AnonymizedAt    *time.Time `json:"anonymizedAt"`
	LegalHoldAt     *time.Time `json:"legalHoldAt"`
	LegalHoldReason string     `json:"legalHoldReason"`
	Applications    int        `json:"applications"`
	ActiveApps      int        `json:"activeApplications"`
}

const candidateSQL = `SELECT u.id, u.display_name, u.email, u.created_at, u.anonymized_at, u.legal_hold_at, u.legal_hold_reason,
  (SELECT count(*) FROM job_applications a WHERE a.candidate_user_id = u.id),
  (SELECT count(*) FROM job_applications a WHERE a.candidate_user_id = u.id AND a.status = 'active')
FROM users u WHERE u.org_id = $1 AND u.kind = 'candidate'`

func scanCandidate(row pgx.Row) (candidateDTO, error) {
	var d candidateDTO
	err := row.Scan(&d.ID, &d.Name, &d.Email, &d.CreatedAt, &d.AnonymizedAt, &d.LegalHoldAt, &d.LegalHoldReason, &d.Applications, &d.ActiveApps)
	return d, err
}

func (h *Handler) load(c *gin.Context) (candidateDTO, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		platform.Fail(c, platform.ErrNotFound("Kandidat tidak ditemukan."))
		return candidateDTO{}, false
	}
	d, err := scanCandidate(h.Pool.QueryRow(c.Request.Context(), candidateSQL+` AND u.id = $2`, h.OrgID, id))
	if platform.IsNoRows(err) {
		platform.Fail(c, platform.ErrNotFound("Kandidat tidak ditemukan."))
		return d, false
	}
	if err != nil {
		platform.Fail(c, err)
		return d, false
	}
	return d, true
}

// GET /admin/privacy/candidates?email= : cari kandidat berdasarkan email persis (permintaan datang dari pemiliknya).
func (h *Handler) Find(c *gin.Context) {
	if !h.allowed(c) {
		return
	}
	email, ok := auth.NormalizeEmail(c.Query("email"))
	if !ok {
		platform.Fail(c, platform.ErrValidation(map[string]string{"email": "Masukkan email kandidat yang valid."}))
		return
	}
	d, err := scanCandidate(h.Pool.QueryRow(c.Request.Context(), candidateSQL+` AND u.email = $2`, h.OrgID, email))
	if platform.IsNoRows(err) {
		c.JSON(http.StatusOK, gin.H{"items": []candidateDTO{}})
		return
	}
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": []candidateDTO{d}})
}

// GET /admin/privacy/candidates/:id/export : salinan data kandidat (JSON), dicatat di audit.
func (h *Handler) Export(c *gin.Context) {
	if !h.allowed(c) {
		return
	}
	d, ok := h.load(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	data, err := Export(ctx, h.Pool, d.ID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	p := auth.Current(c)
	if err := platform.WriteAudit(ctx, h.Q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff", Action: "privacy.export",
		TargetType: "user", TargetID: d.ID.String(), RequestID: platform.RequestID(c)}); err != nil {
		platform.Fail(c, err)
		return
	}
	body, _ := json.MarshalIndent(gin.H{"dibuat": time.Now().UTC(), "kandidat": d.ID, "data": data}, "", "  ")
	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="data-kandidat-%s.json"`, d.ID.String()[:8]))
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

// POST /admin/privacy/candidates/:id/anonymize {reason, confirmEmail}
func (h *Handler) Anonymize(c *gin.Context) {
	if !h.allowed(c) {
		return
	}
	d, ok := h.load(c)
	if !ok {
		return
	}
	var req struct {
		Reason       string `json:"reason"`
		ConfirmEmail string `json:"confirmEmail"`
	}
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	f := map[string]string{}
	if len(req.Reason) < 10 {
		f["reason"] = "Tuliskan dasar penghapusan minimal 10 karakter, misalnya nomor permintaan."
	}
	if e, _ := auth.NormalizeEmail(req.ConfirmEmail); e != d.Email {
		f["confirmEmail"] = "Ketik ulang email kandidat persis untuk konfirmasi."
	}
	if len(f) > 0 {
		platform.Fail(c, platform.ErrValidation(f))
		return
	}
	ctx := c.Request.Context()
	p := auth.Current(c)
	var files []string
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var err error
		if files, err = Anonymize(ctx, tx, d.ID); err != nil {
			return Message(err)
		}
		return platform.WriteAudit(ctx, h.Q.WithTx(tx), platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff", Action: "privacy.anonymize",
			TargetType: "user", TargetID: d.ID.String(), Reason: req.Reason, RequestID: platform.RequestID(c), Detail: map[string]any{"files": len(files)}})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	if failed := RemoveFiles(h.ObjectDir, files); len(failed) > 0 {
		slog.Warn("anonimisasi: berkas belum terhapus", "user", d.ID.String(), "files", failed)
	}
	c.JSON(http.StatusOK, gin.H{"anonymized": true, "filesRemoved": len(files)})
}

// POST /admin/privacy/candidates/:id/legal-hold {hold, reason}
func (h *Handler) LegalHold(c *gin.Context) {
	if !h.allowed(c) {
		return
	}
	d, ok := h.load(c)
	if !ok {
		return
	}
	var req struct {
		Hold   bool   `json:"hold"`
		Reason string `json:"reason"`
	}
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if len(req.Reason) < 5 || len(req.Reason) > 500 {
		platform.Fail(c, platform.ErrValidation(map[string]string{"reason": "Alasan 5 sampai 500 karakter."}))
		return
	}
	ctx := c.Request.Context()
	p := auth.Current(c)
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		action := "privacy.legal_hold_set"
		var err error
		if req.Hold {
			_, err = tx.Exec(ctx, `UPDATE users SET legal_hold_at = now(), legal_hold_reason = $2 WHERE id = $1`, d.ID, req.Reason)
		} else {
			action = "privacy.legal_hold_release"
			_, err = tx.Exec(ctx, `UPDATE users SET legal_hold_at = NULL, legal_hold_reason = '' WHERE id = $1`, d.ID)
		}
		if err != nil {
			return err
		}
		return platform.WriteAudit(ctx, h.Q.WithTx(tx), platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff", Action: action,
			TargetType: "user", TargetID: d.ID.String(), Reason: req.Reason, RequestID: platform.RequestID(c)})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"legalHold": req.Hold})
}

// GET /admin/privacy/retention : kebijakan dan dry-run (siapa yang akan dianonimkan bila retensi otomatis aktif).
func (h *Handler) Retention(c *gin.Context) {
	if !h.allowed(c) {
		return
	}
	ctx := c.Request.Context()
	months, enabled := Policy(ctx, h.Q, h.OrgID)
	list, total, err := Eligible(ctx, h.Pool, h.OrgID, months, 50)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"months": months, "enabled": enabled, "eligible": total, "sample": list})
}
