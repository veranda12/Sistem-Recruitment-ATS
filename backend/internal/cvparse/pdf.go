package cvparse

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var (
	pagesRe    = regexp.MustCompile(`(?m)^Pages:\s+(\d+)`)
	pageSizeRe = regexp.MustCompile(`(?m)^Page\s+(\d+)\s+size:\s+([\d.]+)\s+x\s+([\d.]+)`)
	pageRotRe  = regexp.MustCompile(`(?m)^Page\s+(\d+)\s+rot:\s+(\d+)`)
)

type pdfInfo struct {
	pages int
	sizes map[int][2]float64
	rot   map[int]int
}

func (e *engine) pdfInfo(ctx context.Context, path string) (pdfInfo, error) {
	out, err := run(ctx, e.lim.StageInfo, e.dir, e.env(), 1<<20, e.tools.PDFInfo, "-f", "1", "-l", strconv.Itoa(e.lim.MaxPages+1), path)
	if err != nil {
		var te *toolError
		if errors.As(err, &te) {
			s := strings.ToLower(te.stderr)
			if strings.Contains(s, "password") || strings.Contains(s, "encrypt") {
				return pdfInfo{}, fail(ReasonEncrypted, "pdf terkunci")
			}
			return pdfInfo{}, fail(ReasonCorrupt, "pdfinfo gagal")
		}
		return pdfInfo{}, err
	}
	m := pagesRe.FindSubmatch(out)
	if m == nil {
		return pdfInfo{}, fail(ReasonCorrupt, "jumlah halaman tidak terbaca")
	}
	n, _ := strconv.Atoi(string(m[1]))
	info := pdfInfo{pages: n, sizes: map[int][2]float64{}, rot: map[int]int{}}
	for _, s := range pageSizeRe.FindAllSubmatch(out, -1) {
		p, _ := strconv.Atoi(string(s[1]))
		w, _ := strconv.ParseFloat(string(s[2]), 64)
		h, _ := strconv.ParseFloat(string(s[3]), 64)
		info.sizes[p] = [2]float64{w, h}
	}
	for _, s := range pageRotRe.FindAllSubmatch(out, -1) {
		p, _ := strconv.Atoi(string(s[1]))
		r, _ := strconv.Atoi(string(s[2]))
		info.rot[p] = r
	}
	return info, nil
}

// bbox-layout pdftotext: page > flow > block > line > word.
type bbDoc struct {
	Pages []bbPage `xml:"body>doc>page"`
}
type bbPage struct {
	W     float64  `xml:"width,attr"`
	H     float64  `xml:"height,attr"`
	Flows []bbFlow `xml:"flow"`
}
type bbFlow struct {
	Blocks []bbBlock `xml:"block"`
}
type bbBlock struct {
	Lines []bbLine `xml:"line"`
}
type bbLine struct {
	Words []bbWord `xml:"word"`
}
type bbWord struct {
	X0   float64 `xml:"xMin,attr"`
	Y0   float64 `xml:"yMin,attr"`
	X1   float64 `xml:"xMax,attr"`
	Y1   float64 `xml:"yMax,attr"`
	Text string  `xml:",chardata"`
}

func (e *engine) pdfText(ctx context.Context, path string, pages int) ([]*Page, error) {
	out := filepath.Join(e.dir, "bbox.html")
	if _, err := run(ctx, e.lim.StageText, e.dir, e.env(), e.lim.MaxOutputBytes, e.tools.PDFToText, "-bbox-layout", "-enc", "UTF-8", "-f", "1", "-l", strconv.Itoa(pages), path, out); err != nil {
		var te *toolError
		if errors.As(err, &te) {
			return nil, fail(ReasonCorrupt, "pdftotext gagal")
		}
		return nil, err
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		return nil, err
	}
	_ = os.Remove(out)
	dec := xml.NewDecoder(bytes.NewReader(raw))
	dec.Strict = false
	dec.Entity = xml.HTMLEntity
	var d bbDoc
	if err := dec.Decode(&d); err != nil {
		return nil, fail(ReasonCorrupt, "bbox tidak terbaca")
	}
	var res []*Page
	for i, p := range d.Pages {
		pg := &Page{No: i + 1, W: p.W, H: p.H, Status: "ok"}
		n := 0
		for _, f := range p.Flows {
			for _, b := range f.Blocks {
				for _, l := range b.Lines {
					// Celah horizontal besar di dalam satu baris = sel/kolom berbeda: pecah menjadi baris terpisah.
					ln := &Line{Page: pg.No, Source: "native", Conf: -1}
					flush := func() {
						if len(ln.Words) > 0 {
							n++
							ln.ID = lineID(pg.No, n)
							pg.Lines = append(pg.Lines, ln)
						}
						ln = &Line{Page: pg.No, Source: "native", Conf: -1}
					}
					for _, w := range l.Words {
						t := strings.TrimSpace(w.Text)
						if t == "" {
							continue
						}
						wd := Word{Text: t, B: BBox{w.X0, w.Y0, w.X1, w.Y1}, Conf: -1}
						if k := len(ln.Words); k > 0 && wd.B.X0-ln.Words[k-1].B.X1 > max(5, 0.6*wd.B.H()) {
							flush()
						}
						ln.Words = append(ln.Words, wd)
						ln.B = ln.B.Union(wd.B)
					}
					flush()
				}
			}
		}
		res = append(res, pg)
	}
	return res, nil
}

// pageImg: jumlah gambar bermakna dan cakupan luas halaman oleh gambar (0..1).
type pageImg struct {
	count    int
	coverage float64
}

// pdfImages membaca daftar gambar per halaman lewat pdfimages -list (gambar >= 100x100 px).
// Bila tool tidak ada, kembalikan nil (pemanggil menganggap setiap halaman mungkin bergambar).
func (e *engine) pdfImages(ctx context.Context, path string, pages int, sizes map[int][2]float64) map[int]pageImg {
	if e.tools.PDFImages == "" {
		return nil
	}
	out, err := run(ctx, e.lim.StageInfo, e.dir, e.env(), 1<<20, e.tools.PDFImages, "-list", "-f", "1", "-l", strconv.Itoa(pages), path)
	if err != nil {
		return nil
	}
	m := map[int]pageImg{}
	for ln := range strings.Lines(string(out)) {
		f := strings.Fields(ln)
		if len(f) < 14 || f[2] != "image" {
			continue
		}
		pg, err1 := strconv.Atoi(f[0])
		w, err2 := strconv.Atoi(f[3])
		h, err3 := strconv.Atoi(f[4])
		xp, err4 := strconv.ParseFloat(f[12], 64)
		yp, err5 := strconv.ParseFloat(f[13], 64)
		if err1 != nil || err2 != nil || err3 != nil || w*h < 10000 {
			continue
		}
		pi := m[pg]
		pi.count++
		if err4 == nil && err5 == nil && xp > 0 && yp > 0 {
			if sz, ok := sizes[pg]; ok && sz[0] > 0 && sz[1] > 0 {
				pi.coverage += (float64(w) / xp * 72) * (float64(h) / yp * 72) / (sz[0] * sz[1])
			}
		}
		m[pg] = pi
	}
	return m
}

// vocabOverlap: proporsi kata OCR (>=3 huruf) yang juga ada di layer teks.
func vocabOverlap(native, ocr []*Line) float64 {
	set := map[string]bool{}
	for _, l := range native {
		for _, w := range l.Words {
			set[strings.ToLower(strings.Trim(w.Text, ".,:;()|"))] = true
		}
	}
	hit, n := 0, 0
	for _, l := range ocr {
		for _, w := range l.Words {
			t := strings.ToLower(strings.Trim(w.Text, ".,:;()|"))
			if len([]rune(t)) < 3 {
				continue
			}
			n++
			if set[t] {
				hit++
			}
		}
	}
	if n == 0 {
		return 1
	}
	return float64(hit) / float64(n)
}

// inkRatio: proporsi piksel gelap pada preview (halaman kosong ~0).
func inkRatio(path string) float64 {
	f, err := os.Open(path)
	if err != nil {
		return 1
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return 1
	}
	g := toGray(img)
	dark := 0
	for _, v := range g.Pix {
		if v < 160 {
			dark++
		}
	}
	return float64(dark) / float64(max(1, len(g.Pix)))
}

// assessQuality menilai layer teks; duplikat = kata sama yang bertumpuk >60% dengan kata sebelumnya.
func assessQuality(p *Page) Quality {
	var q Quality
	var letters, repl, total int
	type wk struct {
		t string
		b BBox
	}
	var words []wk
	dup := 0
	for _, l := range p.Lines {
		for _, w := range l.Words {
			q.Words++
			for _, r := range w.Text {
				total++
				switch {
				case r == unicode.ReplacementChar || (r >= 0xE000 && r <= 0xF8FF) || unicode.IsControl(r):
					repl++
				case unicode.IsLetter(r) || unicode.IsDigit(r):
					letters++
				}
			}
			for _, o := range words {
				if o.t == w.Text && w.B.overlapX(o.b)*w.B.overlapY(o.b) > 0.6*math.Max(1, w.B.W()*w.B.H()) {
					dup++
					break
				}
			}
			words = append(words, wk{w.Text, w.B})
		}
	}
	q.Chars = total
	if total > 0 {
		q.ReplacementRate = float64(repl) / float64(total)
		q.LetterRate = float64(letters) / float64(total)
	}
	if q.Words > 0 {
		q.DuplicateRate = float64(dup) / float64(q.Words)
	}
	q.Score = math.Min(1, float64(q.Words)/40) * (1 - q.ReplacementRate*5) * q.LetterRate
	return q
}

// dedupeWords membuang kata ganda yang bertumpuk (layer teks tersembunyi ganda).
func dedupeWords(p *Page) int {
	removed := 0
	var kept []Word
	for _, l := range p.Lines {
		var ws []Word
		for _, w := range l.Words {
			isDup := false
			for _, o := range kept {
				if o.Text == w.Text && w.B.overlapX(o.B)*w.B.overlapY(o.B) > 0.6*math.Max(1, w.B.W()*w.B.H()) {
					isDup = true
					break
				}
			}
			if isDup {
				removed++
				continue
			}
			kept = append(kept, w)
			ws = append(ws, w)
		}
		l.Words = ws
		l.text = ""
		l.B = BBox{}
		for _, w := range ws {
			l.B = l.B.Union(w.B)
		}
	}
	var lines []*Line
	for _, l := range p.Lines {
		if len(l.Words) > 0 {
			lines = append(lines, l)
		}
	}
	p.Lines = lines
	return removed
}

// decideNative: alasan stabil mengapa halaman memakai teks native atau OCR.
func decideNative(q Quality) (bool, string) {
	switch {
	case q.Words == 0:
		return false, "no_text_layer"
	case q.ReplacementRate > 0.05:
		return false, "text_layer_unmapped_glyphs"
	case q.LetterRate < 0.55:
		return false, "text_layer_low_letter_rate"
	case q.Words < 15:
		return false, "text_layer_sparse"
	}
	return true, "text_layer_good"
}

// renderPage merender satu halaman pada DPI yang masih dalam anggaran piksel.
// Format "pgm" (grayscale tanpa kompresi, untuk OCR) atau "jpeg" (preview). Encoder PNG poppler
// sangat lambat pada 300 DPI (~11 detik/halaman pada host uji), sehingga tidak dipakai.
func (e *engine) renderPage(ctx context.Context, path string, page int, w, h float64, dpi int, prefix, format string) (string, int, error) {
	// DPI diturunkan bertahap sampai masuk anggaran, tetapi tidak di bawah batas bawah
	// (150 untuk OCR; preview boleh lebih rendah sesuai permintaannya).
	floor := min(dpi, 150)
	for dpi >= floor {
		px := int64(w/72*float64(dpi)) * int64(h/72*float64(dpi))
		if px <= e.lim.MaxRenderPixelsPage && e.renderBudget-px >= 0 {
			e.renderBudget -= px
			break
		}
		dpi -= 50
	}
	if dpi < floor {
		return "", 0, fail(ReasonTooManyPixels, "anggaran render habis")
	}
	base := filepath.Join(e.dir, fmt.Sprintf("%s-%d", prefix, page))
	args := []string{"-r", strconv.Itoa(dpi), "-f", strconv.Itoa(page), "-l", strconv.Itoa(page)}
	ext := ".pgm"
	if format == "jpeg" {
		args = append(args, "-jpeg", "-jpegopt", "quality=85")
		ext = ".jpg"
	} else {
		args = append(args, "-gray")
	}
	args = append(args, "-singlefile", path, base)
	if _, err := run(ctx, e.lim.StageRender, e.dir, e.env(), 1<<20, e.tools.PDFToPPM, args...); err != nil {
		var te *toolError
		if errors.As(err, &te) {
			return "", 0, transient(ReasonToolCrashed, "pdftoppm gagal")
		}
		return "", 0, err
	}
	return base + ext, dpi, nil
}

// mergeOCR menambahkan baris OCR yang tidak bertumpuk dengan teks native (native diutamakan bila keduanya ada).
func mergeOCR(native []*Line, ocr []*Line) []*Line {
	out := append([]*Line{}, native...)
	for _, o := range ocr {
		covered := false
		for _, n := range native {
			inter := o.B.overlapX(n.B) * o.B.overlapY(n.B)
			if inter > 0.4*math.Max(1, o.B.W()*o.B.H()) {
				covered = true
				break
			}
		}
		if !covered {
			out = append(out, o)
		}
	}
	return out
}

// processPDF: teks native per halaman, OCR hanya untuk halaman bermutu rendah, preview untuk semua halaman.
func (e *engine) processPDF(ctx context.Context, path string, d *Doc) error {
	info, err := e.pdfInfo(ctx, path)
	if err != nil {
		return err
	}
	if info.pages == 0 {
		return fail(ReasonEmpty, "0 halaman")
	}
	if info.pages > e.lim.MaxPages {
		return fail(ReasonTooManyPages, fmt.Sprintf("%d halaman", info.pages))
	}
	e.progress("reading", 0, info.pages)
	e.pageImages = e.pdfImages(ctx, path, info.pages, info.sizes)
	pages, err := e.pdfText(ctx, path, info.pages)
	if err != nil {
		return err
	}
	for len(pages) < info.pages {
		pages = append(pages, &Page{No: len(pages) + 1, Status: "ok"})
	}
	for _, p := range pages {
		if sz, ok := info.sizes[p.No]; ok && (p.W == 0 || p.H == 0) {
			p.W, p.H = sz[0], sz[1]
		}
	}
	for _, p := range pages {
		if err := ctx.Err(); err != nil {
			return err
		}
		e.progress("reading", p.No, info.pages)
		e.pdfPage(ctx, path, p)
		d.Pages = append(d.Pages, p)
	}
	return nil
}

func (e *engine) pdfPage(ctx context.Context, path string, p *Page) {
	p.Quality = assessQuality(p)
	if p.Quality.DuplicateRate > 0.2 {
		n := dedupeWords(p)
		p.Attempts = append(p.Attempts, fmt.Sprintf("dedupe_duplicate_layer:%d", n))
		p.Quality = assessQuality(p)
	}
	ok, reason := decideNative(p.Quality)
	p.Reason = reason
	// Preview untuk panel bukti (koordinat native dalam point; skala preview = DPI/72).
	if img, dpi, err := e.renderPage(ctx, path, p.No, p.W, p.H, e.lim.PreviewDPI, "preview", "jpeg"); err == nil {
		name := fmt.Sprintf("page-%d.jpg", p.No)
		if os.Rename(img, filepath.Join(e.outDir, name)) == nil {
			p.Image = name
			p.Scale = float64(dpi) / 72
		}
	}
	if !ok && e.pageImages != nil && e.pageImages[p.No].count == 0 {
		switch {
		case reason == "text_layer_sparse":
			// teks sedikit tanpa gambar: memang halaman berisi sedikit teks, bukan scan
			ok, p.Reason = true, "text_layer_sparse_no_images"
		case p.Quality.Words == 0 && p.Image != "" && inkRatio(filepath.Join(e.outDir, p.Image)) < 0.002:
			p.Status, p.Method, p.Reason, p.Failure = "failed", "failed", "blank_page", string(ReasonNoText)
			return
		case p.Quality.Words == 0:
			p.Reason = "no_text_layer_vector_content"
		}
	}
	// Halaman berupa gambar penuh dengan layer teks: periksa apakah layer teks cocok dengan isi gambar.
	scanLike := e.pageImages != nil && e.pageImages[p.No].coverage >= 0.5
	if ok && scanLike && e.tools.Tesseract != "" {
		ok = false
		p.Reason = "image_page_with_text_layer"
	}
	if ok {
		p.Method = "native"
		for i, l := range p.Lines {
			l.ID = lineID(p.No, i+1)
		}
		return
	}
	if e.tools.Tesseract == "" {
		p.Status, p.Method, p.Failure = "failed", "failed", string(ReasonToolMissing)
		return
	}
	img, dpi, err := e.renderPage(ctx, path, p.No, p.W, p.H, e.lim.OCRDPI, "ocr", "pgm")
	if err != nil {
		p.Status, p.Method, p.Failure = "failed", "failed", string(reasonOf(err))
		return
	}
	defer os.Remove(img)
	res, err := e.ocrImage(ctx, img, p)
	if err != nil {
		p.Status, p.Method, p.Failure = "failed", "failed", string(reasonOf(err))
		return
	}
	// Koordinat OCR (piksel render) -> point PDF.
	k := 72 / float64(dpi)
	for _, l := range res.Lines {
		l.Page = p.No
		l.B = BBox{l.B.X0 * k, l.B.Y0 * k, l.B.X1 * k, l.B.Y1 * k}
		for i := range l.Words {
			w := &l.Words[i]
			w.B = BBox{w.B.X0 * k, w.B.Y0 * k, w.B.X1 * k, w.B.Y1 * k}
		}
	}
	native := p.Lines
	if p.Quality.LetterRate < 0.55 || p.Quality.ReplacementRate > 0.05 {
		native = nil // layer teks rusak tidak dipakai sama sekali
	}
	if p.Reason == "image_page_with_text_layer" && len(native) > 0 {
		if sim := vocabOverlap(native, res.Lines); sim >= 0.6 {
			// layer teks sesuai gambar: pakai teks native (lebih presisi), OCR tidak dipakai
			p.Lines, p.Method, p.Reason = native, "native", fmt.Sprintf("text_layer_matches_image:%.2f", sim)
			for i, l := range p.Lines {
				l.ID = lineID(p.No, i+1)
			}
			return
		} else {
			p.Reason = fmt.Sprintf("text_layer_mismatch_image:%.2f", sim)
			native = nil
		}
	}
	p.Lines = mergeOCR(native, res.Lines)
	p.Method = "ocr"
	if len(native) > 0 {
		p.Method = "mixed"
	}
	p.Quality.MeanConf = res.MeanConf
	for i, l := range p.Lines {
		l.ID = lineID(p.No, i+1)
		l.Page = p.No
	}
	if len(p.Lines) == 0 {
		p.Status, p.Failure = "failed", string(ReasonNoText)
	}
}

func reasonOf(err error) Reason {
	var f *Failure
	if errors.As(err, &f) {
		return f.Reason
	}
	return ReasonToolCrashed
}
