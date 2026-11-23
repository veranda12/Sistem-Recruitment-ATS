package privacy

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"time"

	"ats/db/sqlc"
	"ats/internal/platform"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Kandidat memenuhi syarat retensi bila: belum dianonimkan, tidak dalam legal hold, tidak punya lamaran aktif, dan
// aktivitas terakhirnya (akun, profil, sesi, lamaran) lebih lama dari masa simpan.
const eligibleSQL = `
SELECT id, display_name, email, last_activity FROM (
  SELECT u.id, u.display_name, u.email,
         greatest(u.updated_at, coalesce(p.updated_at, u.created_at),
                  coalesce((SELECT max(s.last_seen_at) FROM sessions s WHERE s.user_id = u.id), u.created_at),
                  coalesce((SELECT max(a.updated_at) FROM job_applications a WHERE a.candidate_user_id = u.id), u.created_at)) AS last_activity
  FROM users u LEFT JOIN candidate_profiles p ON p.user_id = u.id
  WHERE u.org_id = $1 AND u.kind = 'candidate' AND u.anonymized_at IS NULL AND u.legal_hold_at IS NULL
    AND NOT EXISTS (SELECT 1 FROM job_applications a WHERE a.candidate_user_id = u.id AND a.status = 'active')
) x
WHERE last_activity < now() - make_interval(months => $2)
ORDER BY last_activity`

type Candidate struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	Email        string    `json:"email"`
	LastActivity time.Time `json:"lastActivity"`
}

// Policy membaca pengaturan retensi organisasi (bawaan 24 bulan, otomatis mati).
func Policy(ctx context.Context, q *sqlc.Queries, orgID uuid.UUID) (months int, enabled bool) {
	months = 24
	if s, err := q.GetSetting(ctx, sqlc.GetSettingParams{OrgID: orgID, Key: "retention.candidate_months"}); err == nil {
		_ = json.Unmarshal(s.Value, &months)
	}
	if s, err := q.GetSetting(ctx, sqlc.GetSettingParams{OrgID: orgID, Key: "retention.auto_enabled"}); err == nil {
		_ = json.Unmarshal(s.Value, &enabled)
	}
	return
}

// Eligible: kandidat yang akan dianonimkan oleh retensi (dry-run). limit <= 0 berarti tanpa batas.
func Eligible(ctx context.Context, pool *pgxpool.Pool, orgID uuid.UUID, months, limit int) ([]Candidate, int, error) {
	var total int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM (`+eligibleSQL+`) e`, orgID, months).Scan(&total); err != nil {
		return nil, 0, err
	}
	sql := eligibleSQL
	if limit > 0 {
		sql += " LIMIT " + strconv.Itoa(limit)
	}
	rows, err := pool.Query(ctx, sql, orgID, months)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Candidate{}
	for rows.Next() {
		var c Candidate
		if err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.LastActivity); err != nil {
			return nil, 0, err
		}
		out = append(out, c)
	}
	return out, total, rows.Err()
}

// RunRetention: job berkala di worker. Tidak melakukan apa pun sampai retention.auto_enabled = true.
func RunRetention(ctx context.Context, pool *pgxpool.Pool, orgID uuid.UUID, objectDir string, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if n, err := RetentionOnce(ctx, pool, orgID, objectDir, 200); err != nil {
			slog.Error("retensi", "err", err.Error())
		} else if n > 0 {
			slog.Info("retensi: kandidat dianonimkan", "jumlah", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// RetentionOnce menganonimkan paling banyak batch kandidat yang memenuhi syarat, satu transaksi per kandidat.
func RetentionOnce(ctx context.Context, pool *pgxpool.Pool, orgID uuid.UUID, objectDir string, batch int) (int, error) {
	q := sqlc.New(pool)
	months, enabled := Policy(ctx, q, orgID)
	if !enabled {
		return 0, nil
	}
	list, _, err := Eligible(ctx, pool, orgID, months, batch)
	if err != nil {
		return 0, err
	}
	done := 0
	for _, c := range list {
		var files []string
		err := platform.WithTx(ctx, pool, func(tx pgx.Tx) error {
			var err error
			if files, err = Anonymize(ctx, tx, c.ID); err != nil {
				return err
			}
			return platform.WriteAudit(ctx, sqlc.New(tx), platform.AuditEntry{OrgID: orgID, ActorKind: "system", Action: "privacy.retention_anonymize",
				TargetType: "user", TargetID: c.ID.String(), Detail: map[string]any{"months": months, "lastActivity": c.LastActivity}})
		})
		if err != nil {
			// Kandidat yang berubah status sejak dipilih (mis. melamar lagi) dilewati; dicoba lagi pada putaran berikutnya.
			slog.Warn("retensi dilewati", "user", c.ID.String(), "err", err.Error())
			continue
		}
		if failed := RemoveFiles(objectDir, files); len(failed) > 0 {
			slog.Warn("retensi: berkas belum terhapus", "user", c.ID.String(), "files", failed)
		}
		done++
	}
	return done, nil
}
