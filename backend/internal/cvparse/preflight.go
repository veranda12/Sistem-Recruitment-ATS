package cvparse

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PreflightResult per komponen; Required=false berarti format terkait ditandai tidak tersedia, bukan fatal.
type PreflightResult struct {
	Name     string
	OK       bool
	Required bool
	Detail   string
}

// Preflight benar-benar menjalankan fixture kecil: PDF native (teks + render), OCR bahasa Indonesia
// dan Inggris, OSD, serta konversi Word bila LibreOffice dikonfigurasi. Binary yang terpasang tanpa
// data bahasa atau renderer akan gagal di sini, bukan saat kandidat mengunggah.
func Preflight(ctx context.Context, t Tools, workRoot string) []PreflightResult {
	var out []PreflightResult
	add := func(name string, required bool, err error, detail string) {
		r := PreflightResult{Name: name, Required: required, OK: err == nil, Detail: detail}
		if err != nil {
			r.Detail = err.Error()
		}
		out = append(out, r)
	}
	dir, err := os.MkdirTemp(workRoot, "preflight-")
	if err != nil {
		add("workdir", true, err, "")
		return out
	}
	defer os.RemoveAll(dir)
	lim := DefaultLimits()

	// 1) PDF native: teks + koordinat + render preview.
	pdf := minimalPDF("Pengalaman Kerja Backend Engineer PT Contoh Data 2022 sekarang")
	pdfPath := filepath.Join(dir, "native.pdf")
	_ = os.WriteFile(pdfPath, pdf, 0o600)
	res, err := Process(ctx, t, lim, pdfPath, dir, filepath.Join(dir, "out1"), nil)
	switch {
	case err != nil:
		add("pdf_native", true, err, "")
	case len(res.Doc.Pages) == 0 || res.Doc.Pages[0].Method != "native" || !strings.Contains(strings.ToLower(lineText(res.Doc)), "backend engineer"):
		add("pdf_native", true, fmt.Errorf("teks native tidak terbaca"), "")
	case res.Doc.Pages[0].Image == "":
		add("pdf_render", true, fmt.Errorf("render preview gagal (pdftoppm)"), "")
	default:
		add("pdf_native", true, nil, "teks + koordinat + render")
	}

	// 2) OCR ind+eng + OSD pada gambar teks sintetis (dibuat dari render PDF yang sama).
	if t.Tesseract == "" {
		add("ocr", true, fmt.Errorf("tesseract tidak dikonfigurasi"), "")
	} else {
		for _, l := range []string{"ind", "eng", "osd"} {
			if _, err := os.Stat(filepath.Join(t.TessData, l+".traineddata")); err != nil {
				add("tessdata_"+l, true, fmt.Errorf("%s.traineddata tidak ada di %s", l, t.TessData), "")
			}
		}
		e := &engine{tools: t, lim: lim, dir: dir, outDir: dir, renderBudget: lim.MaxRenderPixelsAll, cacheDir: dir}
		img, _, err := e.renderPage(ctx, pdfPath, 1, 595, 842, 200, "pf", "pgm")
		if err != nil {
			add("ocr", true, err, "")
		} else {
			r, err := e.runOCR(ctx, img, "3")
			if err == nil && !strings.Contains(strings.ToLower(ocrText(r)), "engineer") {
				err = fmt.Errorf("OCR tidak membaca teks uji (periksa data bahasa)")
			}
			add("ocr_ind_eng", true, err, fmt.Sprintf("%d kata, conf %.0f", r.Words, r.MeanConf))
		}
	}

	// 3) Gambar: decode PNG + praproses.
	g := image.NewGray(image.Rect(0, 0, 200, 100))
	draw.Draw(g, g.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)
	var b bytes.Buffer
	_ = savePNGTo(&b, g)
	if _, _, err := image.DecodeConfig(bytes.NewReader(b.Bytes())); err != nil {
		add("image_decode", true, err, "")
	}

	// 4) LibreOffice (opsional): konversi DOCX minimal ke PDF.
	if t.Soffice == "" {
		add("doc_converter", false, fmt.Errorf("LibreOffice tidak dikonfigurasi: .doc lawas dan DOCX bergambar tidak didukung"), "")
	} else {
		e := &engine{tools: t, lim: lim, dir: dir, outDir: dir, renderBudget: lim.MaxRenderPixelsAll, cacheDir: dir}
		cctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
		_, err := e.convertToPDF(cctx, minimalDocx("Preflight konversi dokumen"), ".docx")
		add("doc_converter", false, err, "konversi DOCX->PDF")
	}
	return out
}

func lineText(d *Doc) string {
	var b strings.Builder
	for _, p := range d.Pages {
		for _, l := range p.Lines {
			b.WriteString(l.Text() + " ")
		}
	}
	return b.String()
}

func ocrText(r ocrResult) string {
	var b strings.Builder
	for _, l := range r.Lines {
		b.WriteString(l.Text() + " ")
	}
	return b.String()
}

func minimalPDF(text string) []byte {
	content := fmt.Sprintf("BT /F1 18 Tf 60 760 Td (%s) Tj ET", text)
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offs := make([]int, len(objs))
	for i, o := range objs {
		offs[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	x := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, o := range offs {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, x)
	return b.Bytes()
}
