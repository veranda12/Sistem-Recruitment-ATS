package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ats/db/sqlc"
	"ats/internal/cvparse"
	"ats/internal/cvreview"

	"github.com/google/uuid"
)

// cvEnv menyiapkan fixture dan tool sungguhan; test dilewati bila keduanya tidak tersedia.
func cvEnv(t *testing.T) (*env, string, *cvreview.Processor) {
	t.Helper()
	dir := os.Getenv("CV_FIXTURES")
	if dir == "" {
		dir = filepath.Join("..", "..", "..", "tests", "cv-fixtures", "out")
	}
	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err != nil {
		t.Skip("fixture CV belum dibuat (go run ./cmd/cvfixtures)")
	}
	tools := cvparse.ToolsFromEnv()
	if tools.PDFToText == "" || tools.Tesseract == "" {
		t.Skip("Poppler/Tesseract tidak tersedia")
	}
	e := newEnv(t)
	q := sqlc.New(e.pool)
	proc := &cvreview.Processor{Pool: e.pool, Q: q, ObjectDir: e.cfg.ObjectDir, WorkDir: t.TempDir(), Tools: tools, Limits: cvparse.DefaultLimits(), Owner: "test-1"}
	return e, dir, proc
}

func fixture(t *testing.T, dir, split, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, split, name))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return b
}

// parseDoc: unggah, minta parse, pindai (worker outbox), proses (worker CV), kembalikan run.
func (e *env) parseDoc(c *client, proc *cvreview.Processor, file string, data []byte) map[string]any {
	e.t.Helper()
	up := c.upload("cv", file, data)
	expect(e.t, up, 201, "unggah "+file)
	pr := c.do("POST", "/me/documents/"+idOf(up, "id")+"/parse", nil)
	expect(e.t, pr, 202, "parse "+file)
	if pr.Body["status"] != "waiting_scan" {
		e.t.Fatalf("sebelum pindai status harus waiting_scan: %s", pr.Raw)
	}
	e.worker.Drain(context.Background())
	runID := uuid.MustParse(idOf(pr, "id"))
	if err := proc.Run(context.Background(), runID); err != nil {
		e.t.Fatalf("proses %s: %v", file, err)
	}
	r := c.do("GET", "/me/cv-parses/"+runID.String(), nil)
	expect(e.t, r, 200, "status run")
	return r.Body
}

type sug struct {
	ID, Path, Field, Raw, Status string
	Evidence                     []string
	Normalized                   json.RawMessage
}

func suggestions(run map[string]any) map[string]sug {
	out := map[string]sug{}
	b, _ := json.Marshal(run["suggestions"])
	var list []struct {
		ID          string          `json:"id"`
		Path        string          `json:"path"`
		Field       string          `json:"field"`
		Raw         string          `json:"raw"`
		Status      string          `json:"status"`
		EvidenceIDs []string        `json:"evidenceIds"`
		Normalized  json.RawMessage `json:"normalized"`
	}
	_ = json.Unmarshal(b, &list)
	for _, s := range list {
		out[s.Path] = sug{s.ID, s.Path, s.Field, s.Raw, s.Status, s.EvidenceIDs, s.Normalized}
	}
	return out
}

func evidenceSnippets(run map[string]any) map[string]string {
	out := map[string]string{}
	for _, e := range run["evidence"].([]any) {
		m := e.(map[string]any)
		out[m["id"].(string)] = m["snippet"].(string)
	}
	return out
}

func TestCVParseReviewApplyFlow(t *testing.T) {
	e, dir, proc := cvEnv(t)
	c := e.registerVerified("sinta.akun@contoh.test", "Sinta Akun", "3273014507990001")
	before := c.do("GET", "/me/profile", nil)
	expect(t, before, 200, "profil awal")

	run := e.parseDoc(c, proc, "Sinta CV.pdf", fixture(t, dir, "dev", "sinta.pdf-2col.pdf"))
	if run["status"] != "ready_for_review" {
		t.Fatalf("status: %v", run["status"])
	}
	s := suggestions(run)
	ev := evidenceSnippets(run)
	// CV dua kolom: jabatan, perusahaan, periode, uraian dari entri yang sama.
	if s["experience[1].title"].Raw != "Junior Developer" || s["experience[1].company"].Raw != "CV Kreasi Digital Mandiri" {
		t.Fatalf("pasangan entri 2 salah: %+v %+v", s["experience[1].title"], s["experience[1].company"])
	}
	// "Lihat sumber" pengalaman kedua menunjuk potongan CV yang mendukungnya.
	for _, f := range []string{"title", "period", "description"} {
		x := s["experience[1]."+f]
		if len(x.Evidence) == 0 {
			t.Fatalf("experience[1].%s tanpa bukti", f)
		}
		snip := ev[x.Evidence[0]]
		if f == "title" && !strings.Contains(snip, "Junior Developer") || f == "period" && !strings.Contains(snip, "Jan 2020") {
			t.Fatalf("bukti experience[1].%s = %q", f, snip)
		}
	}
	if st, _ := c.get("/api/v1/me/cv-parses/" + run["id"].(string) + "/pages/1/image"); st != 200 {
		t.Fatalf("preview halaman: %d", st)
	}
	// Nol perubahan profil sebelum persetujuan.
	if n := e.count(`SELECT count(*) FROM experiences e JOIN users u ON u.id = e.user_id WHERE u.email = 'sinta.akun@contoh.test'`); n != 0 {
		t.Fatalf("profil berubah sebelum apply: %d pengalaman", n)
	}

	// Review: terima semua, koreksi satu digit telepon, tolak email CV.
	var decisions []map[string]any
	for path, x := range s {
		switch {
		case path == "contact.email[0]":
			decisions = append(decisions, map[string]any{"suggestionId": x.ID, "action": "reject"})
		case path == "contact.phone[0]":
			decisions = append(decisions, map[string]any{"suggestionId": x.ID, "action": "edit", "value": "0812-3456-7809"})
		case x.Raw != "":
			decisions = append(decisions, map[string]any{"suggestionId": x.ID, "action": "accept"})
		}
	}
	rv := c.do("PATCH", "/me/cv-parses/"+run["id"].(string)+"/review", map[string]any{"reviewVersion": run["reviewVersion"], "decisions": decisions})
	expect(t, rv, 200, "review")
	// Review basi (versi lama) ditolak.
	if r := c.do("PATCH", "/me/cv-parses/"+run["id"].(string)+"/review", map[string]any{"reviewVersion": run["reviewVersion"], "decisions": decisions[:1]}); r.errCode() != "stale_review" {
		t.Fatalf("review basi: %s", r.Raw)
	}
	pv := before.Body["version"]
	body := map[string]any{"reviewVersion": rv.Body["reviewVersion"], "profileVersion": pv}
	ap := c.do("POST", "/me/cv-parses/"+run["id"].(string)+"/apply", body)
	expect(t, ap, 200, "apply")
	again := c.do("POST", "/me/cv-parses/"+run["id"].(string)+"/apply", body)
	expect(t, again, 200, "apply ulang")
	if again.Body["alreadyApplied"] != true {
		t.Fatalf("apply ulang harus mengembalikan hasil yang sama: %s", again.Raw)
	}
	prof := c.do("GET", "/me/profile", nil)
	b, _ := json.Marshal(prof.Body["experiences"])
	var exps []map[string]any
	_ = json.Unmarshal(b, &exps)
	if len(exps) != 2 {
		t.Fatalf("retry apply menggandakan entri: %d", len(exps))
	}
	found := false
	for _, x := range exps {
		if x["company"] == "CV Kreasi Digital Mandiri" && x["title"] == "Junior Developer" && x["startDate"] == "2020-01-01" && x["startPrecision"] == "month" {
			found = true
		}
		if x["company"] == "PT Nusantara Data Teknologi" && (x["isCurrent"] != true || x["endDate"] != nil) {
			t.Fatalf("entri berjalan: %v", x)
		}
	}
	if !found {
		t.Fatalf("pengalaman tidak tersimpan benar: %s", b)
	}
	if prof.Body["phone"] != "+6281234567809" {
		t.Fatalf("koreksi telepon: %v", prof.Body["phone"])
	}
	// Email CV ditolak: email akun dan email kontak tetap.
	if prof.Body["email"] != "sinta.akun@contoh.test" || prof.Body["contactEmail"] != "" {
		t.Fatalf("email akun harus tetap: %v / %v", prof.Body["email"], prof.Body["contactEmail"])
	}
	if sk, _ := json.Marshal(prof.Body["skills"]); !strings.Contains(string(sk), "PostgreSQL") {
		t.Fatalf("skill: %s", sk)
	}

	// Kandidat lain tidak dapat membuka run ini; dokumen orang lain tidak dapat di-parse.
	other := e.registerVerified("orang.lain@contoh.test", "Orang Lain", "3273014507990002")
	if r := other.do("GET", "/me/cv-parses/"+run["id"].(string), nil); r.Status != 404 {
		t.Fatalf("run milik orang lain: %d", r.Status)
	}
	if r := other.do("POST", "/me/documents/"+run["documentId"].(string)+"/parse", nil); r.Status != 404 {
		t.Fatalf("parse dokumen orang lain: %d", r.Status)
	}
	// Staf tanpa scope lamaran kandidat: 404.
	staff := e.client().mustLogin("rekruter.ops@arunika.example")
	if r := staff.do("GET", "/cv-parses/"+run["id"].(string), nil); r.Status != 404 {
		t.Fatalf("staf tanpa scope: %d", r.Status)
	}
	// Permintaan parse ulang dokumen yang sama: run yang sama (idempoten).
	if r := c.do("POST", "/me/documents/"+run["documentId"].(string)+"/parse", nil); r.Body["id"] != run["id"] {
		t.Fatalf("parse ulang harus idempoten: %s", r.Raw)
	}
}

func TestCVParseProfileChangedAndPartial(t *testing.T) {
	e, dir, proc := cvEnv(t)
	c := e.registerVerified("sinta.dua@contoh.test", "Sinta Dua", "3273014507990003")
	prof := c.do("GET", "/me/profile", nil)
	run := e.parseDoc(c, proc, "cv.docx", fixture(t, dir, "dev", "sinta.docx.docx"))
	if run["status"] != "ready_for_review" {
		t.Fatalf("docx: %v", run["status"])
	}
	s := suggestions(run)
	var decisions []map[string]any
	for _, x := range s {
		if x.Raw != "" {
			decisions = append(decisions, map[string]any{"suggestionId": x.ID, "action": "accept"})
		}
	}
	rv := c.do("PATCH", "/me/cv-parses/"+run["id"].(string)+"/review", map[string]any{"reviewVersion": run["reviewVersion"], "decisions": decisions})
	expect(t, rv, 200, "review")
	// Profil diubah dari tab lain.
	upd := completeProfile("Sinta Dua", "Bandung", int(prof.Body["version"].(float64)))
	expect(t, c.do("PUT", "/me/profile", upd), 200, "ubah profil tab lain")
	ap := c.do("POST", "/me/cv-parses/"+run["id"].(string)+"/apply", map[string]any{"reviewVersion": rv.Body["reviewVersion"], "profileVersion": prof.Body["version"]})
	if ap.errCode() != "profile_changed" {
		t.Fatalf("profil berubah harus 409: %s", ap.Raw)
	}
	// Review tidak hilang.
	cur := c.do("GET", "/me/cv-parses/"+run["id"].(string), nil)
	for _, x := range suggestions(cur.Body) {
		if x.Raw != "" && x.Status != "accepted" {
			t.Fatalf("review hilang setelah konflik: %+v", x)
		}
	}
	prof2 := c.do("GET", "/me/profile", nil)
	expect(t, c.do("POST", "/me/cv-parses/"+run["id"].(string)+"/apply", map[string]any{"reviewVersion": rv.Body["reviewVersion"], "profileVersion": prof2.Body["version"]}), 200, "apply ulang setelah resolusi")

	// Halaman 2 gagal: status partial, pendidikan halaman 1 tetap tersedia, alasan halaman terlihat.
	part := e.parseDoc(c, proc, "cv-dua-halaman.pdf", fixture(t, dir, "dev", "sinta.partial-page2.pdf"))
	if part["status"] != "partial" {
		t.Fatalf("harus partial: %v", part["status"])
	}
	ps := suggestions(part)
	if ps["education[0].institution"].Raw != "Universitas Nusa Cendekia" {
		t.Fatalf("pendidikan halaman 1: %+v", ps["education[0].institution"])
	}
	pages, _ := json.Marshal(part["pages"])
	if !strings.Contains(string(pages), `"status":"failed"`) || !strings.Contains(string(pages), "no_readable_text") {
		t.Fatalf("kegagalan halaman 2 tidak terlihat: %s", pages)
	}
}

func TestCVParseFormatsAndFailures(t *testing.T) {
	e, dir, proc := cvEnv(t)
	c := e.registerVerified("format@contoh.test", "Uji Format", "3273014507990004")
	// Foto CV yang sama: alur review sama.
	ph := e.parseDoc(c, proc, "foto.jpg", fixture(t, dir, "dev", "sinta.photo-skew.jpg"))
	if ph["status"] != "ready_for_review" || suggestions(ph)["experience[1].company"].Raw != "CV Kreasi Digital Mandiri" {
		t.Fatalf("foto: %v %+v", ph["status"], suggestions(ph)["experience[1].company"])
	}
	// Foto terlalu buram: gagal dengan petunjuk foto ulang, bukan profil kosong sukses.
	bl := e.parseDoc(c, proc, "buram.jpg", fixture(t, dir, "dev", "sinta.photo-blurry.jpg"))
	if bl["status"] != "failed" || !strings.Contains(fmt.Sprint(bl["failure"]), "image_too_blurry") || len(suggestions(bl)) != 0 {
		t.Fatalf("buram: %v %v", bl["status"], bl["failure"])
	}
	// PDF terkunci: alasan stabil.
	lk := e.parseDoc(c, proc, "terkunci.pdf", fixture(t, dir, "dev", "broken.locked.pdf"))
	if lk["status"] != "failed" || !strings.Contains(fmt.Sprint(lk["failure"]), "encrypted_file") {
		t.Fatalf("terkunci: %v", lk["failure"])
	}
	if cvparse.ToolsFromEnv().Soffice != "" {
		// Word lawas asli (CFB + FIB), bukan DOCX berganti ekstensi.
		dc := e.parseDoc(c, proc, "cv-lama.doc", fixture(t, dir, "dev", "budi.doc.doc"))
		ds := suggestions(dc)
		if dc["status"] != "ready_for_review" || ds["experience[1].title"].Raw != "Staf Gudang" || ds["experience[1].company"].Raw != "PT Sinar Logistik Abadi" {
			t.Fatalf("doc: %v %+v %+v", dc["status"], ds["experience[1].title"], ds["experience[1].company"])
		}
	}
	// DOCX yang diganti ekstensi menjadi .doc ditolak saat unggah (signature bukan CFB).
	if r := c.upload("cv", "palsu.doc", fixture(t, dir, "dev", "budi.docx.docx")); r.Status != 422 {
		t.Fatalf("DOCX berekstensi .doc harus ditolak: %d", r.Status)
	}
}

func TestCVParseFencingAndDeletion(t *testing.T) {
	e, dir, proc := cvEnv(t)
	ctx := context.Background()
	q := sqlc.New(e.pool)
	c := e.registerVerified("fence@contoh.test", "Uji Fence", "3273014507990005")
	up := c.upload("cv", "cv.pdf", fixture(t, dir, "dev", "budi.pdf-1col.pdf"))
	pr := c.do("POST", "/me/documents/"+idOf(up, "id")+"/parse", nil)
	e.worker.Drain(ctx)
	runID := uuid.MustParse(idOf(pr, "id"))
	w1 := "lama"
	old, err := q.ClaimCVParseRun(ctx, sqlc.ClaimCVParseRunParams{Owner: &w1, LeaseSeconds: 30, ID: runID})
	if err != nil {
		t.Fatal(err)
	}
	// Worker lama mati (lost ACK); lease kedaluwarsa; worker baru mengambil alih.
	if _, err := e.pool.Exec(ctx, `UPDATE cv_parse_runs SET lease_until = now() - interval '1 second' WHERE id = $1`, runID); err != nil {
		t.Fatal(err)
	}
	if err := proc.Run(ctx, runID); err != nil {
		t.Fatal(err)
	}
	// Hasil worker lama ditolak (fence sudah naik).
	n, err := q.FinishCVParseRun(ctx, sqlc.FinishCVParseRunParams{Status: "failed", Manifest: []byte(`{}`), FailureReason: ptrS("x"), ID: runID, Fence: old.Fence})
	if err != nil || n != 0 {
		t.Fatalf("worker lama tidak boleh menimpa hasil: n=%d err=%v", n, err)
	}
	if st := e.scalar(`SELECT status FROM cv_parse_runs WHERE id = $1`, runID); st != "ready_for_review" {
		t.Fatalf("status setelah ambil alih: %v", st)
	}
	// Dokumen dihapus: run dibatalkan, saran/teks dibersihkan, preview tidak dapat diakses.
	expect(t, c.do("DELETE", "/me/documents/"+idOf(up, "id"), nil), 204, "hapus dokumen")
	if st := e.scalar(`SELECT status FROM cv_parse_runs WHERE id = $1`, runID); st != "cancelled" {
		t.Fatalf("run harus dibatalkan: %v", st)
	}
	if n := e.count(`SELECT count(*) FROM cv_field_suggestions WHERE run_id = $1`, runID); n != 0 {
		t.Fatalf("saran harus dibersihkan: %d", n)
	}
	if st, _ := c.get("/api/v1/me/cv-parses/" + runID.String() + "/pages/1/image"); st != 404 {
		t.Fatalf("preview dokumen terhapus: %d", st)
	}
	// Job terlambat tidak dapat menghidupkan run dokumen terhapus.
	cur, _ := q.GetCVParseRun(ctx, runID)
	if _, err := e.pool.Exec(ctx, `UPDATE cv_parse_runs SET status = 'processing' WHERE id = $1`, runID); err != nil {
		t.Fatal(err)
	}
	if n, _ := q.FinishCVParseRun(ctx, sqlc.FinishCVParseRunParams{Status: "ready_for_review", Manifest: []byte(`{}`), ID: runID, Fence: cur.Fence}); n != 0 {
		t.Fatal("hasil terlambat untuk dokumen terhapus tidak boleh tersimpan")
	}
	// Konverter/worker mati berulang: setelah percobaan habis -> dead-letter, dapat di-replay oleh admin.
	up2 := c.upload("cv", "cv2.pdf", fixture(t, dir, "dev", "rahmat.pdf-1col.pdf"))
	pr2 := c.do("POST", "/me/documents/"+idOf(up2, "id")+"/parse", nil)
	e.worker.Drain(ctx)
	run2 := uuid.MustParse(idOf(pr2, "id"))
	for i := 0; i < 3; i++ {
		w := fmt.Sprintf("mati-%d", i)
		if _, err := q.ClaimCVParseRun(ctx, sqlc.ClaimCVParseRunParams{Owner: &w, LeaseSeconds: 30, ID: run2}); err != nil {
			t.Fatalf("klaim %d: %v", i, err)
		}
		if _, err := e.pool.Exec(ctx, `UPDATE cv_parse_runs SET lease_until = now() - interval '1 second' WHERE id = $1`, run2); err != nil {
			t.Fatal(err)
		}
	}
	if ids, _ := q.ExpireExhaustedCVRuns(ctx); len(ids) != 1 {
		t.Fatalf("dead-letter: %v", ids)
	}
	admin := e.client().mustLogin("admin@arunika.example")
	expect(t, admin.do("POST", "/admin/cv-parses/"+run2.String()+"/replay", nil), 202, "replay")
	if err := proc.Run(ctx, run2); err != nil {
		t.Fatal(err)
	}
	if st := e.scalar(`SELECT status FROM cv_parse_runs WHERE id = $1`, run2); st != "ready_for_review" {
		t.Fatalf("setelah replay: %v", st)
	}
	_ = time.Second
}

func ptrS(s string) *string { return &s }
