package cvparse

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
)

// docxReader membaca paket OOXML dengan batas: jumlah entri, total ukuran terurai, rasio kompresi.
// encoding/xml tidak memproses entitas eksternal; DOCTYPE tetap ditolak agar tidak ada DTD sama sekali.
type docxReader struct {
	zr    *zip.Reader
	lim   Limits
	total int64
}

func openDocx(data []byte, lim Limits) (*docxReader, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fail(ReasonCorrupt, "zip tidak valid")
	}
	if len(zr.File) > lim.DocxMaxEntries {
		return nil, fail(ReasonArchiveBomb, "entri terlalu banyak")
	}
	var sum int64
	hasDoc := false
	for _, f := range zr.File {
		name := strings.ToLower(f.Name)
		if strings.Contains(name, "..") || strings.HasPrefix(name, "/") {
			return nil, fail(ReasonCorrupt, "nama entri tidak aman")
		}
		if strings.HasSuffix(name, "vbaproject.bin") || strings.HasSuffix(name, "vbadata.xml") {
			return nil, fail(ReasonMacro, "makro VBA")
		}
		u := int64(f.UncompressedSize64)
		if f.CompressedSize64 > 0 && u/int64(f.CompressedSize64) > lim.DocxMaxRatio && u > 1<<20 {
			return nil, fail(ReasonArchiveBomb, "rasio kompresi")
		}
		sum += u
		if name == "word/document.xml" {
			hasDoc = true
		}
	}
	if sum > lim.DocxMaxUncompressed {
		return nil, fail(ReasonArchiveBomb, "ukuran terurai")
	}
	if !hasDoc {
		return nil, fail(ReasonUnsupported, "bukan dokumen Word")
	}
	r := &docxReader{zr: zr, lim: lim}
	ct, err := r.read("[Content_Types].xml")
	if err != nil {
		return nil, err
	}
	if bytes.Contains(bytes.ToLower(ct), []byte("macroenabled")) {
		return nil, fail(ReasonMacro, "dokumen bermakro")
	}
	return r, nil
}

// read membaca satu part dengan batas aktual (tidak percaya ukuran di header zip).
func (r *docxReader) read(name string) ([]byte, error) {
	for _, f := range r.zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fail(ReasonCorrupt, "part tidak dapat dibuka")
		}
		defer rc.Close()
		remain := r.lim.DocxMaxUncompressed - r.total
		b, err := io.ReadAll(io.LimitReader(rc, remain+1))
		if err != nil {
			return nil, fail(ReasonCorrupt, "part rusak")
		}
		if int64(len(b)) > remain {
			return nil, fail(ReasonArchiveBomb, "ukuran terurai")
		}
		r.total += int64(len(b))
		return b, nil
	}
	return nil, fail(ReasonCorrupt, "part hilang: "+name)
}

func (r *docxReader) has(prefix string) bool {
	for _, f := range r.zr.File {
		if strings.HasPrefix(f.Name, prefix) {
			return true
		}
	}
	return false
}

func (r *docxReader) parts(dir, prefix string) []string {
	var out []string
	for _, f := range r.zr.File {
		if path.Dir(f.Name) == dir && strings.HasPrefix(path.Base(f.Name), prefix) && strings.HasSuffix(f.Name, ".xml") {
			out = append(out, f.Name)
		}
	}
	sort.Strings(out)
	return out
}

// para adalah satu paragraf dengan locator semantik.
type para struct {
	loc   string
	text  string
	bold  bool
	cells []string // diisi untuk baris tabel bermode data
}

// cell adalah isi satu sel tabel (paragraf berurutan).
type cell struct{ paras []string }

type docxStats struct {
	paragraphs, tables, textboxes, drawings, chars int
}

// parsePart berjalan atas token XML; paragraf di dalam text box (w:txbxContent) dicatat dengan locator tersendiri.
func parsePart(name string, data []byte, st *docxStats) ([]para, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	var out []para
	var cur *strings.Builder
	boldRuns, runs := 0, 0
	inRunBold := false
	pIdx, tIdx, rIdx, cIdx, txIdx := 0, 0, -1, -1, 0
	depthTbl, depthTx := 0, 0
	var rowCells []cell
	var rowLocs []string
	base := path.Base(name)
	locFor := func() string {
		switch {
		case depthTx > 0:
			return fmt.Sprintf("%s#textbox%d.p%d", base, txIdx, pIdx)
		case depthTbl > 0:
			return fmt.Sprintf("%s#t%d.r%d.c%d.p%d", base, tIdx, rIdx, cIdx, pIdx)
		}
		return fmt.Sprintf("%s#p%d", base, pIdx)
	}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fail(ReasonCorrupt, "xml rusak")
		}
		switch t := tok.(type) {
		case xml.Directive:
			if bytes.HasPrefix(bytes.TrimSpace(bytes.ToUpper(t)), []byte("DOCTYPE")) {
				return nil, fail(ReasonCorrupt, "DOCTYPE tidak diizinkan")
			}
		case xml.StartElement:
			switch t.Name.Local {
			case "tbl":
				depthTbl++
				if depthTbl == 1 {
					tIdx++
					rIdx = -1
				}
				st.tables++
			case "tr":
				if depthTbl == 1 {
					rIdx++
					cIdx = -1
					rowCells = nil
					rowLocs = nil
				}
			case "tc":
				if depthTbl == 1 {
					cIdx++
					rowCells = append(rowCells, cell{})
					rowLocs = append(rowLocs, fmt.Sprintf("%s#t%d.r%d.c%d", base, tIdx, rIdx, cIdx))
				}
			case "txbxContent":
				depthTx++
				txIdx++
				st.textboxes++
			case "drawing", "pict":
				st.drawings++
			case "p":
				cur = &strings.Builder{}
				boldRuns, runs = 0, 0
			case "r":
				inRunBold = false
				runs++
			case "b":
				val := ""
				for _, a := range t.Attr {
					if a.Name.Local == "val" {
						val = a.Value
					}
				}
				if val == "" || val == "1" || val == "true" {
					inRunBold = true
				}
			case "t":
				var s string
				if err := dec.DecodeElement(&s, &t); err != nil {
					return nil, fail(ReasonCorrupt, "teks rusak")
				}
				if cur != nil {
					cur.WriteString(s)
					if inRunBold && strings.TrimSpace(s) != "" {
						boldRuns++
					}
				}
			case "tab":
				if cur != nil {
					cur.WriteString("  ")
				}
			case "br", "cr":
				if cur != nil {
					cur.WriteString("\n")
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "p":
				if cur == nil {
					continue
				}
				txt := cur.String()
				cur = nil
				for i, piece := range strings.Split(txt, "\n") {
					piece = strings.TrimSpace(piece)
					if piece == "" {
						continue
					}
					st.paragraphs++
					st.chars += len(piece)
					loc := locFor()
					if i > 0 {
						loc += fmt.Sprintf(".br%d", i)
					}
					if depthTbl == 1 && depthTx == 0 && len(rowCells) > 0 {
						rowCells[len(rowCells)-1].paras = append(rowCells[len(rowCells)-1].paras, piece)
						continue
					}
					out = append(out, para{loc: loc, text: piece, bold: runs > 0 && boldRuns*2 >= runs})
				}
				pIdx++
			case "tr":
				if depthTbl == 1 {
					out = append(out, rowParas(rowCells, rowLocs)...)
				}
			case "tbl":
				depthTbl--
			case "txbxContent":
				depthTx--
			}
		}
	}
	return out, nil
}

// rowParas: baris tabel data (mis. [tahun | jabatan + uraian]) digabung menjadi satu header agar
// tahun di kolom tersendiri tetap terpasang ke entrinya. Tabel tata letak (sel berisi banyak paragraf)
// dibaca per sel, kiri lalu kanan.
func rowParas(cells []cell, locs []string) []para {
	var out []para
	layout := false
	for _, c := range cells {
		if len(c.paras) > 4 {
			layout = true
		}
	}
	if layout || len(cells) == 1 {
		for i, c := range cells {
			for j, p := range c.paras {
				out = append(out, para{loc: fmt.Sprintf("%s.p%d", locs[i], j), text: p})
			}
		}
		return out
	}
	var head []string
	var headLoc []string
	for i, c := range cells {
		if len(c.paras) > 0 {
			head = append(head, c.paras[0])
			headLoc = append(headLoc, fmt.Sprintf("%s.p0", locs[i]))
		}
	}
	if len(head) > 0 {
		out = append(out, para{loc: strings.Join(headLoc, "+"), text: strings.Join(head, " | "), cells: head})
	}
	for i, c := range cells {
		for j := 1; j < len(c.paras); j++ {
			out = append(out, para{loc: fmt.Sprintf("%s.p%d", locs[i], j), text: c.paras[j]})
		}
	}
	return out
}

// processDocx menghasilkan satu "halaman" semantik (bukan nomor halaman Word) berisi paragraf berurutan.
// needsRender=true bila isi teks terlalu sedikit dibanding gambar/drawing, atau text box banyak (urutan ambigu).
func processDocx(data []byte, lim Limits) (*Page, bool, string, error) {
	r, err := openDocx(data, lim)
	if err != nil {
		return nil, false, "", err
	}
	var st docxStats
	var all []para
	for _, part := range r.parts("word", "header") {
		b, err := r.read(part)
		if err != nil {
			return nil, false, "", err
		}
		ps, err := parsePart(part, b, &st)
		if err != nil {
			return nil, false, "", err
		}
		all = append(all, ps...)
	}
	b, err := r.read("word/document.xml")
	if err != nil {
		return nil, false, "", err
	}
	body, err := parsePart("word/document.xml", b, &st)
	if err != nil {
		return nil, false, "", err
	}
	all = append(all, body...)
	for _, part := range r.parts("word", "footer") {
		b, err := r.read(part)
		if err != nil {
			return nil, false, "", err
		}
		ps, err := parsePart(part, b, &st)
		if err != nil {
			return nil, false, "", err
		}
		all = append(all, ps...)
	}
	hasMedia := r.has("word/media/")
	switch {
	case st.chars < 80 && hasMedia:
		return nil, true, "docx_image_only", nil
	case st.textboxes >= 4:
		return nil, true, "docx_many_textboxes_order_ambiguous", nil
	case st.chars == 0:
		return nil, false, "", fail(ReasonEmpty, "dokumen tanpa teks")
	}
	p := &Page{No: 1, Method: "docx", Reason: "ooxml_structured", Status: "ok"}
	seen := map[string]bool{}
	for i, pr := range all {
		// header/footer berulang antar-section cukup sekali
		k := strings.SplitN(pr.loc, "#", 2)[0] + "|" + pr.text
		if (strings.HasPrefix(pr.loc, "header") || strings.HasPrefix(pr.loc, "footer")) && seen[k] {
			continue
		}
		seen[k] = true
		words := strings.Fields(pr.text)
		l := &Line{ID: fmt.Sprintf("d-%d", i+1), Page: 1, Source: "docx", Conf: -1, Locator: pr.loc, Bold: pr.bold}
		for _, w := range words {
			l.Words = append(l.Words, Word{Text: w, Conf: -1})
		}
		p.Lines = append(p.Lines, l)
	}
	p.Quality = Quality{Words: st.chars / 6, Chars: st.chars, LetterRate: 1, Score: 1}
	return p, false, "", nil
}
