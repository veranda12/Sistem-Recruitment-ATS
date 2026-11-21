package app

import (
	"strings"
	"testing"
	"time"
)

func TestBankImportAndPackageVersioning(t *testing.T) {
	e := newEnv(t)
	ex := e.client().mustLogin("penguji@arunika.example")
	// Peserta aktif pada paket versi 1.
	run := e.startExam("versi.lama@contoh.test", "3201014507997301")

	b := ex.do("POST", "/exam-banks", map[string]any{"name": "Bank Logika", "category": "Logika"})
	expect(t, b, 201, "buat bank")
	bank := b.Body["id"].(string)
	bad := "jenis;pertanyaan;opsi;kunci;bobot;rubrik\ntunggal;2+2?;3|4;B;1;\ntunggal;Kunci salah;A|B;Z;1;"
	pv := ex.do("POST", "/exam-banks/"+bank+"/import/preview", map[string]any{"content": bad})
	expect(t, pv, 200, "preview")
	if pv.Body["valid"] != false || pv.Body["invalidCount"].(float64) != 1 {
		t.Fatalf("preview harus menandai 1 baris salah: %s", pv.Raw)
	}
	if r := ex.do("POST", "/exam-banks/"+bank+"/import", map[string]any{"content": bad, "digest": pv.Body["digest"]}); r.errCode() != "import_invalid" {
		t.Fatalf("impor dengan baris salah: %s", r.Raw)
	}
	good := "jenis;pertanyaan;opsi;kunci;bobot;rubrik\ntunggal;2+2?;3|4;B;1;\njamak;Bilangan prima;2|4|5;A,C;2;\nesai;Uraikan;;;2;Rubrik"
	pv = ex.do("POST", "/exam-banks/"+bank+"/import/preview", map[string]any{"content": good})
	if r := ex.do("POST", "/exam-banks/"+bank+"/import", map[string]any{"content": good + "\n", "digest": "beda"}); r.errCode() != "preview_mismatch" {
		t.Fatalf("digest berbeda: %s", r.Raw)
	}
	expect(t, ex.do("POST", "/exam-banks/"+bank+"/import", map[string]any{"content": good, "digest": pv.Body["digest"]}), 201, "impor")
	qs := ex.do("GET", "/exam-banks/"+bank+"/questions", nil)
	items := qs.Body["questions"].([]any)
	if len(items) != 3 {
		t.Fatalf("soal terimpor = %d", len(items))
	}
	// Ubah soal -> versi 2; versi 1 tetap utuh.
	qid := items[0].(map[string]any)["questionId"].(string)
	expect(t, ex.do("POST", "/questions/"+qid+"/versions", map[string]any{"row": "tunggal;Pertanyaan direvisi;Ya|Tidak;A;1;"}), 201, "versi soal")
	if n := e.count(`SELECT count(*) FROM question_versions WHERE question_id = $1`, qid); n != 2 {
		t.Fatalf("versi soal = %d", n)
	}
	// Terbitkan paket tes-dasar versi 2 dari soal bank baru.
	var vids []string
	for _, it := range ex.do("GET", "/exam-banks/"+bank+"/questions", nil).Body["questions"].([]any) {
		vids = append(vids, it.(map[string]any)["versionId"].(string))
	}
	pub := ex.do("POST", "/exam-packages/tes-dasar/versions", map[string]any{"questionVersionIds": vids, "durationMinutes": 20, "passScore": 70, "shuffleItems": true})
	expect(t, pub, 201, "terbitkan paket v2")
	if pub.Body["versionNo"].(float64) != 2 {
		t.Fatalf("versi paket = %v", pub.Body["versionNo"])
	}
	// Ujian yang sedang berjalan tetap memakai versi 1 (6 soal, isi soal lama).
	cur := run.c.do("GET", "/exam-attempts/"+run.attempt, nil)
	if n := len(cur.Body["items"].([]any)); n != 6 {
		t.Fatalf("attempt berjalan berubah: %d soal", n)
	}
	if strings.Contains(string(cur.Raw), "Pertanyaan direvisi") {
		t.Fatal("revisi bank tidak boleh masuk ke ujian berjalan")
	}
	// Jadwal baru memakai versi terbaru.
	hr := e.client().mustLogin("hr.ops@arunika.example")
	s := hr.do("POST", "/exam-schedules", map[string]any{"packageKey": "tes-dasar", "name": "Gelombang Desember",
		"opensAt": time.Now().Add(-time.Minute).Format(time.RFC3339), "closesAt": time.Now().Add(48 * time.Hour).Format(time.RFC3339)})
	expect(t, s, 201, "jadwal baru")
	if v := e.scalar(`SELECT v.version_no FROM exam_schedules s JOIN exam_package_versions v ON v.id = s.package_version_id WHERE s.id = $1`, s.Body["id"]); v != int32(2) {
		t.Fatalf("jadwal baru memakai versi %v", v)
	}
	// Kandidat dan recruiter tidak bisa melihat bank (berisi kunci).
	expect(t, run.c.do("GET", "/exam-banks/"+bank+"/questions", nil), 403, "kandidat ke bank")
	expect(t, e.client().mustLogin("rekruter.ops@arunika.example").do("GET", "/exam-banks", nil), 403, "recruiter ke bank")
}
