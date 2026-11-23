package app

import (
	"testing"
	"time"
)

func TestExamActivityLog(t *testing.T) {
	e := newEnv(t)
	ex := e.startExam("aktivitas@contoh.test", "3201014507997307")
	path := "/exam-attempts/" + ex.attempt + "/activity"
	now := time.Now().UTC()
	ev := func(seq int, kind string, ms ...int) map[string]any {
		m := map[string]any{"seq": seq, "kind": kind, "at": now.Format(time.RFC3339Nano)}
		if len(ms) > 0 {
			m["durationMs"] = ms[0]
		}
		return m
	}
	r := ex.c.do("POST", path, map[string]any{"events": []any{ev(1, "tab_hidden"), ev(2, "tab_visible", 90000), ev(3, "paste"), ev(4, "screenshot")}})
	expect(t, r, 200, "kirim aktivitas")
	if r.Body["accepted"].(float64) != 3 {
		t.Fatalf("jenis tidak dikenal harus diabaikan: %v", r.Body)
	}
	// Kiriman ulang (seq sama) tidak menggandakan.
	if r := ex.c.do("POST", path, map[string]any{"events": []any{ev(3, "paste"), ev(5, "paste")}}); r.Body["accepted"].(float64) != 1 {
		t.Fatalf("dedup seq: %v", r.Body)
	}
	// Kandidat lain tidak bisa menulis ke percobaan ini.
	other := e.registerVerified("lain.aktivitas@contoh.test", "Kandidat Lain", "3201014507997308")
	expect(t, other.do("POST", path, map[string]any{"events": []any{ev(9, "copy")}}), 404, "percobaan milik orang lain")

	hr := e.client().mustLogin("hr.ops@arunika.example")
	mon := hr.do("GET", "/exam-schedules/"+e.scheduleID()+"/monitor", nil)
	var act map[string]any
	for _, p := range mon.Body["participants"].([]any) {
		if pm := p.(map[string]any); pm["attemptId"] == ex.attempt {
			act = pm["activity"].(map[string]any)
		}
	}
	if act["paste"].(map[string]any)["count"].(float64) != 2 || act["tab_visible"].(map[string]any)["totalMs"].(float64) != 90000 {
		t.Fatalf("ringkasan monitor: %v", act)
	}
	list := hr.do("GET", path, nil)
	expect(t, list, 200, "detail aktivitas")
	if n := len(list.Body["items"].([]any)); n != 4 {
		t.Fatalf("detail aktivitas: %d", n)
	}
	expect(t, ex.c.do("GET", path, nil), 403, "kandidat tidak boleh membaca log staf")

	// Setelah dikirim, catatan baru tidak diterima.
	rev := ex.c.do("GET", "/exam-attempts/"+ex.attempt, nil).Body["revision"]
	expect(t, ex.c.do("POST", "/exam-attempts/"+ex.attempt+"/submit", map[string]any{"expectedRevision": rev}, "Idempotency-Key", "aktivitas-submit"), 200, "kirim jawaban")
	if r := ex.c.do("POST", path, map[string]any{"events": []any{ev(6, "copy")}}); r.Body["accepted"].(float64) != 0 {
		t.Fatalf("aktivitas setelah selesai: %v", r.Body)
	}
}
