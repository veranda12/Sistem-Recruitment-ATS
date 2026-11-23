package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ats/internal/privacy"
)

func TestPrivacyExportAnonymizeAndRetention(t *testing.T) {
	e := newEnv(t)
	cand := e.registerVerified("subjek@contoh.test", "Rina Subjek Data", "3201014507997311")
	up := cand.upload("cv", "CV Rina.pdf", []byte("%PDF-1.4\nCV Rina Subjek Data\n"))
	expect(t, up, 201, "unggah dokumen")
	app := idOf(cand.do("POST", "/jobs/operator-produksi-karawang/apply", nil), "applicationId")
	e.worker.Drain(context.Background())
	userID := e.userID("subjek@contoh.test").String()
	docKey := e.scalar(`SELECT object_key FROM documents WHERE owner_user_id::text = $1`, userID).(string)

	admin := e.client().mustLogin("admin@arunika.example")
	hr := e.client().mustLogin("hr.ops@arunika.example")
	expect(t, hr.do("GET", "/admin/privacy/candidates?email=subjek@contoh.test", nil), 403, "HR berscope unit")
	found := admin.do("GET", "/admin/privacy/candidates?email=SUBJEK@contoh.test", nil)
	expect(t, found, 200, "cari kandidat")
	if items := found.Body["items"].([]any); len(items) != 1 || items[0].(map[string]any)["activeApplications"].(float64) != 1 {
		t.Fatalf("hasil cari: %v", found.Body)
	}

	// Ekspor: berkas JSON dengan data kandidat, tanpa rahasia.
	code, body := admin.get("/api/v1/admin/privacy/candidates/" + userID + "/export")
	if code != 200 || !strings.Contains(string(body), "Rina Subjek Data") || strings.Contains(string(body), "password") || strings.Contains(string(body), "nik_ciphertext") {
		t.Fatalf("ekspor %d: %s", code, body)
	}
	var exp struct {
		Data map[string][]any `json:"data"`
	}
	_ = json.Unmarshal(body, &exp)
	if len(exp.Data["lamaran"]) != 1 || len(exp.Data["pendidikan"]) != 1 || len(exp.Data["dokumen"]) != 1 {
		t.Fatalf("bagian ekspor: lamaran=%d pendidikan=%d dokumen=%d", len(exp.Data["lamaran"]), len(exp.Data["pendidikan"]), len(exp.Data["dokumen"]))
	}

	anon := func(confirm string) resp {
		return admin.do("POST", "/admin/privacy/candidates/"+userID+"/anonymize", map[string]any{"reason": "Permintaan subjek data DSR-2026-07", "confirmEmail": confirm})
	}
	expect(t, anon("salah@contoh.test"), 422, "konfirmasi email salah")
	if r := anon("subjek@contoh.test"); r.errCode() != "active_applications" {
		t.Fatalf("lamaran aktif harus menolak: %s", r.Raw)
	}
	rec := e.client().mustLogin("rekruter.ops@arunika.example")
	d := rec.do("GET", "/applications/"+app, nil)
	expect(t, rec.do("POST", "/applications/"+app+"/transitions", map[string]any{"kind": "reject", "reason": "Tidak memenuhi syarat", "expectedVersion": d.Body["version"]}), 200, "tolak")
	expect(t, admin.do("POST", "/admin/privacy/candidates/"+userID+"/legal-hold", map[string]any{"hold": true, "reason": "Sengketa ketenagakerjaan"}), 200, "legal hold")
	if r := anon("subjek@contoh.test"); r.errCode() != "legal_hold" {
		t.Fatalf("legal hold harus menolak: %s", r.Raw)
	}
	expect(t, admin.do("POST", "/admin/privacy/candidates/"+userID+"/legal-hold", map[string]any{"hold": false, "reason": "Sengketa selesai"}), 200, "lepas legal hold")
	expect(t, anon("subjek@contoh.test"), 200, "anonimkan")

	checks := map[string]int64{
		`SELECT count(*) FROM users WHERE id::text = $1 AND email LIKE 'anonim-%' AND display_name = 'Kandidat dianonimkan' AND status = 'disabled'`: 1,
		`SELECT count(*) FROM candidate_profiles WHERE user_id::text = $1 AND phone = '' AND birth_date IS NULL AND city = ''`:                       1,
		`SELECT count(*) FROM education WHERE user_id::text = $1`:                                                                                  0,
		`SELECT count(*) FROM candidate_identities WHERE user_id::text = $1`:                                                                       0,
		`SELECT count(*) FROM notifications WHERE user_id::text = $1`:                                                                              0,
		`SELECT count(*) FROM sessions WHERE user_id::text = $1`:                                                                                   0,
		`SELECT count(*) FROM documents WHERE owner_user_id::text = $1 AND deleted_at IS NOT NULL AND file_name = 'dihapus'`:                        1,
		`SELECT count(*) FROM email_deliveries WHERE to_user_id::text = $1 AND to_address <> 'anonim'`:                                              0,
		`SELECT count(*) FROM application_snapshots s JOIN job_applications a ON a.id = s.application_id
		  WHERE a.candidate_user_id::text = $1 AND s.profile->'profile'->>'fullName' = 'Kandidat dianonimkan' AND s.profile->>'email' = ''`: 1,
		`SELECT count(*) FROM transition_history t JOIN job_applications a ON a.id = t.application_id WHERE a.candidate_user_id::text = $1`: 2,
	}
	for q, want := range checks {
		if n := e.count(q, userID); n != want {
			t.Errorf("setelah anonimisasi %d (ingin %d): %s", n, want, q)
		}
	}
	if _, err := os.Stat(filepath.Join(e.cfg.ObjectDir, filepath.FromSlash(docKey))); !os.IsNotExist(err) {
		t.Errorf("berkas dokumen masih ada: %v", err)
	}
	// Bukti proses tetap terbaca oleh staf; kandidat tidak bisa masuk lagi.
	if d := rec.do("GET", "/applications/"+app, nil); d.Status != 200 || d.Body["candidate"].(map[string]any)["name"] != "Kandidat dianonimkan" {
		t.Fatalf("detail lamaran setelah anonimisasi: %d %v", d.Status, d.Body["candidate"])
	}
	if r := e.client().login("subjek@contoh.test"); r.Status == 200 {
		t.Fatal("kandidat yang dianonimkan masih bisa masuk")
	}
	if r := anon("subjek@contoh.test"); r.Status == 200 {
		t.Fatalf("anonimisasi ulang harus ditolak: %s", r.Raw)
	}

	// Retensi: mati secara bawaan; setelah diaktifkan, kandidat tanpa aktivitas melewati masa simpan dianonimkan.
	e.registerVerified("lama.retensi@contoh.test", "Kandidat Lama", "3201014507997312")
	oldID := e.userID("lama.retensi@contoh.test").String()
	for _, q := range []string{
		`UPDATE users SET created_at = now() - interval '3 months', updated_at = now() - interval '3 months' WHERE id::text = $1`,
		`UPDATE candidate_profiles SET updated_at = now() - interval '3 months' WHERE user_id::text = $1`,
		`UPDATE sessions SET last_seen_at = now() - interval '3 months' WHERE user_id::text = $1`,
	} {
		if _, err := e.pool.Exec(context.Background(), q, oldID); err != nil {
			t.Fatal(err)
		}
	}
	setting := func(key string, v any) {
		cur := admin.do("GET", "/admin/settings", nil)
		ver := 0.0
		for _, it := range cur.Body["items"].([]any) {
			if m := it.(map[string]any); m["key"] == key {
				ver, _ = m["version"].(float64)
			}
		}
		expect(t, admin.do("PUT", "/admin/settings/"+key, map[string]any{"value": v, "expectedVersion": ver, "reason": "Uji retensi"}), 200, "pengaturan "+key)
	}
	setting("retention.candidate_months", 2)
	dry := admin.do("GET", "/admin/privacy/retention", nil)
	if dry.Body["enabled"] != false || dry.Body["eligible"].(float64) != 1 {
		t.Fatalf("dry-run retensi: %v", dry.Body)
	}
	if n, err := privacy.RetentionOnce(context.Background(), e.pool, e.orgID, e.cfg.ObjectDir, 10); err != nil || n != 0 {
		t.Fatalf("retensi mati harus tidak menganonimkan: %d %v", n, err)
	}
	setting("retention.auto_enabled", true)
	if n, err := privacy.RetentionOnce(context.Background(), e.pool, e.orgID, e.cfg.ObjectDir, 10); err != nil || n != 1 {
		t.Fatalf("retensi aktif: %d %v", n, err)
	}
	if n := e.count(`SELECT count(*) FROM users WHERE id::text = $1 AND anonymized_at IS NOT NULL`, oldID); n != 1 {
		t.Fatal("kandidat lama belum dianonimkan")
	}
	if n := e.count(`SELECT count(*) FROM audit_logs WHERE action IN ('privacy.export','privacy.anonymize','privacy.retention_anonymize','privacy.legal_hold_set','privacy.legal_hold_release')`); n != 5 {
		t.Fatalf("audit privasi: %d", n)
	}
}
