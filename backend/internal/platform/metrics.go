package platform

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Label berkardinalitas rendah: route template, method, kelas status. Tidak ada user ID, attempt ID, atau URL mentah.
var (
	httpRequests = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "ats_http_requests_total", Help: "Request HTTP per route dan kelas status."},
		[]string{"route", "method", "code"})
	httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "ats_http_request_duration_seconds", Help: "Latensi request per route.",
		Buckets: []float64{.01, .025, .05, .1, .2, .4, .8, 1, 1.5, 2, 5}}, []string{"route", "method"})
	ExamOps = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "ats_exam_operations_total", Help: "Operasi ujian per jenis dan hasil (commit/conflict/rejected)."},
		[]string{"op", "outcome"})
	OutboxGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "ats_outbox_events", Help: "Event outbox per status."}, []string{"status"})
	OutboxLag   = prometheus.NewGauge(prometheus.GaugeOpts{Name: "ats_outbox_oldest_pending_seconds", Help: "Umur event pending tertua (lag dispatch)."})
)

func init() {
	prometheus.MustRegister(httpRequests, httpDuration, ExamOps, OutboxGauge, OutboxLag)
}

func MetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		code := strconv.Itoa(c.Writer.Status()/100) + "xx"
		httpRequests.WithLabelValues(route, c.Request.Method, code).Inc()
		httpDuration.WithLabelValues(route, c.Request.Method).Observe(time.Since(start).Seconds())
	}
}

// poolCollector mengekspos statistik pgxpool: koneksi dipakai, idle, total, waktu tunggu akuisisi kumulatif.
type poolCollector struct {
	pool *pgxpool.Pool
	name string
	d    map[string]*prometheus.Desc
}

func newPoolCollector(name string, p *pgxpool.Pool) *poolCollector {
	mk := func(n, h string) *prometheus.Desc {
		return prometheus.NewDesc("ats_db_pool_"+n, h, nil, prometheus.Labels{"process": name})
	}
	return &poolCollector{pool: p, name: name, d: map[string]*prometheus.Desc{
		"acquired": mk("acquired_conns", "Koneksi sedang dipakai."), "idle": mk("idle_conns", "Koneksi idle."),
		"total": mk("total_conns", "Total koneksi."), "max": mk("max_conns", "Batas koneksi pool."),
		"wait": mk("acquire_wait_seconds_total", "Total waktu menunggu koneksi."), "waits": mk("empty_acquire_total", "Akuisisi yang harus menunggu."),
	}}
}

func (pc *poolCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range pc.d {
		ch <- d
	}
}

func (pc *poolCollector) Collect(ch chan<- prometheus.Metric) {
	s := pc.pool.Stat()
	ch <- prometheus.MustNewConstMetric(pc.d["acquired"], prometheus.GaugeValue, float64(s.AcquiredConns()))
	ch <- prometheus.MustNewConstMetric(pc.d["idle"], prometheus.GaugeValue, float64(s.IdleConns()))
	ch <- prometheus.MustNewConstMetric(pc.d["total"], prometheus.GaugeValue, float64(s.TotalConns()))
	ch <- prometheus.MustNewConstMetric(pc.d["max"], prometheus.GaugeValue, float64(s.MaxConns()))
	ch <- prometheus.MustNewConstMetric(pc.d["wait"], prometheus.CounterValue, s.AcquireDuration().Seconds())
	ch <- prometheus.MustNewConstMetric(pc.d["waits"], prometheus.CounterValue, float64(s.EmptyAcquireCount()))
}

// ServeMetrics menjalankan /metrics di alamat internal (mis. 127.0.0.1:9090), terpisah dari endpoint publik.
func ServeMetrics(ctx context.Context, addr, process string, pool *pgxpool.Pool) {
	if addr == "" {
		return
	}
	reg := prometheus.NewRegistry()
	// Collector Go dan proses sudah ada di registry default; di sini hanya statistik pool proses ini.
	reg.MustRegister(newPoolCollector(process, pool))
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(prometheus.Gatherers{prometheus.DefaultGatherer, reg}, promhttp.HandlerOpts{}))
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("metrics server", "err", err.Error())
	}
}
