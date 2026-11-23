package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWhatsAppConsentAndDelivery(t *testing.T) {
	e := newEnv(t)
	cand := e.registerVerified("wa@contoh.test", "Kandidat WhatsApp", "3201014507997306") // telepon 081234567890
	prefs := cand.do("GET", "/me/notification-preferences", nil)
	expect(t, prefs, 200, "preferensi awal")
	if prefs.Body["whatsapp"] != false {
		t.Fatalf("WhatsApp harus mati sampai disetujui: %v", prefs.Body)
	}

	// Tanpa persetujuan: lamaran tidak mengirim WhatsApp.
	cand.do("POST", "/jobs/supervisor-quality-control/apply", nil)
	e.worker.Drain(context.Background())
	if n := waCount(t, e.waDir); n != 0 {
		t.Fatalf("WhatsApp terkirim tanpa persetujuan: %d", n)
	}

	expect(t, cand.do("PUT", "/me/notification-preferences", map[string]any{"whatsapp": true}), 200, "setuju WhatsApp")
	app := idOf(cand.do("POST", "/jobs/operator-produksi-karawang/apply", nil), "applicationId")
	e.worker.Drain(context.Background())
	files, _ := filepath.Glob(filepath.Join(e.waDir, "*.txt"))
	if len(files) != 1 {
		t.Fatalf("pesan WhatsApp lamaran: %d", len(files))
	}
	b, _ := os.ReadFile(files[0])
	if !strings.Contains(string(b), "To: +6281234567890") || !strings.Contains(string(b), "Operator Produksi") || !strings.Contains(string(b), "/akun/lamaran/"+app) {
		t.Fatalf("isi pesan WhatsApp:\n%s", b)
	}
	if n := e.count(`SELECT count(*) FROM email_deliveries WHERE channel = 'whatsapp' AND status = 'sent' AND to_address = '+6281234567890'`); n != 1 {
		t.Fatalf("catatan pengiriman WhatsApp: %d", n)
	}
	// Drain ulang tidak mengirim dua kali.
	e.worker.Drain(context.Background())
	if n := waCount(t, e.waDir); n != 1 {
		t.Fatalf("WhatsApp terkirim ganda: %d", n)
	}

	expect(t, cand.do("PUT", "/me/notification-preferences", map[string]any{"whatsapp": false}), 200, "berhenti WhatsApp")
	if n := e.count(`SELECT count(*) FROM audit_logs WHERE action IN ('candidate.whatsapp_opt_in','candidate.whatsapp_opt_out')`); n != 2 {
		t.Fatalf("audit persetujuan WhatsApp: %d", n)
	}
	// Verifikasi email tidak pernah lewat WhatsApp (tautan sekali pakai).
	if n := e.count(`SELECT count(*) FROM email_deliveries WHERE channel = 'whatsapp' AND template_key = 'verify_email'`); n != 0 {
		t.Fatalf("verifikasi lewat WhatsApp: %d", n)
	}
}

func waCount(t *testing.T, dir string) int {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(dir, "*.txt"))
	return len(files)
}
