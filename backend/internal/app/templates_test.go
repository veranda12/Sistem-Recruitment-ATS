package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMessageTemplateEditor(t *testing.T) {
	e := newEnv(t)
	admin := e.client().mustLogin("admin@arunika.example")
	hr := e.client().mustLogin("hr.ops@arunika.example")
	path := "/admin/message-templates/verify_email/email"

	expect(t, hr.do("GET", "/admin/message-templates", nil), 403, "HR tanpa settings.manage")
	list := admin.do("GET", "/admin/message-templates", nil)
	expect(t, list, 200, "daftar template")
	var found map[string]any
	for _, x := range list.Body["items"].([]any) {
		if m := x.(map[string]any); m["key"] == "verify_email" && m["channel"] == "email" {
			found = m
		}
	}
	if found == nil || found["version"].(float64) != 0 || found["customized"] != false {
		t.Fatalf("verify_email awal harus bawaan: %v", found)
	}

	// Validasi: {{tautan}} wajib; versi basi ditolak; pratinjau memakai data contoh.
	expect(t, admin.do("PUT", path, map[string]any{"subject": "Verifikasi", "body": "Halo {{nama}}", "expectedVersion": 0}), 422, "tanpa tautan")
	prev := admin.do("POST", path+"/preview", map[string]any{"subject": "Aktifkan akun {{organisasi}}", "body": "Halo {{nama}}, klik {{tautan}}"})
	expect(t, prev, 200, "pratinjau")
	if !strings.Contains(prev.Body["body"].(string), "Nadia Pratama") {
		t.Fatalf("pratinjau tanpa data contoh: %v", prev.Body)
	}
	expect(t, admin.do("PUT", path, map[string]any{"subject": "Aktifkan akun {{organisasi}}", "body": "Halo {{nama}}, aktifkan akun Anda: {{tautan}}",
		"note": "Bahasa lebih ringkas", "expectedVersion": 0}), 200, "simpan v1")
	expect(t, admin.do("PUT", path, map[string]any{"subject": "Lain", "body": "{{tautan}}", "expectedVersion": 0}), 409, "versi basi")

	// Email yang dikirim memakai versi admin.
	e.registerVerified("templat@contoh.test", "Kandidat Templat", "3201014507997305")
	e.worker.Drain(context.Background())
	if !mailContains(t, e.mailDir, "templat@contoh.test", "aktifkan akun Anda") {
		t.Fatal("email verifikasi tidak memakai template admin")
	}

	// Kembali ke bawaan dan pulihkan versi lama: masing-masing menambah versi baru; riwayat lengkap.
	expect(t, admin.do("PUT", path, map[string]any{"resetToDefault": true, "expectedVersion": 1}), 200, "kembalikan ke bawaan")
	expect(t, admin.do("PUT", path, map[string]any{"restoreVersion": 1, "expectedVersion": 2}), 200, "pulihkan v1")
	vers := admin.do("GET", path+"/versions", nil).Body["items"].([]any)
	if len(vers) != 3 || vers[1].(map[string]any)["isDefault"] != true || vers[0].(map[string]any)["body"] != "Halo {{nama}}, aktifkan akun Anda: {{tautan}}" {
		t.Fatalf("riwayat versi: %v", vers)
	}
	expect(t, admin.do("PUT", "/admin/message-templates/verify_email/whatsapp", map[string]any{"body": "{{tautan}}", "expectedVersion": 0}), 404, "verifikasi tidak lewat WhatsApp")
	if n := e.count(`SELECT count(*) FROM audit_logs WHERE action = 'admin.template_save'`); n != 3 {
		t.Fatalf("audit simpan template: %d", n)
	}
}

func mailContains(t *testing.T, dir, to, text string) bool {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(dir, "*.eml"))
	for _, f := range files {
		b, _ := os.ReadFile(f)
		if strings.Contains(string(b), "To: "+to) && strings.Contains(string(b), text) {
			return true
		}
	}
	return false
}
