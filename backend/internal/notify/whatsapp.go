package notify

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WhatsAppTemplates: teks ringkas bawaan untuk kanal WhatsApp (tanpa subjek). Hanya kejadian lamaran yang dikirim
// lewat WhatsApp; tautan sekali pakai (verifikasi, atur ulang sandi) tetap hanya lewat email.
var WhatsAppTemplates = map[string]Template{
	"application_submitted": {Key: "application_submitted", Version: 1,
		Body: "Halo {{nama}}, lamaran Anda untuk posisi {{lowongan}} sudah kami terima. Pantau statusnya di {{tautan}}"},
	"stage_changed": {Key: "stage_changed", Version: 1,
		Body: "Halo {{nama}}, lamaran {{lowongan}} Anda kini di tahap {{tahap}}. Detail: {{tautan}}"},
	"exam_invited": {Key: "exam_invited", Version: 1,
		Body: "Halo {{nama}}, Anda diundang ujian untuk posisi {{lowongan}}. Jadwal dan tombol mulai ada di {{tautan}}"},
	"result_released": {Key: "result_released", Version: 1,
		Body: "Halo {{nama}}, hasil ujian {{lowongan}} sudah dirilis. Tahap saat ini: {{tahap}}. Detail: {{tautan}}"},
	"interview_scheduled": {Key: "interview_scheduled", Version: 1,
		Body: "Halo {{nama}}, interview {{lowongan}} dijadwalkan {{waktu}}. Detail: {{tautan}}"},
	"interview_rescheduled": {Key: "interview_rescheduled", Version: 1,
		Body: "Halo {{nama}}, jadwal interview {{lowongan}} berubah dari {{waktu_lama}} menjadi {{waktu}}. Detail: {{tautan}}"},
	"interview_cancelled": {Key: "interview_cancelled", Version: 1,
		Body: "Halo {{nama}}, interview {{lowongan}} pada {{waktu}} dibatalkan. Kami akan menghubungi Anda bila ada jadwal baru."},
	"decision_hired": {Key: "decision_hired", Version: 1,
		Body: "Halo {{nama}}, selamat, Anda diterima untuk posisi {{lowongan}}. Tim kami akan menghubungi Anda."},
	"decision_rejected": {Key: "decision_rejected", Version: 1,
		Body: "Halo {{nama}}, terima kasih atas minat Anda pada posisi {{lowongan}}. Saat ini kami belum dapat melanjutkan lamaran Anda."},
}

// WhatsAppSender mengirim pesan teks ke nomor E.164 (tanpa "+").
type WhatsAppSender interface {
	Send(ctx context.Context, to, body, idempotencyKey string) (providerID string, err error)
}

// FileWhatsApp menulis pesan ke berkas .txt (dev/test), setara FileMailer untuk email. Penyedia produksi
// (mis. WhatsApp Business Platform) disambungkan dengan mengimplementasikan WhatsAppSender.
type FileWhatsApp struct{ Dir string }

func (f FileWhatsApp) Send(_ context.Context, to, body, key string) (string, error) {
	if err := os.MkdirAll(f.Dir, 0o700); err != nil {
		return "", err
	}
	if key == "" {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		key = hex.EncodeToString(b)
	}
	name := fmt.Sprintf("%s_%s_%s.txt", time.Now().UTC().Format("20060102T150405.000"), to, safe(key))
	content := fmt.Sprintf("To: +%s\nMessage-ID: %s\n\n%s\n", to, key, body)
	return key, os.WriteFile(filepath.Join(f.Dir, name), []byte(content), 0o600)
}

// NormalizeWhatsApp mengubah nomor profil (0812..., +62812..., 62812...) menjadi E.164 tanpa "+".
// Mengembalikan "" bila nomor tidak dapat dipakai.
func NormalizeWhatsApp(phone string) string {
	p := strings.TrimPrefix(strings.TrimSpace(phone), "+")
	if strings.HasPrefix(p, "0") {
		p = "62" + p[1:]
	}
	if len(p) < 9 || len(p) > 15 {
		return ""
	}
	for _, r := range p {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return p
}
