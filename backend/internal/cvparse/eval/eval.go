// Package eval mengukur mutu parser CV terhadap fixture berlabel (golden JSON).
// Normalisasi pembanding: huruf kecil, spasi dirapatkan, tanda baca di ujung dibuang.
// Kesalahan menambah data (false positive) dan field terlewat (false negative) masuk denominator.
package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"ats/internal/cvfixture"
	"ats/internal/cvparse"
)

type Counts struct {
	TP, FP, FN int
}

func (c Counts) P() float64  { return ratio(c.TP, c.TP+c.FP) }
func (c Counts) R() float64  { return ratio(c.TP, c.TP+c.FN) }
func (c Counts) F1() float64 { p, r := c.P(), c.R(); return ratio2(2*p*r, p+r) }

func ratio(a, b int) float64 {
	if b == 0 {
		return 1
	}
	return float64(a) / float64(b)
}
func ratio2(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

// FixtureResult adalah hasil satu fixture.
type FixtureResult struct {
	Fixture     cvfixture.Fixture
	Outcome     string // ok | partial | <reason>
	OutcomeOK   bool
	Fields      map[string]*Counts
	Evidence    Counts // TP = bukti valid, FP = bukti tidak memuat nilai, FN = tanpa bukti
	Fabricated  []string
	Duration    time.Duration
	Pages       int
	OCRChars    int
	OCRDist     int
	Mismatches  []string
	PageMethods []string
}

func norm(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Join(strings.Fields(s), " ")
	return strings.Trim(s, " .,;:")
}

func compact(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '@' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// matchSets menghitung TP/FP/FN atas multiset nilai ternormalisasi.
func matchSets(gold, got []string) (Counts, []string) {
	var c Counts
	var miss []string
	pool := map[string]int{}
	for _, g := range gold {
		pool[norm(g)]++
	}
	for _, v := range got {
		if pool[norm(v)] > 0 {
			pool[norm(v)]--
			c.TP++
		} else {
			c.FP++
			miss = append(miss, "+"+v)
		}
	}
	for k, n := range pool {
		for i := 0; i < n; i++ {
			c.FN++
			miss = append(miss, "-"+k)
		}
	}
	return c, miss
}

// Collected: hasil ekstraksi dikelompokkan per entri.
type Collected struct {
	Name, City                                      string
	Emails, Phones, Links, Skills, Languages, Certs []string
	Exp                                             []map[string]string
	Edu                                             []map[string]string
	All                                             []cvparse.Suggestion
}

func collect(ex cvparse.Extraction) Collected {
	var c Collected
	exp := map[string]map[string]string{}
	edu := map[string]map[string]string{}
	var expKeys, eduKeys []string
	for _, s := range ex.Suggestions {
		c.All = append(c.All, s)
		switch {
		case s.Section == "contact" && s.Field == "name":
			c.Name, _ = s.Normalized.(string)
		case s.Field == "city":
			c.City, _ = s.Normalized.(string)
		case s.Field == "email":
			c.Emails = append(c.Emails, fmt.Sprint(s.Normalized))
		case s.Field == "phone":
			c.Phones = append(c.Phones, s.Normalized.(cvparse.Phone).E164)
		case s.Field == "url":
			c.Links = append(c.Links, s.Normalized.(cvparse.SafeURL).Target)
		case s.Field == "skill":
			c.Skills = append(c.Skills, fmt.Sprint(s.Normalized))
		case s.Field == "language":
			c.Languages = append(c.Languages, s.Normalized.(map[string]string)["name"])
		case s.Section == "certifications" && s.Field == "name":
			c.Certs = append(c.Certs, fmt.Sprint(s.Normalized))
		case s.Section == "experience":
			m, ok := exp[s.EntryKey]
			if !ok {
				m = map[string]string{}
				exp[s.EntryKey] = m
				expKeys = append(expKeys, s.EntryKey)
			}
			m[s.Field] = valueString(s)
		case s.Section == "education":
			m, ok := edu[s.EntryKey]
			if !ok {
				m = map[string]string{}
				edu[s.EntryKey] = m
				eduKeys = append(eduKeys, s.EntryKey)
			}
			m[s.Field] = valueString(s)
		}
	}
	for _, k := range expKeys {
		c.Exp = append(c.Exp, exp[k])
	}
	for _, k := range eduKeys {
		c.Edu = append(c.Edu, edu[k])
	}
	return c
}

func valueString(s cvparse.Suggestion) string {
	switch v := s.Normalized.(type) {
	case cvparse.Period:
		start, end := "", ""
		if v.Start != nil {
			start = v.Start.String()
		}
		if v.End != nil {
			end = v.End.String()
		}
		if v.Current {
			end = "current"
		}
		return start + ".." + end
	case nil:
		return ""
	}
	return fmt.Sprint(s.Normalized)
}

func expTuple(g cvfixture.GoldenExp) string {
	end := g.End
	if g.Current {
		end = "current"
	}
	return norm(g.Company) + "|" + norm(g.Title) + "|" + g.Start + ".." + end
}

func eduTuple(g cvfixture.GoldenEdu) string {
	return norm(g.Institution) + "|" + g.Level + "|" + norm(g.Major) + "|" + fmt.Sprint(g.EndYear)
}

func gotEduTuple(m map[string]string) string {
	end := ""
	if p := m["period"]; p != "" {
		parts := strings.SplitN(p, "..", 2)
		if len(parts) == 2 {
			end = parts[1]
			if end == "" || end == "current" {
				end = parts[0]
			}
		}
		if len(end) > 4 {
			end = end[:4]
		}
	}
	if end == "" {
		end = "0"
	}
	return norm(m["institution"]) + "|" + m["level"] + "|" + norm(m["major"]) + "|" + end
}

// Score membandingkan hasil dengan golden.
func Score(doc *cvparse.Doc, ex cvparse.Extraction, g cvfixture.Golden, r *FixtureResult) {
	c := collect(ex)
	r.Fields = map[string]*Counts{}
	add := func(field string, cnt Counts, miss []string) {
		r.Fields[field] = &cnt
		for _, m := range miss {
			r.Mismatches = append(r.Mismatches, field+" "+m)
		}
	}
	name := []string{}
	if c.Name != "" {
		name = []string{c.Name}
	}
	cn, m := matchSets([]string{g.Name}, name)
	add("name", cn, m)
	cn, m = matchSets(g.Emails, c.Emails)
	add("email", cn, m)
	cn, m = matchSets(g.Phones, c.Phones)
	add("phone", cn, m)
	gc := []string{}
	if g.City != "" {
		gc = []string{g.City}
	}
	city := []string{}
	if c.City != "" {
		city = []string{c.City}
	}
	cn, m = matchSets(gc, city)
	add("city", cn, m)
	cn, m = matchSets(g.Links, c.Links)
	add("links", cn, m)
	cn, m = matchSets(g.Skills, c.Skills)
	add("skills", cn, m)
	if g.Languages != nil {
		cn, m = matchSets(g.Languages, c.Languages)
		add("languages", cn, m)
	}
	if g.Certs != nil {
		cn, m = matchSets(g.Certs, c.Certs)
		add("certifications", cn, m)
	}
	var ge, xe []string
	for _, e := range g.Experience {
		ge = append(ge, expTuple(e))
	}
	for _, e := range c.Exp {
		xe = append(xe, norm(e["company"])+"|"+norm(e["title"])+"|"+e["period"])
	}
	cn, m = matchSets(ge, xe)
	add("experience_tuple", cn, m)
	var gd, xd []string
	for _, e := range g.Education {
		gd = append(gd, eduTuple(e))
	}
	for _, e := range c.Edu {
		xd = append(xd, gotEduTuple(e))
	}
	cn, m = matchSets(gd, xd)
	add("education_tuple", cn, m)

	// Bukti: teks baris bukti harus memuat nilai mentah.
	for _, s := range c.All {
		if s.Raw == "" {
			continue
		}
		if len(s.Evidence) == 0 {
			r.Evidence.FN++
			continue
		}
		var txt strings.Builder
		for _, id := range s.Evidence {
			if l := doc.Line(id); l != nil {
				txt.WriteString(l.Text())
				txt.WriteString(" ")
			}
		}
		if strings.Contains(compact(txt.String()), compact(s.Raw)) {
			r.Evidence.TP++
		} else {
			r.Evidence.FP++
			r.Mismatches = append(r.Mismatches, "evidence "+s.Path)
		}
	}
	for _, a := range g.Absent {
		for _, s := range c.All {
			if strings.Contains(strings.ToLower(s.Raw+" "+fmt.Sprint(s.Normalized)), strings.ToLower(a)) {
				r.Fabricated = append(r.Fabricated, a+" @ "+s.Path)
			}
		}
	}
}

// Levenshtein rune untuk CER.
func levenshtein(a, b []rune) int {
	if len(a) == 0 {
		return len(b)
	}
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

func docText(d *cvparse.Doc, only string) string {
	var parts []string
	for _, p := range d.Pages {
		for _, l := range p.Lines {
			if only == "" || l.Source == only {
				parts = append(parts, l.Text())
			}
		}
	}
	return strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
}

// Run mengevaluasi semua fixture di dir (manifest.json + golden/).
func Run(ctx context.Context, dir string, tools cvparse.Tools, lim cvparse.Limits, work string, logf func(string, ...any)) ([]FixtureResult, error) {
	mb, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var list []cvfixture.Fixture
	if err := json.Unmarshal(mb, &list); err != nil {
		return nil, err
	}
	goldens := map[string]cvfixture.Golden{}
	refText := map[string]string{} // teks native persona (pdf-1col) sebagai rujukan CER
	var out []FixtureResult
	for _, f := range list {
		if f.Persona != "" {
			if _, ok := goldens[f.Persona]; !ok {
				b, err := os.ReadFile(filepath.Join(dir, "golden", f.Persona+".json"))
				if err != nil {
					return nil, err
				}
				var g cvfixture.Golden
				_ = json.Unmarshal(b, &g)
				goldens[f.Persona] = g
			}
		}
		r := FixtureResult{Fixture: f}
		t0 := time.Now()
		res, err := cvparse.Process(ctx, tools, lim, filepath.Join(dir, f.Split, f.File), work, filepath.Join(work, "out", f.File), nil)
		r.Duration = time.Since(t0)
		switch {
		case err != nil:
			var fl *cvparse.Failure
			if ok := asFailure(err, &fl); ok {
				r.Outcome = string(fl.Reason)
			} else {
				r.Outcome = "error:" + err.Error()
			}
		default:
			r.Outcome = "ok"
			for _, p := range res.Doc.Pages {
				if p.Status != "ok" {
					r.Outcome = "partial"
				}
			}
		}
		r.OutcomeOK = r.Outcome == f.Expect
		if res != nil && res.Doc != nil {
			r.Pages = len(res.Doc.Pages)
			for _, p := range res.Doc.Pages {
				r.PageMethods = append(r.PageMethods, fmt.Sprintf("%d:%s/%s", p.No, p.Method, p.Reason))
			}
			if f.Persona != "" && (r.Outcome == "ok" || r.Outcome == "partial") {
				ex := cvparse.Extract(res.Doc)
				Score(res.Doc, ex, goldens[f.Persona], &r)
				if f.Variant == "pdf-1col" {
					refText[f.Persona] = docText(res.Doc, "")
				}
				if ref, ok := refText[f.Persona]; ok && (f.Format == "image" || f.Format == "pdf_scan") {
					got := []rune(compactSpace(docText(res.Doc, "ocr")))
					want := []rune(compactSpace(ref))
					if len(got) > 0 && len(want) > 0 && f.Variant != "partial-page2" {
						r.OCRChars = len(want)
						r.OCRDist = levenshtein(got, want)
					}
				}
			}
		}
		logf("%-34s %-16s %-18s %6dms %s", f.File, f.Expect, r.Outcome, r.Duration.Milliseconds(), strings.Join(r.Mismatches, "; "))
		out = append(out, r)
	}
	return out, nil
}

func compactSpace(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

func asFailure(err error, f **cvparse.Failure) bool {
	for e := err; e != nil; {
		if x, ok := e.(*cvparse.Failure); ok {
			*f = x
			return true
		}
		u, ok := e.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		e = u.Unwrap()
	}
	return false
}

// Stratum: kunci agregasi.
func Stratum(f cvfixture.Fixture) string {
	switch f.Format {
	case "pdf_native", "docx", "doc":
		return "digital"
	case "pdf_scan", "image", "pdf_mixed":
		return "ocr"
	}
	return "other"
}

// Aggregate menjumlahkan counts per (split, stratum|format, field).
func Aggregate(rs []FixtureResult, key func(FixtureResult) string) map[string]map[string]*Counts {
	out := map[string]map[string]*Counts{}
	for _, r := range rs {
		k := key(r)
		if out[k] == nil {
			out[k] = map[string]*Counts{}
		}
		for f, c := range r.Fields {
			if out[k][f] == nil {
				out[k][f] = &Counts{}
			}
			out[k][f].TP += c.TP
			out[k][f].FP += c.FP
			out[k][f].FN += c.FN
		}
	}
	return out
}

func SortedKeys[M ~map[string]V, V any](m M) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
