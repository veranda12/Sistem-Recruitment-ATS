package cvfixture

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
)

// RenderPDFPage merender halaman PDF ke gambar grayscale memakai pdftoppm.
func RenderPDFPage(pdftoppm string, pdf []byte, page, dpi int, tmp string) (*image.Gray, error) {
	in := filepath.Join(tmp, "render-in.pdf")
	if err := os.WriteFile(in, pdf, 0o600); err != nil {
		return nil, err
	}
	base := filepath.Join(tmp, fmt.Sprintf("render-%d", page))
	cmd := exec.Command(pdftoppm, "-r", fmt.Sprint(dpi), "-f", fmt.Sprint(page), "-l", fmt.Sprint(page), "-gray", "-png", "-singlefile", in, base)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("pdftoppm: %v %s", err, out)
	}
	f, err := os.ReadFile(base + ".png")
	if err != nil {
		return nil, err
	}
	img, err := png.Decode(bytes.NewReader(f))
	if err != nil {
		return nil, err
	}
	g := image.NewGray(img.Bounds())
	draw.Draw(g, g.Bounds(), img, img.Bounds().Min, draw.Src)
	return g, nil
}

func clamp(v float64) uint8 { return uint8(math.Max(0, math.Min(255, v))) }

// Degrade meniru scanner: noise ringan, sedikit miring, kontras turun.
func Degrade(g *image.Gray, seed int64, skewDeg, noise, contrast float64) *image.Gray {
	r := rand.New(rand.NewSource(seed))
	o := rotate(g, skewDeg, 245)
	for i := range o.Pix {
		v := float64(o.Pix[i])
		v = 128 + (v-128)*contrast + r.NormFloat64()*noise
		o.Pix[i] = clamp(v)
	}
	return o
}

// Photo meniru foto ponsel: kertas di atas meja, miring, bayangan gradasi, noise, sedikit blur.
func Photo(g *image.Gray, seed int64, skewDeg, blur float64) *image.Gray {
	r := rand.New(rand.NewSource(seed))
	w, h := g.Rect.Dx(), g.Rect.Dy()
	pad := w / 12
	canvas := image.NewGray(image.Rect(0, 0, w+2*pad, h+2*pad))
	for i := range canvas.Pix {
		canvas.Pix[i] = 95 // meja
	}
	draw.Draw(canvas, image.Rect(pad, pad, pad+w, pad+h), g, image.Point{}, draw.Src)
	o := rotate(canvas, skewDeg, 95)
	ow, oh := o.Rect.Dx(), o.Rect.Dy()
	for y := 0; y < oh; y++ {
		for x := 0; x < ow; x++ {
			// bayangan diagonal dari kiri atas (tangan/ponsel)
			s := 0.62 + 0.38*math.Min(1, (float64(x)/float64(ow)+float64(y)/float64(oh))/1.1)
			v := float64(o.Pix[y*o.Stride+x])*s + r.NormFloat64()*6
			o.Pix[y*o.Stride+x] = clamp(v)
		}
	}
	if blur > 0 {
		o = boxBlur(o, int(blur))
	}
	return o
}

func boxBlur(g *image.Gray, rad int) *image.Gray {
	if rad <= 0 {
		return g
	}
	w, h := g.Rect.Dx(), g.Rect.Dy()
	tmp := image.NewGray(g.Rect)
	out := image.NewGray(g.Rect)
	for y := 0; y < h; y++ {
		s, n := 0, 0
		for x := -rad; x < w; x++ {
			if x+rad < w {
				s += int(g.Pix[y*g.Stride+x+rad])
				n++
			}
			if x-rad-1 >= 0 {
				s -= int(g.Pix[y*g.Stride+x-rad-1])
				n--
			}
			if x >= 0 {
				tmp.Pix[y*tmp.Stride+x] = uint8(s / n)
			}
		}
	}
	for x := 0; x < w; x++ {
		s, n := 0, 0
		for y := -rad; y < h; y++ {
			if y+rad < h {
				s += int(tmp.Pix[(y+rad)*tmp.Stride+x])
				n++
			}
			if y-rad-1 >= 0 {
				s -= int(tmp.Pix[(y-rad-1)*tmp.Stride+x])
				n--
			}
			if y >= 0 {
				out.Pix[y*out.Stride+x] = uint8(s / n)
			}
		}
	}
	return out
}

func rotate(g *image.Gray, deg float64, fill uint8) *image.Gray {
	if deg == 0 {
		c := image.NewGray(g.Rect)
		copy(c.Pix, g.Pix)
		return c
	}
	a := deg * math.Pi / 180
	sin, cos := math.Sin(a), math.Cos(a)
	w, h := float64(g.Rect.Dx()), float64(g.Rect.Dy())
	o := image.NewGray(g.Rect)
	cx, cy := w/2, h/2
	for y := 0; y < int(h); y++ {
		for x := 0; x < int(w); x++ {
			dx, dy := float64(x)-cx, float64(y)-cy
			sx, sy := int(dx*cos+dy*sin+cx), int(-dx*sin+dy*cos+cy)
			if sx < 0 || sy < 0 || sx >= int(w) || sy >= int(h) {
				o.Pix[y*o.Stride+x] = fill
				continue
			}
			o.Pix[y*o.Stride+x] = g.Pix[sy*g.Stride+sx]
		}
	}
	return o
}

// Rot90s memutar searah jarum jam.
func Rot90s(g *image.Gray, deg int) *image.Gray {
	w, h := g.Rect.Dx(), g.Rect.Dy()
	switch deg {
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
		return Rot90s(Rot90s(Rot90s(g, 90), 90), 90)
	}
	return g
}

func EncodeJPEG(g image.Image, q int) []byte {
	var b bytes.Buffer
	_ = jpeg.Encode(&b, g, &jpeg.Options{Quality: q})
	return b.Bytes()
}

func EncodePNG(g image.Image) []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, g)
	return b.Bytes()
}

// WithEXIFOrientation menyisipkan segmen APP1 Exif berisi tag Orientation.
func WithEXIFOrientation(jpg []byte, orientation uint16) []byte {
	var t bytes.Buffer
	t.WriteString("II*\x00")
	binary.Write(&t, binary.LittleEndian, uint32(8))
	binary.Write(&t, binary.LittleEndian, uint16(1))
	binary.Write(&t, binary.LittleEndian, uint16(0x0112))
	binary.Write(&t, binary.LittleEndian, uint16(3))
	binary.Write(&t, binary.LittleEndian, uint32(1))
	binary.Write(&t, binary.LittleEndian, orientation)
	binary.Write(&t, binary.LittleEndian, uint16(0))
	binary.Write(&t, binary.LittleEndian, uint32(0))
	payload := append([]byte("Exif\x00\x00"), t.Bytes()...)
	seg := []byte{0xFF, 0xE1, byte((len(payload) + 2) >> 8), byte(len(payload) + 2)}
	out := append([]byte{}, jpg[:2]...)
	out = append(out, seg...)
	out = append(out, payload...)
	return append(out, jpg[2:]...)
}

// ImagePDF membuat PDF dari gambar (scan), opsional dengan layer teks tersembunyi yang buruk.
func ImagePDF(pages []*image.Gray, garbage map[int]string) []byte {
	d := &PDF{}
	for i, g := range pages {
		p := d.NewPage()
		p.Image(EncodeJPEG(g, 72), g.Rect.Dx(), g.Rect.Dy(), true, 0, 0, pageW, pageH)
		if s, ok := garbage[i+1]; ok {
			y := 60.0
			for _, l := range wrap(s, 10, 500, false) {
				p.Invisible(40, y, 10, l)
				y += 13
			}
		}
	}
	return d.Bytes()
}
