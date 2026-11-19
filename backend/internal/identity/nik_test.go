package identity

import (
	"bytes"
	"testing"
)

func testKeys() Keys {
	return Keys{EncKey: bytes.Repeat([]byte{1}, 32), EncVersion: 1, IndexKey: bytes.Repeat([]byte{2}, 32), IndexVersion: 1}
}

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"0102030405060708":    "0102030405060708",
		"0102 0304-0506.0708": "0102030405060708",
	}
	for in, want := range cases {
		got, err := Normalize(in)
		if err != nil || got != want {
			t.Fatalf("Normalize(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "123", "01020304050607089", "010203040506070a", "０１０２０３０４０５０６０７０８"} {
		if _, err := Normalize(bad); err == nil {
			t.Fatalf("Normalize(%q) harus gagal", bad)
		}
	}
}

func TestSealOpenAndIndex(t *testing.T) {
	k := testKeys()
	org := []byte("org-1")
	a, err := k.Seal("0102030405060708", org)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := k.Seal("0102030405060708", org)
	if bytes.Equal(a.Nonce, b.Nonce) || bytes.Equal(a.Ciphertext, b.Ciphertext) {
		t.Fatal("nonce/ciphertext harus unik per penyegelan")
	}
	if !bytes.Equal(a.BlindIndex, b.BlindIndex) {
		t.Fatal("blind index harus deterministik")
	}
	if a.Last4 != "0708" {
		t.Fatalf("last4 = %s", a.Last4)
	}
	pt, err := k.Open(a.Ciphertext, a.Nonce, a.KeyVersion, org)
	if err != nil || pt != "0102030405060708" {
		t.Fatalf("open = %q %v", pt, err)
	}
	if _, err := k.Open(a.Ciphertext, a.Nonce, a.KeyVersion, []byte("org-2")); err == nil {
		t.Fatal("ciphertext tidak boleh terbuka dengan org lain")
	}
	other := k
	other.IndexKey = bytes.Repeat([]byte{3}, 32)
	if bytes.Equal(other.BlindIndex("0102030405060708"), a.BlindIndex) {
		t.Fatal("blind index harus bergantung pada kunci")
	}
}
