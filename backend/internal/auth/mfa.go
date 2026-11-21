package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // TOTP RFC 6238 memakai HMAC-SHA1 secara default; kompatibel dengan aplikasi autentikator umum.
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"ats/db/sqlc"
	"ats/internal/platform"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	totpPeriod = 30
	totpDigits = 6
	recoveryN  = 10
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// totpAt menghitung kode untuk langkah waktu tertentu (RFC 6238 / RFC 4226).
func totpAt(secret []byte, step int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	m := hmac.New(sha1.New, secret)
	m.Write(msg[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := (binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff) % 1000000
	return fmt.Sprintf("%06d", v)
}

// verifyTOTP menerima toleransi +-1 langkah dan menolak langkah yang sudah pernah dipakai (anti-replay).
func verifyTOTP(secret []byte, code string, now time.Time, lastStep *int64) (int64, bool) {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != totpDigits {
		return 0, false
	}
	cur := now.Unix() / totpPeriod
	for _, s := range []int64{cur - 1, cur, cur + 1} {
		if lastStep != nil && s <= *lastStep {
			continue
		}
		if hmac.Equal([]byte(totpAt(secret, s)), []byte(code)) {
			return s, true
		}
	}
	return 0, false
}

// Rahasia TOTP dienkripsi AES-GCM dengan kunci turunan SESSION_KEY (label terpisah dari CSRF).
func (s *Sessions) mfaAEAD() (cipher.AEAD, error) {
	m := hmac.New(sha256.New, s.Cfg.SessionKey)
	m.Write([]byte("mfa-secret-key:v1"))
	block, err := aes.NewCipher(m.Sum(nil))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (s *Sessions) sealSecret(secret []byte, user uuid.UUID) ([]byte, error) {
	g, err := s.mfaAEAD()
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, g.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return append(nonce, g.Seal(nil, nonce, secret, user[:])...), nil
}

func (s *Sessions) openSecret(blob []byte, user uuid.UUID) ([]byte, error) {
	g, err := s.mfaAEAD()
	if err != nil {
		return nil, err
	}
	if len(blob) < g.NonceSize() {
		return nil, errors.New("rahasia MFA rusak")
	}
	return g.Open(nil, blob[:g.NonceSize()], blob[g.NonceSize():], user[:])
}

// MFARequired: peran yang wajib MFA (dari konfigurasi MFA_REQUIRED_ROLES).
func (s *Sessions) MFARequired(p *Principal) bool {
	if p == nil || !p.IsStaff() {
		return false
	}
	for _, r := range p.Roles() {
		if slices.Contains(s.Cfg.MFARequiredRoles, r) {
			return true
		}
	}
	return false
}

// checkSecondFactor dipanggil saat login bila MFA aktif. Mengembalikan error netral bila kode salah.
func (s *Sessions) checkSecondFactor(c *gin.Context, user sqlc.User, code string) error {
	ctx := c.Request.Context()
	if strings.TrimSpace(code) == "" {
		return &platform.APIError{Status: http.StatusUnauthorized, Code: "mfa_required", Message: "Masukkan kode dari aplikasi autentikator atau kode pemulihan."}
	}
	return platform.WithTx(ctx, s.Pool, func(tx pgx.Tx) error {
		q := s.Q.WithTx(tx)
		u, err := q.LockUserMFA(ctx, user.ID)
		if err != nil {
			return err
		}
		secret, err := s.openSecret(u.MfaSecret, user.ID)
		if err != nil {
			return err
		}
		if step, ok := verifyTOTP(secret, code, time.Now(), u.MfaLastStep); ok {
			return q.SetMFALastStep(ctx, sqlc.SetMFALastStepParams{ID: user.ID, MfaLastStep: &step})
		}
		n, err := q.UseRecoveryCode(ctx, sqlc.UseRecoveryCodeParams{UserID: user.ID, CodeDigest: platform.Digest(normalizeRecovery(code))})
		if err != nil {
			return err
		}
		if n == 1 {
			return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: user.OrgID, ActorID: &user.ID, ActorKind: "staff", Action: "auth.mfa_recovery_used",
				TargetType: "user", TargetID: user.ID.String(), RequestID: platform.RequestID(c)})
		}
		return &platform.APIError{Status: http.StatusUnauthorized, Code: "mfa_invalid", Message: "Kode verifikasi salah atau sudah dipakai."}
	})
}

func normalizeRecovery(s string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(s), "-", ""), " ", ""))
}

// POST /auth/mfa/setup: buat rahasia baru (belum aktif). Mengembalikan URI otpauth dan rahasia untuk entri manual.
func (h *Handler) MFASetup(c *gin.Context) {
	p := Current(c)
	if !p.IsStaff() {
		platform.Fail(c, platform.ErrForbidden())
		return
	}
	ctx := c.Request.Context()
	if u, err := h.S.Q.GetUser(ctx, p.UserID); err == nil && u.MfaEnabledAt != nil {
		platform.Fail(c, platform.ErrConflict("mfa_already_enabled", "MFA sudah aktif. Nonaktifkan dulu untuk mendaftarkan perangkat baru."))
		return
	}
	secret := make([]byte, 20)
	if _, err := rand.Read(secret); err != nil {
		platform.Fail(c, err)
		return
	}
	blob, err := h.S.sealSecret(secret, p.UserID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	if err := h.S.Q.SetMFASecret(ctx, sqlc.SetMFASecretParams{ID: p.UserID, MfaSecret: blob}); err != nil {
		platform.Fail(c, err)
		return
	}
	issuer := "ATS"
	if o, err := h.S.Q.GetOrganization(ctx, h.OrgID); err == nil {
		issuer = o.Name
	}
	enc := b32.EncodeToString(secret)
	uri := fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s&period=%d&digits=%d", url.PathEscape(issuer), url.PathEscape(p.Email), enc, url.QueryEscape(issuer), totpPeriod, totpDigits)
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"secret": enc, "otpauthUri": uri})
}

// POST /auth/mfa/enable {code}: aktifkan setelah kode pertama benar; recovery code ditampilkan sekali.
func (h *Handler) MFAEnable(c *gin.Context) {
	p := Current(c)
	var req struct {
		Code string `json:"code"`
	}
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	var codes []string
	err := platform.WithTx(ctx, h.S.Pool, func(tx pgx.Tx) error {
		q := h.S.Q.WithTx(tx)
		u, err := q.LockUserMFA(ctx, p.UserID)
		if err != nil {
			return err
		}
		if u.MfaSecret == nil {
			return platform.ErrConflict("mfa_not_setup", "Mulai pendaftaran MFA terlebih dahulu.")
		}
		if u.MfaEnabledAt != nil {
			return platform.ErrConflict("mfa_already_enabled", "MFA sudah aktif.")
		}
		secret, err := h.S.openSecret(u.MfaSecret, p.UserID)
		if err != nil {
			return err
		}
		step, ok := verifyTOTP(secret, req.Code, time.Now(), nil)
		if !ok {
			return platform.ErrValidation(map[string]string{"code": "Kode tidak cocok. Periksa jam perangkat dan coba lagi."})
		}
		if err := q.EnableMFA(ctx, sqlc.EnableMFAParams{ID: p.UserID, MfaLastStep: &step}); err != nil {
			return err
		}
		if err := q.DeleteRecoveryCodes(ctx, p.UserID); err != nil {
			return err
		}
		for i := 0; i < recoveryN; i++ {
			b := make([]byte, 5)
			_, _ = rand.Read(b)
			raw := b32.EncodeToString(b)
			codes = append(codes, raw[:4]+"-"+raw[4:])
			if err := q.InsertRecoveryCode(ctx, sqlc.InsertRecoveryCodeParams{UserID: p.UserID, CodeDigest: platform.Digest(raw)}); err != nil {
				return err
			}
		}
		// Sesi lain dicabut; sesi ini tetap.
		if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at = now(), revoke_reason = 'mfa_enabled' WHERE user_id = $1 AND id <> $2 AND revoked_at IS NULL`, p.UserID, p.SessionID); err != nil {
			return err
		}
		return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff", Action: "auth.mfa_enabled",
			TargetType: "user", TargetID: p.UserID.String(), RequestID: platform.RequestID(c)})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"enabled": true, "recoveryCodes": codes})
}

// GET /auth/mfa: status MFA pengguna saat ini.
func (h *Handler) MFAStatus(c *gin.Context) {
	p := Current(c)
	u, err := h.S.Q.GetUser(c.Request.Context(), p.UserID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	n, _ := h.S.Q.CountRecoveryCodes(c.Request.Context(), p.UserID)
	c.JSON(http.StatusOK, gin.H{"enabled": u.MfaEnabledAt != nil, "enabledAt": u.MfaEnabledAt, "recoveryCodesLeft": n, "required": h.S.MFARequired(p)})
}

// POST /admin/users/:id/mfa-reset: admin (users.manage) mereset MFA pengguna lain yang kehilangan perangkat; teraudit.
func (h *Handler) MFAReset(c *gin.Context) {
	p := Current(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		platform.Fail(c, platform.ErrNotFound(""))
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	if len(strings.TrimSpace(req.Reason)) < 10 {
		platform.Fail(c, platform.ErrValidation(map[string]string{"reason": "Alasan reset MFA minimal 10 karakter (verifikasi identitas pemohon)."}))
		return
	}
	if id == p.UserID {
		platform.Fail(c, platform.ErrConflict("self_reset", "Reset MFA akun sendiri harus dilakukan admin lain."))
		return
	}
	ctx := c.Request.Context()
	err = platform.WithTx(ctx, h.S.Pool, func(tx pgx.Tx) error {
		q := h.S.Q.WithTx(tx)
		u, err := q.GetUser(ctx, id)
		if platform.IsNoRows(err) || (err == nil && (u.OrgID != h.OrgID || u.Kind != "staff")) {
			return platform.ErrNotFound("Pengguna tidak ditemukan.")
		}
		if err != nil {
			return err
		}
		if err := q.DisableMFA(ctx, id); err != nil {
			return err
		}
		if err := q.DeleteRecoveryCodes(ctx, id); err != nil {
			return err
		}
		if _, err := q.RevokeUserSessions(ctx, sqlc.RevokeUserSessionsParams{UserID: id, RevokeReason: strp("mfa_reset")}); err != nil {
			return err
		}
		return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff", Action: "auth.mfa_reset",
			TargetType: "user", TargetID: id.String(), Reason: req.Reason, RequestID: platform.RequestID(c)})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// RequireMFAEnrollment: staf dengan peran wajib-MFA yang belum mendaftar hanya boleh mengakses endpoint akun/MFA.
func (s *Sessions) RequireMFAEnrollment() gin.HandlerFunc {
	return func(c *gin.Context) {
		p := Current(c)
		if p != nil && !p.MFAEnabled && s.MFARequired(p) {
			platform.Fail(c, &platform.APIError{Status: http.StatusForbidden, Code: "mfa_enrollment_required",
				Message: "Peran Anda wajib memakai verifikasi dua langkah. Aktifkan di halaman Keamanan akun."})
			return
		}
		c.Next()
	}
}
