package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ats/internal/app"
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

	pool, err := platform.NewPool(ctx, cfg.DatabaseURL, cfg.DBMaxConns, "ats-api")
	if err != nil {
		slog.Error("database", "err", err.Error())
		os.Exit(1)
	}
	defer pool.Close()
	orgID, err := app.ResolveOrg(ctx, pool, cfg.OrgSlug)
	if err != nil {
		slog.Error("organisasi tidak ditemukan; jalankan migrasi dan seed", "org", cfg.OrgSlug, "err", err.Error())
		os.Exit(1)
	}
	limiter := platform.NewLimiter(cfg.RedisURL)
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           app.NewRouter(app.Deps{Cfg: cfg, Pool: pool, OrgID: orgID, Limiter: limiter}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
	go func() {
		slog.Info("api mulai", "config", cfg.Redacted(), "redis", limiter.RedisStatus())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("listen", "err", err.Error())
			stop()
		}
	}()
	<-ctx.Done()
	// Graceful drain: berhenti menerima koneksi baru, beri waktu request berjalan selesai.
	slog.Info("api berhenti, menguras request berjalan")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown", "err", err.Error())
	}
}
