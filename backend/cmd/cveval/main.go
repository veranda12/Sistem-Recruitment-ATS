// cveval menjalankan parser CV atas fixture berlabel dan menulis laporan kualitas.
//
//	go run ./cmd/cveval -fixtures ../tests/cv-fixtures/out -report ../docs/cv-parsing-eval.json
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"ats/internal/cvparse"
	"ats/internal/cvparse/eval"
)

func main() {
	dir := flag.String("fixtures", "../tests/cv-fixtures/out", "direktori fixture")
	report := flag.String("report", "", "tulis laporan JSON ke path ini")
	verbose := flag.Bool("v", false, "cetak hasil per fixture")
	flag.Parse()
	tools := cvparse.ToolsFromEnv()
	work, _ := os.MkdirTemp("", "cveval-")
	defer os.RemoveAll(work)
	logf := func(string, ...any) {}
	if *verbose {
		logf = func(f string, a ...any) { fmt.Printf(f+"\n", a...) }
	}
	rs, err := eval.Run(context.Background(), *dir, tools, cvparse.DefaultLimits(), work, logf)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// Ringkasan hasil per fixture
	bad := 0
	for _, r := range rs {
		if !r.OutcomeOK {
			bad++
			fmt.Printf("HASIL TIDAK SESUAI: %s harapan=%s hasil=%s\n", r.Fixture.File, r.Fixture.Expect, r.Outcome)
		}
	}
	fmt.Printf("\nfixture: %d, hasil sesuai: %d, tidak sesuai: %d\n", len(rs), len(rs)-bad, bad)
	unique := map[string]map[string]bool{}
	for _, r := range rs {
		if r.Fixture.Persona == "" {
			continue
		}
		if unique[r.Fixture.Split] == nil {
			unique[r.Fixture.Split] = map[string]bool{}
		}
		unique[r.Fixture.Split][r.Fixture.Persona] = true
	}
	for s, m := range unique {
		fmt.Printf("CV unik %s: %d\n", s, len(m))
	}

	type row struct {
		Key, Field string
		TP, FP, FN int
		P, R, F1   float64
	}
	var rows []row
	agg := eval.Aggregate(rs, func(r eval.FixtureResult) string { return r.Fixture.Split + "/" + eval.Stratum(r.Fixture) })
	agg2 := eval.Aggregate(rs, func(r eval.FixtureResult) string { return r.Fixture.Split + "/" + r.Fixture.Format })
	for k, v := range agg2 {
		agg["fmt:"+k] = v
	}
	fmt.Printf("\n%-28s %-18s %4s %4s %4s %6s %6s %6s\n", "split/strata", "field", "TP", "FP", "FN", "P", "R", "F1")
	for _, k := range eval.SortedKeys(agg) {
		for _, f := range eval.SortedKeys(agg[k]) {
			c := agg[k][f]
			rows = append(rows, row{k, f, c.TP, c.FP, c.FN, c.P(), c.R(), c.F1()})
			fmt.Printf("%-28s %-18s %4d %4d %4d %6.3f %6.3f %6.3f\n", k, f, c.TP, c.FP, c.FN, c.P(), c.R(), c.F1())
		}
	}
	// Bukti, nilai rekaan, CER, waktu
	var ev eval.Counts
	var fabricated []string
	var cerDist, cerChars int
	durs := map[string][]time.Duration{}
	for _, r := range rs {
		ev.TP += r.Evidence.TP
		ev.FP += r.Evidence.FP
		ev.FN += r.Evidence.FN
		for _, f := range r.Fabricated {
			fabricated = append(fabricated, r.Fixture.File+": "+f)
		}
		cerDist += r.OCRDist
		cerChars += r.OCRChars
		durs[r.Fixture.Format] = append(durs[r.Fixture.Format], r.Duration)
	}
	fmt.Printf("\nbukti valid: %d, bukti tidak memuat nilai: %d, tanpa bukti: %d\n", ev.TP, ev.FP, ev.FN)
	fmt.Printf("nilai rekaan pada fixture 'absent': %d %v\n", len(fabricated), fabricated)
	cer := 0.0
	if cerChars > 0 {
		cer = float64(cerDist) / float64(cerChars)
	}
	fmt.Printf("CER OCR (terhadap teks native persona yang sama): %.4f (%d/%d karakter)\n", cer, cerDist, cerChars)
	type timing struct {
		Format   string
		N        int
		P50, P95 int64
	}
	var tims []timing
	for f, ds := range durs {
		sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
		p := func(q float64) int64 { return ds[min(len(ds)-1, int(q*float64(len(ds)-1)+0.5))].Milliseconds() }
		tims = append(tims, timing{f, len(ds), p(0.5), p(0.95)})
	}
	sort.Slice(tims, func(i, j int) bool { return tims[i].Format < tims[j].Format })
	fmt.Println("\nwaktu proses (tanpa antre):")
	for _, t := range tims {
		fmt.Printf("  %-12s n=%-3d p50=%5dms p95=%5dms\n", t.Format, t.N, t.P50, t.P95)
	}
	if *report != "" {
		type fx struct {
			File, Expect, Outcome string
			OK                    bool
			Ms                    int64
			Pages                 []string
			Mismatches            []string
		}
		var fxs []fx
		for _, r := range rs {
			fxs = append(fxs, fx{r.Fixture.File, r.Fixture.Expect, r.Outcome, r.OutcomeOK, r.Duration.Milliseconds(), r.PageMethods, r.Mismatches})
		}
		host, _ := os.Hostname()
		b, _ := json.MarshalIndent(map[string]any{"generatedAt": time.Now().Format(time.RFC3339), "host": host, "tools": cvparse.ToolVersions(context.Background(), tools),
			"parser": cvparse.ParserVersion, "rows": rows, "evidence": ev, "fabricated": fabricated, "cer": cer, "cerChars": cerChars, "timings": tims,
			"fixtures": fxs, "uniqueCV": map[string]int{"dev": len(unique["dev"]), "holdout": len(unique["holdout"])}}, "", "  ")
		if err := os.WriteFile(*report, b, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
	_ = strings.Join
}
