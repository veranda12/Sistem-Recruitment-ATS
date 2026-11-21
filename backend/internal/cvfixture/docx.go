package cvfixture

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
)

func xmlEsc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

func wp(text string, bold bool) string {
	rpr := ""
	if bold {
		rpr = "<w:rPr><w:b/></w:rPr>"
	}
	return fmt.Sprintf(`<w:p><w:r>%s<w:t xml:space="preserve">%s</w:t></w:r></w:p>`, rpr, xmlEsc(text))
}

func cell(width int, paras ...string) string {
	return fmt.Sprintf(`<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/></w:tcPr>%s</w:tc>`, width, strings.Join(paras, ""))
}

const docxNS = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:v="urn:schemas-microsoft-com:vml" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"`

// DocxOptions: varian DOCX.
type DocxOptions struct {
	ImageOnly []byte // PNG satu halaman CV: dokumen hanya berisi gambar tertanam
	Macro     bool   // sertakan vbaProject.bin (harus ditolak)
}

// RenderDocx: kontak di header, pengalaman dalam tabel dengan tahun di kolom tersendiri,
// skill dalam tabel, sertifikasi di text box.
func RenderDocx(p Persona, o DocxOptions) []byte {
	L := labelsFor(p.Lang)
	var body strings.Builder
	if o.ImageOnly != nil {
		body.WriteString(`<w:p><w:r><w:drawing><wp:inline><wp:extent cx="5900000" cy="8350000"/><wp:docPr id="1" name="cv"/><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic><pic:nvPicPr><pic:cNvPr id="1" name="cv.png"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:embed="rIdImg"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill><pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="5900000" cy="8350000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>`)
	} else {
		if p.Inject != "" {
			body.WriteString(wp(p.Inject, false))
		}
		if len(p.Personal) > 0 {
			body.WriteString(wp(L.personal, true))
			for _, s := range p.Personal {
				body.WriteString(wp(s, false))
			}
		}
		if p.Summary != "" {
			body.WriteString(wp(L.summary, true))
			body.WriteString(wp(p.Summary, false))
		}
		if len(p.Exps) > 0 {
			body.WriteString(wp(L.exp, true))
			body.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="9000" w:type="dxa"/></w:tblPr>`)
			for _, e := range p.Exps {
				right := []string{wp(e.Title+" — "+e.Company, true)}
				if e.Location != "" {
					right = append(right, wp(e.Location, false))
				}
				for _, b := range e.Bullets {
					right = append(right, wp("• "+b, false))
				}
				body.WriteString("<w:tr>" + cell(2200, wp(e.Period, false)) + cell(6800, right...) + "</w:tr>")
			}
			body.WriteString(`</w:tbl>`)
		}
		if len(p.Edu) > 0 {
			body.WriteString(wp(L.edu, true))
			for _, e := range p.Edu {
				body.WriteString(wp(e.Institution, false))
				body.WriteString(wp(strings.TrimSpace(e.Level+" "+e.Major), false))
				meta := e.Years
				if e.GPA != "" {
					meta += " | " + e.GPA
				}
				body.WriteString(wp(meta, false))
			}
		}
		if len(p.Skills) > 0 {
			body.WriteString(wp(L.skills, true))
			body.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="9000" w:type="dxa"/></w:tblPr><w:tr>`)
			for _, s := range p.Skills {
				body.WriteString(cell(9000/len(p.Skills), wp(s, false)))
			}
			body.WriteString(`</w:tr></w:tbl>`)
		}
		if len(p.Languages) > 0 {
			body.WriteString(wp(L.langs, true))
			body.WriteString(wp(strings.Join(p.Languages, ", "), false))
		}
		if len(p.Certs) > 0 {
			var tb strings.Builder
			tb.WriteString(wp(L.certs, true))
			for _, c := range p.Certs {
				tb.WriteString(wp(c.Display, false))
			}
			body.WriteString(`<w:p><w:r><w:pict><v:shape style="width:440pt;height:80pt"><v:textbox><w:txbxContent>` + tb.String() + `</w:txbxContent></v:textbox></v:shape></w:pict></w:r></w:p>`)
		}
	}
	body.WriteString(`<w:sectPr><w:headerReference w:type="default" r:id="rIdH1"/><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1800" w:right="1200" w:bottom="1200" w:left="1200" w:header="600" w:footer="600" w:gutter="0"/></w:sectPr>`)

	var hdr strings.Builder
	if o.ImageOnly == nil {
		hdr.WriteString(wp(p.Name, true))
		hdr.WriteString(wp(strings.Join(append(append([]string{}, p.Emails...), p.Phones...), " | "), false))
		if p.City != "" {
			hdr.WriteString(wp("Domisili: "+p.City, false))
		}
		if len(p.Links) > 0 {
			hdr.WriteString(wp(strings.Join(p.Links, " | "), false))
		}
	} else {
		hdr.WriteString(`<w:p/>`)
	}
	ct := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Default Extension="png" ContentType="image/png"/>`
	if o.Macro {
		ct += `<Default Extension="bin" ContentType="application/vnd.ms-office.vbaProject"/><Override PartName="/word/document.xml" ContentType="application/vnd.ms-word.document.macroEnabled.main+xml"/>`
	} else {
		ct += `<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>`
	}
	ct += `<Override PartName="/word/header1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.header+xml"/></Types>`
	rels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`
	docRels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rIdH1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/header" Target="header1.xml"/>`
	if o.ImageOnly != nil {
		docRels += `<Relationship Id="rIdImg" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/cv.png"/>`
	}
	docRels += `</Relationships>`
	docXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document ` + docxNS + `><w:body>` + body.String() + `</w:body></w:document>`
	hdrXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:hdr ` + docxNS + `>` + hdr.String() + `</w:hdr>`

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name string, data []byte) {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.Modified = fixedTime
		w, _ := zw.CreateHeader(h)
		_, _ = w.Write(data)
	}
	add("[Content_Types].xml", []byte(ct))
	add("_rels/.rels", []byte(rels))
	add("word/document.xml", []byte(docXML))
	add("word/_rels/document.xml.rels", []byte(docRels))
	add("word/header1.xml", []byte(hdrXML))
	if o.ImageOnly != nil {
		add("word/media/cv.png", o.ImageOnly)
	}
	if o.Macro {
		add("word/vbaProject.bin", []byte("fixture-vba-placeholder"))
	}
	_ = zw.Close()
	return buf.Bytes()
}
