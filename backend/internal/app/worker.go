package app

import (
	"context"
	"log/slog"
	"time"

	"ats/db/sqlc"
	"ats/internal/exam"
	"ats/internal/notify"
	"ats/internal/platform"
	"ats/internal/reporting"
	"ats/internal/workflow"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WorkerHandler merangkai consumer outbox: rilis nilai memicu event workflow (execution ID = event ID,
// sehingga replay tidak menggandakan transisi), lalu notifikasi kandidat. Topik lain ke notify.
func WorkerHandler(pool *pgxpool.Pool, n *notify.Handlers, objectDir string) func(context.Context, sqlc.Outbox) error {
	return func(ctx context.Context, ev sqlc.Outbox) error {
		if ev.Topic == reporting.TopicExport {
			return reporting.Generate(ctx, pool, objectDir, ev.Payload)
		}
		if ev.Topic == exam.TopicReleased {
			appID, key, facts, err := exam.ReleasedFacts(ev.Payload)
			if err != nil {
				return err
			}
			err = platform.WithTx(ctx, pool, func(tx pgx.Tx) error {
				res, err := workflow.Execute(ctx, sqlc.New(tx), workflow.Command{ExecutionID: ev.ID, ApplicationID: appID, Kind: "event",
					Event: "assessment.released", AssessmentKey: key, Facts: facts, Actor: workflow.Actor{Kind: "system"}})
				if err == nil && res.Note != "" {
					slog.Info("event hasil ujian tanpa transisi", "event_id", ev.ID.String(), "note", res.Note)
				}
				return err
			})
			if err != nil {
				return err
			}
		}
		return n.Handle(ctx, ev)
	}
}

// RunExpiry memfinalisasi attempt yang lewat deadline. Tiap attempt di transaksinya sendiri dengan lock yang sama dengan submit.
func RunExpiry(ctx context.Context, pool *pgxpool.Pool, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		ExpireDue(ctx, pool)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func ExpireDue(ctx context.Context, pool *pgxpool.Pool) int {
	ids, err := sqlc.New(pool).ExpiredAttemptIDs(ctx, 100)
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("daftar attempt kedaluwarsa gagal", "err", err.Error())
		}
		return 0
	}
	n := 0
	for _, id := range ids {
		err := platform.WithTx(ctx, pool, func(tx pgx.Tx) error {
			done, err := exam.ExpireOne(ctx, sqlc.New(tx), id)
			if done {
				n++
			}
			return err
		})
		if err != nil {
			slog.Error("finalisasi attempt kedaluwarsa gagal", "attempt_id", id.String(), "err", err.Error())
		}
	}
	return n
}

// RunHousekeeping: pembersihan ekspor kedaluwarsa dan gauge antrean outbox untuk dashboard.
func RunHousekeeping(ctx context.Context, pool *pgxpool.Pool, objectDir string, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	q := sqlc.New(pool)
	for {
		reporting.CleanupExpired(ctx, pool, objectDir)
		if st, err := q.OutboxStats(ctx); err == nil {
			platform.OutboxGauge.WithLabelValues("pending").Set(float64(st.Pending))
			platform.OutboxGauge.WithLabelValues("dead").Set(float64(st.Dead))
			platform.OutboxLag.Set(st.OldestPendingSeconds)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
