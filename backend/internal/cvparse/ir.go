// Package cvparse membaca CV kandidat (PDF, gambar, DOCX, DOC) menjadi saran profil beserta bukti.
// Semua hasil adalah saran: profil hanya berubah setelah kandidat menyetujui (lihat apply.go).
package cvparse

import (
	"fmt"
	"strings"
)

// ParserVersion naik setiap kali aturan ekstraksi berubah; ikut kunci idempotensi run.
const ParserVersion = "cvparse-1"

// BBox dalam satuan halaman: point untuk PDF, piksel untuk gambar/render.
type BBox struct {
	X0, Y0, X1, Y1 float64
}

func (b BBox) W() float64 { return b.X1 - b.X0 }
func (b BBox) H() float64 { return b.Y1 - b.Y0 }

func (b BBox) Union(o BBox) BBox {
	if b == (BBox{}) {
		return o
	}
	return BBox{min(b.X0, o.X0), min(b.Y0, o.Y0), max(b.X1, o.X1), max(b.Y1, o.Y1)}
}

// overlapX: panjang tumpang tindih horizontal.
func (b BBox) overlapX(o BBox) float64 { return max(0, min(b.X1, o.X1)-max(b.X0, o.X0)) }
func (b BBox) overlapY(o BBox) float64 { return max(0, min(b.Y1, o.Y1)-max(b.Y0, o.Y0)) }

// Word adalah token terkecil. Conf -1 untuk teks native (bukan OCR).
type Word struct {
	Text string  `json:"t"`
	B    BBox    `json:"b"`
	Conf float64 `json:"c"`
}

// Line adalah satuan bukti. ID stabil dalam satu run: "p<halaman>-l<urutan>" atau locator DOCX.
type Line struct {
	ID      string  `json:"id"`
	Page    int     `json:"page"`
	B       BBox    `json:"b"`
	Words   []Word  `json:"w"`
	Source  string  `json:"src"` // native | ocr | docx
	Conf    float64 `json:"conf"`
	Locator string  `json:"loc,omitempty"` // DOCX: part + paragraf/tabel, tanpa nomor halaman rekaan
	Column  int     `json:"col"`
	Bold    bool    `json:"bold,omitempty"`
	// Baris sel tabel yang digabung: MergedInto pada sel kiri, MergedFrom pada baris hasil gabungan.
	MergedInto string `json:"mergedInto,omitempty"`
	MergedFrom string `json:"mergedFrom,omitempty"`
	text       string
}

func (l *Line) Text() string {
	if l.text == "" {
		parts := make([]string, 0, len(l.Words))
		for _, w := range l.Words {
			parts = append(parts, w.Text)
		}
		l.text = strings.TrimSpace(strings.Join(parts, " "))
	}
	return l.text
}

// Page menyimpan metode baca dan alasannya agar keputusan fallback dapat diaudit.
type Page struct {
	No       int      `json:"no"`
	W        float64  `json:"w"`
	H        float64  `json:"h"`
	Method   string   `json:"method"` // native | ocr | docx | failed
	Reason   string   `json:"reason"`
	Status   string   `json:"status"` // ok | failed
	Failure  string   `json:"failure,omitempty"`
	Quality  Quality  `json:"quality"`
	ImageKey string   `json:"-"`
	Image    string   `json:"image,omitempty"` // nama berkas render privat di direktori run
	Scale    float64  `json:"scale,omitempty"` // piksel render per satuan BBox
	Lines    []*Line  `json:"lines"`
	Attempts []string `json:"attempts,omitempty"`
}

// Quality adalah sinyal mutu teks per halaman (bukan sekadar ada/tidaknya teks).
type Quality struct {
	Words           int     `json:"words"`
	Chars           int     `json:"chars"`
	ReplacementRate float64 `json:"replacementRate"`
	LetterRate      float64 `json:"letterRate"`
	DuplicateRate   float64 `json:"duplicateRate"`
	MeanConf        float64 `json:"meanConf,omitempty"`
	Score           float64 `json:"score"`
}

// Doc adalah representasi perantara seluruh dokumen.
type Doc struct {
	Format string  `json:"format"`
	Pages  []*Page `json:"pages"`
}

func (d *Doc) Line(id string) *Line {
	for _, p := range d.Pages {
		for _, l := range p.Lines {
			if l.ID == id {
				return l
			}
		}
	}
	return nil
}

func lineID(page, n int) string { return fmt.Sprintf("p%d-l%d", page, n) }

// Reason adalah kode alasan stabil untuk UI dan metrik (kardinalitas rendah).
type Reason string

const (
	ReasonUnsupported   Reason = "unsupported_format"
	ReasonCorrupt       Reason = "corrupt_file"
	ReasonEncrypted     Reason = "encrypted_file"
	ReasonEmpty         Reason = "empty_document"
	ReasonTooLarge      Reason = "too_large"
	ReasonTooManyPages  Reason = "too_many_pages"
	ReasonTooManyPixels Reason = "image_too_large"
	ReasonMacro         Reason = "macro_not_allowed"
	ReasonBlurry        Reason = "image_too_blurry"
	ReasonToolMissing   Reason = "tool_unavailable"
	ReasonTimeout       Reason = "stage_timeout"
	ReasonToolCrashed   Reason = "tool_crashed"
	ReasonNoText        Reason = "no_readable_text"
	ReasonArchiveBomb   Reason = "archive_limits_exceeded"
)

// Failure membawa kode stabil, apakah layak dicoba ulang, dan pesan untuk kandidat.
type Failure struct {
	Reason    Reason
	Transient bool
	Detail    string // hanya untuk log internal; tanpa isi dokumen
}

func (f *Failure) Error() string { return string(f.Reason) + ": " + f.Detail }

func fail(r Reason, detail string) *Failure { return &Failure{Reason: r, Detail: detail} }
func transient(r Reason, detail string) *Failure {
	return &Failure{Reason: r, Transient: true, Detail: detail}
}

// UserMessage: alasan dan langkah pemulihan dalam bahasa kandidat.
func UserMessage(r Reason) string {
	switch r {
	case ReasonUnsupported:
		return "Format berkas tidak didukung. Unggah PDF, DOCX, DOC, JPG, PNG, atau WebP, atau isi profil secara manual."
	case ReasonCorrupt:
		return "Berkas rusak atau tidak dapat dibuka. Simpan ulang dari aplikasi asalnya lalu unggah kembali."
	case ReasonEncrypted:
		return "Berkas dilindungi kata sandi. Simpan salinan tanpa kata sandi lalu unggah kembali."
	case ReasonEmpty:
		return "Berkas tidak berisi halaman atau teks. Periksa berkas Anda atau isi profil secara manual."
	case ReasonTooLarge:
		return "Ukuran berkas melebihi batas 15 MB. Kecilkan ukuran berkas lalu unggah kembali."
	case ReasonTooManyPages:
		return "CV melebihi 20 halaman. Unggah versi yang lebih ringkas."
	case ReasonTooManyPixels:
		return "Resolusi gambar terlalu besar. Kecilkan gambar lalu unggah kembali."
	case ReasonMacro:
		return "Dokumen berisi makro dan tidak dapat diproses. Simpan sebagai DOCX atau PDF tanpa makro."
	case ReasonBlurry:
		return "Foto terlalu buram untuk dibaca. Foto ulang dengan cahaya cukup, kamera tegak lurus, dan seluruh halaman terlihat."
	case ReasonToolMissing:
		return "Layanan pembaca CV sedang tidak tersedia. Coba lagi nanti atau isi profil secara manual."
	case ReasonTimeout, ReasonToolCrashed:
		return "Pembacaan CV terhenti sebelum selesai. Coba lagi nanti atau isi profil secara manual."
	case ReasonNoText:
		return "Tidak ada teks yang dapat dibaca. Unggah berkas yang lebih jelas atau isi profil secara manual."
	case ReasonArchiveBomb:
		return "Struktur dokumen melebihi batas aman. Simpan ulang sebagai PDF lalu unggah kembali."
	}
	return "CV tidak dapat dibaca. Coba unggah ulang atau isi profil secara manual."
}
