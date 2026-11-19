package recruitment

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// PublicCache menyimpan respons publik (daftar lowongan, profil org) dengan TTL pendek.
// Invalidasi lewat generasi: publish/close menaikkan counter sehingga semua key lama tidak terbaca lagi.
// Tanpa Redis (atau saat Redis mati) cache dimatikan; data dibaca langsung dari PostgreSQL.
type PublicCache struct {
	rdb *redis.Client
	up  func() bool
	TTL time.Duration
}

func NewPublicCache(redisURL string, up func() bool) *PublicCache {
	pc := &PublicCache{up: up, TTL: 30 * time.Second}
	if redisURL != "" {
		if opt, err := redis.ParseURL(redisURL); err == nil {
			opt.DialTimeout, opt.ReadTimeout, opt.WriteTimeout = 200*time.Millisecond, 200*time.Millisecond, 200*time.Millisecond
			pc.rdb = redis.NewClient(opt)
		}
	}
	return pc
}

func (p *PublicCache) enabled() bool { return p != nil && p.rdb != nil && p.up != nil && p.up() }

func (p *PublicCache) gen(ctx context.Context) (string, bool) {
	g, err := p.rdb.Get(ctx, "pub:gen").Result()
	if err == redis.Nil {
		return "0", true
	}
	return g, err == nil
}

func (p *PublicCache) Get(ctx context.Context, key string) ([]byte, bool) {
	if !p.enabled() {
		return nil, false
	}
	g, ok := p.gen(ctx)
	if !ok {
		return nil, false
	}
	b, err := p.rdb.Get(ctx, "pub:"+g+":"+key).Bytes()
	return b, err == nil
}

func (p *PublicCache) Set(ctx context.Context, key string, val []byte) {
	if !p.enabled() {
		return
	}
	if g, ok := p.gen(ctx); ok {
		_ = p.rdb.Set(ctx, "pub:"+g+":"+key, val, p.TTL).Err()
	}
}

// Invalidate dipanggil setelah commit publish/close. Kegagalan hanya dicatat: TTL pendek membatasi data basi.
func (p *PublicCache) Invalidate(ctx context.Context) {
	if !p.enabled() {
		return
	}
	if n, err := p.rdb.Incr(ctx, "pub:gen").Result(); err != nil {
		slog.Warn("invalidasi cache publik gagal", "err", err.Error())
	} else {
		slog.Debug("cache publik diinvalidasi", "gen", strconv.FormatInt(n, 10))
	}
}
