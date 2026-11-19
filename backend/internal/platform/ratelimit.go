package platform

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// Limiter: jendela tetap per key. Redis dipakai bila tersedia (lintas instance); bila tidak, limiter lokal
// dengan kuota dibagi DegradedDivisor agar konservatif saat beberapa replika berjalan tanpa koordinasi.
type Limiter struct {
	rdb             *redis.Client
	redisUp         atomic.Bool
	mu              sync.Mutex
	local           map[string]*window
	DegradedDivisor int
}

type window struct {
	start time.Time
	count int
}

func NewLimiter(redisURL string) *Limiter {
	l := &Limiter{local: map[string]*window{}, DegradedDivisor: 2}
	if redisURL != "" {
		opt, err := redis.ParseURL(redisURL)
		if err != nil {
			slog.Error("REDIS_URL tidak valid, limiter lokal dipakai", "err", err.Error())
		} else {
			opt.DialTimeout, opt.ReadTimeout, opt.WriteTimeout = 300*time.Millisecond, 300*time.Millisecond, 300*time.Millisecond
			l.rdb = redis.NewClient(opt)
		}
	}
	l.probe(context.Background())
	go l.loop()
	return l
}

func (l *Limiter) loop() {
	t := time.NewTicker(5 * time.Second)
	for range t.C {
		l.probe(context.Background())
		l.gc()
	}
}

func (l *Limiter) probe(ctx context.Context) {
	if l.rdb == nil {
		l.redisUp.Store(false)
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	up := l.rdb.Ping(ctx).Err() == nil
	if up != l.redisUp.Load() {
		slog.Warn("status redis berubah", "up", up)
	}
	l.redisUp.Store(up)
}

func (l *Limiter) gc() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for k, w := range l.local {
		if time.Since(w.start) > 10*time.Minute {
			delete(l.local, k)
		}
	}
}

// RedisStatus: "ok", "degraded" (dikonfigurasi tapi mati), atau "disabled".
func (l *Limiter) RedisStatus() string {
	switch {
	case l.rdb == nil:
		return "disabled"
	case l.redisUp.Load():
		return "ok"
	default:
		return "degraded"
	}
}

func (l *Limiter) Global() bool { return l.rdb != nil && l.redisUp.Load() }

// Allow mengembalikan (diizinkan, detik sampai jendela berikutnya).
func (l *Limiter) Allow(ctx context.Context, key string, limit int, per time.Duration) (bool, int) {
	if l.Global() {
		bucket := time.Now().Unix() / int64(per.Seconds())
		k := "rl:" + key + ":" + strconv.FormatInt(bucket, 10)
		ctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		defer cancel()
		pipe := l.rdb.TxPipeline()
		incr := pipe.Incr(ctx, k)
		pipe.Expire(ctx, k, per+time.Second)
		if _, err := pipe.Exec(ctx); err == nil {
			retry := int(per.Seconds()) - int(time.Now().Unix()%int64(per.Seconds()))
			return incr.Val() <= int64(limit), retry
		}
		l.redisUp.Store(false)
	}
	local := limit / l.DegradedDivisor
	if local < 1 {
		local = 1
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	w := l.local[key]
	now := time.Now()
	if w == nil || now.Sub(w.start) >= per {
		w = &window{start: now}
		l.local[key] = w
	}
	w.count++
	retry := int((per - now.Sub(w.start)).Seconds()) + 1
	return w.count <= local, retry
}

// RateLimit middleware dengan key dari fungsi (mis. IP + route).
func (l *Limiter) RateLimit(name string, limit int, per time.Duration, keyFn func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ok, retry := l.Allow(c.Request.Context(), name+":"+keyFn(c), limit, per)
		if !ok {
			Fail(c, ErrRateLimited(retry))
			return
		}
		c.Next()
	}
}

// RequireGlobalLimiter menolak endpoint sensitif (mis. ekspor PII) bila koordinasi lintas instance tidak tersedia.
func (l *Limiter) RequireGlobalLimiter(strict bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strict && !l.Global() {
			Fail(c, ErrUnavailable("Fitur ini sementara tidak tersedia karena layanan pendukung bermasalah.", 30))
			return
		}
		c.Next()
	}
}
