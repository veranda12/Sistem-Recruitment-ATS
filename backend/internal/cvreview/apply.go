package cvreview

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"ats/db/sqlc"
	"ats/internal/auth"
	"ats/internal/cvparse"
	"ats/internal/platform"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ---- Nilai ternormalisasi per jenis field ----

type periodVal struct {
	Start   *cvparse.PartialDate `json:"start,omitempty"`
	End     *cvparse.PartialDate `json:"end,omitempty"`
	Current bool                 `json:"current"`
	Raw     string               `json:"raw,omitempty"`
}

// editPeriod adalah bentuk sederhana yang dikirim UI: "2022-02", "2021", "2021-03-04".
type editPeriod struct {
	Start   string `json:"start"`
	End     string `json:"end"`
	Current bool   `json:"current"`
}

var (
	isoDateRe  = regexp.MustCompile(`^(\d{4})(?:-(\d{2})(?:-(\d{2}))?)?$`)
	emailValRe = regexp.MustCompile(`^[^@\s]{1,64}@[^@\s]+\.[^@\s]{2,}$`)
	eduLevels  = []string{"SMA", "SMK", "D3", "D4", "S1", "S2", "S3", "Lainnya"}
)

func parseISO(s string) (*cvparse.PartialDate, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, true
	}
	m := isoDateRe.FindStringSubmatch(s)
	if m == nil {
		return nil, false
	}
	var d cvparse.PartialDate
	fmt.Sscan(m[1], &d.Year)
	d.Precision = "year"
	if m[2] != "" {
		fmt.Sscan(m[2], &d.Month)
		d.Precision = "month"
		if d.Month < 1 || d.Month > 12 {
			return nil, false
		}
	}
	if m[3] != "" {
		fmt.Sscan(m[3], &d.Day)
		d.Precision = "day"
		if d.Day < 1 || d.Day > 31 {
			return nil, false
		}
	}
	if d.Year < 1950 || d.Year > 2100 {
		return nil, false
	}
	return &d, true
}

// validateEdit mengubah nilai edit dari UI ke bentuk normalized yang sama dengan hasil parser.
func validateEdit(field, section string, raw json.RawMessage) (json.RawMessage, string) {
	str := func() (string, bool) {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return "", false
		}
		return strings.TrimSpace(s), true
	}
	switch field {
	case "email":
		s, ok := str()
		if !ok || !emailValRe.MatchString(s) || len(s) > 254 {
			return nil, "Format email tidak valid."
		}
		b, _ := json.Marshal(strings.ToLower(s))
		return b, ""
	case "phone":
		s, ok := str()
		if !ok {
			return nil, "Nomor telepon tidak valid."
		}
		p, ok := cvparse.NormalizePhone(s)
		if !ok {
			return nil, "Nomor telepon 8 sampai 15 digit, boleh diawali +."
		}
		b, _ := json.Marshal(p)
		return b, ""
	case "url":
		s, ok := str()
		u, ok2 := cvparse.NormalizeURL(s)
		if !ok || !ok2 {
			return nil, "Tautan harus alamat web http atau https."
		}
		b, _ := json.Marshal(u)
		return b, ""
	case "period":
		var e editPeriod
		if json.Unmarshal(raw, &e) != nil {
			return nil, "Periode tidak valid."
		}
		st, ok1 := parseISO(e.Start)
		en, ok2 := parseISO(e.End)
		if !ok1 || !ok2 || (st == nil && section == "experience") {
			return nil, "Isi tahun mulai, misalnya 2022 atau 2022-02."
		}
		if e.Current {
			en = nil
		}
		if st != nil && en != nil && en.String() < st.String() {
			return nil, "Akhir periode tidak boleh sebelum mulai."
		}
		b, _ := json.Marshal(periodVal{Start: st, End: en, Current: e.Current})
		return b, ""
	case "date":
		s, ok := str()
		d, ok2 := parseISO(s)
		if !ok || !ok2 || d == nil {
			return nil, "Tanggal tidak valid."
		}
		b, _ := json.Marshal(d)
		return b, ""
	case "level":
		s, ok := str()
		if !ok || !slices.Contains(eduLevels, s) {
			return nil, "Pilih jenjang."
		}
		b, _ := json.Marshal(s)
		return b, ""
	case "gpa":
		var f float64
		if json.Unmarshal(raw, &f) != nil || f < 0 || f > 4 {
			return nil, "IPK 0 sampai 4."
		}
		b, _ := json.Marshal(f)
		return b, ""
	case "language":
		var m map[string]string
		if json.Unmarshal(raw, &m) != nil || strings.TrimSpace(m["name"]) == "" || len(m["name"]) > 60 || len(m["level"]) > 60 {
			return nil, "Nama bahasa wajib diisi."
		}
		b, _ := json.Marshal(map[string]string{"name": strings.TrimSpace(m["name"]), "level": strings.TrimSpace(m["level"])})
		return b, ""
	default:
		s, ok := str()
		limit := 200
		if field == "summary" || field == "description" {
			limit = 2000
		}
		if !ok || len([]rune(s)) > limit || (s == "" && field != "description") {
			return nil, fmt.Sprintf("Isi maksimal %d karakter.", limit)
		}
		b, _ := json.Marshal(s)
		return b, ""
	}
}

// ---- Review ----

type decision struct {
	SuggestionID uuid.UUID       `json:"suggestionId"`
	Action       string          `json:"action"` // accept | edit | reject | reset
	Value        json.RawMessage `json:"value,omitempty"`
}

type reviewReq struct {
	ReviewVersion int32      `json:"reviewVersion"`
	Decisions     []decision `json:"decisions"`
}

// PATCH /me/cv-parses/:id/review — keputusan per field/entri dengan optimistic concurrency.
func (h *Handler) Review(c *gin.Context) {
	r, ok := h.loadOwn(c)
	if !ok {
		return
	}
	var req reviewReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	if len(req.Decisions) == 0 || len(req.Decisions) > 500 {
		platform.Fail(c, platform.ErrValidation(map[string]string{"decisions": "Tidak ada keputusan."}))
		return
	}
	p := auth.Current(c)
	ctx := c.Request.Context()
	var out runDTO
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		cur, err := q.GetCVParseRunForUpdate(ctx, r.ID)
		if err != nil {
			return err
		}
		if cur.Status != "ready_for_review" && cur.Status != "partial" {
			return platform.ErrConflict("not_reviewable", "Hasil ini sudah diterapkan atau tidak dapat diubah lagi.")
		}
		if cur.ReviewVersion != req.ReviewVersion {
			return platform.ErrConflict("stale_review", "Pilihan Anda diubah di tab lain. Muat ulang untuk melihat yang terbaru.")
		}
		sugs, err := q.ListCVSuggestions(ctx, r.ID)
		if err != nil {
			return err
		}
		byID := map[uuid.UUID]sqlc.ListCVSuggestionsRow{}
		for _, s := range sugs {
			byID[s.ID] = s
		}
		fields := map[string]string{}
		for i, d := range req.Decisions {
			s, ok := byID[d.SuggestionID]
			if !ok {
				fields[fmt.Sprintf("decisions.%d", i)] = "Saran tidak ditemukan."
				continue
			}
			var status string
			var edited json.RawMessage
			switch d.Action {
			case "accept":
				if len(s.Normalized) == 0 || string(s.Normalized) == "null" {
					fields[s.ID.String()] = "Nilai kosong; isi secara manual."
					continue
				}
				status = "accepted"
			case "reject":
				status = "rejected"
			case "reset":
				status = "pending"
			case "edit":
				v, msg := validateEdit(s.Field, s.Section, d.Value)
				if msg != "" {
					fields[s.ID.String()] = msg
					continue
				}
				status, edited = "edited", v
			default:
				fields[fmt.Sprintf("decisions.%d.action", i)] = "Aksi tidak dikenal."
				continue
			}
			if _, err := q.SetCVSuggestionDecision(ctx, sqlc.SetCVSuggestionDecisionParams{ID: s.ID, RunID: r.ID, Status: status, EditedValue: edited}); err != nil {
				return err
			}
			detail, _ := json.Marshal(map[string]any{"path": s.Path, "edited": edited != nil})
			sid := s.ID
			if err := q.InsertCVReviewEvent(ctx, sqlc.InsertCVReviewEventParams{RunID: r.ID, SuggestionID: &sid, ActorUserID: &p.UserID, Action: d.Action, Detail: detail}); err != nil {
				return err
			}
		}
		if len(fields) > 0 {
			return platform.ErrValidation(fields)
		}
		if n, err := q.BumpCVReviewVersion(ctx, sqlc.BumpCVReviewVersionParams{ID: r.ID, ReviewVersion: req.ReviewVersion}); err != nil || n == 0 {
			if err == nil {
				err = platform.ErrConflict("stale_review", "Pilihan Anda diubah di tab lain. Muat ulang untuk melihat yang terbaru.")
			}
			return err
		}
		cur, err = q.GetCVParseRun(ctx, r.ID)
		if err != nil {
			return err
		}
		out, err = h.buildRun(ctx, q, cur, true)
		return err
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// ---- Apply ----

type entryChoice struct {
	Mode     string     `json:"mode"` // add | skip | replace
	TargetID *uuid.UUID `json:"targetId,omitempty"`
}

type applyReq struct {
	ReviewVersion  int32                  `json:"reviewVersion"`
	ProfileVersion int32                  `json:"profileVersion"`
	Entries        map[string]entryChoice `json:"entries"`
}

// ApplyResult dicatat di cv_review_events dan dikembalikan sama persis untuk retry/klik ganda.
type ApplyResult struct {
	ProfileVersion int32          `json:"profileVersion"`
	UpdatedFields  []string       `json:"updatedFields"`
	Added          map[string]int `json:"added"`
	Replaced       map[string]int `json:"replaced"`
	Skipped        []string       `json:"skipped"`
}

func val[T any](s sqlc.ListCVSuggestionsRow) (T, bool) {
	var v T
	src := s.Normalized
	if s.Status == "edited" {
		src = s.EditedValue
	}
	if s.Status != "accepted" && s.Status != "edited" {
		return v, false
	}
	return v, json.Unmarshal(src, &v) == nil
}

func dateOf(d *cvparse.PartialDate) (*time.Time, string) {
	if d == nil {
		return nil, "day"
	}
	m, day := max(d.Month, 1), max(d.Day, 1)
	t := time.Date(d.Year, time.Month(m), day, 0, 0, 0, 0, time.UTC)
	return &t, d.Precision
}

// POST /me/cv-parses/:id/apply — menerapkan hanya pilihan yang disetujui dalam satu transaksi.
func (h *Handler) Apply(c *gin.Context) {
	r, ok := h.loadOwn(c)
	if !ok {
		return
	}
	var req applyReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	p := auth.Current(c)
	ctx := c.Request.Context()
	var res ApplyResult
	var already bool
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		cur, err := q.GetCVParseRunForUpdate(ctx, r.ID) // serialisasi apply ganda
		if err != nil {
			return err
		}
		if cur.Status == "applied" {
			// Retry/klik ganda: kembalikan hasil yang sama tanpa efek kedua.
			d, err := q.LastCVApplyEvent(ctx, r.ID)
			if err != nil {
				return err
			}
			already = true
			return json.Unmarshal(d, &res)
		}
		if cur.Status != "ready_for_review" && cur.Status != "partial" {
			return platform.ErrConflict("not_reviewable", "Hasil ini tidak dapat diterapkan.")
		}
		if cur.ReviewVersion != req.ReviewVersion {
			return platform.ErrConflict("stale_review", "Pilihan Anda diubah di tab lain. Muat ulang untuk melihat yang terbaru.")
		}
		res, err = h.applyTx(ctx, q, p.UserID, r.ID, req)
		if err != nil {
			return err
		}
		detail, _ := json.Marshal(res)
		if err := q.InsertCVReviewEvent(ctx, sqlc.InsertCVReviewEventParams{RunID: r.ID, ActorUserID: &p.UserID, Action: "apply", Detail: detail}); err != nil {
			return err
		}
		if n, err := q.MarkCVRunApplied(ctx, sqlc.MarkCVRunAppliedParams{ID: r.ID, ReviewVersion: req.ReviewVersion}); err != nil || n == 0 {
			if err == nil {
				err = platform.ErrConflict("stale_review", "Pilihan Anda diubah di tab lain. Muat ulang untuk melihat yang terbaru.")
			}
			return err
		}
		// Audit tersamar: hanya jumlah dan nama field, tanpa nilai pribadi.
		return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "candidate", Action: "candidate.profile_cv_apply",
			TargetType: "user", TargetID: p.UserID.String(), RequestID: platform.RequestID(c),
			Detail: map[string]any{"run": r.ID.String(), "fields": res.UpdatedFields, "added": res.Added, "replaced": res.Replaced}})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": res, "alreadyApplied": already})
}

func (h *Handler) applyTx(ctx context.Context, q *sqlc.Queries, user, runID uuid.UUID, req applyReq) (ApplyResult, error) {
	res := ApplyResult{Added: map[string]int{}, Replaced: map[string]int{}, UpdatedFields: []string{}, Skipped: []string{}}
	prof, err := q.GetCandidateProfile(ctx, user)
	if err != nil {
		return res, err
	}
	if prof.Version != req.ProfileVersion {
		return res, platform.ErrConflict("profile_changed", "Profil berubah sejak Anda mulai memeriksa. Lihat perbedaannya lalu terapkan ulang; pilihan Anda tetap tersimpan.")
	}
	sugs, err := q.ListCVSuggestions(ctx, runID)
	if err != nil {
		return res, err
	}
	name, phone, city, summary, contact := prof.FullName, prof.Phone, prof.City, prof.Summary, prof.ContactEmail
	entries := map[string][]sqlc.ListCVSuggestionsRow{}
	var entryOrder []string
	for _, s := range sugs {
		switch s.Section {
		case "contact", "summary":
			switch s.Field {
			case "name":
				if v, ok := val[string](s); ok && v != name {
					name = v
					res.UpdatedFields = append(res.UpdatedFields, "fullName")
				}
			case "phone":
				if v, ok := val[cvparse.Phone](s); ok && phone != v.E164 && !slices.Contains(res.UpdatedFields, "phone") {
					phone = v.E164
					res.UpdatedFields = append(res.UpdatedFields, "phone")
				}
			case "email":
				// Email CV hanya menjadi email kontak; email akun (login) tidak pernah diubah dari CV.
				if v, ok := val[string](s); ok && v != contact && !slices.Contains(res.UpdatedFields, "contactEmail") {
					contact = v
					res.UpdatedFields = append(res.UpdatedFields, "contactEmail")
				}
			case "city":
				if v, ok := val[string](s); ok && v != city {
					city = v
					res.UpdatedFields = append(res.UpdatedFields, "city")
				}
			case "summary":
				if v, ok := val[string](s); ok && v != summary {
					summary = v
					res.UpdatedFields = append(res.UpdatedFields, "summary")
				}
			}
		default:
			if _, ok := entries[s.EntryKey]; !ok {
				entryOrder = append(entryOrder, s.EntryKey)
			}
			entries[s.EntryKey] = append(entries[s.EntryKey], s)
		}
	}
	exps, err := q.ListExperiences(ctx, user)
	if err != nil {
		return res, err
	}
	edus, err := q.ListEducation(ctx, user)
	if err != nil {
		return res, err
	}
	for _, key := range entryOrder {
		group := entries[key]
		section := group[0].Section
		get := func(field string) (sqlc.ListCVSuggestionsRow, bool) {
			for _, s := range group {
				if s.Field == field && (s.Status == "accepted" || s.Status == "edited") {
					return s, true
				}
			}
			return sqlc.ListCVSuggestionsRow{}, false
		}
		str := func(field string) string {
			if s, ok := get(field); ok {
				v, _ := val[string](s)
				return strings.TrimSpace(v)
			}
			return ""
		}
		choice := req.Entries[key]
		if choice.Mode == "skip" {
			res.Skipped = append(res.Skipped, key)
			continue
		}
		accepted := false
		for _, s := range group {
			if s.Status == "accepted" || s.Status == "edited" {
				accepted = true
			}
		}
		if !accepted {
			continue
		}
		switch section {
		case "experience":
			title, company := str("title"), str("company")
			ps, _ := get("period")
			per, _ := val[periodVal](ps)
			if len(title) < 2 || len(company) < 2 || per.Start == nil {
				res.Skipped = append(res.Skipped, key+":tidak_lengkap")
				continue
			}
			start, sp := dateOf(per.Start)
			end, ep := dateOf(per.End)
			if per.Current {
				end = nil
			}
			if choice.Mode == "replace" && choice.TargetID != nil {
				if n, err := q.DeleteExperience(ctx, sqlc.DeleteExperienceParams{ID: *choice.TargetID, UserID: user}); err != nil {
					return res, err
				} else if n == 1 {
					res.Replaced["experience"]++
				}
			} else if choice.Mode != "add" {
				// default: lewati bila entri yang sama sudah ada (retry/CV ulang tidak menggandakan)
				dup := false
				for _, e := range exps {
					if strings.EqualFold(e.Company, company) && strings.EqualFold(e.Title, title) && e.StartDate.Year() == start.Year() && (sp == "year" || e.StartDate.Month() == start.Month()) {
						dup = true
					}
				}
				if dup {
					res.Skipped = append(res.Skipped, key+":sudah_ada")
					continue
				}
			}
			if _, err := q.InsertExperienceFull(ctx, sqlc.InsertExperienceFullParams{UserID: user, Company: company, Title: title, Location: str("location"),
				StartDate: *start, StartPrecision: sp, EndDate: end, EndPrecision: ep, IsCurrent: per.Current, Description: str("description")}); err != nil {
				return res, err
			}
			if choice.Mode != "replace" {
				res.Added["experience"]++
			}
		case "education":
			inst := str("institution")
			if len(inst) < 2 {
				res.Skipped = append(res.Skipped, key+":tidak_lengkap")
				continue
			}
			level := "Lainnya"
			if s, ok := get("level"); ok {
				if v, ok := val[string](s); ok && slices.Contains(eduLevels, v) {
					level = v
				}
			}
			var sy, ey *int32
			if s, ok := get("period"); ok {
				if per, ok := val[periodVal](s); ok {
					if per.Start != nil {
						sy = ptr(int32(per.Start.Year))
					}
					if per.End != nil {
						ey = ptr(int32(per.End.Year))
					}
				}
			}
			var gpa *float64
			if s, ok := get("gpa"); ok {
				if v, ok := val[float64](s); ok {
					gpa = &v
				}
			}
			if choice.Mode == "replace" && choice.TargetID != nil {
				if n, err := q.DeleteEducation(ctx, sqlc.DeleteEducationParams{ID: *choice.TargetID, UserID: user}); err != nil {
					return res, err
				} else if n == 1 {
					res.Replaced["education"]++
				}
			} else if choice.Mode != "add" {
				dup := false
				for _, e := range edus {
					if strings.EqualFold(e.Institution, inst) && e.Level == level {
						dup = true
					}
				}
				if dup {
					res.Skipped = append(res.Skipped, key+":sudah_ada")
					continue
				}
			}
			if err := q.InsertEducation(ctx, sqlc.InsertEducationParams{UserID: user, Level: level, Institution: inst, Major: str("major"), StartYear: sy, EndYear: ey, Gpa: gpa}); err != nil {
				return res, err
			}
			if choice.Mode != "replace" {
				res.Added["education"]++
			}
		case "skills":
			if v := str("skill"); v != "" {
				if err := q.InsertSkill(ctx, sqlc.InsertSkillParams{UserID: user, Name: v}); err != nil {
					return res, err
				}
				res.Added["skills"]++
			}
		case "languages":
			if s, ok := get("language"); ok {
				if v, ok := val[map[string]string](s); ok && v["name"] != "" {
					if err := q.InsertLanguage(ctx, sqlc.InsertLanguageParams{UserID: user, Name: v["name"], Level: v["level"]}); err != nil {
						return res, err
					}
					res.Added["languages"]++
				}
			}
		case "links":
			if s, ok := get("url"); ok {
				if v, ok := val[cvparse.SafeURL](s); ok && v.Target != "" {
					if err := q.InsertLink(ctx, sqlc.InsertLinkParams{UserID: user, Label: v.Label, Url: v.Target}); err != nil {
						return res, err
					}
					res.Added["links"]++
				}
			}
		case "certifications":
			n := str("name")
			if len(n) < 2 {
				continue
			}
			var issued *time.Time
			prec := "day"
			if s, ok := get("date"); ok {
				if d, ok := val[cvparse.PartialDate](s); ok {
					issued, prec = dateOf(&d)
				}
			}
			if err := q.InsertCertificationFull(ctx, sqlc.InsertCertificationFullParams{UserID: user, Name: n, Issuer: str("issuer"), IssuedOn: issued, IssuedPrecision: prec}); err != nil {
				return res, err
			}
			res.Added["certifications"]++
		case "projects":
			n := str("name")
			if len(n) < 2 {
				continue
			}
			if err := q.InsertProject(ctx, sqlc.InsertProjectParams{UserID: user, Name: n, Description: str("description")}); err != nil {
				return res, err
			}
			res.Added["projects"]++
		}
	}
	if len(name) < 2 {
		name = prof.FullName
	}
	updated, err := q.TouchCandidateProfile(ctx, sqlc.TouchCandidateProfileParams{UserID: user, Version: req.ProfileVersion, FullName: name, Phone: phone,
		City: city, Summary: summary, ContactEmail: contact})
	if platform.IsNoRows(err) {
		return res, platform.ErrConflict("profile_changed", "Profil berubah sejak Anda mulai memeriksa. Lihat perbedaannya lalu terapkan ulang; pilihan Anda tetap tersimpan.")
	}
	if err != nil {
		return res, err
	}
	res.ProfileVersion = updated.Version
	return res, nil
}
