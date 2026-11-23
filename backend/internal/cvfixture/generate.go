package cvfixture

import (
	"encoding/json"
	"fmt"
	"image"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"time"
)

var fixedTime = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

// Fixture adalah satu berkas uji beserta hasil yang diharapkan.
type Fixture struct {
	File    string `json:"file"`
	Persona string `json:"persona,omitempty"`
	Split   string `json:"split"`
	Variant string `json:"variant"`
	Format  string `json:"format"`
	Expect  string `json:"expect"` // ok | partial | <reason code>
	Note    string `json:"note,omitempty"`
}

type Tools struct {
	PDFToPPM string
	Soffice  string
	Python   string // opsional, hanya untuk menulis WebP (Pillow)
}

type gen struct {
	out, tmp string
	t        Tools
	list     []Fixture
	logf     func(string, ...any)
}

func (g *gen) write(f Fixture, data []byte) {
	dir := filepath.Join(g.out, f.Split)
	_ = os.MkdirAll(dir, 0o755)
	if err := os.WriteFile(filepath.Join(dir, f.File), data, 0o644); err != nil {
		panic(err)
	}
	g.list = append(g.list, f)
}

// soffice menjalankan LibreOffice dengan profil sementara tersendiri.
func (g *gen) soffice(in, filter, outDir string) error {
	prof := filepath.Join(g.tmp, "lo-profile")
	_ = os.MkdirAll(prof, 0o700)
	u := "file:///" + filepath.ToSlash(prof)
	cmd := exec.Command(g.t.Soffice, "-env:UserInstallation="+u, "--headless", "--norestore", "--nolockcheck", "--convert-to", filter, "--outdir", outDir, in)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("soffice %s: %v %s", filter, err, out)
	}
	return nil
}

// convertDocx mengubah DOCX ke format lain lewat LibreOffice (mis. "doc:MS Word 97", "pdf").
func (g *gen) convertDocx(docx []byte, filter, ext string) ([]byte, error) {
	in := filepath.Join(g.tmp, "conv.docx")
	if err := os.WriteFile(in, docx, 0o600); err != nil {
		return nil, err
	}
	outDir := filepath.Join(g.tmp, "conv-out")
	_ = os.RemoveAll(outDir)
	_ = os.MkdirAll(outDir, 0o700)
	if err := g.soffice(in, filter, outDir); err != nil {
		return nil, err
	}
	return os.ReadFile(filepath.Join(outDir, "conv"+ext))
}

func (g *gen) webp(img *image.Gray, name string) ([]byte, error) {
	if g.t.Python == "" {
		return nil, fmt.Errorf("python tidak tersedia")
	}
	src := filepath.Join(g.tmp, name+".png")
	dst := filepath.Join(g.tmp, name+".webp")
	if err := os.WriteFile(src, EncodePNG(img), 0o600); err != nil {
		return nil, err
	}
	// argumen terpisah, tanpa shell; path dioper sebagai argv
	code := "import sys\nfrom PIL import Image\nImage.open(sys.argv[1]).save(sys.argv[2], 'WEBP', quality=80)"
	if out, err := exec.Command(g.t.Python, "-I", "-c", code, src, dst).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("webp: %v %s", err, out)
	}
	return os.ReadFile(dst)
}

// Generate menulis seluruh fixture ke out/{dev,holdout} serta golden/<persona>.json dan manifest.json.
func Generate(out string, t Tools, logf func(string, ...any)) ([]Fixture, error) {
	tmp, err := os.MkdirTemp("", "cvfixture-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	g := &gen{out: out, tmp: tmp, t: t, logf: logf}
	_ = os.MkdirAll(filepath.Join(out, "golden"), 0o755)

	scanSet := []string{"sinta", "budi", "rahmat", "joko", "lina", "andi"}
	mixedSet := []string{"sinta", "joko", "andi"}
	dupSet := []string{"sinta", "budi", "dewi", "maya"}
	// foto dari sudut sempit (keystone): dev dan holdout
	keystoneSet := map[string]float64{"rahmat": 0.40, "nur": 0.25, "lina": 0.38}
	photo := map[string]string{"sinta": "skew", "budi": "rot90", "dewi": "rot180", "kevin": "exif6", "ayu": "skewpng", "maya": "webp", "fajar": "skew", "sari": "skew"}

	for i, p := range Personas() {
		seed := int64(1000 + i)
		gj, _ := json.MarshalIndent(p.Golden, "", "  ")
		_ = os.WriteFile(filepath.Join(out, "golden", p.ID+".json"), gj, 0o644)
		fx := func(variant, format, ext, expect string) Fixture {
			return Fixture{File: p.ID + "." + variant + "." + ext, Persona: p.ID, Split: p.Split, Variant: variant, Format: format, Expect: expect}
		}
		pdf1 := RenderNative(p, NativeOptions{})
		g.write(fx("pdf-1col", "pdf_native", "pdf", "ok"), pdf1)
		g.write(fx("pdf-2col", "pdf_native", "pdf", "ok"), RenderNative(p, NativeOptions{TwoColumn: true}))
		if slices.Contains(dupSet, p.ID) {
			g.write(fx("pdf-duplayer", "pdf_native", "pdf", "ok"), RenderNative(p, NativeOptions{DupLayer: true}))
		}
		docx := RenderDocx(p, DocxOptions{})
		g.write(fx("docx", "docx", "docx", "ok"), docx)
		if t.Soffice != "" {
			if doc, err := g.convertDocx(docx, "doc:MS Word 97", ".doc"); err == nil {
				g.write(fx("doc", "doc", "doc", "ok"), doc)
			} else {
				logf("doc %s: %v", p.ID, err)
			}
		}
		if t.PDFToPPM == "" {
			continue
		}
		page1, err := RenderPDFPage(t.PDFToPPM, pdf1, 1, 200, tmp)
		if err != nil {
			return nil, err
		}
		if slices.Contains(scanSet, p.ID) {
			// joko dan andi: scan dua halaman (pengalaman berlanjut ke halaman 2)
			src := pdf1
			if p.ID == "joko" || p.ID == "andi" {
				src = RenderNative(p, NativeOptions{ForcePage2: true})
			}
			var pages []*image.Gray
			for n := 1; n <= pdfPageCount(src); n++ {
				pg, err := RenderPDFPage(t.PDFToPPM, src, n, 200, tmp)
				if err != nil {
					return nil, err
				}
				pages = append(pages, Degrade(pg, seed+int64(n), 0.8, 9, 0.85))
			}
			g.write(fx("scan", "pdf_scan", "pdf", "ok"), ImagePDF(pages, nil))
		}
		if slices.Contains(mixedSet, p.ID) {
			// halaman 1 native; halaman 2 hasil scan dengan layer teks tersembunyi yang buruk
			two := RenderNative(p, NativeOptions{ForcePage2: true})
			pg2, err := RenderPDFPage(t.PDFToPPM, two, 2, 200, tmp)
			if err != nil {
				return nil, err
			}
			g.write(fx("pdf-mixed", "pdf_mixed", "pdf", "ok"), MixedPDF(p, two, Degrade(pg2, seed, 0.6, 8, 0.85)))
		}
		if v, ok := photo[p.ID]; ok {
			ph := Photo(page1, seed, 4.5, 1)
			switch v {
			case "skew":
				g.write(fx("photo-skew", "image", "jpg", "ok"), EncodeJPEG(ph, 80))
			case "skewpng":
				g.write(fx("photo-skew", "image", "png", "ok"), EncodePNG(ph))
			case "rot90":
				g.write(fx("photo-rot90", "image", "jpg", "ok"), EncodeJPEG(Rot90s(Photo(page1, seed, 2, 1), 90), 80))
			case "rot180":
				g.write(fx("photo-rot180", "image", "jpg", "ok"), EncodeJPEG(Rot90s(Photo(page1, seed, 2, 1), 180), 80))
			case "exif6":
				// piksel diputar 270° CW + EXIF 6 => tampil tegak di penampil yang mematuhi EXIF
				g.write(fx("photo-exif6", "image", "jpg", "ok"), WithEXIFOrientation(EncodeJPEG(Rot90s(Photo(page1, seed, 2, 1), 270), 80), 6))
			case "webp":
				if b, err := g.webp(ph, p.ID); err == nil {
					g.write(fx("photo-skew", "image", "webp", "ok"), b)
				} else {
					logf("webp %s: %v", p.ID, err)
				}
			}
		}
		if k, ok := keystoneSet[p.ID]; ok {
			g.write(fx("photo-perspective", "image", "jpg", "ok"), EncodeJPEG(PhotoKeystone(page1, seed, 2, 1, k), 80))
		}
		if p.ID == "sinta" {
			// halaman 2 gagal dibaca (noise murni); pendidikan di halaman 1 tetap tersedia
			two := RenderNative(p, NativeOptions{EduFirst: true, ForcePage2: true})
			g.write(Fixture{File: "sinta.partial-page2.pdf", Persona: "sinta", Split: "dev", Variant: "partial-page2", Format: "pdf_mixed", Expect: "partial",
				Note: "halaman 2 berisi noise; pendidikan di halaman 1"}, PartialPDF(two, seed))
			g.write(Fixture{File: "sinta.photo-blurry.jpg", Persona: "sinta", Split: "dev", Variant: "photo-blurry", Format: "image", Expect: "image_too_blurry"},
				EncodeJPEG(boxBlur(Photo(page1, seed, 3, 0), 7), 75))
			g.write(fx("docx-imageonly", "docx", "docx", "ok"), RenderDocx(p, DocxOptions{ImageOnly: EncodePNG(page1)}))
		}
	}
	g.failureCases()
	mj, _ := json.MarshalIndent(g.list, "", "  ")
	if err := os.WriteFile(filepath.Join(out, "manifest.json"), mj, 0o644); err != nil {
		return nil, err
	}
	return g.list, nil
}

// MixedPDF: halaman 1 native dari doc dua halaman, halaman 2 diganti gambar + teks tersembunyi rusak.
func MixedPDF(p Persona, twoPage []byte, page2 *image.Gray) []byte {
	d := &PDF{}
	// Halaman 1 dibuat ulang dari renderer yang sama agar identik dengan versi native.
	native := renderPages(p, NativeOptions{ForcePage2: true})
	d.pages = append(d.pages, native.pages[0])
	pg := d.NewPage()
	pg.Image(EncodeJPEG(page2, 72), page2.Rect.Dx(), page2.Rect.Dy(), true, 0, 0, pageW, pageH)
	y := 60.0
	for _, l := range []string{"Pengalarnan Kerja Ex1rakcijfj qwz ~~~ ||| ### 0x0x0x", "M@nager Prqyek ||| 2O19 ~ sekar@ng ^^^ $$$", "lorem@@ ipsum%% 1l1l1l1l ;;;; ::::"} {
		pg.Invisible(40, y, 10, l)
		y += 13
	}
	_ = twoPage
	return d.Bytes()
}

// PartialPDF: halaman 1 native, halaman 2 noise murni (OCR tidak menemukan teks).
func PartialPDF(twoPage []byte, seed int64) []byte {
	_ = twoPage
	r := rand.New(rand.NewSource(seed))
	noise := image.NewGray(image.Rect(0, 0, 1240, 1754))
	for i := range noise.Pix {
		noise.Pix[i] = uint8(150 + r.Intn(100))
	}
	return partialFrom(Personas()[0], noise)
}

func partialFrom(p Persona, noise *image.Gray) []byte {
	native := renderPages(p, NativeOptions{EduFirst: true, ForcePage2: true})
	d := &PDF{}
	d.pages = append(d.pages, native.pages[0])
	pg := d.NewPage()
	pg.Image(EncodeJPEG(noise, 60), noise.Rect.Dx(), noise.Rect.Dy(), true, 0, 0, pageW, pageH)
	return d.Bytes()
}

// failureCases: rusak, terkunci, kosong, terlalu banyak halaman, terlalu besar, bermakro, tidak didukung.
func (g *gen) failureCases() {
	p := Personas()[1]
	pdf1 := RenderNative(p, NativeOptions{})
	f := func(file, format, expect, note string) Fixture {
		return Fixture{File: file, Split: "dev", Variant: "failure", Format: format, Expect: expect, Note: note}
	}
	g.write(f("broken.truncated.pdf", "pdf_native", "corrupt_file", "PDF terpotong"), pdf1[:len(pdf1)*55/100])
	blank := &PDF{}
	blank.NewPage()
	g.write(f("broken.blank.pdf", "pdf_native", "no_readable_text", "satu halaman kosong"), blank.Bytes())
	many := &PDF{}
	for i := 0; i < 21; i++ {
		many.NewPage().Text(40, 40, 11, false, fmt.Sprintf("Halaman %d dari 21", i+1))
	}
	g.write(f("limit.21pages.pdf", "pdf_native", "too_many_pages", "21 halaman (batas 20)"), many.Bytes())
	ok20 := &PDF{}
	for i := 0; i < 20; i++ {
		ok20.NewPage().Text(40, 40, 11, false, fmt.Sprintf("Lampiran portofolio halaman %d dari 20 berisi uraian proyek dan hasil kerja.", i+1))
	}
	g.write(f("limit.20pages.pdf", "pdf_native", "ok", "tepat di batas 20 halaman"), ok20.Bytes())
	// >15 MiB: noise JPEG tidak dapat dikompresi
	r := rand.New(rand.NewSource(7))
	big := image.NewGray(image.Rect(0, 0, 4400, 4400))
	for i := range big.Pix {
		big.Pix[i] = uint8(r.Intn(256))
	}
	huge := &PDF{}
	pg := huge.NewPage()
	pg.Image(EncodeJPEG(big, 100), 4400, 4400, true, 0, 0, pageW, pageH)
	g.write(f("limit.over15mib.pdf", "pdf_native", "too_large", "melebihi 15 MiB"), huge.Bytes())
	g.write(f("broken.macro.docm", "docx", "macro_not_allowed", "berisi vbaProject.bin"), RenderDocx(p, DocxOptions{Macro: true}))
	g.write(f("unsupported.random.bin", "unknown", "unsupported_format", "byte acak"), []byte("ini bukan dokumen CV yang didukung\x00\x01\x02"))
	if g.t.Soffice != "" {
		docx := RenderDocx(p, DocxOptions{})
		filter := `pdf:writer_pdf_Export:{"EncryptFile":{"type":"boolean","value":"true"},"DocumentOpenPassword":{"type":"string","value":"rahasia"}}`
		if b, err := g.convertDocx(docx, filter, ".pdf"); err == nil {
			g.write(f("broken.locked.pdf", "pdf_native", "encrypted_file", "PDF berkata sandi"), b)
		} else {
			g.logf("locked: %v", err)
		}
	}
}

func renderPages(p Persona, o NativeOptions) *PDF {
	doc := &PDF{}
	_ = RenderNativeInto(doc, p, o)
	return doc
}

func pdfPageCount(b []byte) int {
	n := 0
	for i := 0; i+12 < len(b); i++ {
		if string(b[i:i+11]) == "/Type /Page" && b[i+11] != 's' {
			n++
		}
	}
	return max(n, 1)
}
