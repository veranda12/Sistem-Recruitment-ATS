package app

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// Vertical slice: karier -> registrasi/verifikasi -> profil -> apply 2 lowongan -> recruiter membuka lamaran -> transisi teraudit.
func TestCandidateToRecruiterSlice(t *testing.T) {
	e := newEnv(t)
	anon := e.client()

	// Karier publik: hanya published; draft 404; lowongan tutup tampil dengan status jelas.
	jobs := anon.do("GET", "/jobs", nil)
	expect(t, jobs, 200, "daftar lowongan")
	items := jobs.Body["items"].([]any)
	slugs := map[string]bool{}
	for _, it := range items {
		slugs[it.(map[string]any)["slug"].(string)] = true
	}
	if slugs["data-analyst"] || slugs["admin-gudang-surabaya"] || !slugs["operator-produksi-karawang"] {
		t.Fatalf("daftar publik harus hanya lowongan published: %v", slugs)
	}
	expect(t, anon.do("GET", "/jobs/data-analyst", nil), 404, "draft tersembunyi")
	closed := anon.do("GET", "/jobs/admin-gudang-surabaya", nil)
	expect(t, closed, 200, "lowongan tutup")
	if closed.Body["acceptingApplications"] != false || closed.Body["status"] != "closed" {
		t.Fatalf("lowongan tutup harus memberi status jelas: %s", closed.Raw)
	}
	page := anon.do("GET", "/jobs?limit=1", nil)
	if page.Body["nextCursor"] == nil {
		t.Fatal("pagination harus memberi nextCursor")
	}
	page2 := anon.do("GET", "/jobs?limit=1&cursor="+page.Body["nextCursor"].(string), nil)
	if page2.Body["items"].([]any)[0].(map[string]any)["slug"] == page.Body["items"].([]any)[0].(map[string]any)["slug"] {
		t.Fatal("halaman kedua tidak boleh mengulang item")
	}
	expect(t, anon.do("GET", "/jobs?type=bebas", nil), 422, "filter di luar allowlist")

	// Registrasi: respons netral; belum terverifikasi tidak bisa melamar.
	c := e.client()
	expect(t, c.do("POST", "/auth/register", map[string]any{"email": "Rina@Contoh.test", "password": testPassword, "fullName": "Rina Amelia",
		"nik": "3201 0145 0799 0003", "acceptPrivacy": true}), 202, "register")
	expect(t, c.do("POST", "/auth/register", map[string]any{"email": "rina@contoh.test", "password": testPassword, "fullName": "Rina Lain",
		"nik": "3201014507990099", "acceptPrivacy": true}), 202, "register email ganda tetap netral")
	if n := e.count(`SELECT count(*) FROM users WHERE email = 'rina@contoh.test'`); n != 1 {
		t.Fatalf("email ganda membuat %d akun", n)
	}
	c.mustLogin("rina@contoh.test")
	expect(t, c.do("POST", "/jobs/operator-produksi-karawang/apply", nil), 409, "apply sebelum verifikasi")
	expect(t, c.do("POST", "/auth/verify-email", map[string]string{"token": e.latestToken("rina@contoh.test")}), 200, "verifikasi")
	c.mustLogin("rina@contoh.test")
	r := c.do("POST", "/jobs/operator-produksi-karawang/apply", nil)
	if r.Status != 409 || r.errCode() != "profile_incomplete" {
		t.Fatalf("profil belum lengkap harus ditolak: %d %s", r.Status, r.Raw)
	}
	prof := c.do("GET", "/me/profile", nil)
	if prof.Body["identity"].(map[string]any)["nikMasked"] != "************0003" {
		t.Fatalf("NIK harus tersamar dan nol di depan dipertahankan: %s", prof.Raw)
	}
	expect(t, c.do("PUT", "/me/profile", completeProfile("Rina Amelia", "Karawang", 1)), 200, "simpan profil")
	expect(t, c.do("PUT", "/me/profile", completeProfile("Rina Amelia", "Karawang", 1)), 409, "edit basi")

	// CSRF: mutasi bersesi tanpa token ditolak.
	saved := c.csrf
	c.csrf = ""
	expect(t, c.do("POST", "/jobs/operator-produksi-karawang/apply", nil), 403, "tanpa CSRF")
	c.csrf = saved

	// Apply dengan Idempotency-Key: ulangan identik mengembalikan hasil sama.
	a1 := c.do("POST", "/jobs/operator-produksi-karawang/apply", nil, "Idempotency-Key", "apply-op-00000001")
	expect(t, a1, 201, "apply operator")
	a1b := c.do("POST", "/jobs/operator-produksi-karawang/apply", nil, "Idempotency-Key", "apply-op-00000001")
	if a1b.Status != 201 || a1b.Body["applicationId"] != a1.Body["applicationId"] || a1b.Header.Get("Idempotent-Replay") != "true" {
		t.Fatalf("replay idempoten harus sama: %s / %s", a1.Raw, a1b.Raw)
	}
	if r := c.do("POST", "/jobs/operator-produksi-karawang/apply", nil); r.errCode() != "already_applied" {
		t.Fatalf("apply ganda harus already_applied: %s", r.Raw)
	}
	a2 := c.do("POST", "/jobs/backend-engineer/apply", nil)
	expect(t, a2, 201, "apply backend")
	if r := c.do("POST", "/jobs/admin-gudang-surabaya/apply", nil); r.errCode() != "job_closed" {
		t.Fatalf("apply lowongan tutup: %s", r.Raw)
	}
	expect(t, c.do("POST", "/jobs/data-analyst/apply", nil), 404, "apply draft")
	mine := c.do("GET", "/me/applications", nil)
	if n := len(mine.Body["items"].([]any)); n != 2 {
		t.Fatalf("kandidat harus punya 2 lamaran, dapat %d", n)
	}
	opApp, beApp := idOf(a1, "applicationId"), idOf(a2, "applicationId")

	// Profil berubah setelah melamar: snapshot lama tidak berubah.
	p := c.do("GET", "/me/profile", nil)
	expect(t, c.do("PUT", "/me/profile", completeProfile("Rina Amelia", "Bandung", int(p.Body["version"].(float64)))), 200, "ubah kota")

	// Recruiter OPS: melihat lamaran OPS, tidak melihat lamaran TI.
	rec := e.client().mustLogin("rekruter.ops@arunika.example")
	list := rec.do("GET", "/applications", nil)
	expect(t, list, 200, "daftar lamaran recruiter")
	ids := map[string]bool{}
	for _, it := range list.Body["items"].([]any) {
		ids[it.(map[string]any)["id"].(string)] = true
	}
	if !ids[opApp] || ids[beApp] {
		t.Fatalf("scope recruiter OPS salah: %v", ids)
	}
	expect(t, rec.do("GET", "/applications/"+beApp, nil), 404, "lamaran unit lain")
	det := rec.do("GET", "/applications/"+opApp, nil)
	expect(t, det, 200, "detail lamaran")
	snap := det.Body["snapshot"].(map[string]any)["profile"].(map[string]any)
	if snap["city"] != "Karawang" {
		t.Fatalf("snapshot harus tetap Karawang, dapat %v", snap["city"])
	}
	if det.Body["canRevealIdentity"] != false {
		t.Fatal("recruiter tidak boleh membuka NIK")
	}
	if _, has := det.Body["nik"]; has {
		t.Fatal("detail tidak boleh memuat NIK mentah")
	}
	ver := int(det.Body["version"].(float64))

	// Transisi manual: versi basi ditolak, versi benar berhasil, histori + audit tercatat.
	r = rec.do("POST", "/applications/"+opApp+"/transitions", map[string]any{"kind": "manual", "to": "ujian", "expectedVersion": ver + 5})
	if r.errCode() != "stale_version" {
		t.Fatalf("versi basi: %s", r.Raw)
	}
	r = rec.do("POST", "/applications/"+opApp+"/transitions", map[string]any{"kind": "manual", "to": "interview", "expectedVersion": ver})
	if r.errCode() != "transition_not_allowed" {
		t.Fatalf("lompat tahap harus ditolak: %s", r.Raw)
	}
	r = rec.do("POST", "/applications/"+opApp+"/transitions", map[string]any{"kind": "manual", "to": "ujian", "reason": "Lolos screening berkas", "expectedVersion": ver})
	expect(t, r, 200, "transisi screening->ujian")
	det = rec.do("GET", "/applications/"+opApp, nil)
	hist := det.Body["history"].([]any)
	if len(hist) != 2 || det.Body["stage"].(map[string]any)["key"] != "ujian" {
		t.Fatalf("histori harus 2 entri dan tahap ujian: %s", det.Raw)
	}
	last := hist[1].(map[string]any)
	if last["actorName"] != "Maya Sari" || last["reason"] != "Lolos screening berkas" {
		t.Fatalf("histori harus mencatat aktor dan alasan: %v", last)
	}
	if n := e.count(`SELECT count(*) FROM audit_logs WHERE action = 'application.transition' AND target_id = $1 AND request_id IS NOT NULL`, opApp); n != 1 {
		t.Fatalf("audit transisi = %d", n)
	}

	// Hiring Manager: bisa membaca lamaran job miliknya, tidak bisa memindahkan tahap, tidak melihat lowongan lain.
	hm := e.client().mustLogin("hm.ops@arunika.example")
	expect(t, hm.do("GET", "/applications/"+opApp, nil), 200, "HM baca lamaran job miliknya")
	expect(t, hm.do("POST", "/applications/"+opApp+"/transitions", map[string]any{"kind": "reject", "reason": "coba", "expectedVersion": ver + 1}), 403, "HM transisi")
	expect(t, hm.do("GET", "/applications/"+beApp, nil), 404, "HM lowongan lain")

	// Examiner tidak punya akses lamaran (dan NIK).
	ex := e.client().mustLogin("penguji@arunika.example")
	expect(t, ex.do("GET", "/applications", nil), 403, "examiner daftar lamaran")

	// HR Admin OPS: reveal NIK butuh alasan dan tercatat di audit.
	hr := e.client().mustLogin("hr.ops@arunika.example")
	expect(t, hr.do("POST", "/applications/"+opApp+"/reveal-identity", map[string]string{"reason": "x"}), 422, "reveal tanpa alasan")
	rv := hr.do("POST", "/applications/"+opApp+"/reveal-identity", map[string]string{"reason": "Verifikasi dokumen kontrak kerja"})
	expect(t, rv, 200, "reveal")
	if rv.Body["nik"] != "3201014507990003" {
		t.Fatalf("NIK hasil reveal salah: %v", rv.Body["nik"])
	}
	if e.count(`SELECT count(*) FROM audit_logs WHERE action = 'identity.reveal' AND reason IS NOT NULL`) != 1 {
		t.Fatal("reveal harus teraudit")
	}

	// Withdrawal kandidat: recruiter TI melihat status withdrawn.
	tl := c.do("GET", "/me/applications/"+beApp+"/timeline", nil)
	expect(t, tl, 200, "timeline")
	expect(t, c.do("POST", "/me/applications/"+beApp+"/withdraw", map[string]any{"reason": "Sudah diterima di tempat lain", "version": tl.Body["version"]}), 200, "withdraw")
	recTI := e.client().mustLogin("rekruter.ti@arunika.example")
	if st := recTI.do("GET", "/applications/"+beApp, nil).Body["status"]; st != "withdrawn" {
		t.Fatalf("status setelah withdraw = %v", st)
	}
	// Kandidat lain tidak bisa melihat lamaran Rina.
	budi := e.client().mustLogin("budi.kandidat@contoh.test")
	expect(t, budi.do("GET", "/me/applications/"+opApp+"/timeline", nil), 404, "timeline milik orang lain")
	expect(t, budi.do("GET", "/applications", nil), 403, "kandidat ke API staf")

	// Notifikasi inbox dibuat worker; replay event tidak menggandakan notifikasi.
	e.worker.Drain(context.Background())
	before := e.count(`SELECT count(*) FROM notifications`)
	if _, err := e.pool.Exec(context.Background(), `UPDATE outbox SET status = 'pending', available_at = now(), attempts = 0`); err != nil {
		t.Fatal(err)
	}
	e.worker.Drain(context.Background())
	if after := e.count(`SELECT count(*) FROM notifications`); after != before || before == 0 {
		t.Fatalf("replay event menggandakan notifikasi: %d -> %d", before, after)
	}
	inbox := c.do("GET", "/me/notifications", nil)
	if len(inbox.Body["items"].([]any)) == 0 {
		t.Fatal("kandidat harus menerima notifikasi")
	}

	// Logout mencabut sesi.
	expect(t, c.do("POST", "/auth/logout", nil), 204, "logout")
	expect(t, c.do("GET", "/me/profile", nil), 401, "setelah logout")
}

// Dua pendaftaran bersamaan dengan NIK sama menghasilkan maksimal satu master identitas.
func TestConcurrentSameNIKRegistration(t *testing.T) {
	e := newEnv(t)
	var wg sync.WaitGroup
	const n = 6
	statuses := make([]int, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := e.client()
			statuses[i] = c.do("POST", "/auth/register", map[string]any{"email": fmt.Sprintf("kembar%d@contoh.test", i), "password": testPassword,
				"fullName": "Kandidat Kembar", "nik": "3171019901010007", "acceptPrivacy": true}).Status
		}(i)
	}
	wg.Wait()
	for i, s := range statuses {
		if s != 202 {
			t.Fatalf("pendaftaran %d status %d (respons harus netral)", i, s)
		}
	}
	bi := e.keys.BlindIndex("3171019901010007")
	if got := e.count(`SELECT count(*) FROM candidate_identities WHERE nik_blind_index = $1`, bi); got != 1 {
		t.Fatalf("identitas master = %d, ingin 1", got)
	}
	if got := e.count(`SELECT count(*) FROM identity_claims WHERE nik_blind_index = $1 AND status = 'open'`, bi); got != n-1 {
		t.Fatalf("klaim verifikasi manual = %d, ingin %d", got, n-1)
	}
	// Akun tanpa identitas tidak dapat melamar sampai diverifikasi manual.
	var email string
	if err := e.pool.QueryRow(context.Background(), `SELECT u.email FROM identity_claims c JOIN users u ON u.id = c.claimant_user_id LIMIT 1`).Scan(&email); err != nil {
		t.Fatal(err)
	}
	c := e.client()
	expect(t, c.do("POST", "/auth/verify-email", map[string]string{"token": e.latestToken(email)}), 200, "verifikasi")
	c.mustLogin(email)
	expect(t, c.do("PUT", "/me/profile", completeProfile("Kandidat Kembar", "Jakarta", 1)), 200, "profil")
	if r := c.do("POST", "/jobs/operator-produksi-karawang/apply", nil); r.errCode() != "identity_pending" {
		t.Fatalf("akun klaim harus identity_pending: %s", r.Raw)
	}
}

// Apply yang balapan dengan penutupan lowongan: setiap apply berhasil penuh atau ditolak job_closed, tidak ada status lain.
func TestApplyRacesClose(t *testing.T) {
	e := newEnv(t)
	const n = 8
	cands := make([]*client, n)
	for i := range cands {
		cands[i] = e.registerVerified(fmt.Sprintf("balap%d@contoh.test", i), "Kandidat Balap", fmt.Sprintf("32010145079900%02d", 10+i))
	}
	hr := e.client().mustLogin("hr.ops@arunika.example")
	var jobID string
	var ver float64
	for _, it := range hr.do("GET", "/staff/jobs", nil).Body["items"].([]any) {
		m := it.(map[string]any)
		if m["slug"] == "supervisor-quality-control" {
			jobID, ver = m["id"].(string), m["version"].(float64)
		}
	}
	var wg sync.WaitGroup
	statuses := make([]resp, n)
	for i, c := range cands {
		wg.Add(1)
		go func(i int, c *client) {
			defer wg.Done()
			statuses[i] = c.do("POST", "/jobs/supervisor-quality-control/apply", nil)
		}(i, c)
	}
	wg.Add(1)
	var closeResp resp
	go func() {
		defer wg.Done()
		closeResp = hr.do("POST", "/staff/jobs/"+jobID+"/close", map[string]any{"version": ver, "reason": "Kuota terpenuhi"})
	}()
	wg.Wait()
	expect(t, closeResp, 200, "tutup lowongan")
	ok := 0
	for i, s := range statuses {
		switch {
		case s.Status == 201:
			ok++
		case s.Status == 409 && s.errCode() == "job_closed":
		default:
			t.Fatalf("apply %d: hasil tak terduga %d %s", i, s.Status, s.Raw)
		}
	}
	if got := e.count(`SELECT count(*) FROM job_applications WHERE job_id = $1`, jobID); got != int64(ok) {
		t.Fatalf("lamaran tersimpan %d, sukses dilaporkan %d", got, ok)
	}
	t.Logf("apply sukses sebelum tutup: %d dari %d", ok, n)
	if r := cands[0].do("POST", "/jobs/supervisor-quality-control/apply", nil); r.Status == 201 {
		t.Fatal("apply setelah tutup tidak boleh berhasil")
	}
}
