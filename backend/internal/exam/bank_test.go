package exam

import (
	"strings"
	"testing"
)

func TestParseImport(t *testing.T) {
	csv := strings.Join([]string{
		"jenis;pertanyaan;opsi;kunci;bobot;rubrik",
		"tunggal;Ibu kota Jawa Barat?;Bandung|Bogor|Bekasi;A;1;",
		"jamak;Pilih bilangan genap;2|3|4|5;A,C;2;",
		"esai;Jelaskan prosedur K3;;;3;Tiga langkah utama",
		`tunggal;"Pertanyaan; dengan titik koma";Ya|Tidak;B;1,5;`,
	}, "\n")
	rows, digest, err := ParseImport(csv)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 || digest == "" {
		t.Fatalf("baris = %d", len(rows))
	}
	for _, r := range rows {
		if len(r.Errors) > 0 {
			t.Fatalf("baris %d tidak boleh error: %v", r.Line, r.Errors)
		}
	}
	if rows[3].Stem != "Pertanyaan; dengan titik koma" || rows[3].Weight != 1.5 {
		t.Fatalf("kutip dan desimal koma: %+v", rows[3])
	}
	if _, d2, _ := ParseImport(csv); d2 != digest {
		t.Fatal("digest harus deterministik")
	}

	bad := strings.Join([]string{
		"pilihan;Tanpa jenis valid;A|B;A;1;",
		"tunggal;Kunci ganda;A|B;A,B;1;",
		"tunggal;Kunci di luar opsi;A|B;D;1;",
		"jamak;Satu opsi saja;A;A;1;",
		"esai;Esai berkunci;A|B;A;1;",
		"tunggal;;A|B;A;1;",
		"tunggal;Bobot nol;A|B;A;0;",
	}, "\n")
	rows, _, err = ParseImport(bad)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if len(r.Errors) == 0 {
			t.Errorf("baris %d seharusnya error", r.Line)
		}
	}
	if _, _, err := ParseImport(""); err == nil {
		t.Fatal("isi kosong harus ditolak")
	}
}

func TestPermutationDeterministic(t *testing.T) {
	seed := []byte("01234567890123456789012345678901")
	a := permutation(seed, "items", 10)
	b := permutation(seed, "items", 10)
	c := permutation(seed, "opts", 10)
	seen := map[int]bool{}
	for i := range a {
		if a[i] != b[i] {
			t.Fatal("permutasi harus deterministik untuk seed+label sama")
		}
		seen[a[i]] = true
	}
	if len(seen) != 10 {
		t.Fatal("hasil harus permutasi lengkap")
	}
	same := true
	for i := range a {
		same = same && a[i] == c[i]
	}
	if same {
		t.Fatal("label berbeda seharusnya menghasilkan urutan berbeda")
	}
}
