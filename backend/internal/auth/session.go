package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"ats/db/sqlc"
	"ats/internal/platform"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	SessionCookie = "ats_session"
	CSRFHeader    = "X-CSRF-Token"
	principalKey  = "principal"
)

// Principal adalah identitas terautentikasi + grant yang dibaca ulang dari DB pada setiap request,
// sehingga pencabutan role berlaku pada permintaan berikutnya.
type Principal struct {
	UserID        uuid.UUID
	OrgID         uuid.UUID
	Kind          string
	Email         string
	Name          string
	EmailVerified bool
	SessionID     uuid.UUID
	Grants        []Grant
	token         string
}

func (p *Principal) IsStaff() bool     { return p != nil && p.Kind == "staff" }
func (p *Principal) IsCandidate() bool { return p != nil && p.Kind == "candidate" }

// Sessions mengelola cookie, validasi sesi dan CSRF.
type Sessions struct {
	Pool       *pgxpool.Pool
	Q          *sqlc.Queries
	Cfg        platform.Config
	secure     bool
}

func NewSessions(pool *pgxpool.Pool, cfg platform.Config) *Sessions {
	return &Sessions{Pool: pool, Q: sqlc.New(pool), Cfg: cfg, secure: strings.HasPrefix(cfg.PublicOrigin, "https://")}
}

func (s *Sessions) idleFor(kind string) time.Duration {
	if kind == "staff" {
		return s.Cfg.StaffIdleTimeout
	}
	return s.Cfg.CandidateIdleTimeout
}

func (s *Sessions) CSRFToken(sessionToken string) string {
	m := hmac.New(sha256.New, s.Cfg.SessionKey)
	m.Write([]byte("csrf:" + sessionToken))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func (s *Sessions) setCookie(c *gin.Context, token string, maxAge int) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name: SessionCookie, Value: token, Path: "/", MaxAge: maxAge,
		HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode,
	})
}

func (s *Sessions) clearCookie(c *gin.Context) { s.setCookie(c, "", -1) }

// Load membaca sesi dari cookie (bila ada) dan memasang Principal. Tidak menolak request anonim.
func (s *Sessions) Load() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(SessionCookie)
		if err != nil || token == "" || len(token) > 100 {
			c.Next()
			return
		}
		ctx := c.Request.Context()
		row, err := s.Q.GetActiveSessionByDigest(ctx, platform.Digest(token))
		if err != nil {
			if !platform.IsNoRows(err) {
				platform.Fail(c, err)
				return
			}
			s.clearCookie(c)
			c.Next()
			return
		}
		if row.Status != "active" {
			s.clearCookie(c)
			c.Next()
			return
		}
		grants, err := s.loadGrants(c, row.UserID)
		if err != nil {
			platform.Fail(c, err)
			return
		}
		p := &Principal{
			UserID: row.UserID, OrgID: row.OrgID, Kind: row.Kind, Email: row.Email, Name: row.DisplayName,
			EmailVerified: row.EmailVerifiedAt != nil, SessionID: row.SessionID, Grants: grants, token: token,
		}
		// Perpanjang idle expiry paling sering sekali per menit untuk menekan write.
		if time.Since(row.LastSeenAt) > time.Minute {
			_ = s.Q.TouchSession(ctx, sqlc.TouchSessionParams{ID: row.SessionID, NewIdle: time.Now().Add(s.idleFor(row.Kind))})
		}
		c.Set(principalKey, p)
		c.Next()
	}
}

func (s *Sessions) loadGrants(c *gin.Context, userID uuid.UUID) ([]Grant, error) {
	rows, err := s.Q.ListUserGrants(c.Request.Context(), userID)
	if err != nil {
		return nil, err
	}
	out := make([]Grant, 0, len(rows))
	for _, r := range rows {
		out = append(out, Grant{Perm: r.PermissionKey, Unit: r.UnitID, Job: r.JobID, Role: r.RoleKey})
	}
	return out, nil
}

func Current(c *gin.Context) *Principal {
	v, ok := c.Get(principalKey)
	if !ok {
		return nil
	}
	return v.(*Principal)
}

// CSRF: semua mutasi wajib Origin sama (bila header ada) dan, bila bersesi, header X-CSRF-Token yang cocok.
func (s *Sessions) CSRF() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}
		if o := c.GetHeader("Origin"); o != "" && o != s.Cfg.PublicOrigin {
			platform.Fail(c, &platform.APIError{Status: http.StatusForbidden, Code: "csrf_failed", Message: "Permintaan ditolak karena asal tidak dikenal."})
			return
		}
		if c.GetHeader("Sec-Fetch-Site") == "cross-site" {
			platform.Fail(c, &platform.APIError{Status: http.StatusForbidden, Code: "csrf_failed", Message: "Permintaan ditolak karena asal tidak dikenal."})
			return
		}
		if p := Current(c); p != nil {
			want := s.CSRFToken(p.token)
			if !hmac.Equal([]byte(c.GetHeader(CSRFHeader)), []byte(want)) {
				platform.Fail(c, &platform.APIError{Status: http.StatusForbidden, Code: "csrf_failed", Message: "Sesi formulir kedaluwarsa. Muat ulang halaman."})
				return
			}
		}
		c.Next()
	}
}

func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if Current(c) == nil {
			platform.Fail(c, platform.ErrUnauthenticated())
			return
		}
		c.Next()
	}
}

func RequireKind(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := Current(c)
		if p == nil {
			platform.Fail(c, platform.ErrUnauthenticated())
			return
		}
		if p.Kind != kind {
			platform.Fail(c, platform.ErrForbidden())
			return
		}
		c.Next()
	}
}

// RequireAnyPerm: penyaring kasar di router. Pemeriksaan scope objek tetap di service/query.
func RequireAnyPerm(perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := Current(c)
		if p == nil {
			platform.Fail(c, platform.ErrUnauthenticated())
			return
		}
		if !p.CanAny(perm) {
			platform.Fail(c, platform.ErrForbidden())
			return
		}
		c.Next()
	}
}
