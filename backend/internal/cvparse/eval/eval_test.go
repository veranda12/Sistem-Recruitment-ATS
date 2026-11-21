package eval

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ats/internal/cvparse"
)

// TestFixtureQuality menjalankan parser sungguhan (Poppler, Tesseract, LibreOffice) atas fixture hasil
// go run ./cmd/cvfixtures dan menegakkan ambang kualitas. Dilewati bila fixture/tool tidak tersedia.
func TestFixtureQuality(t *testing.T) {
	dir := os.Getenv("CV_FIXTURES")
	if dir == "" {
		dir = filepath.Join("..", "..", "..", "..", "tests", "cv-fixtures", "out")
	}
	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err != nil {
		t.Skip("fixture belum dibuat (go run ./cmd/cvfixtures)")
	}
	tools := cvparse.ToolsFromEnv()
	if tools.PDFToText == "" || tools.Tesseract == "" {
		t.Skip("Poppler/Tesseract tidak tersedia")
	}
	work := t.TempDir()
	rs, err := Run(context.Background(), dir, tools, cvparse.DefaultLimits(), work, func(string, ...any) {})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rs {
		if !r.OutcomeOK {
			// .doc tanpa LibreOffice: alasan operasional yang benar, bukan sukses palsu
			if r.Fixture.Format == "doc" && tools.Soffice == "" && r.Outcome == string(cvparse.ReasonToolMissing) {
				continue
			}
			t.Errorf("%s: harapan %s, hasil %s", r.Fixture.File, r.Fixture.Expect, r.Outcome)
		}
		if len(r.Fabricated) > 0 {
			t.Errorf("%s: nilai rekaan %v", r.Fixture.File, r.Fabricated)
		}
		if r.Evidence.FP > 0 || r.Evidence.FN > 0 {
			t.Errorf("%s: bukti tidak valid %+v", r.Fixture.File, r.Evidence)
		}
	}
	agg := Aggregate(rs, func(r FixtureResult) string { return Stratum(r.Fixture) })
	check := func(stratum, field string, minP, minR, minF1 float64) {
		c := agg[stratum][field]
		if c == nil {
			t.Errorf("%s/%s tidak terukur", stratum, field)
			return
		}
		if c.P() < minP || c.R() < minR || c.F1() < minF1 {
			t.Errorf("%s/%s P=%.3f R=%.3f F1=%.3f di bawah ambang (%.2f/%.2f/%.2f)", stratum, field, c.P(), c.R(), c.F1(), minP, minR, minF1)
		}
	}
	check("digital", "email", 0.95, 0.95, 0)
	check("digital", "phone", 0.95, 0.95, 0)
	check("ocr", "email", 0.90, 0.90, 0)
	check("ocr", "phone", 0.90, 0.90, 0)
	check("digital", "experience_tuple", 0, 0, 0.85)
	check("ocr", "experience_tuple", 0, 0, 0.75)
}
