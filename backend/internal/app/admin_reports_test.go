package app

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"ats/internal/reporting"
)

func TestSafeCell(t *testing.T) {
	for in, want := range map[string]string{"=SUM(A1)": "'=SUM(A1)", "+62": "'+62", "-1": "'-1", "@cmd": "'@cmd", "\tx": "'\tx", "Operator": "Operator", "": ""} {
		if got := reporting.SafeCell(in); got != want {
			t.Fatalf("SafeCell(%q) = %q, ingin %q", in, got, want)
		}
	}
}

func num(r resp, path ...string) float64 {
	var cur any = r.Body
	for _, p := range path {
		cur = cur.(map[string]any)[p]
	}
	if cur == nil {
		return 0
	}
	return cur.(float64)
}

// Fixture: kandidat A melamar 2 lowongan (OPS + TI) lalu menarik yang TI; kandidat B melamar 1 lowongan OPS.
func TestReportsMatchFixtureAndExport(t *testing.T) {
	e := newEnv(t)
	a := e.registerVerified("dua.lamaran@contoh.test", "Kandidat Dua", "3201014507997001")
	op := a.do("POST", "/jobs/operator-produksi-karawang/apply", nil)
	be := a.do("POST", "/jobs/backend-engineer/apply", nil)
	expect(t, op, 201, "apply op")
	expect(t, be, 201, "apply be")
	tl := a.do("GET", "/me/applications/"+idOf(be, "applicationId")+"/timeline", nil)
	expect(t, a.do("POST", "/me/applications/"+idOf(be, "applicationId")+"/withdraw", map[string]any{"version": tl.Body["version"]}), 200, "withdraw")
	b := e.registerVerified("satu.lamaran@contoh.test", "Kandidat Satu", "3201014507997002")
	expect(t, b.do("POST", "/jobs/operator-produksi-karawang/apply", nil), 201, "apply b")

	// Super admin (org-wide): 3 lamaran, 2 kandidat unik, 1 ditarik.
	adm := e.client().mustLogin("admin@arunika.example")
	r := adm.do("GET", "/reports/recruitment", nil)
	expect(t, r, 200, "laporan org")
	if num(r, "totals", "applications") != 3 || num(r, "totals", "candidates") != 2 || num(r, "totals", "withdrawn") != 1 {
		t.Fatalf("total org salah: %v", r.Body["totals"])
	}
	if num(r, "totals", "reached", "screening") != 3 {
		t.Fatalf("reached screening harus 3: %v", r.Body["totals"])
	}
	if r.Body["range"].(map[string]any)["timezone"] != "Asia/Jakarta" {
		t.Fatalf("zona tanggal harus dicantumkan: %v", r.Body["range"])
	}
	// HR OPS (scope unit OPS): hanya 2 lamaran OPS, tidak ada penarikan.
	hr := e.client().mustLogin("hr.ops@arunika.example")
	r = hr.do("GET", "/reports/recruitment", nil)
	if num(r, "totals", "applications") != 2 || num(r, "totals", "withdrawn") != 0 || num(r, "totals", "candidates") != 2 {
		t.Fatalf("total OPS salah: %v", r.Body["totals"])
	}
	// Rentang tanggal yang tidak mencakup hari ini: kosong.
	r = hr.do("GET", "/reports/recruitment?from=2020-01-01&to=2020-01-31", nil)
	if num(r, "totals", "applications") != 0 {
		t.Fatal("rentang lampau harus kosong")
	}
	expect(t, hr.do("GET", "/reports/recruitment?from=2026-02-30", nil), 422, "tanggal tidak valid")
	// Recruiter tidak punya reports.read.
	expect(t, e.client().mustLogin("rekruter.ops@arunika.example").do("GET", "/reports/recruitment", nil), 403, "recruiter laporan")

	// Ekspor funnel: diproses worker, angka sama dengan dashboard untuk filter identik.
	ex := hr.do("POST", "/reports/exports", map[string]any{"kind": "funnel"})
	expect(t, ex, 202, "minta ekspor")
	e.worker.Drain(context.Background())
	list := hr.do("GET", "/reports/exports", nil)
	item := list.Body["items"].([]any)[0].(map[string]any)
	if item["status"] != "ready" {
		t.Fatalf("status ekspor: %v", item)
	}
	link := hr.do("POST", "/reports/exports/"+item["id"].(string)+"/link", nil)
	expect(t, link, 200, "tautan")
	url := link.Body["url"].(string)
	req, _ := http.NewRequest("GET", e.srv.URL+url, nil)
	res, err := hr.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(body), "Operator Produksi;2;2;2;0;0;0") {
		t.Fatalf("CSV ekspor: %d %q", res.StatusCode, body)
	}
	// Tautan milik HR OPS tidak berlaku untuk pengguna lain; tanda tangan rusak ditolak.
	other := e.client().mustLogin("hr.ti@arunika.example")
	req, _ = http.NewRequest("GET", e.srv.URL+url, nil)
	res, _ = other.http.Do(req)
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatalf("tautan dipakai pengguna lain: %d", res.StatusCode)
	}
	req, _ = http.NewRequest("GET", e.srv.URL+strings.Replace(url, "sig=", "sig=x", 1), nil)
	res, _ = hr.http.Do(req)
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatalf("tanda tangan rusak: %d", res.StatusCode)
	}
	// Ekspor data pribadi ditolak 503 saat limiter global (Redis) tidak tersedia dan mode ketat aktif.
	if r := hr.do("POST", "/reports/exports", map[string]any{"kind": "applications"}); r.Status != 503 {
		t.Fatalf("ekspor PII tanpa Redis: %s", r.Raw)
	}
	// Ekspor kedaluwarsa dibersihkan.
	if _, err := e.pool.Exec(context.Background(), `UPDATE exports SET expires_at = now() - interval '1 minute'`); err != nil {
		t.Fatal(err)
	}
	if n := reporting.CleanupExpired(context.Background(), e.pool, e.cfg.ObjectDir); n != 1 {
		t.Fatalf("pembersihan ekspor = %d", n)
	}
	expect(t, hr.do("POST", "/reports/exports/"+item["id"].(string)+"/link", nil), 409, "tautan ekspor kedaluwarsa")
}

func TestAdminAccessSettingsAudit(t *testing.T) {
	e := newEnv(t)
	adm := e.client().mustLogin("admin@arunika.example")
	users := adm.do("GET", "/admin/users", nil)
	expect(t, users, 200, "daftar user")
	var adminAsg, adminID string
	for _, u := range users.Body["items"].([]any) {
		m := u.(map[string]any)
		if m["email"] == "admin@arunika.example" {
			adminID = m["id"].(string)
			adminAsg = m["assignments"].([]any)[0].(map[string]any)["id"].(string)
		}
	}
	// Admin aktif terakhir tidak dapat dicabut atau dinonaktifkan.
	if r := adm.do("POST", "/admin/assignments/"+adminAsg+"/revoke", map[string]any{"reason": "uji"}); r.errCode() != "last_admin" {
		t.Fatalf("cabut admin terakhir: %s", r.Raw)
	}
	if r := adm.do("POST", "/admin/users/"+adminID+"/status", map[string]any{"status": "disabled"}); r.errCode() != "self_disable" {
		t.Fatalf("nonaktifkan diri: %s", r.Raw)
	}

	// Buat staf baru tanpa sandi default: undangan berupa tautan atur sandi.
	nu := adm.do("POST", "/admin/users", map[string]any{"email": "rekruter.baru@arunika.example", "name": "Rekruter Baru"})
	expect(t, nu, 201, "buat user")
	nuID := nu.Body["id"].(string)
	expect(t, adm.do("POST", "/admin/users/"+nuID+"/assignments", map[string]any{"roleKey": "recruiter", "unitCode": "OPS", "reason": "Tim baru"}), 201, "beri peran")
	expect(t, adm.do("POST", "/admin/users/"+nuID+"/assignments", map[string]any{"roleKey": "recruiter", "unitCode": "OPS"}), 409, "peran ganda")
	newc := e.client()
	if r := newc.login("rekruter.baru@arunika.example"); r.Status != 401 {
		t.Fatal("akun baru tidak boleh bisa login sebelum sandi diatur")
	}
	expect(t, newc.do("POST", "/auth/reset-password", map[string]string{"token": e.latestToken("rekruter.baru@arunika.example"), "password": testPassword}), 200, "atur sandi")
	newc.mustLogin("rekruter.baru@arunika.example")
	expect(t, newc.do("GET", "/applications", nil), 200, "akses dengan peran")

	// Pencabutan peran berlaku pada permintaan berikutnya (sesi yang sama).
	users = adm.do("GET", "/admin/users", nil)
	var asg string
	for _, u := range users.Body["items"].([]any) {
		if m := u.(map[string]any); m["id"] == nuID {
			asg = m["assignments"].([]any)[0].(map[string]any)["id"].(string)
		}
	}
	expect(t, adm.do("POST", "/admin/assignments/"+asg+"/revoke", map[string]any{"reason": "Pindah tim"}), 204, "cabut")
	expect(t, newc.do("GET", "/applications", nil), 403, "setelah dicabut")
	// Menonaktifkan pengguna mencabut sesinya.
	expect(t, adm.do("POST", "/admin/users/"+nuID+"/status", map[string]any{"status": "disabled", "reason": "Keluar"}), 204, "nonaktifkan")
	expect(t, newc.do("GET", "/auth/session", nil), 401, "sesi dicabut")

	// Examiner dan HR tanpa users.manage tidak bisa mengelola akses.
	expect(t, e.client().mustLogin("penguji@arunika.example").do("GET", "/admin/users", nil), 403, "examiner admin")

	// Pengaturan berversi: versi basi ditolak, riwayat dan waktu efektif tercatat.
	expect(t, adm.do("PUT", "/admin/settings/retention.candidate_months", map[string]any{"value": 999, "expectedVersion": 0, "reason": "uji batas"}), 422, "nilai di luar batas")
	s1 := adm.do("PUT", "/admin/settings/retention.candidate_months", map[string]any{"value": 24, "expectedVersion": 0, "reason": "Kebijakan awal",
		"effectiveAt": time.Now().Add(24 * time.Hour).Format(time.RFC3339)})
	expect(t, s1, 200, "set pengaturan")
	expect(t, adm.do("PUT", "/admin/settings/retention.candidate_months", map[string]any{"value": 12, "expectedVersion": 0, "reason": "Data basi"}), 409, "pengaturan basi")
	expect(t, adm.do("PUT", "/admin/settings/retention.candidate_months", map[string]any{"value": 18, "expectedVersion": 1, "reason": "Revisi legal"}), 200, "versi 2")
	hist := adm.do("GET", "/admin/settings/retention.candidate_months/history", nil)
	if n := len(hist.Body["items"].([]any)); n != 2 {
		t.Fatalf("riwayat pengaturan = %d", n)
	}
	expect(t, adm.do("PUT", "/admin/settings/tidak.ada", map[string]any{"value": 1, "reason": "xxxxx"}), 404, "kunci tidak dikenal")

	// Branding: perubahan terlihat di portal publik.
	org := adm.do("GET", "/admin/organization", nil).Body["organization"].(map[string]any)
	expect(t, adm.do("PUT", "/admin/organization", map[string]any{"name": org["name"], "version": org["version"],
		"profile": map[string]any{"tagline": "Tagline baru", "about": "Tentang"}, "brand": map[string]any{"displayName": "Karier Arunika Baru", "primaryColor": "#1557B0"}}), 200, "branding")
	pub := e.client().do("GET", "/org", nil)
	if pub.Body["brand"].(map[string]any)["displayName"] != "Karier Arunika Baru" {
		t.Fatalf("branding tidak terlihat di portal: %s", pub.Raw)
	}
	// Master data dinonaktifkan: hilang dari filter publik, lowongan lama tetap ada.
	var sby string
	for _, l := range adm.do("GET", "/admin/organization", nil).Body["locations"].([]any) {
		if m := l.(map[string]any); m["code"] == "SBY" {
			sby = m["id"].(string)
		}
	}
	expect(t, adm.do("POST", "/admin/locations/"+sby+"/active", map[string]any{"active": false}), 204, "nonaktifkan lokasi")
	if strings.Contains(string(e.client().do("GET", "/org", nil).Raw), `"SBY"`) {
		t.Fatal("lokasi nonaktif tidak boleh muncul di filter publik")
	}
	expect(t, e.client().do("GET", "/jobs/admin-gudang-surabaya", nil), 200, "histori lowongan lokasi nonaktif")

	// Audit HR OPS hanya entri dalam unitnya; super admin melihat semua. Audit tidak bisa diubah.
	hr := e.client().mustLogin("hr.ops@arunika.example")
	au := hr.do("GET", "/admin/audit", nil)
	expect(t, au, 200, "audit hr")
	for _, it := range au.Body["items"].([]any) {
		if it.(map[string]any)["action"] == "settings.update" {
			t.Fatal("HR unit tidak boleh melihat audit tanpa scope unitnya")
		}
	}
	all := adm.do("GET", "/admin/audit?action=admin.role_", nil)
	if len(all.Body["items"].([]any)) < 2 {
		t.Fatalf("audit peran: %s", all.Raw)
	}
	if _, err := e.pool.Exec(context.Background(), `DELETE FROM audit_logs`); err == nil {
		t.Fatal("audit harus append-only")
	}
	// Endpoint metrik tidak diekspos di router publik.
	res, _ := http.Get(e.srv.URL + "/metrics")
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatalf("/metrics publik = %d", res.StatusCode)
	}
}
