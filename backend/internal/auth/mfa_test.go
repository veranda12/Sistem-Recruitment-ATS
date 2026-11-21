package auth

import (
	"testing"
	"time"
)

// Vektor uji RFC 6238 Lampiran B (SHA1, rahasia ASCII "12345678901234567890"), dipotong ke 6 digit.
func TestTOTPVectorsRFC6238(t *testing.T) {
	secret := []byte("12345678901234567890")
	cases := map[int64]string{59: "287082", 1111111109: "081804", 1111111111: "050471", 1234567890: "005924", 2000000000: "279037"}
	for ts, want := range cases {
		if got := totpAt(secret, ts/totpPeriod); got != want {
			t.Fatalf("T=%d: %s, ingin %s", ts, got, want)
		}
	}
}

func TestVerifyTOTPWindowAndReplay(t *testing.T) {
	secret := []byte("12345678901234567890")
	now := time.Unix(1234567890, 0)
	step := now.Unix() / totpPeriod
	code := totpAt(secret, step)
	got, ok := verifyTOTP(secret, code, now, nil)
	if !ok || got != step {
		t.Fatal("kode saat ini harus diterima")
	}
	if _, ok := verifyTOTP(secret, code, now, &got); ok {
		t.Fatal("kode yang sama tidak boleh dipakai ulang (replay)")
	}
	if _, ok := verifyTOTP(secret, totpAt(secret, step-1), now, nil); !ok {
		t.Fatal("toleransi satu langkah sebelumnya")
	}
	if _, ok := verifyTOTP(secret, totpAt(secret, step-2), now, nil); ok {
		t.Fatal("dua langkah sebelumnya harus ditolak")
	}
	if _, ok := verifyTOTP(secret, "12345", now, nil); ok {
		t.Fatal("panjang kode salah harus ditolak")
	}
}
