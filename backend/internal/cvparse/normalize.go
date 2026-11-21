package cvparse

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// PartialDate membedakan presisi hari, bulan, dan tahun. "2021" tidak menjadi 1 Januari.
type PartialDate struct {
	Year      int    `json:"year"`
	Month     int    `json:"month,omitempty"`
	Day       int    `json:"day,omitempty"`
	Precision string `json:"precision"` // day | month | year
}

func (d PartialDate) String() string {
	switch d.Precision {
	case "day":
		return pad4(d.Year) + "-" + pad2(d.Month) + "-" + pad2(d.Day)
	case "month":
		return pad4(d.Year) + "-" + pad2(d.Month)
	}
	return pad4(d.Year)
}

func pad2(n int) string { return strings.Repeat("0", max(0, 2-len(strconv.Itoa(n)))) + strconv.Itoa(n) }
func pad4(n int) string { return strings.Repeat("0", max(0, 4-len(strconv.Itoa(n)))) + strconv.Itoa(n) }

// Period hasil normalisasi rentang waktu. Current=true untuk "sekarang"/"present".
type Period struct {
	Start   *PartialDate `json:"start,omitempty"`
	End     *PartialDate `json:"end,omitempty"`
	Current bool         `json:"current"`
	Raw     string       `json:"raw"`
}

var monthNames = map[string]int{
	"jan": 1, "januari": 1, "january": 1,
	"feb": 2, "februari": 2, "pebruari": 2, "february": 2,
	"mar": 3, "maret": 3, "march": 3,
	"apr": 4, "april": 4,
	"mei": 5, "may": 5,
	"jun": 6, "juni": 6, "june": 6,
	"jul": 7, "juli": 7, "july": 7,
	"agu": 8, "agt": 8, "ags": 8, "agus": 8, "agustus": 8, "aug": 8, "august": 8,
	"sep": 9, "sept": 9, "september": 9,
	"okt": 10, "oktober": 10, "oct": 10, "october": 10,
	"nov": 11, "nop": 11, "november": 11, "nopember": 11,
	"des": 12, "desember": 12, "dec": 12, "december": 12,
}

const monthAlt = `jan(?:uari|uary)?|feb(?:ruari|ruary)?|pebruari|mar(?:et|ch)?|apr(?:il)?|mei|may|jun[ie]?|jul[iy]?|agu(?:stus)?|agt|ags|aug(?:ust)?|sep(?:t|tember)?|okt(?:ober)?|oct(?:ober)?|nop(?:ember)?|nov(?:ember)?|des(?:ember)?|dec(?:ember)?`

var (
	dateTok    = `(?:(?:\d{1,2}\s+)?(?:` + monthAlt + `)\.?\s+(?:19|20)\d{2}|(?:0?[1-9]|1[0-2])[/.-](?:19|20)\d{2}|(?:19|20)\d{2}[/.-](?:0?[1-9]|1[0-2])|(?:19|20)\d{2})`
	currentTok = `sekarang|saat\s+ini|kini|present|now|current|today|ongoing`
	rangeSep   = `\s*(?:-|–|—|~|s\.?\s?d\.?|sampai(?:\s+dengan)?|hingga|until|to)\s*`
	rangeRe    = regexp.MustCompile(`(?i)(` + dateTok + `)` + rangeSep + `(` + dateTok + `|` + currentTok + `)`)
	singleRe   = regexp.MustCompile(`(?i)\b(` + dateTok + `)\b`)
	currentRe  = regexp.MustCompile(`(?i)^(?:` + currentTok + `)$`)
	yearOnly   = regexp.MustCompile(`^(19|20)\d{2}$`)
	numMonth1  = regexp.MustCompile(`^(\d{1,2})[/.-]((?:19|20)\d{2})$`)
	numMonth2  = regexp.MustCompile(`^((?:19|20)\d{2})[/.-](\d{1,2})$`)
	textMonth  = regexp.MustCompile(`(?i)^(?:(\d{1,2})\s+)?([a-z]+)\.?\s+((?:19|20)\d{2})$`)
)

// ParseDate menormalkan satu token tanggal. ok=false bila tidak dikenali.
func ParseDate(s string) (PartialDate, bool) {
	s = strings.TrimSpace(s)
	if m := yearOnly.FindString(s); m != "" {
		y, _ := strconv.Atoi(m)
		return PartialDate{Year: y, Precision: "year"}, true
	}
	if m := numMonth1.FindStringSubmatch(s); m != nil {
		mo, _ := strconv.Atoi(m[1])
		y, _ := strconv.Atoi(m[2])
		if mo >= 1 && mo <= 12 {
			return PartialDate{Year: y, Month: mo, Precision: "month"}, true
		}
	}
	if m := numMonth2.FindStringSubmatch(s); m != nil {
		y, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		if mo >= 1 && mo <= 12 {
			return PartialDate{Year: y, Month: mo, Precision: "month"}, true
		}
	}
	if m := textMonth.FindStringSubmatch(s); m != nil {
		mo, ok := monthNames[strings.ToLower(m[2])]
		if !ok {
			return PartialDate{}, false
		}
		y, _ := strconv.Atoi(m[3])
		if m[1] != "" {
			d, _ := strconv.Atoi(m[1])
			if d >= 1 && d <= 31 {
				return PartialDate{Year: y, Month: mo, Day: d, Precision: "day"}, true
			}
		}
		return PartialDate{Year: y, Month: mo, Precision: "month"}, true
	}
	return PartialDate{}, false
}

// FindPeriod mencari rentang waktu pertama dalam teks. Mengembalikan juga posisi agar sisa teks dapat dipakai.
func FindPeriod(s string) (Period, [2]int, bool) {
	if loc := rangeRe.FindStringSubmatchIndex(s); loc != nil {
		raw := s[loc[0]:loc[1]]
		a, _ := ParseDate(s[loc[2]:loc[3]])
		p := Period{Start: &a, Raw: raw}
		endTok := strings.TrimSpace(s[loc[4]:loc[5]])
		if currentRe.MatchString(endTok) {
			p.Current = true
		} else if b, ok := ParseDate(endTok); ok {
			p.End = &b
		}
		return p, [2]int{loc[0], loc[1]}, true
	}
	return Period{}, [2]int{}, false
}

// FindSingleDate untuk sertifikasi atau entri bertahun tunggal ("2021").
func FindSingleDate(s string) (PartialDate, [2]int, bool) {
	if loc := singleRe.FindStringSubmatchIndex(s); loc != nil {
		d, ok := ParseDate(s[loc[2]:loc[3]])
		if ok {
			return d, [2]int{loc[0], loc[1]}, true
		}
	}
	return PartialDate{}, [2]int{}, false
}

// ---- Kontak ----

var (
	emailRe = regexp.MustCompile(`(?i)\b[a-z0-9._%+-]+@[a-z0-9-]+(?:\.[a-z0-9-]+)*\.[a-z]{2,}\b`)
	// Telepon: +62/62/0 diikuti digit dengan pemisah spasi, titik, strip, atau kurung.
	phoneRe = regexp.MustCompile(`(?:\+\s?\d{1,3}|\(?0)[\d\s().-]{7,18}\d`)
	urlRe   = regexp.MustCompile(`(?i)\b(?:https?://[^\s<>"']+|www\.[^\s<>"']+|(?:linkedin\.com|github\.com|gitlab\.com|behance\.net|dribbble\.com|medium\.com)/[^\s<>"']+)`)
)

// Phone dengan asumsi negara eksplisit; tidak menebak bila digit tidak masuk akal.
type Phone struct {
	E164           string `json:"e164"`
	AssumedCountry string `json:"assumedCountry,omitempty"`
}

// NormalizePhone: "0812-3456-7890" -> +6281234567890 dengan asumsi ID; "+44 20..." dipertahankan.
func NormalizePhone(raw string) (Phone, bool) {
	var digits strings.Builder
	plus := strings.HasPrefix(strings.TrimSpace(raw), "+")
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	d := digits.String()
	switch {
	case plus:
		if len(d) < 8 || len(d) > 15 {
			return Phone{}, false
		}
		return Phone{E164: "+" + d}, true
	case strings.HasPrefix(d, "62") && len(d) >= 10:
		if len(d) > 15 {
			return Phone{}, false
		}
		return Phone{E164: "+" + d}, true
	case strings.HasPrefix(d, "0") && len(d) >= 9 && len(d) <= 14:
		return Phone{E164: "+62" + d[1:], AssumedCountry: "ID"}, true
	}
	return Phone{}, false
}

// ProfilePhone mengubah E.164 ke format kolom profil (^\+?[0-9]{8,15}$).
func ProfilePhone(p Phone) string { return p.E164 }

// SafeURL memisahkan label tampilan dari target aman. Hanya http/https; URL tidak pernah dikunjungi.
type SafeURL struct {
	Label  string `json:"label"`
	Target string `json:"target"`
	Host   string `json:"host"`
}

func NormalizeURL(raw string) (SafeURL, bool) {
	r := strings.TrimRight(strings.TrimSpace(raw), ".,;)")
	t := r
	if !strings.HasPrefix(strings.ToLower(t), "http://") && !strings.HasPrefix(strings.ToLower(t), "https://") {
		t = "https://" + t
	}
	u, err := url.Parse(t)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || !strings.Contains(u.Host, ".") {
		return SafeURL{}, false
	}
	u.Fragment = ""
	return SafeURL{Label: r, Target: u.String(), Host: strings.ToLower(strings.TrimPrefix(u.Hostname(), "www."))}, true
}

// ---- Skill ----

// skillAlias: alias eksplisit saja. "SQL" bukan bukti PostgreSQL.
var skillAlias = map[string]string{
	"postgres": "PostgreSQL", "postgresql": "PostgreSQL", "psql": "PostgreSQL",
	"golang": "Go", "go": "Go", "js": "JavaScript", "javascript": "JavaScript", "ts": "TypeScript", "typescript": "TypeScript",
	"reactjs": "React", "react.js": "React", "react": "React", "node": "Node.js", "nodejs": "Node.js", "node.js": "Node.js",
	"ms excel": "Microsoft Excel", "excel": "Microsoft Excel", "microsoft excel": "Microsoft Excel",
	"k8s": "Kubernetes", "kubernetes": "Kubernetes", "mysql": "MySQL", "sql": "SQL",
}

// CanonicalSkill mengembalikan nama kanonik bila ada alias eksplisit, selain itu teks asli yang dirapikan.
func CanonicalSkill(raw string) string {
	k := strings.ToLower(strings.TrimSpace(raw))
	if v, ok := skillAlias[k]; ok {
		return v
	}
	return collapseSpace(strings.TrimSpace(raw))
}

func collapseSpace(s string) string { return strings.Join(strings.Fields(s), " ") }

// normKey: kunci pembanding heading/tokens (huruf kecil, tanpa tanda baca).
func normKey(s string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			space = false
		case r == '&':
			if !space {
				b.WriteRune(' ')
			}
			b.WriteString("dan ")
			space = true
		default:
			if !space && b.Len() > 0 {
				b.WriteRune(' ')
				space = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}
