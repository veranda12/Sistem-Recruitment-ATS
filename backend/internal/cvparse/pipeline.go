package cvparse

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Format terdeteksi dari signature byte, bukan nama berkas.
func DetectFormat(head []byte) string {
	switch {
	case bytes.HasPrefix(head, []byte("%PDF-")):
		return "pdf"
	case bytes.HasPrefix(head, []byte("\x89PNG\r\n\x1a\n")):
		return "png"
	case bytes.HasPrefix(head, []byte("\xff\xd8\xff")):
		return "jpeg"
	case len(head) >= 12 && string(head[:4]) == "RIFF" && string(head[8:12]) == "WEBP":
		return "webp"
	case bytes.HasPrefix(head, []byte("PK\x03\x04")):
		return "docx"
	case bytes.HasPrefix(head, cfbMagic):
		return "doc"
	}
	return ""
}

// Manifest: jejak keputusan pipeline untuk audit dan reproduksi.
type Manifest struct {
	DocumentSHA256 string            `json:"documentSha256"`
	ParserVersion  string            `json:"parserVersion"`
	Tools          map[string]string `json:"tools"`
	Format         string            `json:"format"`
	Path           string            `json:"path"` // pdf | image | docx | docx_rendered | doc_converted
	PathReason     string            `json:"pathReason"`
	Pages          int               `json:"pages"`
	PageDecisions  []PageDecision    `json:"pageDecisions"`
	Transforms     map[int]Transform `json:"transforms,omitempty"`
	DurationsMS    map[string]int64  `json:"durationsMs"`
}

type PageDecision struct {
	Page     int      `json:"page"`
	Method   string   `json:"method"`
	Reason   string   `json:"reason"`
	Status   string   `json:"status"`
	Failure  string   `json:"failure,omitempty"`
	Attempts []string `json:"attempts,omitempty"`
}

// Progress dipanggil per tahap; dipakai worker untuk heartbeat dan status "Membaca halaman 2 dari 4".
type Progress func(stage string, done, total int)

type engine struct {
	tools        Tools
	lim          Limits
	dir          string // direktori kerja sementara job (dibersihkan setelah selesai)
	outDir       string // artefak privat (preview halaman)
	format       string
	renderBudget int64
	progress     Progress
	transforms   map[int]Transform
	pageImages   map[int]pageImg
	cacheDir     string // cache fontconfig bersama antar-job (hanya indeks font sistem)
}

// Result pipeline. Doc selalu terisi untuk dokumen yang sempat dibaca sebagian (status partial).
type Result struct {
	Doc      *Doc
	Manifest Manifest
}

// Process membaca berkas src. workRoot = induk direktori sementara; outDir = artefak privat run.
// Error bertipe *Failure membawa alasan stabil; halaman gagal tercatat per halaman tanpa menggagalkan seluruh dokumen.
func Process(ctx context.Context, tools Tools, lim Limits, src, workRoot, outDir string, progress Progress) (*Result, error) {
	if progress == nil {
		progress = func(string, int, int) {}
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > lim.MaxBytes {
		return nil, fail(ReasonTooLarge, "ukuran")
	}
	if len(data) == 0 {
		return nil, fail(ReasonEmpty, "0 byte")
	}
	if err := os.MkdirAll(workRoot, 0o700); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(workRoot, "job-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	cache := filepath.Join(workRoot, "cache")
	_ = os.MkdirAll(cache, 0o700)
	e := &engine{tools: tools, lim: lim, dir: dir, outDir: outDir, renderBudget: lim.MaxRenderPixelsAll, progress: progress, transforms: map[int]Transform{}, cacheDir: cache}
	e.format = DetectFormat(data)
	m := Manifest{DocumentSHA256: hex.EncodeToString(sum[:]), ParserVersion: ParserVersion, Tools: ToolVersions(ctx, tools), Format: e.format, DurationsMS: map[string]int64{}}
	d := &Doc{Format: e.format}
	t0 := time.Now()
	switch e.format {
	case "pdf":
		m.Path, m.PathReason = "pdf", "pdf_signature"
		in := filepath.Join(dir, "input.pdf")
		if err := os.WriteFile(in, data, 0o400); err != nil {
			return nil, err
		}
		err = e.processPDF(ctx, in, d)
	case "png", "jpeg", "webp":
		m.Path, m.PathReason = "image", e.format+"_signature"
		err = e.processImage(ctx, data, d)
	case "docx":
		m.Path, m.PathReason = "docx", "ooxml_package"
		var p *Page
		var render bool
		var why string
		p, render, why, err = processDocx(data, lim)
		if err == nil && render {
			m.Path, m.PathReason = "docx_rendered", why
			var pdf string
			pdf, err = e.convertToPDF(ctx, data, ".docx")
			if err == nil {
				e.format = "pdf"
				err = e.processPDF(ctx, pdf, d)
			}
		} else if err == nil {
			progress("reading", 1, 1)
			d.Pages = []*Page{p}
		}
	case "doc":
		m.Path, m.PathReason = "doc_converted", "cfb_worddocument"
		if err = validateDoc(data); err == nil {
			progress("converting", 0, 1)
			var pdf string
			pdf, err = e.convertToPDF(ctx, data, ".doc")
			if err == nil {
				e.format = "pdf"
				err = e.processPDF(ctx, pdf, d)
			}
		}
	default:
		return nil, fail(ReasonUnsupported, "signature tidak dikenal")
	}
	m.DurationsMS["read"] = time.Since(t0).Milliseconds()
	if err != nil {
		return nil, err
	}
	m.Pages = len(d.Pages)
	ok := 0
	for _, p := range d.Pages {
		if p.Status == "ok" {
			ok++
		}
		m.PageDecisions = append(m.PageDecisions, PageDecision{Page: p.No, Method: p.Method, Reason: p.Reason, Status: p.Status, Failure: p.Failure, Attempts: p.Attempts})
	}
	if len(e.transforms) > 0 {
		m.Transforms = e.transforms
	}
	if ok == 0 {
		// Semua halaman gagal: kembalikan alasan halaman pertama (mis. foto buram), bukan profil kosong.
		r := ReasonNoText
		for _, p := range d.Pages {
			if p.Failure != "" {
				r = Reason(p.Failure)
				break
			}
		}
		return &Result{Doc: d, Manifest: m}, &Failure{Reason: r, Transient: r == ReasonTimeout || r == ReasonToolCrashed, Detail: "semua halaman gagal"}
	}
	return &Result{Doc: d, Manifest: m}, nil
}

// env: cache fontconfig diarahkan ke direktori cache bersama; tanpa ini poppler di Windows membangun
// ulang cache font di setiap pemanggilan (~10 detik per render).
func (e *engine) env() []string {
	return []string{"LOCALAPPDATA=" + e.cacheDir, "XDG_CACHE_HOME=" + e.cacheDir}
}

func (e *engine) processImage(ctx context.Context, data []byte, d *Doc) error {
	if err := e.checkImageConfig(data); err != nil {
		return err
	}
	if e.tools.Tesseract == "" {
		return fail(ReasonToolMissing, "ocr tidak dikonfigurasi")
	}
	e.progress("reading", 0, 1)
	src := filepath.Join(e.dir, "input."+e.format)
	if err := os.WriteFile(src, data, 0o400); err != nil {
		return err
	}
	p := &Page{No: 1, Status: "ok", Method: "ocr", Reason: "image_input"}
	res, err := e.ocrImage(ctx, src, p)
	if err != nil {
		var f *Failure
		if errors.As(err, &f) && !f.Transient {
			p.Status, p.Method, p.Failure = "failed", "failed", string(f.Reason)
			d.Pages = append(d.Pages, p)
			return nil
		}
		return err
	}
	tr := e.transforms[1]
	p.W, p.H = float64(tr.Width), float64(tr.Height)
	p.Lines = res.Lines
	p.Quality = Quality{Words: res.Words, MeanConf: res.MeanConf, LetterRate: 1}
	for i, l := range p.Lines {
		l.ID = lineID(1, i+1)
		l.Page = 1
	}
	e.progress("reading", 1, 1)
	d.Pages = append(d.Pages, p)
	return nil
}

var (
	versionsMu sync.Mutex
	versions   map[string]string
)

// ToolVersions dicatat di manifest (dicache per proses). Checksum tessdata ikut agar model OCR dapat dilacak.
func ToolVersions(ctx context.Context, t Tools) map[string]string {
	versionsMu.Lock()
	defer versionsMu.Unlock()
	if versions != nil {
		return versions
	}
	v := map[string]string{}
	// Versi dibaca langsung (argumen tetap, tanpa input pengguna); poppler menulis versi ke stderr.
	ver := func(bin string, args ...string) string {
		if bin == "" {
			return ""
		}
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		out, _ := exec.CommandContext(cctx, bin, args...).CombinedOutput()
		line, _, _ := strings.Cut(string(out), "\n")
		return strings.TrimSpace(line)
	}
	v["poppler"] = ver(t.PDFToText, "-v")
	v["tesseract"] = ver(t.Tesseract, "--version")
	for _, l := range []string{"ind", "eng", "osd"} {
		if b, err := os.ReadFile(filepath.Join(t.TessData, l+".traineddata")); err == nil {
			s := sha256.Sum256(b)
			v["tessdata."+l] = hex.EncodeToString(s[:8])
		}
	}
	if t.Soffice != "" {
		v["soffice"] = filepath.Base(t.Soffice)
	}
	versions = v
	return v
}
