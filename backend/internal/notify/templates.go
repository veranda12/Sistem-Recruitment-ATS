package notify

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"ats/db/sqlc"
	"ats/internal/platform"

	"github.com/google/uuid"
)

// TemplateMeta menjelaskan template untuk editor admin.
type TemplateMeta struct {
	Label string `json:"label"`
	When  string `json:"when"`
}

var TemplateCatalog = map[string]TemplateMeta{
	"verify_email":          {"Verifikasi email", "Setelah kandidat mendaftar"},
	"password_reset":        {"Atur ulang kata sandi", "Saat pengguna meminta atur ulang sandi"},
	"account_exists":        {"Akun sudah terdaftar", "Saat seseorang mendaftar dengan email yang sudah punya akun"},
	"application_submitted": {"Lamaran diterima", "Setelah kandidat mengirim lamaran"},
	"stage_changed":         {"Tahap lamaran berubah", "Saat lamaran pindah ke tahap aktif lain"},
	"exam_invited":          {"Undangan ujian", "Saat HR mengundang kandidat ke jadwal ujian"},
	"result_released":       {"Hasil ujian dirilis", "Saat HR merilis nilai"},
	"interview_scheduled":   {"Jadwal interview", "Saat interview dijadwalkan"},
	"interview_rescheduled": {"Perubahan jadwal interview", "Saat interview dijadwalkan ulang"},
	"interview_cancelled":   {"Interview dibatalkan", "Saat interview dibatalkan"},
	"decision_hired":        {"Keputusan: diterima", "Saat recruiter mengirim pemberitahuan keputusan diterima"},
	"decision_rejected":     {"Keputusan: tidak dilanjutkan", "Saat recruiter mengirim pemberitahuan keputusan ditolak"},
	"application_withdrawn": {"Lamaran ditarik", "Saat kandidat menarik lamaran"},
}

// PlaceholderLabels menjelaskan isi setiap placeholder di editor.
var PlaceholderLabels = map[string]string{
	"nama": "Nama penerima", "lowongan": "Judul lowongan", "tahap": "Tahap lamaran saat ini", "tautan": "Tautan tindakan",
	"organisasi": "Nama organisasi", "waktu": "Waktu interview", "waktu_lama": "Waktu interview sebelumnya",
}

// SampleVars dipakai untuk pratinjau; jelas fiktif.
var SampleVars = map[string]string{
	"nama": "Nadia Pratama", "lowongan": "Staf Administrasi Operasional", "tahap": "Interview",
	"tautan": "https://karier.contoh.id/akun/lamaran/contoh", "organisasi": "PT Contoh Sejahtera",
	"waktu": "24 November 2026 10.00 WIB", "waktu_lama": "23 November 2026 14.00 WIB",
}

// essential: placeholder yang membawa informasi wajib (tautan sekali pakai, waktu interview). Bila ada di template
// bawaan, versi admin wajib tetap memuatnya.
var essential = []string{"tautan", "waktu", "waktu_lama"}

func placeholdersIn(s string) []string {
	out := []string{}
	for _, m := range placeholderRe.FindAllStringSubmatch(s, -1) {
		if !slices.Contains(out, m[1]) {
			out = append(out, m[1])
		}
	}
	return out
}

// Required mengembalikan placeholder wajib untuk sebuah template.
func Required(key string) []string {
	t, ok := Templates[key]
	if !ok {
		return nil
	}
	in := placeholdersIn(t.Subject + "\n" + t.Body)
	out := []string{}
	for _, e := range essential {
		if slices.Contains(in, e) {
			out = append(out, e)
		}
	}
	return out
}

// ValidateTemplate memeriksa isi template admin; hasilnya error per field (kosong = valid).
func ValidateTemplate(key, channel, subject, body string) map[string]string {
	f := map[string]string{}
	if channel == "email" {
		switch {
		case strings.TrimSpace(subject) == "":
			f["subject"] = "Subjek wajib diisi."
		case utf8.RuneCountInString(subject) > 200:
			f["subject"] = "Subjek maksimal 200 karakter."
		case strings.ContainsAny(subject, "\r\n"):
			f["subject"] = "Subjek harus satu baris."
		}
	}
	if strings.TrimSpace(body) == "" {
		f["body"] = "Isi pesan wajib diisi."
	} else if utf8.RuneCountInString(body) > 5000 {
		f["body"] = "Isi pesan maksimal 5000 karakter."
	}
	used := placeholdersIn(subject + "\n" + body)
	for _, p := range used {
		if !slices.Contains(Placeholders, p) {
			f["body"] = fmt.Sprintf("Placeholder {{%s}} tidak dikenal. Gunakan salah satu yang tersedia.", p)
		}
	}
	for _, r := range Required(key) {
		if !slices.Contains(used, r) {
			f["body"] = fmt.Sprintf("Placeholder {{%s}} wajib ada pada template ini (%s).", r, strings.ToLower(PlaceholderLabels[r]))
		}
	}
	return f
}

// Resolve memilih versi terbaru yang disimpan admin untuk organisasi dan kanal; bila belum ada, template bawaan.
func Resolve(ctx context.Context, q *sqlc.Queries, orgID uuid.UUID, key, channel string) (Template, error) {
	def, ok := defaultFor(key, channel)
	if !ok {
		return Template{}, fmt.Errorf("template %s/%s tidak dikenal", key, channel)
	}
	row, err := q.LatestMessageTemplate(ctx, sqlc.LatestMessageTemplateParams{OrgID: orgID, Key: key, Channel: channel})
	if platform.IsNoRows(err) {
		return def, nil
	}
	if err != nil {
		return Template{}, err
	}
	return Template{Key: key, Version: int(row.Version), Subject: row.Subject, Body: row.Body}, nil
}

// defaultFor: template bawaan per kanal. WhatsApp memakai teks ringkas sendiri bila ada, selain itu isi email.
func defaultFor(key, channel string) (Template, bool) {
	if channel == "whatsapp" {
		if t, ok := WhatsAppTemplates[key]; ok {
			return t, true
		}
	}
	t, ok := Templates[key]
	return t, ok
}

// Default mengembalikan template bawaan (dipakai editor untuk "kembalikan ke bawaan").
func Default(key, channel string) (Template, bool) { return defaultFor(key, channel) }
