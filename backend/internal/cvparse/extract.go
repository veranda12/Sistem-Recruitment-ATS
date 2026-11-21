package cvparse

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Suggestion adalah satu field usulan. Path stabil dalam satu run, mis. "experience[1].title".
type Suggestion struct {
	Path       string   `json:"path"`
	Section    string   `json:"section"`
	EntryKey   string   `json:"entryKey,omitempty"`
	Field      string   `json:"field"`
	Raw        string   `json:"raw"`
	Normalized any      `json:"normalized"`
	Evidence   []string `json:"evidence"`
	ReadDoubt  []string `json:"readDoubt,omitempty"`  // keraguan pembacaan karakter
	AssocDoubt []string `json:"assocDoubt,omitempty"` // keraguan memasangkan field ke entri
}

// Extraction adalah hasil interpretasi deterministik.
type Extraction struct {
	Suggestions []Suggestion `json:"suggestions"`
	Skipped     []string     `json:"skipped"` // kode: sensitive_line, instruction_text, reference_contact
}

type sec string

const (
	secNone     sec = ""
	secSummary  sec = "summary"
	secExp      sec = "experience"
	secEdu      sec = "education"
	secSkills   sec = "skills"
	secLang     sec = "languages"
	secCert     sec = "certifications"
	secProjects sec = "projects"
	secContact  sec = "contact"
	secRefs     sec = "references"
	secPersonal sec = "personal"
	secOther    sec = "other"
)

var headings = map[string]sec{
	"ringkasan": secSummary, "ringkasan profil": secSummary, "profil": secSummary, "profil singkat": secSummary, "profil diri": secSummary,
	"tentang saya": secSummary, "summary": secSummary, "professional summary": secSummary, "profile": secSummary, "about me": secSummary,
	"objective": secSummary, "career objective": secSummary, "tujuan karier": secSummary, "ringkasan profesional": secSummary,
	"pengalaman": secExp, "pengalaman kerja": secExp, "pengalaman profesional": secExp, "riwayat pekerjaan": secExp, "riwayat kerja": secExp,
	"pengalaman organisasi dan kerja": secExp, "experience": secExp, "work experience": secExp, "professional experience": secExp,
	"employment history": secExp, "employment": secExp, "career history": secExp, "work history": secExp, "pengalaman magang": secExp,
	"pendidikan": secEdu, "riwayat pendidikan": secEdu, "pendidikan formal": secEdu, "education": secEdu, "academic background": secEdu,
	"educational background": secEdu, "latar belakang pendidikan": secEdu,
	"keahlian": secSkills, "keterampilan": secSkills, "kemampuan": secSkills, "skill": secSkills, "skills": secSkills, "technical skills": secSkills,
	"hard skills": secSkills, "soft skills": secSkills, "kompetensi": secSkills, "keahlian teknis": secSkills, "core skills": secSkills,
	"bahasa": secLang, "kemampuan bahasa": secLang, "language": secLang, "languages": secLang, "penguasaan bahasa": secLang,
	"sertifikasi": secCert, "sertifikat": secCert, "pelatihan": secCert, "pelatihan dan sertifikasi": secCert, "sertifikasi dan pelatihan": secCert,
	"certification": secCert, "certifications": secCert, "licenses dan certifications": secCert, "training": secCert, "trainings": secCert,
	"courses": secCert, "kursus": secCert, "certifications dan training": secCert,
	"proyek": secProjects, "projects": secProjects, "portfolio": secProjects, "portofolio": secProjects, "proyek pilihan": secProjects, "selected projects": secProjects,
	"kontak": secContact, "contact": secContact, "informasi kontak": secContact, "contact information": secContact, "contacts": secContact,
	"referensi": secRefs, "references": secRefs, "reference": secRefs,
	"data pribadi": secPersonal, "data diri": secPersonal, "personal information": secPersonal, "personal details": secPersonal, "biodata": secPersonal,
	"organisasi": secOther, "pengalaman organisasi": secOther, "penghargaan": secOther, "awards": secOther, "hobi": secOther, "interests": secOther,
	"minat": secOther, "volunteer": secOther, "kegiatan": secOther,
}

// detectHeading: baris pendek tanpa angka yang sama dengan (atau diawali) frasa heading.
func detectHeading(l *Line) (sec, bool) {
	t := strings.TrimSpace(strings.TrimRight(l.Text(), ":"))
	if t == "" || strings.ContainsAny(t, "@0123456789") || len(strings.Fields(t)) > 5 {
		return "", false
	}
	k := normKey(t)
	if s, ok := headings[k]; ok {
		return s, true
	}
	return "", false
}

var (
	sensitiveRe = regexp.MustCompile(`(?i)^\s*(agama|religion|jenis\s+kelamin|gender|sex|status\s+(perkawinan|pernikahan|kawin|marital)|marital|nik|no\.?\s*ktp|nomor\s+ktp|ktp|tinggi\s+badan|berat\s+badan|tempat[,/\s]+tanggal\s+lahir|tanggal\s+lahir|ttl|date\s+of\s+birth|birth\s*date|usia|umur|age|kewarganegaraan|nationality|suku|golongan\s+darah)\b`)
	// Teks yang menyuruh "sistem" berbuat sesuatu tidak pernah menjadi data atau kebijakan.
	instructionRe  = regexp.MustCompile(`(?i)(ignore|abaikan|disregard|lupakan)\b.{0,40}\b(instruction|instruksi|previous|sebelumnya|aturan|rules)|\b(system|assistant|sistem)\s*:|as an ai|you are (now )?(an?|the) |\b(set|ubah|ganti|tambahkan|add)\b.{0,30}\b(email|skill|keahlian|nilai|score|status)\b.{0,30}\b(to|menjadi|jadi|ke)\b|\b(ats|parser|ai|model)\b.{0,20}\b(harus|wajib|must|should)\b`)
	instruction2Re = regexp.MustCompile(`(?i)(tambahkan|add|ubah|ganti|set|change)\s+(skill|keahlian|email|nilai|score|status)`)
	// termasuk salah-baca OCR yang umum untuk bullet (“ » °)
	bulletRe      = regexp.MustCompile(`^\s*(?:[•▪●◦·‣∙○■□➢➤✓✔*“”»«°]|-\s|–\s|\d{1,2}[.)]\s)`)
	splitHeaderRe = regexp.MustCompile(`\s+(?:\||—|–|-|•|·|@|at|di)\s+|\s*\|\s*`)
	ocrPlusRe     = regexp.MustCompile(`(^|[\s|:])[tT](62\s?\d)`)
	gpaRe         = regexp.MustCompile(`(?i)\b(?:ipk|gpa|ip)\s*[:=]?\s*([0-4][.,]\d{1,2})(?:\s*/\s*4(?:[.,]0{1,2})?)?`)
)

var companyCues = []string{"pt", "cv", "tbk", "ltd", "inc", "llc", "corp", "corporation", "co", "bank", "group", "persero", "koperasi", "yayasan",
	"rumah sakit", "rs", "toko", "ud", "firma", "studio", "agency", "tech", "technologies", "technology", "solutions", "digital", "consulting",
	"logistik", "logistics", "manufacturing", "industries", "industri", "indonesia", "startup", "company", "perum", "bumn", "gmbh", "pte"}

var titleCues = []string{"engineer", "manager", "staff", "staf", "analyst", "analis", "developer", "programmer", "supervisor", "admin", "administrator",
	"operator", "officer", "intern", "magang", "kepala", "lead", "leader", "specialist", "spesialis", "consultant", "konsultan", "designer", "desainer",
	"accountant", "akuntan", "sales", "marketing", "teknisi", "technician", "asisten", "assistant", "koordinator", "coordinator", "direktur", "director",
	"head", "executive", "eksekutif", "associate", "trainee", "guru", "teacher", "dosen", "perawat", "nurse", "drafter", "surveyor", "kasir", "cashier",
	"customer service", "representative", "architect", "arsitek", "scientist", "researcher", "peneliti", "editor", "writer", "penulis", "freelance",
	"freelancer", "owner", "founder", "chief", "cto", "ceo", "qa", "tester", "devops", "sre", "recruiter", "hr", "foreman", "mandor", "helper", "driver", "sopir"}

var institutionCues = []string{"universitas", "university", "institut", "institute", "politeknik", "polytechnic", "sekolah", "school", "sma", "sman",
	"smk", "smkn", "ma", "man", "college", "akademi", "academy", "stie", "stmik", "stt", "sekolah tinggi", "univ"}

var cities = []string{"jakarta", "jakarta selatan", "jakarta barat", "jakarta timur", "jakarta utara", "jakarta pusat", "bandung", "surabaya", "medan",
	"semarang", "yogyakarta", "jogja", "makassar", "palembang", "tangerang", "tangerang selatan", "bekasi", "depok", "bogor", "malang", "solo", "surakarta",
	"denpasar", "batam", "pekanbaru", "padang", "balikpapan", "samarinda", "pontianak", "banjarmasin", "manado", "cirebon", "karawang", "cikarang",
	"purwakarta", "serang", "cilegon", "sidoarjo", "gresik", "kediri", "jember", "lampung", "bandar lampung", "jambi", "bengkulu", "mataram", "kupang",
	"ambon", "jayapura", "sukabumi", "tasikmalaya", "cimahi", "magelang", "salatiga", "tegal", "pekalongan", "kudus", "madiun", "probolinggo",
	"pasuruan", "mojokerto", "singapore", "singapura", "kuala lumpur"}

func hasCue(s string, cues []string) bool {
	k := " " + normKey(s) + " "
	for _, c := range cues {
		if strings.Contains(k, " "+c+" ") {
			return true
		}
	}
	return false
}

func knownCity(s string) string {
	k := normKey(s)
	best := ""
	for _, c := range cities {
		if k == c || strings.HasPrefix(k, c+" ") || strings.HasSuffix(k, " "+c) || strings.Contains(k, " "+c+" ") {
			if len(c) > len(best) {
				best = c
			}
		}
	}
	return best
}

type secLine struct {
	l       *Line
	sec     sec
	heading bool
}

// sections memberi tiap baris section-nya. Section diingat per kolom dan berlanjut antarhalaman.
func sections(o Ordered) []secLine {
	out := make([]secLine, 0, len(o.Lines))
	byCol := map[int]sec{}
	last := secNone
	page, col := 0, -1
	for _, l := range o.Lines {
		if l.Page != page {
			page, col = l.Page, -1
		}
		if l.Column != col {
			if s, ok := byCol[l.Column]; ok {
				last = s
			} else if col != -1 {
				last = secNone // kolom baru di halaman yang sama tanpa heading: belum diketahui
			}
			col = l.Column
		}
		if s, ok := detectHeading(l); ok {
			last = s
			byCol[l.Column] = s
			out = append(out, secLine{l, s, true})
			continue
		}
		byCol[l.Column] = last
		out = append(out, secLine{l, last, false})
	}
	return out
}

type extractor struct {
	doc  *Doc
	res  Extraction
	seen map[string]int
}

func (x *extractor) add(s Suggestion) {
	x.res.Suggestions = append(x.res.Suggestions, s)
}

func (x *extractor) ocrDoubt(ids ...string) []string {
	for _, id := range ids {
		if l := x.doc.Line(id); l != nil && l.Source == "ocr" && l.Conf >= 0 && l.Conf < 80 {
			return []string{"Hasil pembacaan gambar kurang jelas, periksa ulang"}
		}
	}
	return nil
}

// Extract menjalankan interpretasi deterministik atas dokumen.
func Extract(d *Doc) Extraction {
	x := &extractor{doc: d, seen: map[string]int{}}
	all := sections(ReadingOrder(d))
	instr := markInstructions(all)
	var usable []secLine
	for i, sl := range all {
		t := sl.l.Text()
		switch {
		case instr[i]:
			x.res.Skipped = append(x.res.Skipped, "instruction_text")
			continue
		case sl.sec == secPersonal && !emailRe.MatchString(t) && !phoneRe.MatchString(t) && !urlRe.MatchString(t) && !strings.Contains(strings.ToLower(t), "domisili"):
			x.res.Skipped = append(x.res.Skipped, "personal_line")
			continue
		case sensitiveRe.MatchString(t):
			x.res.Skipped = append(x.res.Skipped, "sensitive_line")
			continue
		}
		usable = append(usable, sl)
	}
	x.contact(usable)
	x.summary(usable)
	x.experience(usable)
	x.education(usable)
	x.listSection(usable, secSkills, "skills", "skill")
	x.listSection(usable, secLang, "languages", "language")
	x.certifications(usable)
	x.projects(usable)
	return x.res
}

// markInstructions menandai teks yang menyuruh sistem berbuat sesuatu, termasuk baris lanjutan
// paragraf yang sama (kolom sama, ukuran huruf mirip, jarak vertikal rapat).
func markInstructions(all []secLine) map[int]bool {
	out := map[int]bool{}
	isInstr := func(t string) bool { return instructionRe.MatchString(t) || instruction2Re.MatchString(t) }
	for i, sl := range all {
		if !isInstr(sl.l.Text()) {
			continue
		}
		out[i] = true
		for _, dir := range []int{-1, 1} {
			prev := sl.l
			for j := i + dir; j >= 0 && j < len(all); j += dir {
				c := all[j].l
				if all[j].heading || c.Page != prev.Page || c.Column != prev.Column {
					break
				}
				h := max(prev.B.H(), c.B.H())
				gap := c.B.Y0 - prev.B.Y1
				if dir < 0 {
					gap = prev.B.Y0 - c.B.Y1
				}
				sameSize := c.Source == "docx" || (h > 0 && min(prev.B.H(), c.B.H())/h > 0.8)
				if c.Source != "docx" && (gap > h*0.8 || !sameSize) {
					break
				}
				if c.Source == "docx" && c.Locator != "" && prev.Locator != "" && !sameParagraph(c.Locator, prev.Locator) {
					break
				}
				out[j] = true
				prev = c
			}
		}
	}
	return out
}

// sameParagraph: locator DOCX dengan dasar paragraf yang sama (berbeda hanya pada .brN).
func sameParagraph(a, b string) bool {
	cut := func(s string) string {
		if i := strings.Index(s, ".br"); i >= 0 {
			return s[:i]
		}
		return s
	}
	return cut(a) == cut(b)
}

func of(ls []secLine, s sec) []secLine {
	var out []secLine
	for _, l := range ls {
		if l.sec == s && !l.heading {
			out = append(out, l)
		}
	}
	return out
}

// ---- Kontak ----

func (x *extractor) contact(ls []secLine) {
	contactAllowed := func(s sec) bool {
		return s == secNone || s == secContact || s == secPersonal || s == secSummary || s == secOther || s == secSkills || s == secLang
	}
	var emails, phones, links []Suggestion
	seenE, seenP, seenL := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, sl := range ls {
		if sl.heading {
			continue
		}
		t := sl.l.Text()
		if sl.sec == secRefs {
			if emailRe.MatchString(t) || phoneRe.MatchString(t) {
				x.res.Skipped = append(x.res.Skipped, "reference_contact")
			}
			continue
		}
		if !contactAllowed(sl.sec) {
			continue
		}
		for _, m := range emailRe.FindAllString(t, -1) {
			n := strings.ToLower(m)
			if !seenE[n] {
				seenE[n] = true
				emails = append(emails, Suggestion{Section: "contact", Field: "email", Raw: m, Normalized: n, Evidence: []string{sl.l.ID}, ReadDoubt: x.ocrDoubt(sl.l.ID)})
			}
		}
		masked := emailRe.ReplaceAllString(urlRe.ReplaceAllString(t, " "), " ")
		var ocrFix bool
		if sl.l.Source == "ocr" {
			// OCR sering membaca "+" sebagai "t" di depan kode negara; tandai agar kandidat memeriksa.
			fixed := ocrPlusRe.ReplaceAllString(masked, "${1}+${2}")
			ocrFix = fixed != masked
			masked = fixed
		}
		for _, m := range phoneRe.FindAllString(masked, -1) {
			if p, ok := NormalizePhone(m); ok && !seenP[p.E164] {
				seenP[p.E164] = true
				doubt := x.ocrDoubt(sl.l.ID)
				if sl.l.Source == "ocr" {
					doubt = append(doubt, "Nomor dibaca dari gambar, periksa setiap digit")
				}
				if ocrFix {
					doubt = append(doubt, "Tanda + mungkin salah baca")
				}
				phones = append(phones, Suggestion{Section: "contact", Field: "phone", Raw: strings.TrimSpace(m), Normalized: p, Evidence: []string{sl.l.ID}, ReadDoubt: doubt})
			}
		}
		for _, m := range urlRe.FindAllString(t, -1) {
			if emailRe.MatchString(m) {
				continue
			}
			if u, ok := NormalizeURL(m); ok && !seenL[u.Target] {
				seenL[u.Target] = true
				links = append(links, Suggestion{Section: "links", Field: "url", Raw: m, Normalized: u, Evidence: []string{sl.l.ID}, ReadDoubt: x.ocrDoubt(sl.l.ID)})
			}
		}
	}
	for i, s := range emails {
		s.Path = fmt.Sprintf("contact.email[%d]", i)
		if len(emails) > 1 {
			s.AssocDoubt = append(s.AssocDoubt, "Ada lebih dari satu email")
		}
		x.add(s)
	}
	for i, s := range phones {
		s.Path = fmt.Sprintf("contact.phone[%d]", i)
		if len(phones) > 1 {
			s.AssocDoubt = append(s.AssocDoubt, "Ada dua nomor telepon")
		}
		x.add(s)
	}
	for i, s := range links {
		s.Path = fmt.Sprintf("links[%d]", i)
		s.EntryKey = fmt.Sprintf("links[%d]", i)
		x.add(s)
	}
	x.name(ls)
	x.city(ls)
}

func (x *extractor) name(ls []secLine) {
	var best *Line
	var firstPage *Page
	for _, p := range x.doc.Pages {
		if p.Status == "ok" {
			firstPage = p
			break
		}
	}
	if firstPage == nil {
		return
	}
	for i, sl := range ls {
		l := sl.l
		if l.Page != firstPage.No || sl.heading || i > 25 {
			continue
		}
		if l.Source != "docx" && firstPage.H > 0 && l.B.Y0 > firstPage.H*0.35 {
			continue
		}
		t := strings.TrimSpace(l.Text())
		w := strings.Fields(t)
		if len(w) < 2 || len(w) > 5 || strings.ContainsAny(t, "@:/|0123456789,") || hasCue(t, titleCues) || hasCue(t, companyCues) || knownCity(t) != "" {
			continue
		}
		letters := true
		for _, r := range t {
			if !(unicode.IsLetter(r) || r == ' ' || r == '.' || r == '\'' || r == '-') {
				letters = false
			}
		}
		if !letters {
			continue
		}
		if best == nil || l.B.H() > best.B.H()*1.15 || (l.Source == "docx" && l.Bold && !best.Bold) {
			best = l
		}
	}
	if best == nil {
		return
	}
	raw := best.Text()
	norm := raw
	if strings.ToUpper(raw) == raw {
		norm = titleCase(raw)
	}
	x.add(Suggestion{Path: "contact.name", Section: "contact", Field: "name", Raw: raw, Normalized: collapseSpace(norm), Evidence: []string{best.ID}, ReadDoubt: x.ocrDoubt(best.ID)})
}

func titleCase(s string) string {
	w := strings.Fields(strings.ToLower(s))
	for i, p := range w {
		r := []rune(p)
		r[0] = unicode.ToUpper(r[0])
		w[i] = string(r)
	}
	return strings.Join(w, " ")
}

var cityLabelRe = regexp.MustCompile(`(?i)^\s*(domisili|kota|city|lokasi|location|tempat tinggal|alamat|address)\s*[:\-]\s*(.+)$`)

// city hanya bila eksplisit: berlabel (Domisili: ...) atau baris kontak yang berisi nama kota dikenal.
func (x *extractor) city(ls []secLine) {
	for _, sl := range ls {
		if sl.heading || !(sl.sec == secNone || sl.sec == secContact || sl.sec == secPersonal) {
			continue
		}
		t := sl.l.Text()
		if m := cityLabelRe.FindStringSubmatch(t); m != nil {
			c := knownCity(m[2])
			if c == "" {
				continue
			}
			x.add(Suggestion{Path: "contact.city", Section: "contact", Field: "city", Raw: strings.TrimSpace(m[2]), Normalized: titleCase(c), Evidence: []string{sl.l.ID}, ReadDoubt: x.ocrDoubt(sl.l.ID)})
			return
		}
	}
	for _, sl := range ls {
		if sl.heading || !(sl.sec == secNone || sl.sec == secContact) {
			continue
		}
		t := sl.l.Text()
		if len(strings.Fields(t)) > 5 || emailRe.MatchString(t) || phoneRe.MatchString(t) {
			continue
		}
		if c := knownCity(t); c != "" {
			x.add(Suggestion{Path: "contact.city", Section: "contact", Field: "city", Raw: t, Normalized: titleCase(c), Evidence: []string{sl.l.ID}, ReadDoubt: x.ocrDoubt(sl.l.ID)})
			return
		}
	}
}

// ---- Ringkasan ----

func (x *extractor) summary(ls []secLine) {
	lines := of(ls, secSummary)
	if len(lines) == 0 {
		return
	}
	var parts, ids []string
	for _, sl := range lines {
		parts = append(parts, stripBullet(sl.l.Text()))
		ids = append(ids, sl.l.ID)
	}
	text := joinParagraph(parts)
	if len([]rune(text)) > 2000 {
		text = string([]rune(text)[:2000])
	}
	x.add(Suggestion{Path: "summary", Section: "summary", Field: "summary", Raw: text, Normalized: text, Evidence: ids, ReadDoubt: x.ocrDoubt(ids...)})
}

func stripBullet(s string) string {
	return strings.TrimSpace(bulletRe.ReplaceAllString(s, ""))
}

// joinParagraph menyambung baris terpotong; poin bullet tetap per baris.
func joinParagraph(parts []string) string {
	var b strings.Builder
	for i, p := range parts {
		if i > 0 {
			prev := parts[i-1]
			if strings.HasSuffix(prev, ".") || strings.HasSuffix(prev, ":") {
				b.WriteString("\n")
			} else {
				b.WriteString(" ")
			}
		}
		b.WriteString(p)
	}
	return strings.TrimSpace(b.String())
}

// ---- Pengalaman ----

type piece struct {
	text string
	line string
}

func isBulletLine(l *Line) bool { return bulletRe.MatchString(l.Text()) }

func shortHeader(l *Line) bool {
	t := strings.TrimSpace(l.Text())
	return !isBulletLine(l) && len(strings.Fields(t)) <= 9 && !strings.HasSuffix(t, ".")
}

// entryBlocks membagi baris section menjadi entri berdasarkan jangkar tanggal.
// header = maks. 2 baris pendek sebelum jangkar + jangkar + baris pendek sesudahnya (total maks. 3).
type entryBlock struct {
	header []*Line
	desc   []*Line
	period Period
	anchor *Line
	single bool // tanggal tunggal, bukan rentang
}

func entryBlocks(lines []secLine) []entryBlock {
	var anchors []int
	periods := map[int]Period{}
	single := map[int]bool{}
	for i, sl := range lines {
		if p, _, ok := FindPeriod(sl.l.Text()); ok {
			anchors = append(anchors, i)
			periods[i] = p
		}
	}
	if len(anchors) == 0 {
		for i, sl := range lines {
			if !shortHeader(sl.l) {
				continue
			}
			if d, _, ok := FindSingleDate(sl.l.Text()); ok {
				anchors = append(anchors, i)
				dd := d
				periods[i] = Period{Start: &dd, Raw: dd.String()}
				single[i] = true
			}
		}
	}
	var out []entryBlock
	used := make([]bool, len(lines))
	for k, a := range anchors {
		lo := 0
		if k > 0 {
			lo = anchors[k-1] + 1
		}
		var pre []int
		for i := a - 1; i >= lo && len(pre) < 2; i-- {
			if used[i] || !shortHeader(lines[i].l) {
				break
			}
			pre = append([]int{i}, pre...)
		}
		// Baris sebelum header milik deskripsi entri sebelumnya kecuali tidak ada entri sebelumnya.
		hdr := append(pre, a)
		hi := len(lines)
		if k+1 < len(anchors) {
			hi = anchors[k+1]
		}
		for i := a + 1; i < hi && len(hdr) < 3; i++ {
			if !shortHeader(lines[i].l) || len(strings.Fields(lines[i].l.Text())) > 7 {
				break
			}
			// baris pendek sesudah jangkar hanya header bila jangkar sendiri tidak memuat jabatan/perusahaan
			rest := strings.TrimSpace(removeSpan(lines[a].l.Text(), periods[a].Raw))
			if len(strings.Fields(rest)) >= 2 && len(pre) > 0 {
				break
			}
			hdr = append(hdr, i)
		}
		for _, i := range hdr {
			used[i] = true
		}
		b := entryBlock{period: periods[a], anchor: lines[a].l, single: single[a]}
		for _, i := range hdr {
			b.header = append(b.header, lines[i].l)
		}
		out = append(out, b)
	}
	// Deskripsi: baris tak terpakai antara header entri dan header entri berikutnya.
	for k := range out {
		start := indexOf(lines, out[k].header[len(out[k].header)-1]) + 1
		end := len(lines)
		if k+1 < len(out) {
			end = indexOf(lines, out[k+1].header[0])
		}
		for i := start; i < end; i++ {
			if !used[i] {
				out[k].desc = append(out[k].desc, lines[i].l)
				used[i] = true
			}
		}
	}
	return out
}

func indexOf(ls []secLine, l *Line) int {
	for i, s := range ls {
		if s.l == l {
			return i
		}
	}
	return -1
}

func removeSpan(s, span string) string {
	if span == "" {
		return s
	}
	return strings.Replace(s, span, " ", 1)
}

// headerPieces memecah header menjadi potongan teks (tanpa tanggal) dengan asal barisnya.
func headerPieces(b entryBlock) []piece {
	var out []piece
	for _, l := range b.header {
		t := l.Text()
		if l == b.anchor {
			t = removeSpan(t, b.period.Raw)
		}
		t = strings.Trim(stripBullet(t), " ,|()–—-:")
		if t == "" {
			continue
		}
		for _, p := range splitHeaderRe.Split(t, -1) {
			p = strings.Trim(p, " ,|()–—-:")
			if p == "" {
				continue
			}
			// "Jabatan, PT X" -> dua potongan bila salah satu sisi berpenanda perusahaan.
			if i := strings.LastIndex(p, ", "); i > 0 && (hasCue(p[i+2:], companyCues) || hasCue(p[:i], companyCues) || knownCity(p[i+2:]) != "") {
				out = append(out, piece{strings.TrimSpace(p[:i]), l.ID}, piece{strings.TrimSpace(p[i+2:]), l.ID})
				continue
			}
			out = append(out, piece{p, l.ID})
		}
	}
	return out
}

func isLocation(p string) bool {
	k := normKey(p)
	if k == "remote" || k == "hybrid" || k == "wfh" || k == "onsite" {
		return true
	}
	c := knownCity(p)
	return c != "" && len(strings.Fields(k)) <= len(strings.Fields(c))+2 && !hasCue(p, companyCues)
}

func (x *extractor) experience(ls []secLine) {
	lines := of(ls, secExp)
	blocks := entryBlocks(lines)
	lastCompany := piece{}
	lastCompanyOnly := false
	n := 0
	for _, b := range blocks {
		ps := headerPieces(b)
		var title, company, loc piece
		var rest []piece
		for _, p := range ps {
			switch {
			case loc.text == "" && isLocation(p.text):
				loc = p
			case company.text == "" && hasCue(p.text, companyCues) && !hasCue(p.text, titleCues):
				company = p
			case title.text == "" && hasCue(p.text, titleCues):
				title = p
			default:
				rest = append(rest, p)
			}
		}
		var assoc []string
		byOrder := false
		for _, p := range rest {
			switch {
			case title.text == "":
				title, byOrder = p, true
			case company.text == "":
				company, byOrder = p, true
			}
		}
		companyOnly := false
		if company.text == "" && lastCompanyOnly && title.text != "" {
			company = lastCompany // beberapa jabatan di bawah satu perusahaan
		}
		// Perusahaan sendirian di baris tersendiri menjadi induk untuk jabatan berikutnya.
		for _, l := range b.header {
			if company.text != "" && l.ID == company.line && l != b.anchor && len(strings.Fields(l.Text())) == len(strings.Fields(company.text)) {
				companyOnly = true
			}
		}
		if byOrder && title.text != "" && company.text != "" {
			assoc = append(assoc, "Jabatan dan perusahaan ditentukan dari urutan, pastikan tidak tertukar")
		}
		if title.text == "" && company.text == "" {
			continue
		}
		key := fmt.Sprintf("experience[%d]", n)
		n++
		add := func(field, raw string, norm any, ev []string, a []string) {
			x.add(Suggestion{Path: key + "." + field, Section: "experience", EntryKey: key, Field: field, Raw: raw, Normalized: norm, Evidence: ev, ReadDoubt: x.ocrDoubt(ev...), AssocDoubt: a})
		}
		if title.text != "" {
			add("title", title.text, collapseSpace(title.text), []string{title.line}, assoc)
		} else {
			add("title", "", nil, nil, []string{"Jabatan tidak ditemukan"})
		}
		if company.text != "" {
			add("company", company.text, collapseSpace(company.text), []string{company.line}, assoc)
		} else {
			add("company", "", nil, nil, []string{"Perusahaan tidak ditemukan"})
		}
		if loc.text != "" {
			add("location", loc.text, collapseSpace(loc.text), []string{loc.line}, nil)
		}
		var pd []string
		if b.single {
			pd = append(pd, "Hanya satu tahun tertulis")
		}
		add("period", strings.TrimSpace(b.period.Raw), b.period, []string{b.anchor.ID}, pd)
		if len(b.desc) > 0 {
			var parts, ids []string
			for _, l := range b.desc {
				parts = append(parts, stripBullet(l.Text()))
				ids = append(ids, l.ID)
			}
			d := strings.Join(parts, "\n")
			if len([]rune(d)) > 2000 {
				d = string([]rune(d)[:2000])
			}
			add("description", d, d, ids, nil)
		}
		if company.text != "" {
			lastCompany = company
		}
		lastCompanyOnly = companyOnly || (lastCompanyOnly && company == lastCompany)
	}
}

// ---- Pendidikan ----

var levelPatterns = []struct {
	re    *regexp.Regexp
	level string
}{
	{regexp.MustCompile(`(?i)\b(s3|doktor(al)?|doctor(ate)?|ph\.?d)\b`), "S3"},
	{regexp.MustCompile(`(?i)\b(s2|magister|master(?:'s)?|m\.?sc|mba|m\.?kom|m\.?t\.?)\b`), "S2"},
	{regexp.MustCompile(`(?i)\b(d4|diploma\s*(iv|4)|sarjana\s+terapan|s\.?tr)\b`), "D4"},
	{regexp.MustCompile(`(?i)\b(s1|sarjana|bachelor(?:'s)?|b\.?sc|b\.?eng|s\.?kom|s\.?t\.?|s\.?e\.?|s\.?h\.?|s\.?pd|undergraduate)\b`), "S1"},
	{regexp.MustCompile(`(?i)\b(d3|diploma\s*(iii|3)|ahli\s+madya|a\.?md|associate\s+degree)\b`), "D3"},
	{regexp.MustCompile(`(?i)\b(smk|smkn|stm)\b`), "SMK"},
	{regexp.MustCompile(`(?i)\b(sma|sman|smu|ma|man|slta|high\s+school)\b`), "SMA"},
}

// degreeLineRe: baris gelar ("S1 Teknik Sipil", "SMK Teknik Mesin", "Bachelor of ...") bukan baris institusi,
// kecuali memuat penanda nama sekolah (Negeri, Swasta, angka).
var (
	degreeLineRe = regexp.MustCompile(`(?i)^\s*(s1|s2|s3|d3|d4|sma|smk|smu|stm|sarjana|magister|diploma|bachelor|master|doctor)\b`)
	schoolNameRe = regexp.MustCompile(`(?i)\b(negeri|swasta|\d+)\b`)
	degreeTypeRe = regexp.MustCompile(`(?i)^(science|arts|engineering|commerce|business|laws?)\s+(.+)$`)
)

var majorRe = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:jurusan|program\s+studi|prodi|major(?:ing)?\s+in|majoring)\s*:?\s*(.+)$`),
	regexp.MustCompile(`(?i)\b(?:bachelor|master|doctor|diploma|sarjana|magister)\b[^,]*?\b(?:of|in)\s+(?:science\s+in\s+|arts\s+in\s+)?(.+)$`),
	regexp.MustCompile(`(?i)^\s*(?:s1|s2|s3|d3|d4|sma|smk|smu|stm)\s*[-:]?\s+(.+)$`),
}

func detectLevel(s string) string {
	lv, _ := detectLevelRaw(s)
	return lv
}

// detectLevelRaw mengembalikan jenjang ternormalisasi dan teks aslinya (untuk bukti).
func detectLevelRaw(s string) (string, string) {
	for _, lp := range levelPatterns {
		if m := lp.re.FindString(s); m != "" {
			return lp.level, m
		}
	}
	return "", ""
}

func (x *extractor) education(ls []secLine) {
	lines := of(ls, secEdu)
	if len(lines) == 0 {
		return
	}
	// Entri dimulai di baris institusi; tanpa penanda institusi, pakai jangkar tanggal.
	type eduEntry struct{ lines []*Line }
	var entries []eduEntry
	for _, sl := range lines {
		t := sl.l.Text()
		isInst := hasCue(t, institutionCues) && !isBulletLine(sl.l) && !(degreeLineRe.MatchString(t) && !schoolNameRe.MatchString(t) && len(entries) > 0)
		if isInst || len(entries) == 0 {
			entries = append(entries, eduEntry{})
		}
		entries[len(entries)-1].lines = append(entries[len(entries)-1].lines, sl.l)
	}
	n := 0
	for _, e := range entries {
		var inst, level, major, gpa piece
		levelRaw := ""
		var per Period
		var perLine string
		perSingle := false
		for _, l := range e.lines {
			t := l.Text()
			rest := t
			if p, span, ok := FindPeriod(t); ok && perLine == "" {
				per, perLine = p, l.ID
				rest = t[:span[0]] + " " + t[span[1]:]
			} else if d, span, ok := FindSingleDate(t); ok && perLine == "" && shortHeader(l) {
				dd := d
				per, perLine, perSingle = Period{End: &dd, Raw: d.String()}, l.ID, true
				rest = t[:span[0]] + " " + t[span[1]:]
			}
			if m := gpaRe.FindStringSubmatch(rest); m != nil && gpa.text == "" {
				gpa = piece{m[0], l.ID}
				rest = strings.Replace(rest, m[0], " ", 1)
			}
			rest = strings.Trim(stripBullet(rest), " ,|()–—-:")
			if rest == "" {
				continue
			}
			for _, p := range splitHeaderRe.Split(rest, -1) {
				p = strings.Trim(p, " ,|()–—-:")
				if p == "" {
					continue
				}
				switch {
				case inst.text == "" && hasCue(p, institutionCues) && !bareLevelRe.MatchString(p):
					// "SMK Negeri 2 Bekasi, Teknik Mesin" -> institusi + jurusan
					if i := strings.Index(p, ", "); i > 0 && !hasCue(p[i+2:], institutionCues) && knownCity(p[i+2:]) == "" {
						inst = piece{p[:i], l.ID}
						major = piece{p[i+2:], l.ID}
					} else {
						inst = piece{p, l.ID}
					}
					if lv, raw := detectLevelRaw(p); lv != "" && level.text == "" {
						level, levelRaw = piece{lv, l.ID}, raw
					}
				case level.text == "" && detectLevel(p) != "":
					lv, raw := detectLevelRaw(p)
					level, levelRaw = piece{lv, l.ID}, raw
					for _, re := range majorRe {
						if m := re.FindStringSubmatch(p); m != nil && major.text == "" {
							major = piece{strings.Trim(m[1], " ,.()"), l.ID}
						}
					}
				case major.text == "" && len(strings.Fields(p)) <= 8:
					mm := p
					for _, re := range majorRe {
						if m := re.FindStringSubmatch(p); m != nil {
							mm = strings.Trim(m[1], " ,.()")
						}
					}
					major = piece{mm, l.ID}
				}
			}
		}
		if inst.text == "" {
			continue
		}
		key := fmt.Sprintf("education[%d]", n)
		n++
		add := func(field, raw string, norm any, ev []string, doubt []string) {
			x.add(Suggestion{Path: key + "." + field, Section: "education", EntryKey: key, Field: field, Raw: raw, Normalized: norm, Evidence: ev, ReadDoubt: x.ocrDoubt(ev...), AssocDoubt: doubt})
		}
		add("institution", inst.text, collapseSpace(inst.text), []string{inst.line}, nil)
		if level.text != "" {
			add("level", levelRaw, level.text, []string{level.line}, nil)
		} else {
			add("level", "", "Lainnya", nil, []string{"Jenjang tidak tertulis"})
		}
		if major.text != "" {
			mj := collapseSpace(major.text)
			if m := degreeTypeRe.FindStringSubmatch(mj); m != nil && level.text != "" && !strings.Contains(strings.ToLower(mj), " in ") {
				mj = m[2] // "Science Statistics" dari "Bachelor of Science Statistics"
			}
			add("major", major.text, mj, []string{major.line}, nil)
		}
		if perLine != "" {
			var d []string
			if perSingle {
				d = append(d, "Hanya tahun lulus tertulis")
			}
			add("period", strings.TrimSpace(per.Raw), per, []string{perLine}, d)
		}
		if gpa.text != "" {
			m := gpaRe.FindStringSubmatch(gpa.text)
			v, err := strconv.ParseFloat(strings.Replace(m[1], ",", ".", 1), 64)
			if err == nil && v <= 4 {
				add("gpa", gpa.text, v, []string{gpa.line}, nil)
			}
		}
	}
}

// ---- Daftar (skill, bahasa) ----

// bareLevelRe: "SMA"/"SMK" tanpa nama sekolah adalah jenjang, bukan institusi.
var bareLevelRe = regexp.MustCompile(`(?i)^\s*(sma|smk|ma)\s*$`)
var langSplitRe = regexp.MustCompile(`\s*(?:,|;|\||•|·)\s*`)

var listSplitRe = regexp.MustCompile(`\s*(?:,|;|\||•|·|▪|●|\s/\s|\s-\s)\s*`)
var langLevelRe = regexp.MustCompile(`(?i)^(.+?)\s*(?:\(([^)]+)\)|[-:–]\s*(.+))$`)

func (x *extractor) listSection(ls []secLine, s sec, section, field string) {
	n := 0
	seen := map[string]bool{}
	for _, sl := range of(ls, s) {
		t := stripBullet(sl.l.Text())
		if i := strings.Index(t, ":"); i > 0 && i < 40 && section == "skills" {
			t = t[i+1:] // "Bahasa pemrograman: Go, Python"
		}
		items := listSplitRe.Split(t, -1)
		if section == "languages" {
			items = langSplitRe.Split(t, -1)
		}
		for _, it := range items {
			it = strings.Trim(it, " .")
			if it == "" || len([]rune(it)) > 60 || len(strings.Fields(it)) > 6 {
				continue
			}
			var norm any
			switch section {
			case "skills":
				c := CanonicalSkill(it)
				if seen[strings.ToLower(c)] {
					continue
				}
				seen[strings.ToLower(c)] = true
				norm = c
			case "languages":
				name, level := it, ""
				if m := langLevelRe.FindStringSubmatch(it); m != nil {
					name, level = strings.TrimSpace(m[1]), strings.TrimSpace(m[2]+m[3])
				}
				if seen[strings.ToLower(name)] {
					continue
				}
				seen[strings.ToLower(name)] = true
				norm = map[string]string{"name": name, "level": level}
			}
			key := fmt.Sprintf("%s[%d]", section, n)
			n++
			x.add(Suggestion{Path: key, Section: section, EntryKey: key, Field: field, Raw: it, Normalized: norm, Evidence: []string{sl.l.ID}, ReadDoubt: x.ocrDoubt(sl.l.ID)})
		}
	}
}

// ---- Sertifikasi ----

var issuerSplitRe = regexp.MustCompile(`\s+(?:—|–|-|\||oleh|by|dari|from)\s+|\s*\|\s*|,\s+`)

func (x *extractor) certifications(ls []secLine) {
	n := 0
	for _, sl := range of(ls, secCert) {
		t := stripBullet(sl.l.Text())
		if len(strings.Fields(t)) > 18 {
			continue // kalimat panjang: uraian, bukan nama sertifikat
		}
		var date *PartialDate
		if p, span, ok := FindPeriod(t); ok {
			date = p.Start
			t = t[:span[0]] + " " + t[span[1]:]
		} else if d, span, ok := FindSingleDate(t); ok {
			dd := d
			date = &dd
			t = t[:span[0]] + " " + t[span[1]:]
		}
		t = strings.Trim(t, " ,|()–—-:")
		if t == "" {
			continue
		}
		parts := issuerSplitRe.Split(t, 2)
		name := strings.Trim(parts[0], " ,()")
		issuer := ""
		if len(parts) == 2 {
			issuer = strings.Trim(parts[1], " ,()")
		}
		if len([]rune(name)) < 2 {
			continue
		}
		key := fmt.Sprintf("certifications[%d]", n)
		n++
		ev := []string{sl.l.ID}
		x.add(Suggestion{Path: key + ".name", Section: "certifications", EntryKey: key, Field: "name", Raw: name, Normalized: collapseSpace(name), Evidence: ev, ReadDoubt: x.ocrDoubt(ev...)})
		if issuer != "" {
			x.add(Suggestion{Path: key + ".issuer", Section: "certifications", EntryKey: key, Field: "issuer", Raw: issuer, Normalized: collapseSpace(issuer), Evidence: ev, ReadDoubt: x.ocrDoubt(ev...)})
		}
		if date != nil {
			x.add(Suggestion{Path: key + ".date", Section: "certifications", EntryKey: key, Field: "date", Raw: date.String(), Normalized: *date, Evidence: ev, ReadDoubt: x.ocrDoubt(ev...)})
		}
	}
}

// ---- Proyek ----

func (x *extractor) projects(ls []secLine) {
	lines := of(ls, secProjects)
	n := -1
	var key string
	var desc []string
	var ids []string
	flush := func() {
		if n >= 0 && len(desc) > 0 {
			d := strings.Join(desc, "\n")
			x.add(Suggestion{Path: key + ".description", Section: "projects", EntryKey: key, Field: "description", Raw: d, Normalized: d, Evidence: ids, ReadDoubt: x.ocrDoubt(ids...)})
		}
		desc, ids = nil, nil
	}
	for _, sl := range lines {
		t := sl.l.Text()
		if !isBulletLine(sl.l) && len(strings.Fields(t)) <= 8 && !strings.HasSuffix(strings.TrimSpace(t), ".") {
			flush()
			n++
			key = fmt.Sprintf("projects[%d]", n)
			name := strings.Trim(urlRe.ReplaceAllString(t, ""), " -–|()")
			if p, span, ok := FindPeriod(name); ok {
				_ = p
				name = strings.Trim(name[:span[0]]+name[span[1]:], " -–|()")
			}
			x.add(Suggestion{Path: key + ".name", Section: "projects", EntryKey: key, Field: "name", Raw: name, Normalized: collapseSpace(name), Evidence: []string{sl.l.ID}, ReadDoubt: x.ocrDoubt(sl.l.ID)})
			continue
		}
		if n >= 0 {
			desc = append(desc, stripBullet(t))
			ids = append(ids, sl.l.ID)
		}
	}
	flush()
}
