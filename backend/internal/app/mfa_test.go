package app

import (
	"encoding/base32"
	"fmt"
	"strings"
	"testing"
	"time"

	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // RFC 6238
	"encoding/binary"
)

// totpNow: implementasi independen dari kode aplikasi (oracle) untuk menghitung kode TOTP.
func totpNow(secretB32 string, offsetSteps int64) string {
	secret, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secretB32)
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(time.Now().Unix()/30+offsetSteps))
	m := hmac.New(sha1.New, secret)
	m.Write(msg[:])
	s := m.Sum(nil)
	o := s[len(s)-1] & 0x0f
	return fmt.Sprintf("%06d", (binary.BigEndian.Uint32(s[o:o+4])&0x7fffffff)%1000000)
}

func TestMFAEnrollmentLoginRecoveryAndReset(t *testing.T) {
	e := newEnv(t)
	e.cfg.MFARequiredRoles = nil
	hr := e.client().mustLogin("hr.ops@arunika.example")
	st := hr.do("GET", "/auth/mfa", nil)
	if st.Body["enabled"] != false {
		t.Fatalf("status awal: %s", st.Raw)
	}
	setup := hr.do("POST", "/auth/mfa/setup", nil)
	expect(t, setup, 200, "setup")
	secret := setup.Body["secret"].(string)
	if !strings.HasPrefix(setup.Body["otpauthUri"].(string), "otpauth://totp/") {
		t.Fatal("URI otpauth")
	}
	if strings.Contains(string(e.scalarBytes(`SELECT mfa_secret FROM users WHERE email = 'hr.ops@arunika.example'`)), secret) {
		t.Fatal("rahasia MFA tidak boleh tersimpan sebagai teks biasa")
	}
	expect(t, hr.do("POST", "/auth/mfa/enable", map[string]string{"code": "000000"}), 422, "kode salah")
	en := hr.do("POST", "/auth/mfa/enable", map[string]string{"code": totpNow(secret, 0)})
	expect(t, en, 200, "aktifkan")
	codes := en.Body["recoveryCodes"].([]any)
	if len(codes) != 10 {
		t.Fatalf("kode pemulihan = %d", len(codes))
	}

	// Login tanpa kode -> mfa_required; kode yang sama tidak bisa dipakai ulang (replay); kode berikutnya diterima.
	c := e.client()
	if r := c.login("hr.ops@arunika.example"); r.errCode() != "mfa_required" {
		t.Fatalf("login tanpa kode: %s", r.Raw)
	}
	r := c.do("POST", "/auth/login", map[string]string{"email": "hr.ops@arunika.example", "password": testPassword, "code": totpNow(secret, 0)})
	if r.errCode() != "mfa_invalid" {
		t.Fatalf("replay kode yang sama harus ditolak: %s", r.Raw)
	}
	r = c.do("POST", "/auth/login", map[string]string{"email": "hr.ops@arunika.example", "password": testPassword, "code": totpNow(secret, 1)})
	expect(t, r, 200, "login dengan langkah berikutnya")
	if r.Body["mfa"].(map[string]any)["enabled"] != true {
		t.Fatalf("sesi harus menandai MFA aktif: %s", r.Raw)
	}
	// Kode pemulihan sekali pakai.
	rc := codes[0].(string)
	c2 := e.client()
	expect(t, c2.do("POST", "/auth/login", map[string]string{"email": "hr.ops@arunika.example", "password": testPassword, "code": rc}), 200, "kode pemulihan")
	if r := e.client().do("POST", "/auth/login", map[string]string{"email": "hr.ops@arunika.example", "password": testPassword, "code": rc}); r.errCode() != "mfa_invalid" {
		t.Fatalf("kode pemulihan dipakai ulang: %s", r.Raw)
	}
	if e.count(`SELECT count(*) FROM audit_logs WHERE action = 'auth.mfa_recovery_used'`) != 1 {
		t.Fatal("pemakaian kode pemulihan harus teraudit")
	}

	// Reset oleh admin: wajib alasan, tidak untuk diri sendiri, mencabut sesi target.
	adm := e.client().mustLogin("admin@arunika.example")
	uid := e.userID("hr.ops@arunika.example").String()
	expect(t, adm.do("POST", "/admin/users/"+uid+"/mfa-reset", map[string]string{"reason": "hilang"}), 422, "alasan pendek")
	expect(t, adm.do("POST", "/admin/users/"+e.userID("admin@arunika.example").String()+"/mfa-reset", map[string]string{"reason": "Uji reset diri sendiri"}), 409, "reset diri")
	expect(t, adm.do("POST", "/admin/users/"+uid+"/mfa-reset", map[string]string{"reason": "Ponsel hilang, identitas diverifikasi via HR"}), 204, "reset")
	expect(t, c2.do("GET", "/applications", nil), 401, "sesi target dicabut")
	e.client().mustLogin("hr.ops@arunika.example")
}

func TestMFAEnforcedForPrivilegedRoles(t *testing.T) {
	e := newEnv(t)
	e.cfg.MFARequiredRoles = []string{"hr_admin"}
	e.rebuildServer()
	hr := e.client().mustLogin("hr.ops@arunika.example")
	if r := hr.do("GET", "/applications", nil); r.errCode() != "mfa_enrollment_required" {
		t.Fatalf("HR tanpa MFA harus diarahkan aktivasi: %s", r.Raw)
	}
	expect(t, hr.do("GET", "/auth/mfa", nil), 200, "status MFA tetap boleh")
	// Peran yang tidak wajib MFA tetap bekerja.
	expect(t, e.client().mustLogin("rekruter.ops@arunika.example").do("GET", "/applications", nil), 200, "recruiter")
	secret := hr.do("POST", "/auth/mfa/setup", nil).Body["secret"].(string)
	expect(t, hr.do("POST", "/auth/mfa/enable", map[string]string{"code": totpNow(secret, 0)}), 200, "aktifkan")
	expect(t, hr.do("GET", "/applications", nil), 200, "setelah aktif")
}
