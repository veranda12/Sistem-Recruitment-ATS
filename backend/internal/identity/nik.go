// Package identity menangani NIK: normalisasi, enkripsi AEAD, blind index, dan penyamaran.
package identity

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidNIK = errors.New("NIK harus 16 digit angka")

// Normalize membuang spasi, titik dan strip lalu mewajibkan tepat 16 digit ASCII. Nol di depan dipertahankan.
func Normalize(raw string) (string, error) {
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r == ' ' || r == '.' || r == '-':
			continue
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			return "", ErrInvalidNIK
		}
	}
	s := b.String()
	if len(s) != 16 {
		return "", ErrInvalidNIK
	}
	return s, nil
}

// Keys memegang kunci enkripsi dan kunci blind index secara terpisah, masing-masing berversi.
type Keys struct {
	EncKey      []byte
	EncVersion  int
	IndexKey    []byte
	IndexVersion int
}

type Sealed struct {
	Ciphertext   []byte
	Nonce        []byte
	KeyVersion   int
	BlindIndex   []byte
	IndexVersion int
	Last4        string
}

// Seal mengenkripsi NIK ternormalisasi. AAD mengikat ciphertext ke organisasi agar tidak bisa dipindah antar org.
func (k Keys) Seal(nik string, orgID []byte) (Sealed, error) {
	block, err := aes.NewCipher(k.EncKey)
	if err != nil {
		return Sealed{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return Sealed{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return Sealed{}, err
	}
	ct := gcm.Seal(nil, nonce, []byte(nik), aad(orgID, k.EncVersion))
	return Sealed{
		Ciphertext: ct, Nonce: nonce, KeyVersion: k.EncVersion,
		BlindIndex: k.BlindIndex(nik), IndexVersion: k.IndexVersion, Last4: nik[12:],
	}, nil
}

func (k Keys) Open(ct, nonce []byte, keyVersion int, orgID []byte) (string, error) {
	if keyVersion != k.EncVersion {
		return "", fmt.Errorf("kunci NIK versi %d tidak dimuat", keyVersion)
	}
	block, err := aes.NewCipher(k.EncKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	pt, err := gcm.Open(nil, nonce, ct, aad(orgID, keyVersion))
	if err != nil {
		return "", errors.New("dekripsi NIK gagal")
	}
	return string(pt), nil
}

// BlindIndex: HMAC-SHA256 berkunci terpisah. Bukan hash polos, sehingga tanpa kunci tidak bisa ditebak dari ruang 16 digit.
func (k Keys) BlindIndex(nik string) []byte {
	m := hmac.New(sha256.New, k.IndexKey)
	m.Write([]byte("nik:v1:" + nik))
	return m.Sum(nil)
}

func aad(orgID []byte, v int) []byte { return append(append([]byte("nik|"), orgID...), byte(v)) }

// Mask menampilkan NIK tersamar, hanya 4 digit terakhir.
func Mask(last4 string) string { return "************" + last4 }
