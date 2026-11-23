package cvparse

import (
	"image"
	"math"
)

type pt struct{ X, Y float64 }

// findPaper mencari tepi kertas (lebih terang dari latar) pada foto dan mengembalikan empat sudutnya
// (kiri atas, kanan atas, kanan bawah, kiri bawah) dalam koordinat g. ok hanya bila kertas jelas terpisah dari
// latar dan bentuknya trapesium (difoto dari sudut sempit); foto yang hanya miring tetap ditangani deskew.
func findPaper(g *image.Gray) (q [4]pt, ok bool) {
	s, f := downscale(g, 600)
	w, h := s.Rect.Dx(), s.Rect.Dy()
	if w < 50 || h < 50 {
		return q, false
	}
	th := otsu(s)
	mask := make([]bool, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			mask[y*w+x] = s.Pix[y*s.Stride+x] > th
		}
	}
	comp, n := largestComponent(mask, w, h)
	if n < w*h/5 || n > w*h*95/100 {
		return q, false
	}
	// Kertas yang menyentuh tepi gambar terpotong; sudutnya tidak dapat dipercaya.
	for x := 0; x < w; x++ {
		if comp[x] || comp[(h-1)*w+x] {
			return q, false
		}
	}
	for y := 0; y < h; y++ {
		if comp[y*w] || comp[y*w+w-1] {
			return q, false
		}
	}
	// Sudut = titik ekstrem x+y dan x-y; berlaku untuk kertas yang miring kurang dari ~30°.
	best := [4]float64{math.Inf(1), math.Inf(-1), math.Inf(-1), math.Inf(1)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !comp[y*w+x] {
				continue
			}
			fx, fy := float64(x)+0.5, float64(y)+0.5
			if v := fx + fy; v < best[0] {
				best[0], q[0] = v, pt{fx, fy}
			}
			if v := fx - fy; v > best[1] {
				best[1], q[1] = v, pt{fx, fy}
			}
			if v := fx + fy; v > best[2] {
				best[2], q[2] = v, pt{fx, fy}
			}
			if v := fx - fy; v < best[3] {
				best[3], q[3] = v, pt{fx, fy}
			}
		}
	}
	// Komponen harus mengisi segi empatnya (teks gelap mengurangi isi, tetapi tidak sampai setengah).
	if area := quadArea(q); area <= 0 || float64(n) < 0.75*area {
		return q, false
	}
	top, bottom := dist(q[0], q[1]), dist(q[3], q[2])
	left, right := dist(q[0], q[3]), dist(q[1], q[2])
	if math.Max(top, bottom)/math.Min(top, bottom) < 1.06 && math.Max(left, right)/math.Min(left, right) < 1.06 {
		return q, false
	}
	for i := range q {
		q[i] = pt{q[i].X * f, q[i].Y * f}
	}
	return q, true
}

func dist(a, b pt) float64 { return math.Hypot(a.X-b.X, a.Y-b.Y) }

func quadArea(q [4]pt) float64 {
	a := 0.0
	for i := range q {
		j := (i + 1) % 4
		a += q[i].X*q[j].Y - q[j].X*q[i].Y
	}
	return a / 2
}

// otsu memilih ambang yang memaksimalkan varians antarkelas histogram.
func otsu(g *image.Gray) uint8 {
	var hist [256]float64
	w, h := g.Rect.Dx(), g.Rect.Dy()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			hist[g.Pix[y*g.Stride+x]]++
		}
	}
	total := float64(w * h)
	sum := 0.0
	for i, c := range hist {
		sum += float64(i) * c
	}
	var wb, sb, bestV float64
	best := 0
	for t := 0; t < 256; t++ {
		wb += hist[t]
		if wb == 0 || wb == total {
			continue
		}
		sb += float64(t) * hist[t]
		mb, mf := sb/wb, (sum-sb)/(total-wb)
		if v := wb * (total - wb) * (mb - mf) * (mb - mf); v > bestV {
			bestV, best = v, t
		}
	}
	return uint8(best)
}

// largestComponent: komponen 4-terhubung terbesar dari mask.
func largestComponent(mask []bool, w, h int) ([]bool, int) {
	label := make([]int32, len(mask))
	var bestLabel int32
	bestN := 0
	stack := make([]int, 0, 1024)
	var next int32
	for i, m := range mask {
		if !m || label[i] != 0 {
			continue
		}
		next++
		n := 0
		stack = append(stack[:0], i)
		label[i] = next
		for len(stack) > 0 {
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			n++
			x, y := p%w, p/w
			for _, nb := range [4]int{p - 1, p + 1, p - w, p + w} {
				if (nb == p-1 && x == 0) || (nb == p+1 && x == w-1) || (nb == p-w && y == 0) || (nb == p+w && y == h-1) {
					continue
				}
				if mask[nb] && label[nb] == 0 {
					label[nb] = next
					stack = append(stack, nb)
				}
			}
		}
		if n > bestN {
			bestN, bestLabel = n, next
		}
	}
	out := make([]bool, len(mask))
	for i, l := range label {
		out[i] = l == bestLabel && l != 0
	}
	return out, bestN
}

// warpQuad meluruskan segi empat q menjadi persegi panjang (homografi, interpolasi bilinear). Ukuran keluaran
// mengikuti sisi terpanjang dikali scale: teks di bagian yang jauh dari kamera kecil, dan OCR lebih akurat bila
// diperbesar langsung dari foto asli (satu kali interpolasi).
func warpQuad(g *image.Gray, q [4]pt, scale float64) *image.Gray {
	ow := int(math.Round(scale * math.Max(dist(q[0], q[1]), dist(q[3], q[2]))))
	oh := int(math.Round(scale * math.Max(dist(q[0], q[3]), dist(q[1], q[2]))))
	H, ok := homography([4]pt{{0, 0}, {float64(ow), 0}, {float64(ow), float64(oh)}, {0, float64(oh)}}, q)
	o := image.NewGray(image.Rect(0, 0, ow, oh))
	if !ok {
		return g
	}
	w, h := g.Rect.Dx(), g.Rect.Dy()
	for y := 0; y < oh; y++ {
		for x := 0; x < ow; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			d := H[6]*fx + H[7]*fy + 1
			sx := (H[0]*fx+H[1]*fy+H[2])/d - 0.5
			sy := (H[3]*fx+H[4]*fy+H[5])/d - 0.5
			x0, y0 := int(math.Floor(sx)), int(math.Floor(sy))
			if x0 < 0 || y0 < 0 || x0+1 >= w || y0+1 >= h {
				o.Pix[y*o.Stride+x] = 255
				continue
			}
			tx, ty := sx-float64(x0), sy-float64(y0)
			p := func(xx, yy int) float64 { return float64(g.Pix[yy*g.Stride+xx]) }
			v := p(x0, y0)*(1-tx)*(1-ty) + p(x0+1, y0)*tx*(1-ty) + p(x0, y0+1)*(1-tx)*ty + p(x0+1, y0+1)*tx*ty
			o.Pix[y*o.Stride+x] = uint8(v + 0.5)
		}
	}
	return o
}

// homography menghitung H (h33 = 1) yang memetakan src[i] ke dst[i], via eliminasi Gauss 8x8.
func homography(src, dst [4]pt) ([8]float64, bool) {
	var a [8][9]float64
	for i := 0; i < 4; i++ {
		x, y, u, v := src[i].X, src[i].Y, dst[i].X, dst[i].Y
		a[2*i] = [9]float64{x, y, 1, 0, 0, 0, -u * x, -u * y, u}
		a[2*i+1] = [9]float64{0, 0, 0, x, y, 1, -v * x, -v * y, v}
	}
	for c := 0; c < 8; c++ {
		p := c
		for r := c + 1; r < 8; r++ {
			if math.Abs(a[r][c]) > math.Abs(a[p][c]) {
				p = r
			}
		}
		if math.Abs(a[p][c]) < 1e-12 {
			return [8]float64{}, false
		}
		a[c], a[p] = a[p], a[c]
		for r := 0; r < 8; r++ {
			if r == c {
				continue
			}
			k := a[r][c] / a[c][c]
			for j := c; j < 9; j++ {
				a[r][j] -= k * a[c][j]
			}
		}
	}
	var h [8]float64
	for i := range h {
		h[i] = a[i][8] / a[i][i]
	}
	return h, true
}
