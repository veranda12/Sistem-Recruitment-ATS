package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"ats/db"
	"ats/db/sqlc"
	"ats/internal/auth"
	"ats/internal/identity"
	"ats/internal/notify"
	"ats/internal/platform"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testOrigin = "http://localhost:5173"
const testPassword = "Uji-Sandi-2026"

// env adalah lingkungan integrasi: PostgreSQL sungguhan (DATABASE_URL_TEST), skema dibuat ulang dari migrasi, seed fiktif.
type env struct {
	t       *testing.T
	pool    *pgxpool.Pool
	srv     *httptest.Server
	cfg     platform.Config
	orgID   uuid.UUID
	mailDir string
	worker  *platform.OutboxWorker
	keys    identity.Keys
}

func key(b byte) []byte { return bytes.Repeat([]byte{b}, 32) }

func newEnv(t *testing.T) *env {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL_TEST")
	if dsn == "" {
		t.Skip("DATABASE_URL_TEST tidak diisi; integration test dilewati")
	}
	ctx := context.Background()
	resetSchema(t, dsn)
	pool, err := platform.NewPool(ctx, dsn, 30, "ats-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	keys := identity.Keys{EncKey: key(1), EncVersion: 1, IndexKey: key(2), IndexVersion: 1}
	hash, err := auth.HashPassword(ctx, testPassword)
	if err != nil {
		t.Fatal(err)
	}
	if err := platform.WithTx(ctx, pool, func(tx pgx.Tx) error { return Seed(ctx, tx, "arunika", hash, keys) }); err != nil {
		t.Fatalf("seed: %v", err)
	}
	orgID, err := ResolveOrg(ctx, pool, "arunika")
	if err != nil {
		t.Fatal(err)
	}
	cfg := platform.Config{Env: "test", PublicOrigin: testOrigin, SessionKey: key(3), NIKEncKey: key(1), NIKEncKeyVer: 1,
		NIKIndexKey: key(2), NIKIndexKeyVer: 1, StaffIdleTimeout: 30 * time.Minute, CandidateIdleTimeout: 2 * time.Hour,
		SessionAbsolute: 12 * time.Hour}
	limiter := platform.NewLimiter("")
	limiter.DegradedDivisor = 1
	srv := httptest.NewServer(NewRouter(Deps{Cfg: cfg, Pool: pool, OrgID: orgID, Limiter: limiter}))
	t.Cleanup(srv.Close)
	mailDir := t.TempDir()
	h := &notify.Handlers{Pool: pool, Q: sqlc.New(pool), Mailer: notify.FileMailer{Dir: mailDir, From: "uji@arunika.example"}, Origin: testOrigin}
	w := &platform.OutboxWorker{Pool: pool, Consumer: "notify", Handle: h.Handle, Interval: time.Second, Concurrency: 2, Lease: 30 * time.Second}
	return &env{t: t, pool: pool, srv: srv, cfg: cfg, orgID: orgID, mailDir: mailDir, worker: w, keys: keys}
}

func resetSchema(t *testing.T, dsn string) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("koneksi DB test: %v", err)
	}
	if _, err := conn.Exec(ctx, `DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatal(err)
	}
	conn.Close(ctx)
	src, err := iofs.New(db.Migrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	url := "pgx5://" + strings.TrimPrefix(strings.TrimPrefix(dsn, "postgres://"), "postgresql://")
	m, err := migrate.NewWithSourceInstance("iofs", src, url)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("migrasi: %v", err)
	}
	m.Close()
}

// client adalah browser tiruan: cookie jar + token CSRF dari respons sesi.
type client struct {
	e    *env
	http *http.Client
	csrf string
}

func (e *env) client() *client {
	jar, _ := cookiejar.New(nil)
	return &client{e: e, http: &http.Client{Jar: jar}}
}

type resp struct {
	Status int
	Body   map[string]any
	Raw    []byte
	Header http.Header
}

func (r resp) errCode() string {
	if e, ok := r.Body["error"].(map[string]any); ok {
		s, _ := e["code"].(string)
		return s
	}
	return ""
}

func (c *client) do(method, path string, body any, hdr ...string) resp {
	c.e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.e.srv.URL+"/api/v1"+path, rd)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", testOrigin)
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := resp{Status: res.StatusCode, Raw: raw, Header: res.Header}
	_ = json.Unmarshal(raw, &out.Body)
	return out
}

func (c *client) login(email string) resp {
	c.e.t.Helper()
	r := c.do("POST", "/auth/login", map[string]string{"email": email, "password": testPassword})
	if r.Status == 200 {
		c.csrf = r.Body["csrfToken"].(string)
	}
	return r
}

func (c *client) mustLogin(email string) *client {
	c.e.t.Helper()
	if r := c.login(email); r.Status != 200 {
		c.e.t.Fatalf("login %s: %d %s", email, r.Status, r.Raw)
	}
	return c
}

func expect(t *testing.T, r resp, status int, what string) {
	t.Helper()
	if r.Status != status {
		t.Fatalf("%s: status %d, ingin %d; body %s", what, r.Status, status, r.Raw)
	}
}

var tokenRe = regexp.MustCompile(`token=([A-Za-z0-9_-]+)`)

// latestToken membaca email terbaru untuk alamat tertentu dari sink file.
func (e *env) latestToken(email string) string {
	e.t.Helper()
	e.worker.Drain(context.Background())
	files, _ := filepath.Glob(filepath.Join(e.mailDir, "*.eml"))
	sort.Strings(files)
	for i := len(files) - 1; i >= 0; i-- {
		b, _ := os.ReadFile(files[i])
		if strings.Contains(string(b), "To: "+email) {
			if m := tokenRe.FindSubmatch(b); m != nil {
				return string(m[1])
			}
		}
	}
	e.t.Fatalf("token email untuk %s tidak ditemukan", email)
	return ""
}

func (e *env) scalar(sql string, args ...any) any {
	e.t.Helper()
	var v any
	if err := e.pool.QueryRow(context.Background(), sql, args...).Scan(&v); err != nil {
		e.t.Fatalf("query %q: %v", sql, err)
	}
	return v
}

func (e *env) count(sql string, args ...any) int64 {
	e.t.Helper()
	switch v := e.scalar(sql, args...).(type) {
	case int64:
		return v
	case int32:
		return int64(v)
	default:
		e.t.Fatalf("count: tipe %T", v)
		return 0
	}
}

// registerVerified mendaftarkan kandidat baru, memverifikasi email, melengkapi profil, dan login.
func (e *env) registerVerified(email, name, nik string) *client {
	e.t.Helper()
	c := e.client()
	r := c.do("POST", "/auth/register", map[string]any{"email": email, "password": testPassword, "fullName": name, "nik": nik, "acceptPrivacy": true})
	expect(e.t, r, 202, "register")
	expect(e.t, c.do("POST", "/auth/verify-email", map[string]string{"token": e.latestToken(email)}), 200, "verify")
	c.mustLogin(email)
	p := c.do("GET", "/me/profile", nil)
	r = c.do("PUT", "/me/profile", completeProfile(name, "Karawang", int(p.Body["version"].(float64))))
	expect(e.t, r, 200, "lengkapi profil")
	return c
}

func completeProfile(name, city string, version int) map[string]any {
	return map[string]any{"fullName": name, "phone": "081234567890", "birthDate": "1999-04-12", "city": city,
		"address": "", "summary": "", "version": version, "experiences": []any{}, "certifications": []any{},
		"education": []any{map[string]any{"level": "SMK", "institution": "SMK Negeri 1 Karawang", "major": "Mekatronika", "startYear": 2014, "endYear": 2017}}}
}

func idOf(r resp, k string) string { return fmt.Sprint(r.Body[k]) }

