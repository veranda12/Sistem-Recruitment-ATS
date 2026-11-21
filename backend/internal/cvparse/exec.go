package cvparse

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Tools: lokasi binary eksternal. Kosong = tidak tersedia (format terkait gagal dengan alasan operasional).
type Tools struct {
	PDFInfo   string
	PDFToText string
	PDFToPPM  string
	PDFImages string
	Tesseract string
	TessData  string // direktori *.traineddata (ind, eng, osd)
	Soffice   string
}

// Limits eksplisit; tidak ada pemotongan halaman diam-diam.
type Limits struct {
	MaxBytes            int64
	MaxPages            int
	MaxImagePixels      int64
	MaxImageSide        int
	MaxRenderPixelsPage int64
	MaxRenderPixelsAll  int64
	OCRDPI              int
	PreviewDPI          int
	DocxMaxEntries      int
	DocxMaxUncompressed int64
	DocxMaxRatio        int64
	MaxOCRAttempts      int
	StageInfo           time.Duration
	StageText           time.Duration
	StageRender         time.Duration
	StageOCR            time.Duration
	StageConvert        time.Duration
	MaxOutputBytes      int64
}

func DefaultLimits() Limits {
	return Limits{
		MaxBytes: 15 << 20, MaxPages: 20, MaxImagePixels: 40_000_000, MaxImageSide: 12000,
		MaxRenderPixelsPage: 36_000_000, MaxRenderPixelsAll: 220_000_000, OCRDPI: 300, PreviewDPI: 110,
		DocxMaxEntries: 3000, DocxMaxUncompressed: 120 << 20, DocxMaxRatio: 150, MaxOCRAttempts: 3,
		StageInfo: 20 * time.Second, StageText: 40 * time.Second, StageRender: 60 * time.Second,
		StageOCR: 120 * time.Second, StageConvert: 150 * time.Second, MaxOutputBytes: 64 << 20,
	}
}

// limitedBuffer menolak keluaran tool yang melebihi batas (mencegah memori habis).
type limitedBuffer struct {
	buf   bytes.Buffer
	limit int64
	over  bool
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if int64(l.buf.Len()+len(p)) > l.limit {
		l.over = true
		return 0, errors.New("keluaran melebihi batas")
	}
	return l.buf.Write(p)
}

// run menjalankan binary dengan argumen terpisah (tanpa shell), direktori kerja job, env minimal, dan timeout.
// Saat timeout seluruh pohon proses dimatikan.
func run(ctx context.Context, timeout time.Duration, dir string, env []string, maxOut int64, bin string, args ...string) ([]byte, error) {
	if bin == "" {
		return nil, fail(ReasonToolMissing, "binary tidak dikonfigurasi")
	}
	if _, err := os.Stat(bin); err != nil {
		if lp, err2 := exec.LookPath(bin); err2 == nil {
			bin = lp
		} else {
			return nil, fail(ReasonToolMissing, filepath.Base(bin)+" tidak ditemukan")
		}
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = baseEnv(dir, env)
	setProcAttr(cmd)
	cmd.Cancel = func() error { return killTree(cmd) }
	cmd.WaitDelay = 5 * time.Second
	out := &limitedBuffer{limit: maxOut}
	errb := &limitedBuffer{limit: 64 << 10}
	cmd.Stdout, cmd.Stderr = out, errb
	t0 := time.Now()
	err := cmd.Run()
	if os.Getenv("CVPARSE_TRACE") != "" {
		// hanya nama tool dan durasi; tidak ada argumen (path) atau isi dokumen
		fmt.Fprintln(os.Stderr, "cvparse trace:", filepath.Base(bin), time.Since(t0).Milliseconds(), "ms")
	}
	switch {
	case cctx.Err() == context.DeadlineExceeded:
		return nil, transient(ReasonTimeout, filepath.Base(bin)+" melewati batas waktu")
	case ctx.Err() != nil:
		return nil, ctx.Err()
	case out.over:
		return nil, fail(ReasonArchiveBomb, filepath.Base(bin)+" keluaran melebihi batas")
	case err != nil:
		return out.buf.Bytes(), &toolError{bin: filepath.Base(bin), err: err, stderr: firstLine(errb.buf.String())}
	}
	return out.buf.Bytes(), nil
}

type toolError struct {
	bin, stderr string
	err         error
}

func (e *toolError) Error() string { return fmt.Sprintf("%s: %v: %s", e.bin, e.err, e.stderr) }

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// baseEnv: hanya variabel yang dibutuhkan; HOME/TEMP diarahkan ke direktori job agar tool tidak menyentuh profil pengguna.
func baseEnv(dir string, extra []string) []string {
	env := []string{"OMP_THREAD_LIMIT=1", "LANG=C.UTF-8", "LC_ALL=C.UTF-8"}
	keep := []string{"PATH", "SystemRoot", "SYSTEMROOT", "windir", "ComSpec", "PATHEXT", "NUMBER_OF_PROCESSORS", "PROCESSOR_ARCHITECTURE", "FONTCONFIG_PATH", "FONTCONFIG_FILE"}
	for _, k := range keep {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	env = append(env, "HOME="+dir, "TMPDIR="+dir, "TEMP="+dir, "TMP="+dir)
	if runtime.GOOS == "windows" {
		env = append(env, "USERPROFILE="+dir, "APPDATA="+filepath.Join(dir, "appdata"), "LOCALAPPDATA="+filepath.Join(dir, "localappdata"))
	}
	return append(env, extra...)
}
