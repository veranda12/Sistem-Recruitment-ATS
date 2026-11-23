package notify

import (
	"strings"
	"testing"
)

func TestDefaultTemplatesValid(t *testing.T) {
	for key := range TemplateCatalog {
		if _, ok := Templates[key]; !ok {
			t.Errorf("katalog %s tanpa template email bawaan", key)
		}
		for _, ch := range []string{"email", "whatsapp"} {
			def, ok := Default(key, ch)
			if !ok {
				continue
			}
			if f := ValidateTemplate(key, ch, def.Subject, def.Body); len(f) > 0 {
				t.Errorf("bawaan %s/%s tidak valid: %v", key, ch, f)
			}
			if _, _, err := Render(def, SampleVars); err != nil {
				t.Errorf("bawaan %s/%s gagal dirender: %v", key, ch, err)
			}
		}
	}
	for key := range WhatsAppTemplates {
		if _, ok := TemplateCatalog[key]; !ok {
			t.Errorf("template WhatsApp %s tidak ada di katalog", key)
		}
	}
}

func TestValidateTemplate(t *testing.T) {
	cases := []struct {
		name, key, ch, subject, body, field, want string
	}{
		{"placeholder asing", "stage_changed", "email", "Pembaruan", "Halo {{nama}} {{gaji}} {{tautan}}", "body", "{{gaji}} tidak dikenal"},
		{"tautan verifikasi dihapus", "verify_email", "email", "Verifikasi", "Halo {{nama}}, silakan cek email.", "body", "{{tautan}} wajib"},
		{"waktu interview dihapus", "interview_scheduled", "whatsapp", "", "Halo {{nama}}, Anda dijadwalkan interview. {{tautan}}", "body", "{{waktu}} wajib"},
		{"subjek multi-baris", "decision_hired", "email", "Hasil\nseleksi", "Halo {{nama}}", "subject", "satu baris"},
		{"subjek kosong", "decision_hired", "email", " ", "Halo {{nama}}", "subject", "wajib"},
	}
	for _, c := range cases {
		f := ValidateTemplate(c.key, c.ch, c.subject, c.body)
		if !strings.Contains(f[c.field], c.want) {
			t.Errorf("%s: %s = %q, ingin memuat %q", c.name, c.field, f[c.field], c.want)
		}
	}
	if f := ValidateTemplate("decision_hired", "email", "Selamat, {{nama}}", "Halo {{nama}}, Anda diterima untuk {{lowongan}}."); len(f) > 0 {
		t.Errorf("template tanpa placeholder wajib harus valid: %v", f)
	}
}
