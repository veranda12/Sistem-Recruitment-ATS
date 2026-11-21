package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ats/db/sqlc"
	"ats/internal/app"
	"ats/internal/notify"
	"ats/internal/platform"
)

func main() {
	cfg, err := platform.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	slog.SetDefault(platform.NewLogger(cfg.LogLevel))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Pool worker terpisah dan lebih kecil agar pekerjaan berat tidak menghabiskan koneksi API.
	pool, err := platform.NewPool(ctx, cfg.DatabaseURL, int32(cfg.WorkerConcurrency+2), "ats-worker")
	if err != nil {
		slog.Error("database", "err", err.Error())
		os.Exit(1)
	}
	defer pool.Close()
	q := sqlc.New(pool)

	var mailer notify.Mailer = notify.FileMailer{Dir: cfg.MailDir, From: cfg.MailFrom}
	if cfg.SMTPAddr != "" {
		mailer = notify.SMTPMailer{Addr: cfg.SMTPAddr, From: cfg.MailFrom}
	}
	h := &notify.Handlers{Pool: pool, Q: q, Mailer: mailer, Origin: cfg.PublicOrigin}

	// Health worker terpisah dari readiness API: melaporkan lag outbox dan dead-letter.
	addr := os.Getenv("WORKER_HEALTH_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8081"
	}
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
			st, err := q.OutboxStats(r.Context())
			if err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				fmt.Fprintf(w, `{"status":"unavailable"}`)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"status":"ok","outbox_pending":%d,"outbox_dead":%d,"oldest_pending_seconds":%.1f}`, st.Pending, st.Dead, st.OldestPendingSeconds)
		})
		srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
		_ = srv.ListenAndServe()
	}()

	go app.RunExpiry(ctx, pool, 5*time.Second)
	go app.RunHousekeeping(ctx, pool, cfg.ObjectDir, 15*time.Second)
	go platform.ServeMetrics(ctx, os.Getenv("WORKER_METRICS_ADDR"), "worker", pool)
	w := &platform.OutboxWorker{Pool: pool, Consumer: "worker", Handle: app.WorkerHandler(pool, h, cfg.ObjectDir), Interval: cfg.WorkerPollInterval,
		Concurrency: cfg.WorkerConcurrency, Lease: 60 * time.Second}
	slog.Info("worker mulai", "config", cfg.Redacted(), "mailer", fmt.Sprintf("%T", mailer))
	w.Run(ctx)
	slog.Info("worker selesai")
}
