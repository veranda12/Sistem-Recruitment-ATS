package platform

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config adalah konfigurasi proses yang divalidasi saat start. Nilai rahasia tidak pernah dicetak.
type Config struct {
	Env            string // development | test | staging | production
	HTTPAddr       string
	PublicOrigin   string
	DatabaseURL    string
	DBMaxConns     int32
	RedisURL       string
	OrgSlug        string
	LogLevel       string
	TrustedProxies []string

	SessionKey     []byte // HMAC untuk token CSRF
	NIKEncKey      []byte // AES-256-GCM
	NIKEncKeyVer   int
	NIKIndexKey    []byte // HMAC-SHA256 blind index
	NIKIndexKeyVer int

	StaffIdleTimeout     time.Duration
	CandidateIdleTimeout time.Duration
	SessionAbsolute      time.Duration

	MailDir               string // sink email dev (file .eml)
	SMTPAddr              string
	MailFrom              string
	ObjectDir             string
	StrictSensitiveLimits bool

	WorkerPollInterval time.Duration
	WorkerConcurrency  int

	// RateLimitScale mengalikan semua kuota rate limit. Hanya untuk uji otomatis dari satu IP; wajib 1 di produksi.
	RateLimitScale int

	MetricsAddr string // /metrics internal; kosong = nonaktif

	// ScanCommand: pemindai malware eksternal (mis. "clamdscan --no-summary"). Kosong = dokumen tetap pending.
	ScanCommand string
}

func (c Config) IsProduction() bool { return c.Env == "production" }

// LoadConfig membaca environment dan mengembalikan semua kesalahan sekaligus agar operator bisa memperbaiki sekali jalan.
func LoadConfig() (Config, error) {
	var errs []string
	get := func(k, def string) string {
		if v, ok := os.LookupEnv(k); ok && v != "" {
			return v
		}
		return def
	}
	key := func(k string, required bool) []byte {
		v := os.Getenv(k)
		if v == "" {
			if required {
				errs = append(errs, k+" wajib diisi (32 byte base64)")
			}
			return nil
		}
		b, err := base64.StdEncoding.DecodeString(v)
		if err != nil || len(b) != 32 {
			errs = append(errs, k+" harus 32 byte dalam base64")
			return nil
		}
		return b
	}
	atoi := func(k string, def int) int {
		v := get(k, strconv.Itoa(def))
		n, err := strconv.Atoi(v)
		if err != nil {
			errs = append(errs, k+" harus bilangan bulat")
			return def
		}
		return n
	}
	dur := func(k string, def time.Duration) time.Duration {
		v := get(k, def.String())
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			errs = append(errs, k+" harus durasi Go, contoh 30m")
			return def
		}
		return d
	}

	c := Config{
		Env:          get("APP_ENV", "development"),
		HTTPAddr:     get("HTTP_ADDR", "127.0.0.1:8080"),
		PublicOrigin: strings.TrimRight(get("PUBLIC_ORIGIN", "http://localhost:5173"), "/"),
		DatabaseURL:  os.Getenv("DATABASE_URL"),
		DBMaxConns:   int32(atoi("DB_MAX_CONNS", 20)),
		RedisURL:     os.Getenv("REDIS_URL"),
		OrgSlug:      get("ORG_SLUG", "arunika"),
		LogLevel:     get("LOG_LEVEL", "info"),

		SessionKey:     key("SESSION_KEY", true),
		NIKEncKey:      key("NIK_ENC_KEY", true),
		NIKEncKeyVer:   atoi("NIK_ENC_KEY_VERSION", 1),
		NIKIndexKey:    key("NIK_INDEX_KEY", true),
		NIKIndexKeyVer: atoi("NIK_INDEX_KEY_VERSION", 1),

		StaffIdleTimeout:     dur("SESSION_IDLE_STAFF", 30*time.Minute),
		CandidateIdleTimeout: dur("SESSION_IDLE_CANDIDATE", 2*time.Hour),
		SessionAbsolute:      dur("SESSION_ABSOLUTE", 12*time.Hour),

		MailDir:               get("MAIL_DIR", ".data/mail"),
		SMTPAddr:              os.Getenv("SMTP_ADDR"),
		MailFrom:              get("MAIL_FROM", "Rekrutmen <no-reply@arunika.example>"),
		ObjectDir:             get("OBJECT_DIR", ".data/objects"),
		StrictSensitiveLimits: get("STRICT_SENSITIVE_LIMITS", "true") == "true",

		WorkerPollInterval: dur("WORKER_POLL_INTERVAL", time.Second),
		WorkerConcurrency:  atoi("WORKER_CONCURRENCY", 4),
		RateLimitScale:     atoi("RATE_LIMIT_SCALE", 1),
		MetricsAddr:        os.Getenv("METRICS_ADDR"),
		ScanCommand:        os.Getenv("SCAN_COMMAND"),
	}
	if tp := os.Getenv("TRUSTED_PROXIES"); tp != "" {
		for _, p := range strings.Split(tp, ",") {
			c.TrustedProxies = append(c.TrustedProxies, strings.TrimSpace(p))
		}
	}

	switch c.Env {
	case "development", "test", "staging", "production":
	default:
		errs = append(errs, "APP_ENV harus development|test|staging|production")
	}
	if c.DatabaseURL == "" {
		errs = append(errs, "DATABASE_URL wajib diisi")
	}
	if u, err := url.Parse(c.PublicOrigin); err != nil || u.Scheme == "" || u.Host == "" {
		errs = append(errs, "PUBLIC_ORIGIN harus URL absolut, contoh https://karier.contoh.id")
	} else if c.IsProduction() && u.Scheme != "https" {
		errs = append(errs, "PUBLIC_ORIGIN wajib https di produksi")
	}
	if c.RateLimitScale < 1 || c.RateLimitScale > 100 || (c.IsProduction() && c.RateLimitScale != 1) {
		errs = append(errs, "RATE_LIMIT_SCALE harus 1..100 dan tepat 1 di produksi")
	}
	if c.DBMaxConns < 2 || c.DBMaxConns > 200 {
		errs = append(errs, "DB_MAX_CONNS di luar rentang 2..200")
	}
	if c.SessionKey != nil && c.NIKIndexKey != nil && string(c.SessionKey) == string(c.NIKIndexKey) {
		errs = append(errs, "SESSION_KEY dan NIK_INDEX_KEY tidak boleh sama")
	}
	if c.NIKEncKey != nil && c.NIKIndexKey != nil && string(c.NIKEncKey) == string(c.NIKIndexKey) {
		errs = append(errs, "NIK_ENC_KEY dan NIK_INDEX_KEY harus kunci terpisah")
	}
	if len(errs) > 0 {
		return c, errors.New("konfigurasi tidak valid:\n  - " + strings.Join(errs, "\n  - "))
	}
	return c, nil
}

// Redacted mengembalikan ringkasan konfigurasi aman untuk log startup.
func (c Config) Redacted() map[string]any {
	db := "set"
	if u, err := url.Parse(c.DatabaseURL); err == nil {
		db = fmt.Sprintf("%s/%s", u.Host, strings.TrimPrefix(u.Path, "/"))
	}
	return map[string]any{
		"env": c.Env, "http_addr": c.HTTPAddr, "public_origin": c.PublicOrigin, "db": db,
		"db_max_conns": c.DBMaxConns, "redis": c.RedisURL != "", "org": c.OrgSlug,
	}
}
