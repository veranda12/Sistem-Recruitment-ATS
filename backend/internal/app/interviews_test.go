package app

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestInterviewsScorecardDecision(t *testing.T) {
	e := newEnv(t)
	cand := e.registerVerified("interview@contoh.test", "Kandidat Interview", "3201014507997201")
	app := idOf(cand.do("POST", "/jobs/operator-produksi-karawang/apply", nil), "applicationId")
	rec := e.client().mustLogin("rekruter.ops@arunika.example")
	hmID := e.userID("hm.ops@arunika.example").String()
	recID := e.userID("rekruter.ops@arunika.example").String()
	start := time.Now().Add(48 * time.Hour).Truncate(time.Minute)

	sched := func(c *client, at time.Time, ivs ...string) resp {
		return c.do("POST", "/applications/"+app+"/interviews", map[string]any{"startsAt": at.Format(time.RFC3339), "durationMinutes": 60,
			"timezone": "Asia/Jakarta", "mode": "video", "location": "https://meet.contoh.test/abc", "interviewerIds": ivs})
	}
	expect(t, sched(rec, start, "bukan-uuid"), 400, "id interviewer salah format")
	expect(t, rec.do("POST", "/applications/"+app+"/interviews", map[string]any{"startsAt": start.Format(time.RFC3339), "durationMinutes": 60,
		"timezone": "Mars/Olympus", "mode": "video", "interviewerIds": []string{hmID}}), 422, "zona waktu tidak dikenal")
	first := sched(rec, start, hmID, recID)
	expect(t, first, 201, "jadwal pertama")
	ivID := first.Body["id"].(string)

	// Bentrok jadwal interviewer: ditolak, juga saat dua penjadwalan berjalan bersamaan.
	if r := sched(rec, start.Add(30*time.Minute), hmID); r.errCode() != "interviewer_conflict" {
		t.Fatalf("bentrok: %s", r.Raw)
	}
	var wg sync.WaitGroup
	res := make([]resp, 2)
	slot := start.Add(5 * time.Hour)
	for i := range res {
		wg.Add(1)
		go func(i int) { defer wg.Done(); res[i] = sched(rec, slot.Add(time.Duration(i)*10*time.Minute), hmID) }(i)
	}
	wg.Wait()
	if (res[0].Status == 201) == (res[1].Status == 201) {
		t.Fatalf("penjadwalan bersamaan harus tepat satu berhasil: %d %d", res[0].Status, res[1].Status)
	}

	// Reschedule mencatat zona waktu dan mengirim pembaruan; versi basi ditolak.
	ivs := rec.do("GET", "/applications/"+app, nil).Body["interviews"].([]any)
	var ver float64
	for _, x := range ivs {
		if m := x.(map[string]any); m["id"] == ivID {
			ver = m["version"].(float64)
		}
	}
	newAt := start.Add(26 * time.Hour)
	rs := rec.do("POST", "/interviews/"+ivID+"/reschedule", map[string]any{"startsAt": newAt.Format(time.RFC3339), "durationMinutes": 45,
		"timezone": "Asia/Makassar", "version": ver, "reason": "Permintaan kandidat"})
	expect(t, rs, 200, "reschedule")
	expect(t, rec.do("POST", "/interviews/"+ivID+"/reschedule", map[string]any{"startsAt": newAt.Format(time.RFC3339), "durationMinutes": 45,
		"timezone": "Asia/Makassar", "version": ver, "reason": "Basi lagi"}), 409, "reschedule basi")
	if tz := e.scalar(`SELECT timezone FROM interview_changes WHERE interview_id = $1 AND action = 'reschedule'`, ivID); tz != "Asia/Makassar" {
		t.Fatalf("zona waktu reschedule = %v", tz)
	}
	e.worker.Drain(context.Background())
	var body string
	if err := e.pool.QueryRow(context.Background(), `SELECT body FROM notifications WHERE template_key = 'interview_rescheduled'`).Scan(&body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "WITA") || !strings.Contains(body, "WIB") {
		t.Fatalf("pembaruan harus menyebut zona lama dan baru: %q", body)
	}

	// Scorecard hanya oleh interviewer panel; staf unit lain tidak melihat.
	hm := e.client().mustLogin("hm.ops@arunika.example")
	expect(t, hm.do("POST", "/interviews/"+ivID+"/evaluations", map[string]any{"rating": 6, "recommendation": "lanjut"}), 422, "rating di luar skala")
	expect(t, hm.do("POST", "/interviews/"+ivID+"/evaluations", map[string]any{"rating": 4, "recommendation": "lanjut", "notes": "Komunikatif"}), 200, "scorecard HM")
	hr := e.client().mustLogin("hr.ops@arunika.example")
	expect(t, hr.do("POST", "/interviews/"+ivID+"/evaluations", map[string]any{"rating": 3, "recommendation": "pertimbangkan"}), 403, "bukan panel")
	expect(t, e.client().mustLogin("rekruter.ti@arunika.example").do("POST", "/interviews/"+ivID+"/evaluations", map[string]any{"rating": 3, "recommendation": "lanjut"}), 404, "unit lain")
	if n := len(rec.do("GET", "/applications/"+app, nil).Body["evaluations"].([]any)); n != 1 {
		t.Fatalf("scorecard di detail = %d", n)
	}
	// Kalender interviewer.
	cal := hm.do("GET", "/staff/interviews", nil)
	if len(cal.Body["items"].([]any)) < 2 {
		t.Fatalf("kalender HM: %s", cal.Raw)
	}

	// Lamaran ditolak -> interview terjadwal dibatalkan otomatis, slot interviewer bebas lagi.
	det := rec.do("GET", "/applications/"+app, nil)
	expect(t, rec.do("POST", "/applications/"+app+"/transitions", map[string]any{"kind": "reject", "reason": "Tidak memenuhi kualifikasi", "expectedVersion": det.Body["version"]}), 200, "tolak")
	if n := e.count(`SELECT count(*) FROM interviews WHERE application_id = $1 AND status = 'scheduled'`, app); n != 0 {
		t.Fatalf("interview terjadwal tersisa: %d", n)
	}
	// Pemberitahuan keputusan dikirim recruiter sekali saja.
	expect(t, rec.do("POST", "/applications/"+app+"/decision-notice", nil), 200, "kirim keputusan")
	expect(t, rec.do("POST", "/applications/"+app+"/decision-notice", nil), 409, "kirim keputusan kedua")
	e.worker.Drain(context.Background())
	if n := e.count(`SELECT count(*) FROM notifications WHERE template_key = 'decision_rejected'`); n != 1 {
		t.Fatalf("notifikasi keputusan = %d", n)
	}
	// Lamaran aktif lain: withdrawal membatalkan interview.
	c2 := e.registerVerified("interview2@contoh.test", "Kandidat Dua", "3201014507997202")
	app2 := idOf(c2.do("POST", "/jobs/operator-produksi-karawang/apply", nil), "applicationId")
	expect(t, rec.do("POST", "/applications/"+app2+"/interviews", map[string]any{"startsAt": start.Add(30 * time.Hour).Format(time.RFC3339), "durationMinutes": 60,
		"timezone": "Asia/Jakarta", "mode": "onsite", "location": "Kantor Karawang", "interviewerIds": []string{hmID}}), 201, "jadwal 2")
	tl := c2.do("GET", "/me/applications/"+app2+"/timeline", nil)
	expect(t, c2.do("POST", "/me/applications/"+app2+"/withdraw", map[string]any{"version": tl.Body["version"]}), 200, "withdraw")
	if n := e.count(`SELECT count(*) FROM interviews WHERE application_id = $1 AND status = 'cancelled'`, app2); n != 1 {
		t.Fatalf("interview setelah withdraw belum dibatalkan")
	}
}

func TestRequisitionApprovalAndJob(t *testing.T) {
	e := newEnv(t)
	hm := e.client().mustLogin("hm.ops@arunika.example")
	r := hm.do("POST", "/requisitions", map[string]any{"unitCode": "OPS", "locationCode": "KRW", "title": "Teknisi Perawatan Mesin", "headcount": 2,
		"employmentType": "contract", "justification": "Penambahan lini produksi"})
	expect(t, r, 201, "buat requisition")
	id := r.Body["id"].(string)
	expect(t, hm.do("POST", "/requisitions/"+id+"/approve", map[string]any{"version": 1}), 403, "HM tidak bisa menyetujui")
	expect(t, hm.do("POST", "/requisitions/"+id+"/submit", map[string]any{"version": 1}), 200, "ajukan")
	// HR unit lain tidak melihat; HR unit yang sama menyetujui.
	expect(t, e.client().mustLogin("hr.ti@arunika.example").do("POST", "/requisitions/"+id+"/approve", map[string]any{"version": 2}), 404, "HR TI")
	hr := e.client().mustLogin("hr.ops@arunika.example")
	expect(t, hr.do("POST", "/requisitions/"+id+"/reject", map[string]any{"version": 2}), 422, "tolak tanpa alasan")
	expect(t, hr.do("POST", "/requisitions/"+id+"/approve", map[string]any{"version": 2, "note": "Sesuai anggaran"}), 200, "setujui")
	// HM melihat requisition miliknya.
	if n := len(hm.do("GET", "/requisitions", nil).Body["items"].([]any)); n != 1 {
		t.Fatalf("HM melihat %d requisition", n)
	}
	// Lowongan dibuat dari requisition yang disetujui (publikasi terpisah dari requisition internal).
	job := hr.do("POST", "/staff/jobs", map[string]any{"unitCode": "OPS", "locationCode": "KRW", "workflowKey": "standar", "title": "Teknisi Perawatan Mesin",
		"slug": "teknisi-perawatan-mesin", "employmentType": "contract", "description": "Perawatan mesin produksi.", "requirements": "SMK Mesin", "requisitionId": id})
	expect(t, job, 201, "lowongan dari requisition")
	if job.Body["requisitionId"] != id || job.Body["status"] != "draft" {
		t.Fatalf("lowongan harus draft dan tertaut requisition: %s", job.Raw)
	}
	// Requisition yang belum disetujui tidak bisa dipakai.
	r2 := hm.do("POST", "/requisitions", map[string]any{"unitCode": "OPS", "locationCode": "KRW", "title": "Operator Forklift", "headcount": 1, "employmentType": "contract"})
	expect(t, hr.do("POST", "/staff/jobs", map[string]any{"unitCode": "OPS", "locationCode": "KRW", "workflowKey": "standar", "title": "Operator Forklift",
		"slug": "operator-forklift", "employmentType": "contract", "requisitionId": r2.Body["id"]}), 422, "requisition draft")
	if e.count(`SELECT count(*) FROM audit_logs WHERE action LIKE 'requisition.%'`) < 4 {
		t.Fatal("aksi requisition harus teraudit")
	}
}
