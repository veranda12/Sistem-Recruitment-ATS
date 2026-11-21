// cvworker: worker khusus pembacaan CV, terpisah dari worker outbox/ujian (pool DB, antrean Asynq "cv",
// concurrency, dan batas sumber daya sendiri). PostgreSQL adalah sumber kebenaran job; bila Redis tidak
// tersedia, dispatcher memproses langsung dari PostgreSQL.
//
//	cvworker            menjalankan worker
//	cvworker preflight  menjalankan fixture kecil untuk memeriksa Poppler, Tesseract (ind/eng/osd), LibreOffice
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"ats/db/sqlc"
	"ats/internal/cvparse"
	"ats/internal/cvreview"
	"ats/internal/platform"

	"github.com/hibiken/asynq"
)

var version = "dev"

func main() {
	tools := cvparse.ToolsFromEnv()
	workDir := os.Getenv("CV_WORK_DIR")
	if workDir == "" {
		workDir = filepath.Join(os.TempDir(), "ats-cv")
	}
	_ = os.MkdirAll(workDir, 0o700)
	if len(os.Args) > 1 && os.Args[1] == "preflight" {
		os.Exit(preflight(tools, workDir))
	}
	cfg, err := platform.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	slog.SetDefault(platform.NewLogger(cfg.LogLevel))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Preflight saat start: tool yang wajib harus benar-benar bekerja.
	for _, r := range cvparse.Preflight(ctx, tools, workDir) {
		if !r.OK && r.Required {
			slog.Error("preflight cv gagal", "komponen", r.Name, "detail", r.Detail)
			os.Exit(1)
		}
		if !r.OK {
			slog.Warn("preflight cv", "komponen", r.Name, "detail", r.Detail)
		}
	}
	conc := 2
	if v, err := strconv.Atoi(os.Getenv("CV_WORKER_CONCURRENCY")); err == nil && v > 0 && v <= 16 {
		conc = v
	}
	pool, err := platform.NewPool(ctx, cfg.DatabaseURL, int32(conc+3), "ats-cvworker")
	if err != nil {
		slog.Error("database", "err", err.Error())
		os.Exit(1)
	}
	defer pool.Close()
	q := sqlc.New(pool)
	host, _ := os.Hostname()
	proc := &cvreview.Processor{Pool: pool, Q: q, ObjectDir: cfg.ObjectDir, WorkDir: workDir, Tools: tools, Limits: cvparse.DefaultLimits(),
		Owner: fmt.Sprintf("%s:%d", host, os.Getpid())}
	cvreview.RegisterMetrics()
	go platform.ServeMetrics(ctx, os.Getenv("CV_WORKER_METRICS_ADDR"), "cvworker", pool)

	enq, err := cvreview.NewEnqueuer(cfg.RedisURL)
	if err != nil {
		slog.Warn("REDIS_URL tidak valid; memakai PostgreSQL saja", "err", err.Error())
		enq = &cvreview.Enqueuer{}
	}
	defer enq.Close()
	if cfg.RedisURL != "" && enq.Client != nil {
		opt, _ := asynq.ParseRedisURI(cfg.RedisURL)
		srv := asynq.NewServer(opt, asynq.Config{Concurrency: conc, Queues: map[string]int{cvreview.QueueName: 1},
			ShutdownTimeout: 20 * time.Second, Logger: asynqLogger{}})
		mux := asynq.NewServeMux()
		mux.Handle(cvreview.TaskParse, cvreview.AsynqHandler(proc))
		go func() {
			if err := srv.Run(mux); err != nil {
				slog.Error("asynq berhenti; dispatcher PostgreSQL tetap berjalan", "err", err.Error())
			}
		}()
		defer srv.Shutdown()
	}
	go health(ctx, q)
	d := &cvreview.Dispatcher{Q: q, Enq: enq, Proc: proc, Interval: 3 * time.Second, Local: make(chan struct{}, conc)}
	slog.Info("cv worker mulai", "version", version, "concurrency", conc, "redis", cfg.RedisURL != "", "tools", cvparse.ToolVersions(ctx, tools))
	d.Run(ctx)
	slog.Info("cv worker selesai")
}

func health(ctx context.Context, q *sqlc.Queries) {
	addr := os.Getenv("CV_WORKER_HEALTH_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8082"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		st, err := q.CVQueueStats(r.Context())
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"status":"unavailable"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","queued":%d,"processing":%d,"dead":%d,"oldest_queued_seconds":%.1f}`, st.Queued, st.Processing, st.Dead, st.OldestQueuedSeconds)
	})
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { <-ctx.Done(); _ = srv.Close() }()
	_ = srv.ListenAndServe()
}

func preflight(t cvparse.Tools, work string) int {
	code := 0
	for _, r := range cvparse.Preflight(context.Background(), t, work) {
		mark := "OK  "
		if !r.OK {
			mark = "GAGAL"
			if r.Required {
				code = 1
			} else {
				mark = "OPSI "
			}
		}
		fmt.Printf("[%s] %-16s %s\n", mark, r.Name, r.Detail)
	}
	fmt.Println("versi:", cvparse.ToolVersions(context.Background(), t))
	return code
}

// asynqLogger meneruskan log Asynq ke slog.
type asynqLogger struct{}

func (asynqLogger) Debug(args ...any) {}
func (asynqLogger) Info(args ...any)  { slog.Info(fmt.Sprint(args...), "src", "asynq") }
func (asynqLogger) Warn(args ...any)  { slog.Warn(fmt.Sprint(args...), "src", "asynq") }
func (asynqLogger) Error(args ...any) { slog.Error(fmt.Sprint(args...), "src", "asynq") }
func (asynqLogger) Fatal(args ...any) { slog.Error(fmt.Sprint(args...), "src", "asynq"); os.Exit(1) }
