// Package exam: protokol ujian (start, resume, autosave berevisi, submit sekali), skoring, esai, rilis, expiry.
package exam

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
)

// AlgoVersion menandai algoritma pengacakan yang disimpan per attempt agar urutan dapat direkonstruksi.
const AlgoVersion = 1

// permutation menghasilkan permutasi deterministik 0..n-1 dari seed privat dan label (Fisher-Yates dengan HMAC-SHA256).
func permutation(seed []byte, label string, n int) []int {
	p := make([]int, n)
	for i := range p {
		p[i] = i
	}
	for i := n - 1; i > 0; i-- {
		m := hmac.New(sha256.New, seed)
		m.Write([]byte(label))
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(i))
		m.Write(b[:])
		r := binary.BigEndian.Uint64(m.Sum(nil)[:8])
		j := int(r % uint64(i+1))
		p[i], p[j] = p[j], p[i]
	}
	return p
}
