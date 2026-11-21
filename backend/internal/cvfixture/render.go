package cvfixture

import (
	"strings"
)

type labels struct{ summary, exp, edu, skills, langs, certs, contact, personal string }

func labelsFor(lang string) labels {
	switch lang {
	case "en":
		return labels{"SUMMARY", "EMPLOYMENT HISTORY", "EDUCATION", "SKILLS", "LANGUAGES", "CERTIFICATIONS", "CONTACT", "PERSONAL DETAILS"}
	case "bi":
		return labels{"Profil", "Work Experience", "Pendidikan", "Skills", "Bahasa", "Sertifikasi", "Contact", "Data Pribadi"}
	}
	return labels{"RINGKASAN", "PENGALAMAN KERJA", "PENDIDIKAN", "KEAHLIAN", "BAHASA", "SERTIFIKASI", "KONTAK", "DATA PRIBADI"}
}

// cursor menulis ke kolom (x, lebar) dan membuat halaman baru bila penuh.
type cursor struct {
	doc        *PDF
	page       *pdfPage
	x, w, y    float64
	top        float64
	bottom     float64
	onNewPage  func()
	pageBreaks int
}

func (c *cursor) need(h float64) {
	if c.y+h > c.bottom {
		c.page = c.doc.NewPage()
		c.y = c.top
		c.pageBreaks++
		if c.onNewPage != nil {
			c.onNewPage()
		}
	}
}

func (c *cursor) line(s string, size float64, bold bool) {
	for _, l := range wrap(s, size, c.w, bold) {
		c.need(size * 1.35)
		c.page.Text(c.x, c.y, size, bold, l)
		c.y += size * 1.35
	}
}

func (c *cursor) bullet(s string, size float64) {
	for i, l := range wrap(s, size, c.w-12, false) {
		c.need(size * 1.35)
		if i == 0 {
			c.page.Text(c.x, c.y, size, false, "• "+l)
		} else {
			c.page.Text(c.x+9, c.y, size, false, l)
		}
		c.y += size * 1.35
	}
}

func (c *cursor) heading(s string) {
	c.y += 8
	c.need(30)
	c.page.Text(c.x, c.y, 11.5, true, s)
	c.y += 18
}

func (c *cursor) gap(h float64) { c.y += h }

// experience menulis entri sesuai gaya persona.
func (c *cursor) experience(p Persona) {
	prevCompany := ""
	for _, e := range p.Exps {
		c.need(48)
		switch p.Style {
		case 0: // "Jabatan — Perusahaan" / "Periode | Lokasi"
			c.line(e.Title+" — "+e.Company, 10.5, true)
			meta := e.Period
			if e.Location != "" {
				meta += " | " + e.Location
			}
			c.line(meta, 9.5, false)
		case 1: // Perusahaan (sekali untuk beberapa jabatan) / "Jabatan | Periode"
			if e.Company != prevCompany {
				c.line(e.Company, 10.5, true)
			}
			c.line(e.Title+" | "+e.Period, 10, false)
		case 2: // Jabatan / "Perusahaan, Lokasi" / Periode
			c.line(e.Title, 10.5, true)
			loc := e.Company
			if e.Location != "" {
				loc += ", " + e.Location
			}
			c.line(loc, 10, false)
			c.line(e.Period, 9.5, false)
		default: // Periode / Jabatan / Perusahaan
			c.line(e.Period, 9.5, false)
			c.line(e.Title, 10.5, true)
			c.line(e.Company, 10, false)
		}
		for _, b := range e.Bullets {
			c.bullet(b, 9.5)
		}
		c.gap(6)
		prevCompany = e.Company
	}
}

func (c *cursor) education(p Persona) {
	for _, e := range p.Edu {
		c.need(40)
		c.line(e.Institution, 10.5, true)
		deg := strings.TrimSpace(e.Level + " " + e.Major)
		if p.Lang == "en" && e.Major != "" {
			deg = e.Level + " in " + e.Major
		}
		c.line(deg, 10, false)
		meta := e.Years
		if e.GPA != "" {
			meta += " | " + e.GPA
		}
		c.line(meta, 9.5, false)
		c.gap(4)
	}
}

// NativeOptions mengatur varian PDF native.
type NativeOptions struct {
	TwoColumn  bool
	DupLayer   bool // tulis ulang setiap baris sebagai teks tak terlihat di posisi sama
	EduFirst   bool // pendidikan sebelum pengalaman (untuk kasus halaman 2 gagal)
	ForcePage2 bool // pengalaman dipaksa mulai di halaman 2
}

// RenderNative membuat PDF native satu atau dua kolom.
func RenderNative(p Persona, o NativeOptions) []byte {
	doc := &PDF{}
	_ = RenderNativeInto(doc, p, o)
	return doc.Bytes()
}

// RenderNativeInto menggambar CV ke dokumen yang sudah ada (dipakai varian campuran).
func RenderNativeInto(doc *PDF, p Persona, o NativeOptions) error {
	L := labelsFor(p.Lang)
	first := doc.NewPage()
	first.Text(40, 36, 22, true, p.Name)
	if o.TwoColumn {
		first.Rect(30, 74, 168, pageH-104, 0.95)
		side := &cursor{doc: doc, page: first, x: 40, w: 150, y: 84, top: 40, bottom: pageH - 40}
		side.heading(L.contact)
		for _, e := range p.Emails {
			side.line(e, 9, false)
		}
		for _, ph := range p.Phones {
			side.line(ph, 9, false)
		}
		if p.City != "" {
			side.line(p.City, 9, false)
		}
		for _, l := range p.Links {
			side.line(l, 8.5, false)
		}
		side.heading(L.skills)
		for _, s := range p.Skills {
			side.line(s, 9.5, false)
		}
		if len(p.Languages) > 0 {
			side.heading(L.langs)
			for _, s := range p.Languages {
				side.line(s, 9.5, false)
			}
		}
		if p.Inject != "" {
			side.gap(10)
			for _, l := range wrap(p.Inject, 7, 150, false) {
				side.page.Gray(40, side.y, 7, false, l, 0.75)
				side.y += 9
			}
		}
		main := &cursor{doc: doc, page: first, x: 215, w: 340, y: 84, top: 40, bottom: pageH - 40}
		mainBody(main, p, L, o)
	} else {
		c := &cursor{doc: doc, page: first, x: 40, w: 515, y: 66, top: 40, bottom: pageH - 40}
		contact := strings.Join(append(append([]string{}, p.Emails...), p.Phones...), " | ")
		c.line(contact, 9.5, false)
		if p.City != "" {
			c.line("Domisili: "+p.City, 9.5, false)
		}
		if len(p.Links) > 0 {
			c.line(strings.Join(p.Links, " | "), 9, false)
		}
		if p.Inject != "" {
			for _, l := range wrap(p.Inject, 7, 515, false) {
				c.page.Gray(40, c.y, 7, false, l, 0.75)
				c.y += 9
			}
		}
		if len(p.Personal) > 0 {
			c.heading(L.personal)
			for _, s := range p.Personal {
				c.line(s, 9.5, false)
			}
		}
		mainBody(c, p, L, o)
		if len(p.Skills) > 0 {
			c.heading(L.skills)
			c.line(strings.Join(p.Skills, ", "), 9.5, false)
		}
		if len(p.Languages) > 0 {
			c.heading(L.langs)
			c.line(strings.Join(p.Languages, ", "), 9.5, false)
		}
	}
	if o.DupLayer {
		for _, pg := range doc.pages {
			dupLayer(pg)
		}
	}
	return nil
}

func mainBody(c *cursor, p Persona, L labels, o NativeOptions) {
	if p.Summary != "" {
		c.heading(L.summary)
		c.line(p.Summary, 9.5, false)
	}
	if len(p.Personal) > 0 && c.x > 100 {
		c.heading(L.personal)
		for _, s := range p.Personal {
			c.line(s, 9.5, false)
		}
	}
	exp := func() {
		if len(p.Exps) > 0 {
			if o.ForcePage2 {
				c.y = c.bottom + 1
			}
			c.heading(L.exp)
			c.experience(p)
		}
	}
	edu := func() {
		if len(p.Edu) > 0 {
			c.heading(L.edu)
			c.education(p)
		}
	}
	if o.EduFirst {
		edu()
		exp()
	} else {
		exp()
		edu()
	}
	if len(p.Certs) > 0 {
		c.heading(L.certs)
		for _, ce := range p.Certs {
			c.line(ce.Display, 9.5, false)
		}
	}
}

// dupLayer menyalin setiap perintah teks menjadi teks tak terlihat di posisi yang sama
// (meniru PDF yang sudah ber-OCR lalu di-OCR ulang).
func dupLayer(pg *pdfPage) {
	src := pg.content.String()
	var extra strings.Builder
	for _, ln := range strings.Split(src, "\n") {
		if strings.HasPrefix(ln, "BT ") && strings.Contains(ln, " 0 Tr ") {
			extra.WriteString(strings.Replace(ln, " 0 Tr ", " 3 Tr ", 1) + "\n")
		}
	}
	pg.content.WriteString(extra.String())
}
