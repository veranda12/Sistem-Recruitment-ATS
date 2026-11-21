package cvparse

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// ocrResult: kata hasil Tesseract (TSV) dikelompokkan per baris, koordinat dalam piksel gambar input.
type ocrResult struct {
	Lines    []*Line
	MeanConf float64
	Words    int
}

// runOCR menjalankan Tesseract dengan paket bahasa Indonesia + Inggris. psm 3 = segmentasi otomatis (mengenali kolom), psm 4 = satu kolom.
func (e *engine) runOCR(ctx context.Context, img, psm string, extra ...string) (ocrResult, error) {
	args := []string{img, "stdout", "-l", "ind+eng", "--oem", "1", "--psm", psm, "--tessdata-dir", e.tools.TessData,
		"-c", "preserve_interword_spaces=1"}
	// tessedit_create_tsv, bukan config "tsv": config bawaan tidak ada di --tessdata-dir khusus.
	args = append(args, "-c", "tessedit_create_tsv=1")
	args = append(args, extra...)
	out, err := run(ctx, e.lim.StageOCR, e.dir, e.env(), e.lim.MaxOutputBytes, e.tools.Tesseract, args...)
	if err != nil {
		var te *toolError
		if errors.As(err, &te) {
			if strings.Contains(te.stderr, "Failed loading language") || strings.Contains(te.stderr, "tessdata") {
				return ocrResult{}, fail(ReasonToolMissing, "data bahasa OCR tidak lengkap")
			}
			return ocrResult{}, transient(ReasonToolCrashed, te.Error())
		}
		return ocrResult{}, err
	}
	return parseTSV(out), nil
}

func parseTSV(b []byte) ocrResult {
	type key struct{ blk, par, ln int }
	var order []key
	lines := map[key]*Line{}
	var sum float64
	var n int
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	first := true
	for sc.Scan() {
		if first {
			first = false
			continue
		}
		f := strings.Split(sc.Text(), "\t")
		if len(f) < 12 || f[0] != "5" {
			continue
		}
		text := strings.TrimSpace(f[11])
		if text == "" {
			continue
		}
		at := func(i int) int { v, _ := strconv.Atoi(f[i]); return v }
		conf, _ := strconv.ParseFloat(f[10], 64)
		k := key{at(2), at(3), at(4)}
		l, ok := lines[k]
		if !ok {
			l = &Line{Source: "ocr"}
			lines[k] = l
			order = append(order, k)
		}
		x, y, w, h := float64(at(6)), float64(at(7)), float64(at(8)), float64(at(9))
		wd := Word{Text: text, B: BBox{x, y, x + w, y + h}, Conf: conf}
		l.Words = append(l.Words, wd)
		l.B = l.B.Union(wd.B)
		sum += conf
		n++
	}
	res := ocrResult{Words: n}
	if n > 0 {
		res.MeanConf = sum / float64(n)
	}
	for _, k := range order {
		l := lines[k]
		var c float64
		for _, w := range l.Words {
			c += w.Conf
		}
		l.Conf = c / float64(len(l.Words))
		res.Lines = append(res.Lines, l)
	}
	return res
}

var osdRotateRe = regexp.MustCompile(`Rotate:\s*(\d+)`)
var osdConfRe = regexp.MustCompile(`Orientation confidence:\s*([\d.]+)`)

// detectRotation memakai OSD Tesseract. Mengembalikan derajat searah jarum jam yang perlu diterapkan.
func (e *engine) detectRotation(ctx context.Context, img string) (int, float64) {
	out, err := run(ctx, e.lim.StageOCR, e.dir, e.env(), 1<<20, e.tools.Tesseract, img, "stdout", "--psm", "0", "--tessdata-dir", e.tools.TessData, "-l", "osd")
	if err != nil {
		return 0, 0
	}
	m := osdRotateRe.FindSubmatch(out)
	c := osdConfRe.FindSubmatch(out)
	if m == nil || c == nil {
		return 0, 0
	}
	deg, _ := strconv.Atoi(string(m[1]))
	conf, _ := strconv.ParseFloat(string(c[1]), 64)
	return deg, conf
}
