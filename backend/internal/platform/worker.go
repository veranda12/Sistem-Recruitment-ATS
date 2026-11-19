package platform

import (
	"context"
	"log/slog"
	"math"
	"math/rand/v2"
	"sync"
	"time"

	"ats/db/sqlc"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OutboxWorker mengonsumsi outbox: klaim dengan lease, proses, lalu tandai selesai/retry/dead.
// Delivery minimal satu kali; handler wajib idempoten dan processed_events mencegah efek ganda per consumer.
type OutboxWorker struct {
	Pool        *pgxpool.Pool
	Consumer    string
	Handle      func(context.Context, sqlc.Outbox) error
	Interval    time.Duration
	Concurrency int
	Lease       time.Duration
}

func (w *OutboxWorker) Run(ctx context.Context) {
	q := sqlc.New(w.Pool)
	sem := make(chan struct{}, w.Concurrency)
	var wg sync.WaitGroup
	t := time.NewTicker(w.Interval)
	defer t.Stop()
	for {
		n := w.tick(ctx, q, sem, &wg)
		if ctx.Err() != nil {
			break
		}
		if n > 0 {
			continue // masih ada antrean, langsung klaim lagi
		}
		select {
		case <-ctx.Done():
		case <-t.C:
		}
		if ctx.Err() != nil {
			break
		}
	}
	slog.Info("worker berhenti mengklaim, menunggu pekerjaan berjalan selesai")
	wg.Wait()
}

// Drain memproses semua event yang siap secara sinkron (untuk test dan perintah operator).
func (w *OutboxWorker) Drain(ctx context.Context) int {
	q := sqlc.New(w.Pool)
	total := 0
	for {
		rows, err := q.ClaimOutbox(ctx, sqlc.ClaimOutboxParams{LeaseSeconds: int32(w.Lease.Seconds()), BatchSize: 50})
		if err != nil || len(rows) == 0 {
			return total
		}
		for _, ev := range rows {
			w.process(ctx, q, ev)
		}
		total += len(rows)
	}
}

func (w *OutboxWorker) tick(ctx context.Context, q *sqlc.Queries, sem chan struct{}, wg *sync.WaitGroup) int {
	rows, err := q.ClaimOutbox(ctx, sqlc.ClaimOutboxParams{LeaseSeconds: int32(w.Lease.Seconds()), BatchSize: int32(w.Concurrency * 2)})
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("klaim outbox gagal", "err", err.Error())
		}
		return 0
	}
	for _, ev := range rows {
		sem <- struct{}{}
		wg.Add(1)
		go func(ev sqlc.Outbox) {
			defer func() { <-sem; wg.Done() }()
			// Pekerjaan yang sudah diklaim diselesaikan walau proses diminta berhenti (graceful drain dibatasi lease).
			jobCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), w.Lease-time.Second)
			defer cancel()
			w.process(jobCtx, q, ev)
		}(ev)
	}
	return len(rows)
}

func (w *OutboxWorker) process(ctx context.Context, q *sqlc.Queries, ev sqlc.Outbox) {
	log := slog.With("event_id", ev.ID.String(), "topic", ev.Topic, "attempt", ev.Attempts)
	herr := w.Handle(ctx, ev)
	if herr == nil {
		err := WithTx(ctx, w.Pool, func(tx pgx.Tx) error {
			qt := q.WithTx(tx)
			if _, err := qt.MarkEventProcessed(ctx, sqlc.MarkEventProcessedParams{Consumer: w.Consumer, EventID: ev.ID}); err != nil {
				return err
			}
			return qt.CompleteOutbox(ctx, ev.ID)
		})
		if err != nil {
			log.Error("gagal menandai event selesai", "err", err.Error())
		}
		return
	}
	msg := herr.Error()
	if len(msg) > 1000 {
		msg = msg[:1000]
	}
	if ev.Attempts >= ev.MaxAttempts {
		log.Error("event dipindah ke dead-letter", "err", msg)
		_ = q.DeadOutbox(ctx, sqlc.DeadOutboxParams{ID: ev.ID, LastError: &msg})
		return
	}
	next := time.Now().Add(Backoff(int(ev.Attempts)))
	log.Warn("event gagal, dijadwalkan ulang", "err", msg, "next", next)
	_ = q.RetryOutbox(ctx, sqlc.RetryOutboxParams{ID: ev.ID, AvailableAt: next, LastError: &msg})
}

// Backoff eksponensial 2^n detik dibatasi 10 menit, dengan jitter +-30%.
func Backoff(attempt int) time.Duration {
	base := math.Min(math.Pow(2, float64(attempt)), 600)
	j := base * (0.7 + 0.6*rand.Float64())
	return time.Duration(j * float64(time.Second))
}
