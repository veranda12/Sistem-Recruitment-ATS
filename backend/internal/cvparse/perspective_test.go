package cvparse

import (
	"image"
	"math"
	"testing"
)

// fillQuad menggambar kertas putih berbentuk segi empat q di atas meja abu-abu, dengan beberapa garis teks gelap.
func fillQuad(w, h int, q [4]pt) *image.Gray {
	g := image.NewGray(image.Rect(0, 0, w, h))
	in := func(x, y float64) bool {
		for i := range q {
			a, b := q[i], q[(i+1)%4]
			if (b.X-a.X)*(y-a.Y)-(b.Y-a.Y)*(x-a.X) < 0 {
				return false
			}
		}
		return true
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := uint8(90)
			if in(float64(x)+0.5, float64(y)+0.5) {
				v = 240
				if y%60 < 8 && x%40 < 30 {
					v = 30
				}
			}
			g.Pix[y*g.Stride+x] = v
		}
	}
	return g
}

func TestFindPaperKeystone(t *testing.T) {
	want := [4]pt{{420, 150}, {1180, 170}, {1400, 1900}, {200, 1880}}
	g := fillQuad(1600, 2000, want)
	q, ok := findPaper(g)
	if !ok {
		t.Fatal("trapesium tidak terdeteksi")
	}
	for i := range q {
		if dist(q[i], want[i]) > 12 {
			t.Errorf("sudut %d = %v, ingin sekitar %v", i, q[i], want[i])
		}
	}
	o := warpQuad(g, q, 1)
	// Hasil: persegi panjang selebar sisi terpanjang, tanpa sisa meja di sudut.
	if w, h := o.Rect.Dx(), o.Rect.Dy(); math.Abs(float64(w)-1200) > 15 || math.Abs(float64(h)-1735) > 15 {
		t.Errorf("ukuran hasil %dx%d", w, h)
	}
	for _, c := range [][2]int{{5, 5}, {o.Rect.Dx() - 6, 5}, {5, o.Rect.Dy() - 6}, {o.Rect.Dx() - 6, o.Rect.Dy() - 6}} {
		if v := o.Pix[c[1]*o.Stride+c[0]]; v < 200 && v > 40 {
			t.Errorf("sudut hasil %v masih latar meja (%d)", c, v)
		}
	}
}

func TestFindPaperIgnoresNonKeystone(t *testing.T) {
	cases := map[string]*image.Gray{
		// kertas tegak lurus: cukup deskew/tanpa transformasi
		"persegi panjang": fillQuad(1600, 2000, [4]pt{{200, 150}, {1400, 150}, {1400, 1850}, {200, 1850}}),
		// kertas memenuhi gambar (pindaian atau foto rapat)
		"penuh": fillQuad(1600, 2000, [4]pt{{0, 0}, {1600, 0}, {1600, 2000}, {0, 2000}}),
		// kertas terpotong tepi gambar: sudut tidak dapat dipercaya
		"terpotong": fillQuad(1600, 2000, [4]pt{{300, -100}, {1300, -80}, {1500, 1900}, {100, 1880}}),
	}
	for name, g := range cases {
		if q, ok := findPaper(g); ok {
			t.Errorf("%s: tidak boleh diluruskan, terdeteksi %v", name, q)
		}
	}
}

func TestHomographyMapsCorners(t *testing.T) {
	src := [4]pt{{0, 0}, {100, 0}, {100, 200}, {0, 200}}
	dst := [4]pt{{10, 5}, {90, 8}, {120, 210}, {-5, 190}}
	H, ok := homography(src, dst)
	if !ok {
		t.Fatal("homografi gagal")
	}
	for i, s := range src {
		d := H[6]*s.X + H[7]*s.Y + 1
		u, v := (H[0]*s.X+H[1]*s.Y+H[2])/d, (H[3]*s.X+H[4]*s.Y+H[5])/d
		if math.Abs(u-dst[i].X) > 1e-6 || math.Abs(v-dst[i].Y) > 1e-6 {
			t.Errorf("titik %d -> (%.3f, %.3f), ingin %v", i, u, v, dst[i])
		}
	}
}
