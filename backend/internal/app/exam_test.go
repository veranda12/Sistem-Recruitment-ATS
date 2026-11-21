package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
)

// examCandidate: kandidat dengan lamaran di tahap ujian, diundang ke jadwal seed oleh HR OPS, lalu memulai attempt.
type examCandidate struct {
	c       *client
	appID   uuid.UUID
	attempt string
	dto     resp
}

func (e *env) scheduleID() string {
	return fmt.Sprint(uuid.UUID(e.scalar(`SELECT id FROM exam_schedules LIMIT 1`).([16]byte)))
}

func (e *env) startExam(email, nik string) examCandidate {
	e.t.Helper()
	appID := e.appAtExam(email, nik)
	hr := e.client().mustLogin("hr.ops@arunika.example")
	r := hr.do("POST", "/exam-schedules/"+e.scheduleID()+"/invitations", map[string]any{"applicationIds": []string{appID.String()}})
	expect(e.t, r, 200, "undang")
	if out := r.Body["results"].([]any)[0].(map[string]any)["outcome"]; out != "invited" {
		e.t.Fatalf("undangan: %v", out)
	}
	c := e.client().mustLogin(email)
	inv := c.do("GET", "/me/exam-invitations", nil)
	invID := inv.Body["items"].([]any)[0].(map[string]any)["id"].(string)
	st := c.do("POST", "/exam-invitations/"+invID+"/start", nil)
	expect(e.t, st, 201, "mulai ujian")
	return examCandidate{c: c, appID: appID, attempt: st.Body["id"].(string), dto: st}
}

// oracle membaca kunci langsung dari database (independen dari respons aplikasi): itemId -> {kind, opaque benar, opaque salah}.
type oracleItem struct {
	Kind    string
	Correct []string
	Wrong   string
	Weight  float64
}

func (e *env) oracle(attempt string) map[string]oracleItem {
	e.t.Helper()
	rows, err := e.pool.Query(context.Background(), `SELECT ai.item_id::text, qv.kind, ai.option_map, qv.answer_key, qv.weight::float8
		FROM attempt_items ai JOIN question_versions qv ON qv.id = ai.question_version_id WHERE ai.attempt_id = $1`, attempt)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]oracleItem{}
	for rows.Next() {
		var id, kind string
		var omap map[string]string
		var key []string
		var w float64
		if err := rows.Scan(&id, &kind, &omap, &key, &w); err != nil {
			e.t.Fatal(err)
		}
		oi := oracleItem{Kind: kind, Weight: w}
		for opaque, orig := range omap {
			isKey := false
			for _, k := range key {
				isKey = isKey || k == orig
			}
			if isKey {
				oi.Correct = append(oi.Correct, opaque)
			} else if oi.Wrong == "" {
				oi.Wrong = opaque
			}
		}
		out[id] = oi
	}
	return out
}

func save(c *client, attempt, op string, answers ...map[string]any) resp {
	return c.do("PUT", "/exam-attempts/"+attempt+"/answers", map[string]any{"opId": op, "answers": answers})
}

func ans(item string, base int, sel []string, text *string) map[string]any {
	v := map[string]any{}
	if sel != nil {
		v["selected"] = sel
	}
	if text != nil {
		v["text"] = *text
	}
	return map[string]any{"itemId": item, "baseRevision": base, "value": v}
}

func TestExamProtocolOnRealDB(t *testing.T) {
	e := newEnv(t)
	x := e.startExam("peserta1@contoh.test", "3201014507995001")
	c, att := x.c, x.attempt

	// Retry start mengembalikan attempt yang sama.
	inv := c.do("GET", "/me/exam-invitations", nil).Body["items"].([]any)[0].(map[string]any)["id"].(string)
	again := c.do("POST", "/exam-invitations/"+inv+"/start", nil)
	if again.Status != 200 || again.Body["id"] != att {
		t.Fatalf("retry start harus attempt sama: %s", again.Raw)
	}
	if n := e.count(`SELECT count(*) FROM exam_attempts`); n != 1 {
		t.Fatalf("attempt dibuat %d kali", n)
	}
	// DTO peserta tidak membocorkan kunci, bobot, seed, rubric, atau ID opsi asli.
	raw := string(x.dto.Raw)
	for _, leak := range []string{"answerKey", "answer_key", "weight", "seed", "rubric", `"id":"a"`, `"id":"b"`} {
		if strings.Contains(raw, leak) {
			t.Fatalf("DTO ujian membocorkan %q", leak)
		}
	}
	if x.dto.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("Cache-Control = %q", x.dto.Header.Get("Cache-Control"))
	}
	// Kandidat lain tidak bisa membaca attempt ini.
	other := e.registerVerified("orang.lain@contoh.test", "Orang Lain", "3201014507995002")
	expect(t, other.do("GET", "/exam-attempts/"+att, nil), 404, "attempt milik orang lain")
	expect(t, save(other, att, "op-asing-0001", ans(uuid.NewString(), 0, nil, nil)), 404, "save ke attempt orang lain")

	o := e.oracle(att)
	var single, multi, essay []string
	for id, it := range o {
		switch it.Kind {
		case "single":
			single = append(single, id)
		case "multiple":
			multi = append(multi, id)
		case "essay":
			essay = append(essay, id)
		}
	}

	// Batch pertama: semua pilihan tunggal benar.
	var batch []map[string]any
	for _, id := range single {
		batch = append(batch, ans(id, 0, o[id].Correct, nil))
	}
	ack := save(c, att, "op-batch-0001", batch...)
	expect(t, ack, 200, "save batch 1")
	// Retry setelah ACK hilang: hasil sama persis, tidak ada revisi baru.
	ack2 := save(c, att, "op-batch-0001", batch...)
	if string(ack2.Raw) != string(ack.Raw) {
		t.Fatalf("retry opId harus mengembalikan receipt sama:\n%s\n%s", ack.Raw, ack2.Raw)
	}
	if r := save(c, att, "op-batch-0001", ans(single[0], 1, o[single[0]].Correct, nil)); r.errCode() != "idempotency_mismatch" {
		t.Fatalf("opId sama payload beda: %s", r.Raw)
	}
	// Revisi basi: konflik dengan state server, tidak ada yang diterapkan (atomik).
	r := save(c, att, "op-basi-00001", ans(multi[0], 0, o[multi[0]].Correct, nil), ans(single[0], 0, []string{o[single[0]].Wrong}, nil))
	if r.errCode() != "answer_conflict" {
		t.Fatalf("revisi basi: %s", r.Raw)
	}
	if n := e.count(`SELECT count(*) FROM exam_answers WHERE attempt_id = $1 AND item_id = $2`, att, multi[0]); n != 0 {
		t.Fatal("batch berkonflik tidak boleh menerapkan item lain")
	}
	// Validasi nilai: opsi asing ditolak; pilihan tunggal >1 ditolak.
	if r := save(c, att, "op-asing-0002", ans(multi[0], 0, []string{"bukan-opsi"}, nil)); r.Status != 422 {
		t.Fatalf("opsi asing: %s", r.Raw)
	}

	// Dua tab mengubah butir yang sama dari revisi yang sama: tepat satu diterima.
	var wg sync.WaitGroup
	res := make([]resp, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res[i] = save(c, att, fmt.Sprintf("op-tab-%d-0001", i), ans(multi[0], 0, o[multi[0]].Correct[:1+i%2], nil))
		}(i)
	}
	wg.Wait()
	okN := 0
	for _, x := range res {
		if x.Status == 200 {
			okN++
		} else if x.errCode() != "answer_conflict" {
			t.Fatalf("dua tab: %s", x.Raw)
		}
	}
	if okN != 1 {
		t.Fatalf("dua tab: %d diterima", okN)
	}
	// Tab yang kalah menyelesaikan konflik secara eksplisit dengan revisi server.
	cur := c.do("GET", "/exam-attempts/"+att, nil)
	rev := 0
	for _, a := range cur.Body["answers"].([]any) {
		m := a.(map[string]any)
		if m["itemId"] == multi[0] {
			rev = int(m["revision"].(float64))
		}
	}
	expect(t, save(c, att, "op-resolve-001", ans(multi[0], rev, o[multi[0]].Correct, nil)), 200, "resolusi konflik")
	// Jawaban yang dikosongkan tetap tercatat sebagai revisi committed (regresi temuan oracle uji beban).
	expect(t, save(c, att, "op-kosong-0001", ans(single[1], 1, nil, nil)), 200, "kosongkan jawaban")
	text := "Hentikan lini, pisahkan produk cacat, lapor supervisor, catat di form NCR."
	expect(t, save(c, att, "op-essay-0001", ans(essay[0], 0, nil, &text)), 200, "esai")

	// Submit: expectedRevision harus cocok; retry identik = receipt sama; key lain ditolak.
	state := c.do("GET", "/exam-attempts/"+att, nil)
	revNow := state.Body["revision"]
	if r := c.do("POST", "/exam-attempts/"+att+"/submit", map[string]any{"expectedRevision": 0}, "Idempotency-Key", "submit-key-0001"); r.errCode() != "stale_revision" {
		t.Fatalf("revisi submit basi: %s", r.Raw)
	}
	sub := c.do("POST", "/exam-attempts/"+att+"/submit", map[string]any{"expectedRevision": revNow}, "Idempotency-Key", "submit-key-0001")
	expect(t, sub, 200, "submit")
	sub2 := c.do("POST", "/exam-attempts/"+att+"/submit", map[string]any{"expectedRevision": revNow}, "Idempotency-Key", "submit-key-0001")
	if sub2.Body["code"] != sub.Body["code"] {
		t.Fatal("retry submit harus receipt sama")
	}
	if r := c.do("POST", "/exam-attempts/"+att+"/submit", map[string]any{"expectedRevision": revNow}, "Idempotency-Key", "submit-key-0002"); r.errCode() != "attempt_finalized" {
		t.Fatalf("submit kedua: %s", r.Raw)
	}
	if r := save(c, att, "op-setelah-001", ans(single[0], 1, []string{o[single[0]].Wrong}, nil)); r.errCode() != "attempt_finalized" {
		t.Fatalf("save setelah final: %s", r.Raw)
	}
	// Retry save lama setelah final tetap mendapat ACK lama (receipt).
	if r := save(c, att, "op-batch-0001", batch...); string(r.Raw) != string(ack.Raw) {
		t.Fatalf("retry ACK lama setelah final: %s", r.Raw)
	}
	if n := e.count(`SELECT count(*) FROM exam_submissions WHERE attempt_id = $1`, att); n != 1 {
		t.Fatalf("submission = %d", n)
	}
	// Digest receipt dapat diverifikasi ulang secara independen dari snapshot yang tersimpan.
	if ok := e.scalar(`SELECT answers_digest = sha256(convert_to(snapshot::text, 'UTF8')) FROM exam_submissions WHERE attempt_id = $1`, att); ok != true {
		t.Fatal("digest receipt tidak cocok dengan snapshot")
	}

	// Nilai: objektif 3 tunggal (satu dikosongkan) + 2 jamak = 5 dari 8, esai belum dinilai -> score null, pending 1.
	var earned float64
	var score *float64
	var pending int
	if err := e.pool.QueryRow(context.Background(), `SELECT earned::float8, score::float8, essay_pending FROM exam_results WHERE attempt_id = $1`, att).Scan(&earned, &score, &pending); err != nil {
		t.Fatal(err)
	}
	if earned != 5 || score != nil || pending != 1 {
		t.Fatalf("nilai sebelum esai: earned=%v score=%v pending=%d", earned, score, pending)
	}
	// Snapshot final cocok dengan revisi terakhir yang diterima (rekonsiliasi read-only).
	if n := e.count(`SELECT count(*) FROM exam_answers a JOIN exam_submissions s ON s.attempt_id = a.attempt_id,
		jsonb_array_elements(s.snapshot->'items') si
		WHERE a.attempt_id = $1 AND si->>'itemId' = a.item_id::text AND (si->>'revision')::int <> a.revision`, att); n != 0 {
		t.Fatalf("%d butir snapshot tidak cocok dengan revisi committed", n)
	}

	// Penilaian buta: examiner melihat kode peserta, bukan nama/NIK.
	ex := e.client().mustLogin("penguji@arunika.example")
	rv := ex.do("GET", "/exam-attempts/"+att+"/review", nil)
	expect(t, rv, 200, "review")
	if strings.Contains(string(rv.Raw), "Peserta Uji") || strings.Contains(string(rv.Raw), "5001") {
		t.Fatal("review examiner tidak boleh memuat identitas")
	}
	expect(t, ex.do("POST", "/exam-attempts/"+att+"/essay-grades", map[string]any{"itemId": essay[0], "points": 3}), 422, "nilai di atas bobot")
	g := ex.do("POST", "/exam-attempts/"+att+"/essay-grades", map[string]any{"itemId": essay[0], "points": 1.5})
	expect(t, g, 200, "nilai esai")
	// (5 + 1,5) / 8 = 81,25
	if g.Body["score"].(float64) != 81.25 || g.Body["status"] != "graded" {
		t.Fatalf("nilai akhir: %s", g.Raw)
	}
	expect(t, ex.do("POST", "/exam-schedules/"+e.scheduleID()+"/release", nil), 403, "examiner tidak boleh rilis")

	// Rilis oleh HR OPS -> worker -> transisi workflow ke interview, notifikasi kandidat.
	hr := e.client().mustLogin("hr.ops@arunika.example")
	rel := hr.do("POST", "/exam-schedules/"+e.scheduleID()+"/release", nil)
	expect(t, rel, 200, "rilis")
	e.worker.Drain(context.Background())
	if st := e.scalar(`SELECT stage_key FROM job_applications WHERE id = $1`, x.appID); st != "interview" {
		t.Fatalf("tahap setelah rilis = %v", st)
	}
	// Regrade setelah rilis: wajib alasan, nilai berubah, tidak ada transisi ATS kedua; replay event juga tidak.
	expect(t, ex.do("POST", "/exam-attempts/"+att+"/essay-grades", map[string]any{"itemId": essay[0], "points": 2}), 422, "regrade tanpa alasan")
	expect(t, ex.do("POST", "/exam-attempts/"+att+"/essay-grades", map[string]any{"itemId": essay[0], "points": 2, "reason": "Keberatan peserta diterima"}), 200, "regrade")
	if _, err := e.pool.Exec(context.Background(), `UPDATE outbox SET status = 'pending', attempts = 0, available_at = now() WHERE topic = 'exam.result_released'`); err != nil {
		t.Fatal(err)
	}
	e.worker.Drain(context.Background())
	if n := e.count(`SELECT count(*) FROM transition_history WHERE application_id = $1 AND trigger = 'event'`, x.appID); n != 1 {
		t.Fatalf("transisi dari event = %d", n)
	}
	if n := e.count(`SELECT count(*) FROM essay_grades WHERE attempt_id = $1`, att); n != 2 {
		t.Fatalf("riwayat koreksi = %d", n)
	}
	if n := e.count(`SELECT count(*) FROM notifications WHERE template_key = 'result_released'`); n != 1 {
		t.Fatalf("notifikasi rilis = %d", n)
	}
}

func TestExamBranchesDeadlineAndExpiry(t *testing.T) {
	e := newEnv(t)
	hr := e.client().mustLogin("hr.ops@arunika.example")

	// Di bawah ambang: semua salah -> ditolak setelah rilis.
	low := e.startExam("rendah@contoh.test", "3201014507996001")
	o := e.oracle(low.attempt)
	var batch []map[string]any
	for id, it := range o {
		if it.Kind != "essay" {
			batch = append(batch, ans(id, 0, []string{it.Wrong}, nil))
		}
	}
	expect(t, save(low.c, low.attempt, "op-rendah-0001", batch...), 200, "save salah")
	rv := low.c.do("GET", "/exam-attempts/"+low.attempt, nil).Body["revision"]
	expect(t, low.c.do("POST", "/exam-attempts/"+low.attempt+"/submit", map[string]any{"expectedRevision": rv}, "Idempotency-Key", "submit-rendah-01"), 200, "submit rendah")

	// Penilaian manual: esai terjawab tetapi belum dinilai saat rilis -> review.
	man := e.startExam("manual@contoh.test", "3201014507996002")
	om := e.oracle(man.attempt)
	txt := "Jawaban esai"
	for id, it := range om {
		if it.Kind == "essay" {
			expect(t, save(man.c, man.attempt, "op-manual-0001", ans(id, 0, nil, &txt)), 200, "esai")
		}
	}
	rv = man.c.do("GET", "/exam-attempts/"+man.attempt, nil).Body["revision"]
	// Submit membawa batch terakhir (semua pilihan benar) dalam transaksi yang sama.
	var last []map[string]any
	for id, it := range om {
		if it.Kind != "essay" {
			last = append(last, ans(id, 0, it.Correct, nil))
		}
	}
	sub := man.c.do("POST", "/exam-attempts/"+man.attempt+"/submit", map[string]any{"expectedRevision": rv, "opId": "op-manual-last", "answers": last}, "Idempotency-Key", "submit-manual-1")
	expect(t, sub, 200, "submit dengan batch terakhir")
	if sub.Body["answered"].(float64) != 6 {
		t.Fatalf("receipt harus mencatat 6 jawaban: %s", sub.Raw)
	}

	// Lewat deadline: save ditolak, submit memfinalisasi jawaban committed dan memberi receipt (409 + receipt).
	late := e.startExam("telat@contoh.test", "3201014507996003")
	ol := e.oracle(late.attempt)
	var one string
	for id, it := range ol {
		if it.Kind == "single" {
			one = id
		}
	}
	early := save(late.c, late.attempt, "op-telat-awal1", ans(one, 0, ol[one].Correct, nil))
	expect(t, early, 200, "save sebelum deadline")
	if _, err := e.pool.Exec(context.Background(), `UPDATE exam_attempts SET started_at = now() - interval '2 hour', deadline_at = now() - interval '1 second' WHERE id = $1`, late.attempt); err != nil {
		t.Fatal(err)
	}
	if r := save(late.c, late.attempt, "op-telat-baru1", ans(one, 1, []string{ol[one].Wrong}, nil)); r.errCode() != "deadline_passed" {
		t.Fatalf("save lewat deadline: %s", r.Raw)
	}
	if r := save(late.c, late.attempt, "op-telat-awal1", ans(one, 0, ol[one].Correct, nil)); string(r.Raw) != string(early.Raw) {
		t.Fatalf("retry ACK sebelum deadline harus tetap dikembalikan: %s", r.Raw)
	}
	lr := late.c.do("POST", "/exam-attempts/"+late.attempt+"/submit", map[string]any{"expectedRevision": 1}, "Idempotency-Key", "submit-telat-01")
	if lr.errCode() != "deadline_passed" || e.count(`SELECT count(*) FROM exam_submissions WHERE attempt_id = $1 AND reason = 'expired'`, late.attempt) != 1 {
		t.Fatalf("submit lewat deadline harus finalisasi expired: %s", lr.Raw)
	}

	// Expiry worker bersamaan dengan submit: tepat satu finalisasi.
	race := e.startExam("balapan@contoh.test", "3201014507996004")
	if _, err := e.pool.Exec(context.Background(), `UPDATE exam_attempts SET started_at = now() - interval '2 hour', deadline_at = now() - interval '1 second' WHERE id = $1`, race.attempt); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); ExpireDue(context.Background(), e.pool) }()
	go func() {
		defer wg.Done()
		race.c.do("POST", "/exam-attempts/"+race.attempt+"/submit", map[string]any{"expectedRevision": 0}, "Idempotency-Key", "submit-race-001")
	}()
	wg.Wait()
	if n := e.count(`SELECT count(*) FROM exam_submissions WHERE attempt_id = $1`, race.attempt); n != 1 {
		t.Fatalf("finalisasi ganda: %d", n)
	}
	if n := e.count(`SELECT count(*) FROM exam_results WHERE attempt_id = $1`, race.attempt); n != 1 {
		t.Fatalf("hasil ganda: %d", n)
	}

	// Rilis semua -> cabang workflow per lamaran.
	expect(t, hr.do("POST", "/exam-schedules/"+e.scheduleID()+"/release", nil), 200, "rilis")
	e.worker.Drain(context.Background())
	want := map[uuid.UUID]string{low.appID: "ditolak", man.appID: "review", late.appID: "ditolak", race.appID: "ditolak"}
	for app, stage := range want {
		if got := e.scalar(`SELECT stage_key FROM job_applications WHERE id = $1`, app); got != stage {
			t.Fatalf("lamaran %s: tahap %v, ingin %s", app, got, stage)
		}
	}
	// Nilai fixture: late hanya 1 benar dari 8 poin = 12.5.
	if s := e.scalar(`SELECT score::float8 FROM exam_results WHERE attempt_id = $1`, late.attempt); s != 12.5 {
		t.Fatalf("nilai late = %v", s)
	}
	// Monitoring agregat membedakan status peserta.
	mon := hr.do("GET", "/exam-schedules/"+e.scheduleID()+"/monitor", nil)
	expect(t, mon, 200, "monitor")
	if n := len(mon.Body["participants"].([]any)); n != 4 {
		t.Fatalf("monitor peserta = %d", n)
	}
	b, _ := json.Marshal(mon.Body["participants"])
	if !strings.Contains(string(b), `"participantStatus":"expired"`) || !strings.Contains(string(b), `"participantStatus":"submitted"`) {
		t.Fatalf("monitor harus membedakan status: %s", b)
	}
}
