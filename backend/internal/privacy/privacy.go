// Package privacy: hak subjek data (ekspor, anonimisasi), legal hold, dan retensi otomatis kandidat.
//
// Anonimisasi menghapus atau mengosongkan data yang mengidentifikasi kandidat, sambil mempertahankan baris bukti
// proses tanpa identitas (lamaran, riwayat tahap, nilai, audit). Snapshot lamaran dan pengiriman ujian yang
// append-only hanya boleh diubah dalam transaksi yang menyetel ats.privacy_erasure (lihat migrasi 0014).
package privacy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"ats/internal/platform"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const AnonName = "Kandidat dianonimkan"

var (
	ErrNotCandidate = errors.New("bukan akun kandidat")
	ErrAnonymized   = errors.New("sudah dianonimkan")
	ErrLegalHold    = errors.New("legal hold")
	ErrActive       = errors.New("masih ada lamaran aktif")
)

// Anonymize menjalankan anonimisasi dalam transaksi tx. Mengembalikan path berkas (relatif ObjectDir) yang harus
// dihapus setelah transaksi berhasil.
func Anonymize(ctx context.Context, tx pgx.Tx, userID uuid.UUID) ([]string, error) {
	if _, err := tx.Exec(ctx, "SET LOCAL ats.privacy_erasure = 'on'"); err != nil {
		return nil, err
	}
	var kind string
	var anonymized, hold *string
	err := tx.QueryRow(ctx, `SELECT kind, anonymized_at::text, legal_hold_at::text FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&kind, &anonymized, &hold)
	if err != nil {
		return nil, err
	}
	switch {
	case kind != "candidate":
		return nil, ErrNotCandidate
	case anonymized != nil:
		return nil, ErrAnonymized
	case hold != nil:
		return nil, ErrLegalHold
	}
	var active int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM job_applications WHERE candidate_user_id = $1 AND status = 'active'`, userID).Scan(&active); err != nil {
		return nil, err
	}
	if active > 0 {
		return nil, ErrActive
	}

	var files []string
	rows, err := tx.Query(ctx, `SELECT object_key FROM documents WHERE owner_user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			rows.Close()
			return nil, err
		}
		files = append(files, filepath.FromSlash(k))
	}
	rows.Close()
	rows, err = tx.Query(ctx, `SELECT id::text FROM cv_parse_runs WHERE candidate_user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		files = append(files, filepath.Join("cvparse", id))
	}
	rows.Close()

	stmts := []string{
		`UPDATE users SET email = 'anonim-' || id::text || '@anonim.invalid', display_name = '` + AnonName + `', password_hash = '!dianonimkan',
		        status = 'disabled', email_verified_at = NULL, anonymized_at = now(), updated_at = now(), version = version + 1 WHERE id = $1`,
		`DELETE FROM sessions WHERE user_id = $1`,
		`DELETE FROM email_tokens WHERE user_id = $1`,
		`UPDATE candidate_profiles SET full_name = '` + AnonName + `', phone = '', birth_date = NULL, city = '', address = '', summary = '',
		        contact_email = '', whatsapp_opt_in_at = NULL, version = version + 1, updated_at = now() WHERE user_id = $1`,
		`DELETE FROM education WHERE user_id = $1`,
		`DELETE FROM experiences WHERE user_id = $1`,
		`DELETE FROM certifications WHERE user_id = $1`,
		`DELETE FROM candidate_skills WHERE user_id = $1`,
		`DELETE FROM candidate_languages WHERE user_id = $1`,
		`DELETE FROM candidate_links WHERE user_id = $1`,
		`DELETE FROM candidate_projects WHERE user_id = $1`,
		`DELETE FROM identity_claims WHERE claimant_user_id = $1`,
		`DELETE FROM candidate_identities WHERE user_id = $1`,
		`UPDATE documents SET file_name = 'dihapus', deleted_at = coalesce(deleted_at, now()) WHERE owner_user_id = $1`,
		`DELETE FROM cv_parse_runs WHERE candidate_user_id = $1`,
		`DELETE FROM notifications WHERE user_id = $1`,
		`UPDATE email_deliveries SET to_address = 'anonim' WHERE to_user_id = $1`,
		// Isi jawaban esai bisa memuat data pribadi; nilai dan struktur jawaban tetap ada.
		`UPDATE exam_answers SET value = jsonb_set(value, '{text}', '"[dihapus atas permintaan subjek data]"')
		 WHERE value ? 'text' AND attempt_id IN (SELECT id FROM exam_attempts WHERE candidate_user_id = $1)`,
		`UPDATE exam_submissions s SET snapshot = jsonb_set(s.snapshot, '{items}', (
		   SELECT coalesce(jsonb_agg(CASE WHEN jsonb_typeof(it->'value') = 'object' AND it->'value' ? 'text'
		          THEN jsonb_set(it, '{value,text}', '"[dihapus atas permintaan subjek data]"') ELSE it END), '[]'::jsonb)
		   FROM jsonb_array_elements(s.snapshot->'items') it))
		 WHERE s.attempt_id IN (SELECT id FROM exam_attempts WHERE candidate_user_id = $1)`,
	}
	for _, s := range stmts {
		if _, err := tx.Exec(ctx, s, userID); err != nil {
			return nil, fmt.Errorf("anonimisasi: %w", err)
		}
	}
	if err := redactSnapshots(ctx, tx, userID); err != nil {
		return nil, err
	}
	return files, nil
}

// redactSnapshots mengosongkan isi salinan profil di setiap lamaran tanpa mengubah strukturnya, agar halaman
// lamaran lama tetap dapat dibuka (nama diganti, teks dikosongkan, daftar dikosongkan, tanggal dihapus).
func redactSnapshots(ctx context.Context, tx pgx.Tx, userID uuid.UUID) error {
	rows, err := tx.Query(ctx, `SELECT s.application_id, s.profile FROM application_snapshots s
		JOIN job_applications a ON a.id = s.application_id WHERE a.candidate_user_id = $1`, userID)
	if err != nil {
		return err
	}
	type snap struct {
		id  uuid.UUID
		raw []byte
	}
	var all []snap
	for rows.Next() {
		var s snap
		if err := rows.Scan(&s.id, &s.raw); err != nil {
			rows.Close()
			return err
		}
		all = append(all, s)
	}
	rows.Close()
	for _, s := range all {
		var m map[string]any
		if err := json.Unmarshal(s.raw, &m); err != nil {
			return err
		}
		m["email"], m["nikMasked"] = "", ""
		if p, ok := m["profile"].(map[string]any); ok {
			for k, v := range p {
				switch v.(type) {
				case string:
					p[k] = ""
				case []any:
					p[k] = []any{}
				case map[string]any:
					p[k] = map[string]any{}
				default:
					if k == "birthDate" {
						p[k] = nil
					}
				}
			}
			p["fullName"] = AnonName
		}
		b, _ := json.Marshal(m)
		if _, err := tx.Exec(ctx, `UPDATE application_snapshots SET profile = $2 WHERE application_id = $1`, s.id, b); err != nil {
			return err
		}
	}
	return nil
}

// RemoveFiles menghapus berkas hasil Anonymize. Kegagalan dicatat agar dapat dibersihkan manual; data di basis data
// sudah tidak menunjuk berkas tersebut.
func RemoveFiles(objectDir string, files []string) []string {
	var failed []string
	for _, f := range files {
		if err := os.RemoveAll(filepath.Join(objectDir, f)); err != nil {
			failed = append(failed, f)
		}
	}
	return failed
}

// Message menerjemahkan error domain ke pesan konflik untuk API.
func Message(err error) error {
	switch {
	case errors.Is(err, ErrNotCandidate):
		return platform.ErrNotFound("Kandidat tidak ditemukan.")
	case errors.Is(err, ErrAnonymized):
		return platform.ErrConflict("already_anonymized", "Data kandidat ini sudah dianonimkan.")
	case errors.Is(err, ErrLegalHold):
		return platform.ErrConflict("legal_hold", "Kandidat sedang dalam legal hold. Lepas legal hold terlebih dahulu bila penghapusan sudah diizinkan.")
	case errors.Is(err, ErrActive):
		return platform.ErrConflict("active_applications", "Kandidat masih memiliki lamaran aktif. Tolak atau tarik lamaran tersebut terlebih dahulu.")
	}
	return err
}
