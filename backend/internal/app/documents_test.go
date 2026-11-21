package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
)

func (c *client) upload(kind, name string, data []byte) resp {
	c.e.t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("kind", kind)
	fw, _ := w.CreateFormFile("file", name)
	_, _ = fw.Write(data)
	_ = w.Close()
	req, _ := http.NewRequest("POST", c.e.srv.URL+"/api/v1/me/documents", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Origin", testOrigin)
	req.Header.Set("X-CSRF-Token", c.csrf)
	res, err := c.http.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := resp{Status: res.StatusCode, Raw: raw}
	_ = jsonUnmarshal(raw, &out.Body)
	return out
}

func (c *client) get(url string) (int, []byte) {
	req, _ := http.NewRequest("GET", c.e.srv.URL+url, nil)
	res, err := c.http.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, b
}

func TestDocumentUploadScanAndScopedDownload(t *testing.T) {
	e := newEnv(t)
	c := e.registerVerified("dokumen@contoh.test", "Kandidat Dokumen", "3201014507997101")
	expect(t, c.do("POST", "/jobs/operator-produksi-karawang/apply", nil), 201, "apply")
	pdf := append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte("isi cv "), 200)...)

	// Signature tidak cocok dengan ekstensi, tipe terlarang, dan ukuran berlebih ditolak.
	expect(t, c.upload("cv", "cv.pdf", []byte("MZ\x90\x00 bukan pdf")), 422, "exe berekstensi pdf")
	expect(t, c.upload("cv", "skrip.html", []byte("<html><script>")), 422, "html")
	expect(t, c.upload("bebas", "cv.pdf", pdf), 422, "jenis tidak dikenal")
	if r := c.upload("cv", "besar.pdf", append([]byte("%PDF-"), make([]byte, 6<<20)...)); r.Status != 413 {
		t.Fatalf("berkas 6MB: %d", r.Status)
	}

	up := c.upload("cv", `..\..\cv rina.pdf`, pdf)
	expect(t, up, 201, "unggah pdf")
	docID := up.Body["id"].(string)
	if up.Body["fileName"] != "cv rina.pdf" || up.Body["scanStatus"] != "pending" {
		t.Fatalf("nama dibersihkan dan status pending: %s", up.Raw)
	}
	var key string
	if err := e.pool.QueryRow(context.Background(), `SELECT object_key FROM documents WHERE id = $1`, docID).Scan(&key); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key, "quarantine/") || strings.Contains(key, "rina") {
		t.Fatalf("objek harus di karantina dengan nama acak: %s", key)
	}
	// Sebelum pindai: tidak dapat diunduh.
	if r := c.do("POST", "/documents/"+docID+"/link", nil); r.errCode() != "scan_pending" {
		t.Fatalf("unduh sebelum pindai: %s", r.Raw)
	}
	e.worker.Drain(context.Background())
	link := c.do("POST", "/documents/"+docID+"/link", nil)
	expect(t, link, 200, "tautan pemilik")
	st, body := c.get(link.Body["url"].(string))
	if st != 200 || !bytes.Equal(body, pdf) {
		t.Fatalf("unduhan pemilik: %d, %d byte", st, len(body))
	}

	// Staf dalam scope lamaran kandidat dapat membuka; staf unit lain tidak (404); tautan tidak dapat dipindah.
	rec := e.client().mustLogin("rekruter.ops@arunika.example")
	det := rec.do("GET", "/applications", nil).Body["items"].([]any)[0].(map[string]any)["id"].(string)
	if docs := rec.do("GET", "/applications/"+det, nil).Body["documents"].([]any); len(docs) != 1 {
		t.Fatalf("detail lamaran harus memuat dokumen: %v", docs)
	}
	rl := rec.do("POST", "/documents/"+docID+"/link", nil)
	expect(t, rl, 200, "tautan recruiter OPS")
	if st, _ := rec.get(rl.Body["url"].(string)); st != 200 {
		t.Fatalf("unduhan recruiter OPS: %d", st)
	}
	if e.count(`SELECT count(*) FROM audit_logs WHERE action = 'document.download'`) != 1 {
		t.Fatal("unduhan staf harus teraudit")
	}
	ti := e.client().mustLogin("rekruter.ti@arunika.example")
	expect(t, ti.do("POST", "/documents/"+docID+"/link", nil), 404, "recruiter TI")
	if st, _ := ti.get(rl.Body["url"].(string)); st != 403 {
		t.Fatalf("tautan recruiter OPS dipakai recruiter TI: %d", st)
	}
	other := e.registerVerified("lain.dok@contoh.test", "Kandidat Lain", "3201014507997102")
	expect(t, other.do("POST", "/documents/"+docID+"/link", nil), 404, "kandidat lain")

	// Berkas uji EICAR ditandai terinfeksi dan tidak pernah dapat diunduh.
	bad := c.upload("lainnya", "virus.pdf", []byte("%PDF-1.4 X5O!P%@AP[4\\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*"))
	expect(t, bad, 201, "unggah eicar")
	e.worker.Drain(context.Background())
	if r := c.do("POST", "/documents/"+bad.Body["id"].(string)+"/link", nil); r.errCode() != "scan_infected" {
		t.Fatalf("berkas terinfeksi: %s", r.Raw)
	}
	// Hapus lunak oleh pemilik.
	expect(t, c.do("DELETE", "/me/documents/"+docID, nil), 204, "hapus")
	expect(t, c.do("POST", "/documents/"+docID+"/link", nil), 404, "setelah dihapus")
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
