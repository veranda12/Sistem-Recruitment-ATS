package cvparse

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

// Urutan baca dibentuk dari hubungan spasial, bukan sort Y global:
//  1. buang header/footer yang berulang antarhalaman,
//  2. cari gutter kolom (posisi x yang paling sedikit dilintasi baris),
//  3. baris yang melintasi gutter (nama, heading lebar) menjadi pemisah wilayah,
//  4. di setiap wilayah: kolom kiri dari atas ke bawah, lalu kolom kanan.

var pageNumRe = regexp.MustCompile(`(?i)^(?:hal(?:aman)?\.?\s*|page\s*)?\d{1,2}(?:\s*(?:/|dari|of)\s*\d{1,2})?$`)

// Ordered adalah baris dalam urutan baca beserta kolomnya (0 = lebar penuh/tanpa kolom).
type Ordered struct {
	Lines []*Line
}

// ReadingOrder menghasilkan urutan baca seluruh dokumen.
func ReadingOrder(d *Doc) Ordered {
	repeated := repeatedMargins(d)
	var out []*Line
	for _, p := range d.Pages {
		if p.Status != "ok" {
			continue
		}
		var body []*Line
		for _, l := range p.Lines {
			if strings.TrimSpace(l.Text()) == "" || l.MergedInto != "" {
				continue
			}
			if l.Source != "docx" && p.H > 0 && (l.B.Y0 < p.H*0.06 || l.B.Y1 > p.H*0.94) {
				if repeated[normKey(l.Text())] || pageNumRe.MatchString(strings.TrimSpace(l.Text())) {
					continue
				}
			}
			body = append(body, l)
		}
		if p.Method == "docx" {
			out = append(out, body...) // urutan dokumen Word sudah semantik
			continue
		}
		out = append(out, orderPage(p, body)...)
	}
	return Ordered{Lines: out}
}

// repeatedMargins: teks di margin atas/bawah yang muncul di >=2 halaman.
func repeatedMargins(d *Doc) map[string]bool {
	count := map[string]int{}
	for _, p := range d.Pages {
		seen := map[string]bool{}
		for _, l := range p.Lines {
			if p.H > 0 && (l.B.Y0 < p.H*0.06 || l.B.Y1 > p.H*0.94) {
				k := normKey(l.Text())
				if k != "" && !seen[k] {
					seen[k] = true
					count[k]++
				}
			}
		}
	}
	out := map[string]bool{}
	for k, n := range count {
		if n >= 2 {
			out[k] = true
		}
	}
	return out
}

func orderPage(p *Page, lines []*Line) []*Line {
	byY := func(ls []*Line) {
		sort.SliceStable(ls, func(i, j int) bool {
			a, b := ls[i].B, ls[j].B
			if a.overlapY(b) > 0.5*min(a.H(), b.H()) {
				return a.X0 < b.X0
			}
			return a.Y0 < b.Y0
		})
	}
	gx, ok := findGutter(p, lines)
	if !ok {
		byY(lines)
		for _, l := range lines {
			l.Column = 0
		}
		return lines
	}
	// Baris melintasi gutter = lebar penuh; menjadi pemisah wilayah.
	var spans, left, right []*Line
	for _, l := range lines {
		switch {
		case l.B.X0 < gx-2 && l.B.X1 > gx+2:
			l.Column = 0
			spans = append(spans, l)
		case l.B.X1 <= gx+2:
			l.Column = 1
			left = append(left, l)
		default:
			l.Column = 2
			right = append(right, l)
		}
	}
	// Heading hanya di sisi kiri dan sisi kanan tanpa heading = satu kolom berisi tabel
	// (mis. periode di sel kiri, jabatan di sel kanan), bukan dua kolom halaman.
	if (headingCount(left) > 0 && headingCount(right) == 0) || isTableRows(left, right) {
		byY(left)
		left = joinWrappedCells(left, right)
		rows := mergeRows(left, right)
		rows = append(rows, spans...)
		byY(rows)
		for _, l := range rows {
			l.Column = 0
		}
		return rows
	}
	byY(spans)
	byY(left)
	byY(right)
	var out []*Line
	prev := -1.0
	emit := func(limit float64) {
		for _, side := range [][]*Line{left, right} {
			for _, l := range side {
				c := (l.B.Y0 + l.B.Y1) / 2
				if c > prev && c <= limit {
					out = append(out, l)
				}
			}
		}
	}
	for _, s := range spans {
		emit((s.B.Y0 + s.B.Y1) / 2)
		out = append(out, s)
		prev = (s.B.Y0 + s.B.Y1) / 2
	}
	emit(1e18)
	return out
}

// findGutter mencari pita x di bagian tengah halaman yang paling sedikit dilintasi baris; gutter = tengah
// pita kosong terlebar (bukan titik yang kebetulan seimbang). Baris lebar penuh (<=20%) boleh melintas.
func findGutter(p *Page, lines []*Line) (float64, bool) {
	if len(lines) < 8 || p.W <= 0 {
		return 0, false
	}
	step := p.W / 300
	type cand struct {
		x           float64
		cross, l, r int
	}
	var cs []cand
	minCross := len(lines) + 1
	for x := p.W * 0.2; x <= p.W*0.8; x += step {
		c := cand{x: x}
		for _, ln := range lines {
			switch {
			case ln.B.X0 < x-1 && ln.B.X1 > x+1:
				c.cross++
			case ln.B.X1 <= x+1:
				c.l++
			default:
				c.r++
			}
		}
		if min(c.l, c.r) < 4 {
			continue
		}
		cs = append(cs, c)
		minCross = min(minCross, c.cross)
	}
	if len(cs) == 0 || minCross > len(lines)/5 {
		return 0, false
	}
	// pita kontigu yang hanya dilintasi segelintir baris lebar penuh (nama, heading); pilih yang terlebar
	tol := max(minCross, max(1, len(lines)/20))
	bestW, bestX := 0.0, 0.0
	for i := 0; i < len(cs); {
		if cs[i].cross > tol {
			i++
			continue
		}
		j := i
		for j+1 < len(cs) && cs[j+1].cross <= tol && cs[j+1].x-cs[j].x < step*1.5 {
			j++
		}
		w := cs[j].x - cs[i].x
		if w > bestW || bestX == 0 {
			bestW, bestX = w, (cs[i].x+cs[j].x)/2
		}
		i = j + 1
	}
	if bestW < p.W*0.008 {
		return 0, false // tidak ada celah nyata
	}
	var lTop, lBot, rTop, rBot = 1e18, -1.0, 1e18, -1.0
	for _, ln := range lines {
		if ln.B.X0 < bestX-1 && ln.B.X1 > bestX+1 {
			continue
		}
		if ln.B.X1 <= bestX+1 {
			lTop, lBot = min(lTop, ln.B.Y0), max(lBot, ln.B.Y1)
		} else {
			rTop, rBot = min(rTop, ln.B.Y0), max(rBot, ln.B.Y1)
		}
	}
	// Sidebar pendek tetap kolom: cukup bertumpuk vertikal >= 6% tinggi halaman.
	if min(lBot, rBot)-max(lTop, rTop) < p.H*0.06 {
		return 0, false
	}
	return bestX, true
}

func headingCount(ls []*Line) int {
	n := 0
	for _, l := range ls {
		if _, ok := detectHeading(l); ok {
			n++
		}
	}
	return n
}

// isTableRows: kolom kiri jarang dan hampir setiap barisnya sejajar dengan baris kanan
// (mis. kolom periode di samping kolom uraian) => tabel, bukan kolom halaman.
func isTableRows(left, right []*Line) bool {
	if len(left) == 0 || len(left)*2 > len(right) {
		return false
	}
	aligned := 0
	for _, l := range left {
		for _, r := range right {
			if l.B.overlapY(r.B) > 0.6*min(l.B.H(), r.B.H()) {
				aligned++
				break
			}
		}
	}
	return float64(aligned) >= 0.8*float64(len(left))
}

// joinWrappedCells menyatukan potongan teks sel kiri yang terbungkus (mis. "Agustus 2020 s.d. saat" + "ini")
// ke baris di atasnya, hanya bila baris di atasnya sejajar dengan baris kanan (sel tabel) dan potongannya
// pendek serta tepat di bawahnya. Idempoten.
func joinWrappedCells(left, right []*Line) []*Line {
	aligned := func(l *Line) bool {
		for _, r := range right {
			if l.B.overlapY(r.B) > 0.6*min(l.B.H(), r.B.H()) {
				return true
			}
		}
		return false
	}
	var out []*Line
	for _, l := range left {
		if l.MergedInto != "" {
			continue
		}
		if k := len(out); k > 0 {
			prev := out[k-1]
			gap := l.B.Y0 - prev.B.Y1
			if prev.MergedFrom == "" && aligned(prev) && len(l.Words) <= 2 && gap >= -1 && gap < 0.6*prev.B.H() && math.Abs(l.B.X0-prev.B.X0) < 3 {
				l.MergedInto = prev.ID
				prev.Words = append(prev.Words, l.Words...)
				prev.B = prev.B.Union(l.B)
				prev.text = ""
				continue
			}
		}
		out = append(out, l)
	}
	return out
}

// mergeRows menggabungkan baris kiri ke baris kanan yang sejajar ("kiri | kanan") di tempat, sehingga
// ID bukti menunjuk teks gabungan yang sama dengan yang ditampilkan. Idempoten: aman dipanggil ulang.
func mergeRows(left, right []*Line) []*Line {
	var out []*Line
	for _, r := range right {
		if r.MergedFrom != "" {
			out = append(out, r)
			continue
		}
		for _, l := range left {
			if l.MergedInto == "" && l.B.overlapY(r.B) > 0.6*min(l.B.H(), r.B.H()) {
				l.MergedInto, r.MergedFrom = r.ID, l.ID
				r.Words = append(append(append([]Word{}, l.Words...), Word{Text: "|", B: BBox{l.B.X1, l.B.Y0, l.B.X1, l.B.Y1}, Conf: -1}), r.Words...)
				r.B = l.B.Union(r.B)
				r.Conf = min(l.Conf, r.Conf)
				r.text = ""
				break
			}
		}
		out = append(out, r)
	}
	for _, l := range left {
		if l.MergedInto == "" {
			out = append(out, l)
		}
	}
	return out
}

// DebugOrder: baris dalam urutan baca beserta kolom dan section (hanya untuk alat pengembang).
func DebugOrder(d *Doc) []string {
	var out []string
	for _, sl := range sections(ReadingOrder(d)) {
		l := sl.l
		out = append(out, fmt.Sprintf("p%d c%d %-14s %-6s conf=%3.0f y=%6.1f x=%6.1f-%6.1f | %s", l.Page, l.Column, string(sl.sec), l.Source, l.Conf, l.B.Y0, l.B.X0, l.B.X1, l.Text()))
	}
	return out
}
