package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"unicode"

	"golang.org/x/crypto/argon2"
)

// Parameter Argon2id: m=19 MiB, t=2, p=1 (rekomendasi minimum OWASP Password Storage Cheat Sheet).
// Konkurensi hashing dibatasi semaphore agar burst login tidak melampaui anggaran memori:
// 2 x NumCPU slot x 19 MiB. Dengan 8 CPU = 16 slot = ~304 MiB puncak.
const (
	argonMemory  = 19 * 1024
	argonTime    = 2
	argonThreads = 1
	argonKeyLen  = 32
)

var hashSlots = make(chan struct{}, 2*runtime.NumCPU())

func acquire(ctx context.Context) error {
	select {
	case hashSlots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func release() { <-hashSlots }

func HashPassword(ctx context.Context, pw string) (string, error) {
	if err := acquire(ctx); err != nil {
		return "", err
	}
	defer release()
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// dummyHash dipakai saat email tidak ditemukan agar waktu respons login tidak membocorkan keberadaan akun.
var dummyHash = "$argon2id$v=19$m=19456,t=2,p=1$c29tZXNhbHRzb21lc2FsdA$5b3TgD4mJ0XTVyw8kQdZ1lF0ifFBv7dAUnnk6E8GxQo"

func VerifyPassword(ctx context.Context, encoded, pw string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("format hash tidak dikenal")
	}
	var m uint32
	var t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false, err
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, err
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, err
	}
	if err := acquire(ctx); err != nil {
		return false, err
	}
	defer release()
	got := argon2.IDKey([]byte(pw), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// ValidatePassword: minimal 10 karakter, maksimal 128, memuat huruf dan angka.
func ValidatePassword(pw string) string {
	if len(pw) < 10 {
		return "Kata sandi minimal 10 karakter."
	}
	if len(pw) > 128 {
		return "Kata sandi maksimal 128 karakter."
	}
	var letter, digit bool
	for _, r := range pw {
		switch {
		case unicode.IsLetter(r):
			letter = true
		case unicode.IsDigit(r):
			digit = true
		}
	}
	if !letter || !digit {
		return "Kata sandi harus memuat huruf dan angka."
	}
	return ""
}
