package cvfixture

import (
	"bytes"
	"fmt"
	"strings"
)

// Penulis PDF minimal: font base-14 Helvetica (WinAnsi), gambar JPEG, teks tak terlihat (render mode 3).
// Cukup untuk fixture yang dapat direproduksi tanpa dependensi luar.

const pageW, pageH = 595.0, 842.0

type pdfImage struct {
	name         string
	data         []byte
	w, h         int
	gray         bool
	x, y, dw, dh float64
}

type pdfPage struct {
	content bytes.Buffer
	images  []pdfImage
}

type PDF struct {
	pages []*pdfPage
}

func (d *PDF) NewPage() *pdfPage {
	p := &pdfPage{}
	d.pages = append(d.pages, p)
	return p
}

var winAnsi = map[rune]byte{'–': 0x96, '—': 0x97, '•': 0x95, '’': 0x92, '‘': 0x91, '“': 0x93, '”': 0x94, 'é': 0xE9, '·': 0xB7}

func encodeWinAnsi(s string) string {
	var b strings.Builder
	for _, r := range s {
		var c byte
		switch {
		case r < 128:
			c = byte(r)
		default:
			v, ok := winAnsi[r]
			if !ok {
				v = '?'
			}
			c = v
		}
		switch c {
		case '(', ')', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			if c >= 128 {
				fmt.Fprintf(&b, "\\%03o", c)
			} else {
				b.WriteByte(c)
			}
		}
	}
	return b.String()
}

// Text menulis teks dengan y dihitung dari atas halaman (baseline = y + size*0.8).
func (p *pdfPage) Text(x, y, size float64, bold bool, s string) {
	p.text(x, y, size, bold, s, 0, 0)
}

// Gray menulis teks abu-abu (level 0..1).
func (p *pdfPage) Gray(x, y, size float64, bold bool, s string, level float64) {
	p.text(x, y, size, bold, s, 0, level)
}

// Invisible menulis teks tak terlihat (seperti layer OCR tersembunyi).
func (p *pdfPage) Invisible(x, y, size float64, s string) {
	p.text(x, y, size, false, s, 3, 0)
}

func (p *pdfPage) text(x, y, size float64, bold bool, s string, mode int, gray float64) {
	f := "F1"
	if bold {
		f = "F2"
	}
	base := pageH - y - size*0.8
	fmt.Fprintf(&p.content, "BT %.3f g /%s %.2f Tf %d Tr %.2f %.2f Td (%s) Tj ET\n", gray, f, size, mode, x, base, encodeWinAnsi(s))
}

func (p *pdfPage) Rect(x, y, w, h, gray float64) {
	fmt.Fprintf(&p.content, "%.3f g %.2f %.2f %.2f %.2f re f\n", gray, x, pageH-y-h, w, h)
}

// Image menempatkan JPEG pada kotak (x,y dari atas).
func (p *pdfPage) Image(data []byte, w, h int, gray bool, x, y, dw, dh float64) {
	name := fmt.Sprintf("Im%d", len(p.images)+1)
	p.images = append(p.images, pdfImage{name, data, w, h, gray, x, y, dw, dh})
	fmt.Fprintf(&p.content, "q %.2f 0 0 %.2f %.2f %.2f cm /%s Do Q\n", dw, dh, x, pageH-y-dh, name)
}

// Bytes menulis dokumen lengkap dengan tabel xref.
func (d *PDF) Bytes() []byte {
	var out bytes.Buffer
	var offs []int
	obj := func(body string) int {
		offs = append(offs, out.Len())
		n := len(offs)
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", n, body)
		return n
	}
	stream := func(dict string, data []byte) int {
		offs = append(offs, out.Len())
		n := len(offs)
		fmt.Fprintf(&out, "%d 0 obj\n<< %s /Length %d >>\nstream\n", n, dict, len(data))
		out.Write(data)
		out.WriteString("\nendstream\nendobj\n")
		return n
	}
	out.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	catalog := obj("<< /Type /Catalog /Pages 2 0 R >>")
	_ = catalog
	// placeholder Pages (objek 2) ditulis ulang di akhir; simpan indeks
	offs = append(offs, -1)
	f1 := obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	f2 := obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")
	var kids []string
	for _, p := range d.pages {
		var xo []string
		for _, im := range p.images {
			cs := "/DeviceRGB"
			if im.gray {
				cs = "/DeviceGray"
			}
			n := stream(fmt.Sprintf("/Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace %s /BitsPerComponent 8 /Filter /DCTDecode", im.w, im.h, cs), im.data)
			xo = append(xo, fmt.Sprintf("/%s %d 0 R", im.name, n))
		}
		c := stream("", p.content.Bytes())
		res := fmt.Sprintf("<< /Font << /F1 %d 0 R /F2 %d 0 R >> /XObject << %s >> >>", f1, f2, strings.Join(xo, " "))
		pg := obj(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.0f %.0f] /Resources %s /Contents %d 0 R >>", pageW, pageH, res, c))
		kids = append(kids, fmt.Sprintf("%d 0 R", pg))
	}
	offs[1] = out.Len()
	fmt.Fprintf(&out, "2 0 obj\n<< /Type /Pages /Kids [%s] /Count %d >>\nendobj\n", strings.Join(kids, " "), len(kids))
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offs)+1)
	for _, o := range offs {
		fmt.Fprintf(&out, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offs)+1, xref)
	return out.Bytes()
}

// Lebar glyph Helvetica (1/1000 em) untuk pembungkusan baris.
var helvW = [95]int{278, 278, 355, 556, 556, 889, 667, 191, 333, 333, 389, 584, 278, 333, 278, 278, 556, 556, 556, 556, 556, 556, 556, 556, 556, 556,
	278, 278, 584, 584, 584, 556, 1015, 667, 667, 722, 722, 667, 611, 778, 722, 278, 500, 667, 556, 833, 722, 778, 667, 778, 722, 667, 611, 722, 667,
	944, 667, 667, 611, 278, 278, 278, 469, 556, 333, 556, 556, 500, 556, 556, 278, 556, 556, 222, 222, 500, 222, 833, 556, 556, 556, 556, 333, 500,
	278, 556, 500, 722, 500, 500, 500, 334, 260, 334, 584}

func textWidth(s string, size float64, bold bool) float64 {
	var w float64
	for _, r := range s {
		if r >= 32 && r < 127 {
			w += float64(helvW[r-32])
		} else {
			w += 556
		}
	}
	if bold {
		w *= 1.07
	}
	return w * size / 1000
}

// wrap memecah teks agar muat lebar maxW.
func wrap(s string, size, maxW float64, bold bool) []string {
	var lines []string
	cur := ""
	for _, w := range strings.Fields(s) {
		try := strings.TrimSpace(cur + " " + w)
		if cur != "" && textWidth(try, size, bold) > maxW {
			lines = append(lines, cur)
			cur = w
			continue
		}
		cur = try
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}
