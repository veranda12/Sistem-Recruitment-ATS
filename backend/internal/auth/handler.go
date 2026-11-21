package auth

import (
	"net/http"
	"net/mail"
	"strings"
	"time"

	"ats/db/sqlc"
	"ats/internal/platform"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Topik outbox yang dikonsumsi worker untuk email akun. Token dibuat oleh worker, tidak pernah disimpan mentah.
const (
	TopicVerifyEmail   = "auth.verify_email"
	TopicPasswordReset = "auth.password_reset"
	TopicAccountExists = "auth.account_exists"
)

type Handler struct {
	S     *Sessions
	OrgID uuid.UUID
}

type sessionDTO struct {
	User struct {
		ID            uuid.UUID `json:"id"`
		Kind          string    `json:"kind"`
		Email         string    `json:"email"`
		Name          string    `json:"name"`
		EmailVerified bool      `json:"emailVerified"`
	} `json:"user"`
	CSRFToken string `json:"csrfToken"`
	MFA       struct {
		Enabled  bool `json:"enabled"`
		Required bool `json:"required"`
	} `json:"mfa"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
}

func (h *Handler) sessionBody(p *Principal) sessionDTO {
	var d sessionDTO
	d.User.ID, d.User.Kind, d.User.Email, d.User.Name, d.User.EmailVerified = p.UserID, p.Kind, p.Email, p.Name, p.EmailVerified
	d.CSRFToken = h.S.CSRFToken(p.token)
	d.MFA.Enabled, d.MFA.Required = p.MFAEnabled, h.S.MFARequired(p)
	d.Roles = nonNil(p.Roles())
	d.Permissions = nonNil(p.Permissions())
	return d
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func NormalizeEmail(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s || len(s) > 254 {
		return "", false
	}
	return s, true
}

// GET /auth/session
func (h *Handler) Session(c *gin.Context) {
	p := Current(c)
	if p == nil {
		platform.Fail(c, platform.ErrUnauthenticated())
		return
	}
	c.JSON(http.StatusOK, h.sessionBody(p))
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Code     string `json:"code"` // TOTP atau kode pemulihan, wajib bila MFA aktif
}

// POST /auth/login. Respons gagal identik untuk email tidak dikenal, sandi salah, atau akun nonaktif.
func (h *Handler) Login(c *gin.Context) {
	var req loginReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	email, _ := NormalizeEmail(req.Email)
	invalid := &platform.APIError{Status: http.StatusUnauthorized, Code: "invalid_credentials", Message: "Email atau kata sandi salah."}
	user, err := h.S.Q.GetUserByEmail(ctx, sqlc.GetUserByEmailParams{OrgID: h.OrgID, Email: email})
	if err != nil {
		if !platform.IsNoRows(err) {
			platform.Fail(c, err)
			return
		}
		_, _ = VerifyPassword(ctx, dummyHash, req.Password)
		h.audit(c, nil, "auth.login", "denied", "unknown_account")
		platform.Fail(c, invalid)
		return
	}
	ok, err := VerifyPassword(ctx, user.PasswordHash, req.Password)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	if !ok || user.Status != "active" {
		h.audit(c, &user.ID, "auth.login", "denied", "bad_password_or_disabled")
		platform.Fail(c, invalid)
		return
	}
	if user.MfaEnabledAt != nil {
		if err := h.S.checkSecondFactor(c, user, req.Code); err != nil {
			h.audit(c, &user.ID, "auth.login", "denied", "mfa")
			platform.Fail(c, err)
			return
		}
	}
	// Rotasi: sesi lama dari cookie yang sama dicabut, sesi baru selalu dibuat.
	if old := Current(c); old != nil {
		_ = h.S.Q.RevokeSession(ctx, sqlc.RevokeSessionParams{ID: old.SessionID, RevokeReason: strp("rotated_on_login")})
	}
	token, digest := platform.RandomToken()
	now := time.Now()
	ip, ua := c.ClientIP(), c.Request.UserAgent()
	if len(ua) > 300 {
		ua = ua[:300]
	}
	sess, err := h.S.Q.CreateSession(ctx, sqlc.CreateSessionParams{
		UserID: user.ID, TokenDigest: digest, IdleExpiresAt: now.Add(h.S.idleFor(user.Kind)),
		AbsoluteExpiresAt: now.Add(h.S.Cfg.SessionAbsolute), Ip: &ip, UserAgent: &ua,
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	h.S.setCookie(c, token, int(h.S.Cfg.SessionAbsolute.Seconds()))
	grants, err := h.S.loadGrants(c, user.ID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	p := &Principal{UserID: user.ID, OrgID: user.OrgID, Kind: user.Kind, Email: user.Email, Name: user.DisplayName,
		EmailVerified: user.EmailVerifiedAt != nil, MFAEnabled: user.MfaEnabledAt != nil, SessionID: sess.ID, Grants: grants, token: token}
	h.audit(c, &user.ID, "auth.login", "success", "")
	c.JSON(http.StatusOK, h.sessionBody(p))
}

// POST /auth/logout
func (h *Handler) Logout(c *gin.Context) {
	if p := Current(c); p != nil {
		if err := h.S.Q.RevokeSession(c.Request.Context(), sqlc.RevokeSessionParams{ID: p.SessionID, RevokeReason: strp("logout")}); err != nil {
			platform.Fail(c, err)
			return
		}
		h.audit(c, &p.UserID, "auth.logout", "success", "")
	}
	h.S.clearCookie(c)
	c.Status(http.StatusNoContent)
}

type tokenReq struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// POST /auth/verify-email
func (h *Handler) VerifyEmail(c *gin.Context) {
	var req tokenReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	err := platform.WithTx(ctx, h.S.Pool, func(tx pgx.Tx) error {
		q := h.S.Q.WithTx(tx)
		uid, err := q.ConsumeEmailToken(ctx, sqlc.ConsumeEmailTokenParams{TokenDigest: platform.Digest(req.Token), Purpose: "verify_email"})
		if platform.IsNoRows(err) {
			return &platform.APIError{Status: http.StatusBadRequest, Code: "token_invalid", Message: "Tautan verifikasi tidak valid atau sudah kedaluwarsa."}
		}
		if err != nil {
			return err
		}
		if err := q.MarkEmailVerified(ctx, uid); err != nil {
			return err
		}
		return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &uid, ActorKind: "candidate",
			Action: "auth.email_verified", TargetType: "user", TargetID: uid.String(), RequestID: platform.RequestID(c)})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"verified": true})
}

// POST /auth/resend-verification (bersesi)
func (h *Handler) ResendVerification(c *gin.Context) {
	p := Current(c)
	if p.EmailVerified {
		c.JSON(http.StatusOK, gin.H{"queued": false})
		return
	}
	ctx := c.Request.Context()
	if _, err := platform.Enqueue(ctx, h.S.Q, h.OrgID, TopicVerifyEmail, "user", p.UserID.String(), gin.H{"userId": p.UserID}); err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"queued": true})
}

type emailReq struct {
	Email string `json:"email"`
}

// POST /auth/forgot-password. Respons netral apa pun hasilnya.
func (h *Handler) ForgotPassword(c *gin.Context) {
	var req emailReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	if email, ok := NormalizeEmail(req.Email); ok {
		u, err := h.S.Q.GetUserByEmail(ctx, sqlc.GetUserByEmailParams{OrgID: h.OrgID, Email: email})
		if err == nil && u.Status == "active" {
			if _, err := platform.Enqueue(ctx, h.S.Q, h.OrgID, TopicPasswordReset, "user", u.ID.String(), gin.H{"userId": u.ID}); err != nil {
				platform.Fail(c, err)
				return
			}
		} else if err != nil && !platform.IsNoRows(err) {
			platform.Fail(c, err)
			return
		}
	}
	c.JSON(http.StatusAccepted, gin.H{"message": "Jika email terdaftar, kami telah mengirim tautan untuk mengatur ulang kata sandi."})
}

// POST /auth/reset-password: token sekali pakai, ganti hash, cabut semua sesi.
func (h *Handler) ResetPassword(c *gin.Context) {
	var req tokenReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	if msg := ValidatePassword(req.Password); msg != "" {
		platform.Fail(c, platform.ErrValidation(map[string]string{"password": msg}))
		return
	}
	ctx := c.Request.Context()
	hash, err := HashPassword(ctx, req.Password)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	err = platform.WithTx(ctx, h.S.Pool, func(tx pgx.Tx) error {
		q := h.S.Q.WithTx(tx)
		uid, err := q.ConsumeEmailToken(ctx, sqlc.ConsumeEmailTokenParams{TokenDigest: platform.Digest(req.Token), Purpose: "reset_password"})
		if platform.IsNoRows(err) {
			return &platform.APIError{Status: http.StatusBadRequest, Code: "token_invalid", Message: "Tautan atur ulang tidak valid atau sudah kedaluwarsa."}
		}
		if err != nil {
			return err
		}
		if err := q.UpdatePassword(ctx, sqlc.UpdatePasswordParams{ID: uid, PasswordHash: hash}); err != nil {
			return err
		}
		if err := q.InvalidateEmailTokens(ctx, sqlc.InvalidateEmailTokensParams{UserID: uid, Purpose: "reset_password"}); err != nil {
			return err
		}
		// Reset lewat email juga membuktikan akses email.
		if err := q.MarkEmailVerified(ctx, uid); err != nil {
			return err
		}
		n, err := q.RevokeUserSessions(ctx, sqlc.RevokeUserSessionsParams{UserID: uid, RevokeReason: strp("password_reset")})
		if err != nil {
			return err
		}
		return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &uid, ActorKind: "anonymous",
			Action: "auth.password_reset", TargetType: "user", TargetID: uid.String(), RequestID: platform.RequestID(c),
			Detail: map[string]any{"sessions_revoked": n}})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	h.S.clearCookie(c)
	c.JSON(http.StatusOK, gin.H{"reset": true})
}

func (h *Handler) audit(c *gin.Context, uid *uuid.UUID, action, outcome, reason string) {
	kind := "anonymous"
	if uid != nil {
		kind = "staff"
		if u, err := h.S.Q.GetUser(c.Request.Context(), *uid); err == nil {
			kind = u.Kind
		}
	}
	target := ""
	if uid != nil {
		target = uid.String()
	}
	_ = platform.WriteAudit(c.Request.Context(), h.S.Q, platform.AuditEntry{OrgID: h.OrgID, ActorID: uid, ActorKind: kind,
		Action: action, TargetType: "user", TargetID: target, Outcome: outcome, Reason: reason, RequestID: platform.RequestID(c)})
}

func strp(s string) *string { return &s }
