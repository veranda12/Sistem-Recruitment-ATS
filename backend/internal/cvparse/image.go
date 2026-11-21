package cvparse

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"time"

	_ "golang.org/x/image/webp" // registrasi decoder WebP
)

// Transform mencatat langkah praproses agar koordinat OCR dapat dipetakan ke gambar preview.
type Transform struct {
	ExifOrientation int     `json:"exifOrientation,omitempty"`
	Rotate          int     `json:"rotate,omitempty"` // derajat searah jarum jam setelah EXIF
	DeskewDeg       float64 `json:"deskewDeg,omitempty"`
	BlurScore       float64 `json:"blurScore"`
	Width           int     `json:"width"`
	Height          int     `json:"height"`
}

// checkImageConfig menolak dimensi berlebihan sebelum decode penuh (mencegah bom dekompresi).
func (e *engine) checkImageConfig(data []byte) error {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fail(ReasonCorrupt, "gambar tidak dapat dibaca")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return fail(ReasonEmpty, "gambar kosong")
	}
	if cfg.Width > e.lim.MaxImageSide || cfg.Height > e.lim.MaxImageSide || int64(cfg.Width)*int64(cfg.Height) > e.lim.MaxImagePixels {
		return fail(ReasonTooManyPixels, fmt.Sprintf("%dx%d", cfg.Width, cfg.Height))
	}
	return nil
}

// exifOrientation membaca tag 0x0112 dari segmen APP1 JPEG. 1 = normal.
func exifOrientation(b []byte) int {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return 1
	}
	i := 2
	for i+4 < len(b) {
		if b[i] != 0xFF {
			return 1
		}
		marker := b[i+1]
		size := int(binary.BigEndian.Uint16(b[i+2:]))
		if marker == 0xDA || size < 2 || i+2+size > len(b) {
			return 1
		}
		seg := b[i+4 : i+2+size]
		if marker == 0xE1 && len(seg) > 14 && string(seg[:6]) == "Exif\x00\x00" {
			t := seg[6:]
			var bo binary.ByteOrder
			switch string(t[:2]) {
			case "II":
				bo = binary.LittleEndian
			case "MM":
				bo = binary.BigEndian
			default:
				return 1
			}
			off := int(bo.Uint32(t[4:]))
			if off+2 > len(t) {
				return 1
			}
			n := int(bo.Uint16(t[off:]))
			for k := 0; k < n; k++ {
				e := off + 2 + k*12
				if e+12 > len(t) {
					return 1
				}
				if bo.Uint16(t[e:]) == 0x0112 {
					v := int(bo.Uint16(t[e+8:]))
					if v >= 1 && v <= 8 {
						return v
					}
					return 1
				}
			}
			return 1
		}
		i += 2 + size
	}
	return 1
}

func toGray(img image.Image) *image.Gray {
	b := img.Bounds()
	g := image.NewGray(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(g, g.Bounds(), img, b.Min, draw.Src)
	return g
}

// orient menerapkan orientasi EXIF (termasuk cermin) pada gambar grayscale.
func orient(g *image.Gray, o int) *image.Gray {
	switch o {
	case 2:
		return flipH(g)
	case 3:
		return rotate90s(g, 180)
	case 4:
		return flipH(rotate90s(g, 180))
	case 5:
		return flipH(rotate90s(g, 90))
	case 6:
		return rotate90s(g, 90)
	case 7:
		return flipH(rotate90s(g, 270))
	case 8:
		return rotate90s(g, 270)
	}
	return g
}

func flipH(g *image.Gray) *image.Gray {
	w, h := g.Rect.Dx(), g.Rect.Dy()
	o := image.NewGray(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o.Pix[y*o.Stride+x] = g.Pix[y*g.Stride+(w-1-x)]
		}
	}
	return o
}

// rotate90s memutar searah jarum jam sebesar 90/180/270 derajat.
func rotate90s(g *image.Gray, deg int) *image.Gray {
	w, h := g.Rect.Dx(), g.Rect.Dy()
	switch ((deg % 360) + 360) % 360 {
	case 90:
		o := image.NewGray(image.Rect(0, 0, h, w))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				o.Pix[x*o.Stride+(h-1-y)] = g.Pix[y*g.Stride+x]
			}
		}
		return o
	case 180:
		o := image.NewGray(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				o.Pix[(h-1-y)*o.Stride+(w-1-x)] = g.Pix[y*g.Stride+x]
			}
		}
		return o
	case 270:
		o := image.NewGray(image.Rect(0, 0, h, w))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				o.Pix[(w-1-x)*o.Stride+y] = g.Pix[y*g.Stride+x]
			}
		}
		return o
	}
	return g
}

// downscale dengan rata-rata kotak sampai sisi terpanjang <= max.
func downscale(g *image.Gray, maxSide int) (*image.Gray, float64) {
	w, h := g.Rect.Dx(), g.Rect.Dy()
	f := 1
	for max(w, h)/f > maxSide {
		f++
	}
	if f == 1 {
		return g, 1
	}
	o := image.NewGray(image.Rect(0, 0, w/f, h/f))
	for y := 0; y < h/f; y++ {
		for x := 0; x < w/f; x++ {
			s := 0
			for dy := 0; dy < f; dy++ {
				row := (y*f + dy) * g.Stride
				for dx := 0; dx < f; dx++ {
					s += int(g.Pix[row+x*f+dx])
				}
			}
			o.Pix[y*o.Stride+x] = uint8(s / (f * f))
		}
	}
	return o, float64(f)
}

// blurScore: varians Laplacian pada skala kerja ~1200 px. Rendah = buram.
func blurScore(g *image.Gray) float64 {
	s, _ := downscale(g, 1200)
	w, h := s.Rect.Dx(), s.Rect.Dy()
	var sum, sum2 float64
	n := 0
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			c := int(s.Pix[y*s.Stride+x])
			l := int(s.Pix[y*s.Stride+x-1]) + int(s.Pix[y*s.Stride+x+1]) + int(s.Pix[(y-1)*s.Stride+x]) + int(s.Pix[(y+1)*s.Stride+x]) - 4*c
			v := float64(l)
			sum += v
			sum2 += v * v
			n++
		}
	}
	if n == 0 {
		return 0
	}
	m := sum / float64(n)
	return sum2/float64(n) - m*m
}

// normalizeBackground membagi piksel dengan estimasi warna kertas (maksimum lokal pada skala kecil,
// diperhalus) sehingga bayangan dan gradasi cahaya hilang tanpa halo di sekitar blok teks.
// Tidak membinerkan agar @, +, titik, dan digit tetap utuh.
func normalizeBackground(g *image.Gray) *image.Gray {
	w, h := g.Rect.Dx(), g.Rect.Dy()
	const f = 8
	sw, sh := (w+f-1)/f, (h+f-1)/f
	small := make([]float64, sw*sh)
	for y := 0; y < sh; y++ {
		for x := 0; x < sw; x++ {
			m := 0
			for dy := 0; dy < f && y*f+dy < h; dy++ {
				row := (y*f + dy) * g.Stride
				for dx := 0; dx < f && x*f+dx < w; dx++ {
					m = max(m, int(g.Pix[row+x*f+dx]))
				}
			}
			small[y*sw+x] = float64(m)
		}
	}
	// maksimum lokal radius r (blok huruf terbesar ~ 2-3 sel) lalu rata-rata untuk menghaluskan
	r := 3
	tmp := make([]float64, len(small))
	for y := 0; y < sh; y++ {
		for x := 0; x < sw; x++ {
			m := 0.0
			for dy := -r; dy <= r; dy++ {
				for dx := -r; dx <= r; dx++ {
					xx, yy := x+dx, y+dy
					if xx >= 0 && yy >= 0 && xx < sw && yy < sh {
						m = math.Max(m, small[yy*sw+xx])
					}
				}
			}
			tmp[y*sw+x] = m
		}
	}
	bg := make([]float64, len(tmp))
	for y := 0; y < sh; y++ {
		for x := 0; x < sw; x++ {
			s, n := 0.0, 0
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					xx, yy := x+dx, y+dy
					if xx >= 0 && yy >= 0 && xx < sw && yy < sh {
						s += tmp[yy*sw+xx]
						n++
					}
				}
			}
			bg[y*sw+x] = s / float64(n)
		}
	}
	o := image.NewGray(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		fy := math.Min(float64(sh-1), math.Max(0, (float64(y)+0.5)/f-0.5))
		y0 := int(fy)
		y1 := min(sh-1, y0+1)
		ty := fy - float64(y0)
		for x := 0; x < w; x++ {
			fx := math.Min(float64(sw-1), math.Max(0, (float64(x)+0.5)/f-0.5))
			x0 := int(fx)
			x1 := min(sw-1, x0+1)
			tx := fx - float64(x0)
			b := bg[y0*sw+x0]*(1-tx)*(1-ty) + bg[y0*sw+x1]*tx*(1-ty) + bg[y1*sw+x0]*(1-tx)*ty + bg[y1*sw+x1]*tx*ty
			b = math.Max(b, 60)
			v := float64(g.Pix[y*g.Stride+x]) / b * 255
			o.Pix[y*o.Stride+x] = uint8(math.Max(0, math.Min(255, v)))
		}
	}
	return o
}

// estimateSkew mencari sudut (derajat) yang memaksimalkan varians profil proyeksi baris.
func estimateSkew(g *image.Gray) float64 {
	s, _ := downscale(g, 900)
	w, h := s.Rect.Dx(), s.Rect.Dy()
	type pt struct{ x, y float64 }
	var dark []pt
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if s.Pix[y*s.Stride+x] < 110 {
				dark = append(dark, pt{float64(x) - float64(w)/2, float64(y) - float64(h)/2})
			}
		}
	}
	if len(dark) < 200 {
		return 0
	}
	score := func(deg float64) float64 {
		a := deg * math.Pi / 180
		sin, cos := math.Sin(a), math.Cos(a)
		bins := make([]float64, h*2+1)
		for _, p := range dark {
			yy := int(p.x*sin+p.y*cos) + h
			if yy >= 0 && yy < len(bins) {
				bins[yy]++
			}
		}
		var sum, sum2 float64
		for _, b := range bins {
			sum += b
			sum2 += b * b
		}
		m := sum / float64(len(bins))
		return sum2/float64(len(bins)) - m*m
	}
	best, bestS := 0.0, score(0)
	for d := -10.0; d <= 10.0; d += 0.5 {
		if v := score(d); v > bestS {
			best, bestS = d, v
		}
	}
	for d := best - 0.4; d <= best+0.4; d += 0.1 {
		if v := score(d); v > bestS {
			best, bestS = d, v
		}
	}
	if math.Abs(best) < 0.3 {
		return 0
	}
	return best
}

// rotateDeg memutar dengan interpolasi bilinear; area kosong diisi putih.
func rotateDeg(g *image.Gray, deg float64) *image.Gray {
	a := deg * math.Pi / 180
	sin, cos := math.Sin(a), math.Cos(a)
	w, h := float64(g.Rect.Dx()), float64(g.Rect.Dy())
	nw := int(math.Abs(w*cos) + math.Abs(h*sin))
	nh := int(math.Abs(w*sin) + math.Abs(h*cos))
	o := image.NewGray(image.Rect(0, 0, nw, nh))
	cx, cy := w/2, h/2
	ncx, ncy := float64(nw)/2, float64(nh)/2
	for y := 0; y < nh; y++ {
		for x := 0; x < nw; x++ {
			dx, dy := float64(x)-ncx, float64(y)-ncy
			sx := dx*cos + dy*sin + cx
			sy := -dx*sin + dy*cos + cy
			x0, y0 := int(math.Floor(sx)), int(math.Floor(sy))
			if x0 < 0 || y0 < 0 || x0+1 >= int(w) || y0+1 >= int(h) {
				o.Pix[y*o.Stride+x] = 255
				continue
			}
			fx, fy := sx-float64(x0), sy-float64(y0)
			p := func(xx, yy int) float64 { return float64(g.Pix[yy*g.Stride+xx]) }
			v := p(x0, y0)*(1-fx)*(1-fy) + p(x0+1, y0)*fx*(1-fy) + p(x0, y0+1)*(1-fx)*fy + p(x0+1, y0+1)*fx*fy
			o.Pix[y*o.Stride+x] = uint8(v)
		}
	}
	return o
}

func savePNG(path string, img image.Image) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func saveJPEG(path string, img image.Image) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 82}); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// BlurThreshold dikalibrasi pada fixture foto (lihat docs/cv-parsing.md).
const BlurThreshold = 60.0

// ocrImage: praproses + OSD + deskew + OCR, dengan percobaan alternatif terbatas bila mutu rendah.
// Bila gambar ditransformasi, preview halaman diganti gambar hasil transformasi agar highlight sesuai.
func (e *engine) ocrImage(ctx context.Context, src string, p *Page) (ocrResult, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return ocrResult{}, err
	}
	if err := e.checkImageConfig(data); err != nil {
		return ocrResult{}, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return ocrResult{}, fail(ReasonCorrupt, "decode gambar gagal")
	}
	tStart := time.Now()
	stamp := func(name string) {
		p.Attempts = append(p.Attempts, fmt.Sprintf("t:%s=%dms", name, time.Since(tStart).Milliseconds()))
	}
	tr := Transform{ExifOrientation: exifOrientation(data)}
	g := orient(toGray(img), tr.ExifOrientation)
	tr.BlurScore = blurScore(g)
	// Cek blur hanya untuk foto; render PDF tidak pernah buram dan halaman kosong ditangani terpisah.
	if e.format != "pdf" && tr.BlurScore < BlurThreshold {
		return ocrResult{}, fail(ReasonBlurry, fmt.Sprintf("blur=%.1f", tr.BlurScore))
	}
	// OSD pada gambar mentah dan hasil normalisasi; pakai keyakinan tertinggi.
	raw := g
	stamp("decode")
	g = normalizeBackground(g)
	stamp("normalize")
	tmp := filepath.Join(e.dir, fmt.Sprintf("osd-%d.png", p.No))
	bestDeg, bestConf := 0, 0.0
	cands := []*image.Gray{raw, g}
	if e.format == "pdf" {
		// Halaman PDF jarang terputar; lewati OSD (hemat ~2-4 detik/halaman). Halaman terbalik tetap
		// ditangani verifikasi orientasi berbasis keyakinan OCR di bawah.
		cands = nil
	}
	for i, cand := range cands {
		small, _ := downscale(cand, 3500)
		if err := savePNG(tmp, small); err != nil {
			continue
		}
		deg, conf := e.detectRotation(ctx, tmp)
		_ = os.Remove(tmp)
		p.Attempts = append(p.Attempts, fmt.Sprintf("osd#%d rotate=%d conf=%.1f", i+1, deg, conf))
		if conf > bestConf {
			bestDeg, bestConf = deg, conf
		}
		if conf >= 3 {
			break
		}
	}
	if bestDeg != 0 && bestConf >= 1.5 {
		g = rotate90s(g, bestDeg)
		tr.Rotate = bestDeg
	}
	stamp("osd")
	// Deskew dengan verifikasi: putar, ukur ulang; bila tidak membaik, pakai arah sebaliknya.
	if sk := estimateSkew(g); sk != 0 {
		a := rotateDeg(g, -sk)
		if math.Abs(estimateSkew(a)) > math.Abs(sk)/2 {
			if b := rotateDeg(g, sk); math.Abs(estimateSkew(b)) < math.Abs(estimateSkew(a)) {
				a, sk = b, -sk
			}
		}
		g = a
		tr.DeskewDeg = sk
	}
	stamp("deskew")
	tr.Width, tr.Height = g.Rect.Dx(), g.Rect.Dy()
	proc := filepath.Join(e.dir, fmt.Sprintf("proc-%d.png", p.No))
	if err := savePNG(proc, g); err != nil {
		return ocrResult{}, err
	}
	stamp("save")
	defer os.Remove(proc)

	// Percobaan berbatas: default, threshold Sauvola (bayangan/kontras tak rata), segmentasi kolom tunggal.
	attempts := []struct{ psm, thresh string }{{"3", ""}, {"3", "2"}, {"4", "2"}}
	var best ocrResult
	for i, a := range attempts {
		if i >= e.lim.MaxOCRAttempts {
			break
		}
		var extra []string
		if a.thresh != "" {
			extra = []string{"-c", "thresholding_method=" + a.thresh}
		}
		res, err := e.runOCR(ctx, proc, a.psm, extra...)
		if err != nil {
			if i == 0 {
				return ocrResult{}, err
			}
			break
		}
		p.Attempts = append(p.Attempts, fmt.Sprintf("ocr#%d psm=%s thr=%s words=%d conf=%.0f", i+1, a.psm, a.thresh, res.Words, res.MeanConf))
		if res.Words > 0 && (best.Words == 0 || res.MeanConf*math.Log(float64(res.Words)+1) > best.MeanConf*math.Log(float64(best.Words)+1)) {
			best = res
		}
		if best.MeanConf >= 75 && best.Words >= 15 {
			break
		}
	}
	stamp("ocr")
	// Verifikasi orientasi lewat mutu OCR: bila rendah, coba sekali orientasi alternatif.
	if best.MeanConf < 60 {
		alt := 180
		if bestDeg != 0 && tr.Rotate == 0 {
			alt = bestDeg
		}
		cand := rotate90s(g, alt)
		altPath := filepath.Join(e.dir, fmt.Sprintf("alt-%d.png", p.No))
		if err := savePNG(altPath, cand); err == nil {
			res, err := e.runOCR(ctx, altPath, "3")
			_ = os.Remove(altPath)
			if err == nil {
				p.Attempts = append(p.Attempts, fmt.Sprintf("ocr-alt rotate=%d words=%d conf=%.0f", alt, res.Words, res.MeanConf))
				if res.MeanConf > best.MeanConf+10 && res.Words > 0 {
					best, g = res, cand
					tr.Rotate = (tr.Rotate + alt) % 360
					tr.Width, tr.Height = g.Rect.Dx(), g.Rect.Dy()
				}
			}
		}
	}
	if best.Words == 0 {
		return ocrResult{}, fail(ReasonNoText, "ocr tanpa kata")
	}
	if tr.Rotate != 0 || tr.DeskewDeg != 0 || p.Image == "" || e.format != "pdf" {
		prev, f := downscale(g, 1400)
		name := fmt.Sprintf("page-%d.jpg", p.No)
		if err := saveJPEG(filepath.Join(e.outDir, name), prev); err == nil {
			p.Image = name
			if e.format == "pdf" {
				// satuan BBox halaman PDF = point; render OCR = OCRDPI.
				p.Scale = (1 / f) * float64(e.lim.OCRDPI) / 72
				p.W = float64(tr.Width) * 72 / float64(e.lim.OCRDPI)
				p.H = float64(tr.Height) * 72 / float64(e.lim.OCRDPI)
			} else {
				p.Scale = 1 / f
				p.W, p.H = float64(tr.Width), float64(tr.Height)
			}
		}
	}
	e.transforms[p.No] = tr
	return best, nil
}

func savePNGTo(w io.Writer, img image.Image) error {
	return (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(w, img)
}
