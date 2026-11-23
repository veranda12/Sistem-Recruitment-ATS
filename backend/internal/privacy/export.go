package privacy

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// exportSections: data milik kandidat dalam berkas ekspor permintaan akses. Rahasia (hash sandi, NIK terenkripsi,
// token) tidak disertakan; NIK hanya 4 digit terakhir.
var exportSections = []struct{ name, sql string }{
	{"akun", `SELECT email, display_name, created_at, email_verified_at, status, anonymized_at FROM users WHERE id = $1`},
	{"profil", `SELECT full_name, phone, birth_date, city, address, summary, contact_email, whatsapp_opt_in_at, updated_at FROM candidate_profiles WHERE user_id = $1`},
	{"identitas", `SELECT '************' || nik_last4 AS nik, created_at FROM candidate_identities WHERE user_id = $1`},
	{"pendidikan", `SELECT * FROM education WHERE user_id = $1`},
	{"pengalaman", `SELECT * FROM experiences WHERE user_id = $1`},
	{"sertifikasi", `SELECT * FROM certifications WHERE user_id = $1`},
	{"keahlian", `SELECT name FROM candidate_skills WHERE user_id = $1`},
	{"bahasa", `SELECT * FROM candidate_languages WHERE user_id = $1`},
	{"tautan", `SELECT * FROM candidate_links WHERE user_id = $1`},
	{"proyek", `SELECT * FROM candidate_projects WHERE user_id = $1`},
	{"dokumen", `SELECT kind, file_name, content_type, size_bytes, scan_status, created_at, deleted_at FROM documents WHERE owner_user_id = $1`},
	{"lamaran", `SELECT a.id, j.title AS lowongan, a.status, a.stage_key AS tahap, a.applied_at, a.updated_at
	             FROM job_applications a JOIN job_openings j ON j.id = a.job_id WHERE a.candidate_user_id = $1`},
	{"riwayat_tahap", `SELECT t.application_id, t.from_stage, t.to_stage, t.to_status, t.trigger, t.created_at
	                   FROM transition_history t JOIN job_applications a ON a.id = t.application_id WHERE a.candidate_user_id = $1`},
	{"ujian", `SELECT a.id, a.status, a.started_at, a.finalized_at, r.score, r.status AS status_nilai
	           FROM exam_attempts a LEFT JOIN exam_results r ON r.attempt_id = a.id WHERE a.candidate_user_id = $1`},
	{"interview", `SELECT i.starts_at, i.ends_at, i.timezone, i.mode, i.status FROM interviews i
	               JOIN job_applications a ON a.id = i.application_id WHERE a.candidate_user_id = $1`},
	{"pemberitahuan", `SELECT subject, body, created_at FROM notifications WHERE user_id = $1`},
}

// Export mengumpulkan data kandidat sebagai peta bagian -> daftar baris (JSON).
func Export(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID) (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	for _, s := range exportSections {
		var b []byte
		if err := pool.QueryRow(ctx, `SELECT coalesce(jsonb_agg(to_jsonb(x)), '[]'::jsonb) FROM (`+s.sql+`) x`, userID).Scan(&b); err != nil {
			return nil, err
		}
		out[s.name] = b
	}
	return out, nil
}
