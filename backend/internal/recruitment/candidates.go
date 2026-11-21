package recruitment

import (
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"ats/db/sqlc"
	"ats/internal/auth"
	"ats/internal/identity"
	"ats/internal/platform"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Versi pemberitahuan privasi yang disetujui kandidat saat mendaftar. Isi final menunggu pemilik kebijakan.
const PrivacyNoticeVersion = "2026-11-draft"

type registerReq struct {
	Email         string `json:"email"`
	Password      string `json:"password"`
	FullName      string `json:"fullName"`
	NIK           string `json:"nik"`
	AcceptPrivacy bool   `json:"acceptPrivacy"`
}

var neutralRegister = gin.H{"message": "Pendaftaran diproses. Periksa email Anda untuk langkah berikutnya."}

// POST /auth/register. Respons selalu netral (202) agar tidak membocorkan keberadaan email atau NIK.
// Dua pendaftaran bersamaan dengan NIK sama: unique(org_id, nik_blind_index) memastikan maksimal satu master;
// pendaftar lain mendapat akun tanpa identitas + klaim verifikasi manual, tanpa merge otomatis.
func (h *Handler) Register(c *gin.Context) {
	var req registerReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	f := map[string]string{}
	email, ok := auth.NormalizeEmail(req.Email)
	if !ok {
		f["email"] = "Masukkan alamat email yang valid."
	}
	if msg := auth.ValidatePassword(req.Password); msg != "" {
		f["password"] = msg
	}
	name := strings.TrimSpace(req.FullName)
	if len(name) < 2 || len(name) > 200 {
		f["fullName"] = "Nama lengkap wajib 2 sampai 200 karakter."
	}
	nik, err := identity.Normalize(req.NIK)
	if err != nil {
		f["nik"] = "NIK harus 16 digit angka."
	}
	if !req.AcceptPrivacy {
		f["acceptPrivacy"] = "Anda perlu menyetujui pemberitahuan privasi."
	}
	if len(f) > 0 {
		platform.Fail(c, platform.ErrValidation(f))
		return
	}
	ctx := c.Request.Context()
	hash, err := auth.HashPassword(ctx, req.Password)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	sealed, err := h.Keys.Seal(nik, h.OrgID[:])
	if err != nil {
		platform.Fail(c, err)
		return
	}
	err = platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		u, err := q.CreateUser(ctx, sqlc.CreateUserParams{OrgID: h.OrgID, Kind: "candidate", Email: email, DisplayName: name, PasswordHash: hash})
		if err != nil {
			return err
		}
		if err := q.CreateCandidateProfile(ctx, sqlc.CreateCandidateProfileParams{UserID: u.ID, OrgID: h.OrgID, FullName: name}); err != nil {
			return err
		}
		_, err = q.InsertCandidateIdentity(ctx, sqlc.InsertCandidateIdentityParams{OrgID: h.OrgID, UserID: u.ID,
			NikCiphertext: sealed.Ciphertext, NikNonce: sealed.Nonce, KeyVersion: int32(sealed.KeyVersion),
			NikBlindIndex: sealed.BlindIndex, BlindKeyVersion: int32(sealed.IndexVersion), NikLast4: sealed.Last4})
		identityState := "linked"
		if platform.IsNoRows(err) {
			identityState = "claim_opened"
			if err := q.OpenIdentityClaim(ctx, sqlc.OpenIdentityClaimParams{OrgID: h.OrgID, ClaimantUserID: u.ID,
				NikBlindIndex: sealed.BlindIndex, Note: "NIK sudah terkait akun lain saat pendaftaran"}); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if _, err := platform.Enqueue(ctx, q, h.OrgID, auth.TopicVerifyEmail, "user", u.ID.String(), gin.H{"userId": u.ID}); err != nil {
			return err
		}
		return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &u.ID, ActorKind: "candidate",
			Action: "candidate.register", TargetType: "user", TargetID: u.ID.String(), RequestID: platform.RequestID(c),
			Detail: map[string]any{"identity": identityState, "privacy_notice_version": PrivacyNoticeVersion}})
	})
	if platform.IsUniqueViolation(err, "users_org_id_email_key") {
		// Email sudah terdaftar: kirim email informasi ke pemilik alamat, respons tetap netral.
		if u, gerr := h.Q.GetUserByEmail(ctx, sqlc.GetUserByEmailParams{OrgID: h.OrgID, Email: email}); gerr == nil {
			_, _ = platform.Enqueue(ctx, h.Q, h.OrgID, auth.TopicAccountExists, "user", u.ID.String(), gin.H{"userId": u.ID})
		}
		c.JSON(http.StatusAccepted, neutralRegister)
		return
	}
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, neutralRegister)
}

// ---- Profil (/me/profile) ----

type educationDTO struct {
	ID          *uuid.UUID `json:"id,omitempty"` // hanya respons; dipakai pilihan "ganti entri" saat menerapkan CV
	Level       string     `json:"level"`
	Institution string     `json:"institution"`
	Major       string     `json:"major"`
	StartYear   *int32     `json:"startYear"`
	EndYear     *int32     `json:"endYear"`
	GPA         *float64   `json:"gpa"`
}

type experienceDTO struct {
	ID             *uuid.UUID `json:"id,omitempty"` // hanya respons
	Company        string     `json:"company"`
	Title          string     `json:"title"`
	Location       string     `json:"location"`
	StartDate      string     `json:"startDate"`
	StartPrecision string     `json:"startPrecision"` // day | month | year (dari CV); form manual = day
	EndDate        *string    `json:"endDate"`
	EndPrecision   string     `json:"endPrecision"`
	IsCurrent      bool       `json:"isCurrent"`
	Description    string     `json:"description"`
}

type languageDTO struct {
	Name  string `json:"name"`
	Level string `json:"level"`
}

type linkDTO struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type projectDTO struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type certificationDTO struct {
	Name      string  `json:"name"`
	Issuer    string  `json:"issuer"`
	IssuedOn  *string `json:"issuedOn"`
	ExpiresOn *string `json:"expiresOn"`
}

type profileDTO struct {
	FullName       string             `json:"fullName"`
	Phone          string             `json:"phone"`
	BirthDate      *string            `json:"birthDate"`
	City           string             `json:"city"`
	Address        string             `json:"address"`
	Summary        string             `json:"summary"`
	Education      []educationDTO     `json:"education"`
	Experiences    []experienceDTO    `json:"experiences"`
	Certifications []certificationDTO `json:"certifications"`
	// Opsional pada PUT: nil = tidak diubah (klien lama tidak menghapus data ini).
	ContactEmail *string        `json:"contactEmail"`
	Skills       *[]string      `json:"skills"`
	Languages    *[]languageDTO `json:"languages"`
	Links        *[]linkDTO     `json:"links"`
	Projects     *[]projectDTO  `json:"projects"`
	Version      int32          `json:"version"`
}

type profileResp struct {
	profileDTO
	Email         string `json:"email"`
	EmailVerified bool   `json:"emailVerified"`
	Identity      struct {
		Status    string `json:"status"` // linked | pending_verification | missing
		NIKMasked string `json:"nikMasked,omitempty"`
	} `json:"identity"`
	Complete bool      `json:"complete"`
	Missing  []string  `json:"missing"`
	Updated  time.Time `json:"updatedAt"`
}

func dateStr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format("2006-01-02")
	return &s
}

func parseDate(s *string) (*time.Time, bool) {
	if s == nil || *s == "" {
		return nil, true
	}
	t, err := time.Parse("2006-01-02", *s)
	if err != nil {
		return nil, false
	}
	return &t, true
}

func (h *Handler) loadProfile(c *gin.Context, q *sqlc.Queries, p *auth.Principal) (profileResp, error) {
	ctx := c.Request.Context()
	var r profileResp
	prof, err := q.GetCandidateProfile(ctx, p.UserID)
	if err != nil {
		return r, err
	}
	ed, err := q.ListEducation(ctx, p.UserID)
	if err != nil {
		return r, err
	}
	ex, err := q.ListExperiences(ctx, p.UserID)
	if err != nil {
		return r, err
	}
	ce, err := q.ListCertifications(ctx, p.UserID)
	if err != nil {
		return r, err
	}
	r.FullName, r.Phone, r.BirthDate, r.City, r.Address, r.Summary, r.Version, r.Updated =
		prof.FullName, prof.Phone, dateStr(prof.BirthDate), prof.City, prof.Address, prof.Summary, prof.Version, prof.UpdatedAt
	r.Education, r.Experiences, r.Certifications = []educationDTO{}, []experienceDTO{}, []certificationDTO{}
	for _, e := range ed {
		id := e.ID
		r.Education = append(r.Education, educationDTO{ID: &id, Level: e.Level, Institution: e.Institution, Major: e.Major, StartYear: e.StartYear, EndYear: e.EndYear, GPA: e.Gpa})
	}
	for _, e := range ex {
		id := e.ID
		r.Experiences = append(r.Experiences, experienceDTO{ID: &id, Company: e.Company, Title: e.Title, Location: e.Location, StartDate: e.StartDate.Format("2006-01-02"),
			StartPrecision: e.StartPrecision, EndDate: dateStr(e.EndDate), EndPrecision: e.EndPrecision, IsCurrent: e.IsCurrent, Description: e.Description})
	}
	contact := prof.ContactEmail
	r.ContactEmail = &contact
	sk, err := q.ListSkills(ctx, p.UserID)
	if err != nil {
		return r, err
	}
	skills := []string{}
	for _, v := range sk {
		skills = append(skills, v.Name)
	}
	r.Skills = &skills
	lg, err := q.ListLanguages(ctx, p.UserID)
	if err != nil {
		return r, err
	}
	langs := []languageDTO{}
	for _, v := range lg {
		langs = append(langs, languageDTO{v.Name, v.Level})
	}
	r.Languages = &langs
	lk, err := q.ListLinks(ctx, p.UserID)
	if err != nil {
		return r, err
	}
	links := []linkDTO{}
	for _, v := range lk {
		links = append(links, linkDTO{v.Label, v.Url})
	}
	r.Links = &links
	pj, err := q.ListProjects(ctx, p.UserID)
	if err != nil {
		return r, err
	}
	projects := []projectDTO{}
	for _, v := range pj {
		projects = append(projects, projectDTO{v.Name, v.Description})
	}
	r.Projects = &projects
	for _, e := range ce {
		r.Certifications = append(r.Certifications, certificationDTO{e.Name, e.Issuer, dateStr(e.IssuedOn), dateStr(e.ExpiresOn)})
	}
	r.Email, r.EmailVerified = p.Email, p.EmailVerified
	id, err := q.GetCandidateIdentity(ctx, p.UserID)
	switch {
	case err == nil:
		r.Identity.Status, r.Identity.NIKMasked = "linked", identity.Mask(id.NikLast4)
	case platform.IsNoRows(err):
		r.Identity.Status = "pending_verification"
	default:
		return r, err
	}
	r.Missing = []string{}
	if r.Phone == "" {
		r.Missing = append(r.Missing, "phone")
	}
	if r.BirthDate == nil {
		r.Missing = append(r.Missing, "birthDate")
	}
	if r.City == "" {
		r.Missing = append(r.Missing, "city")
	}
	if len(r.Education) == 0 {
		r.Missing = append(r.Missing, "education")
	}
	r.Complete = len(r.Missing) == 0
	return r, nil
}

// GET /me/profile
func (h *Handler) GetProfile(c *gin.Context) {
	r, err := h.loadProfile(c, h.Q, auth.Current(c))
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, r)
}

var phoneRe = regexp.MustCompile(`^\+?[0-9]{8,15}$`)
var contactEmailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
var eduLevels = []string{"SMA", "SMK", "D3", "D4", "S1", "S2", "S3", "Lainnya"}

// PUT /me/profile: optimistic concurrency via version. Riwayat diganti utuh dalam satu transaksi.
func (h *Handler) UpdateProfile(c *gin.Context) {
	p := auth.Current(c)
	var req profileDTO
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	f := map[string]string{}
	req.FullName = strings.TrimSpace(req.FullName)
	if l := len(req.FullName); l < 2 || l > 200 {
		f["fullName"] = "Nama lengkap wajib 2 sampai 200 karakter."
	}
	req.Phone = strings.ReplaceAll(strings.TrimSpace(req.Phone), " ", "")
	if req.Phone != "" && !phoneRe.MatchString(req.Phone) {
		f["phone"] = "Nomor telepon 8 sampai 15 digit, boleh diawali +."
	}
	birth, ok := parseDate(req.BirthDate)
	if !ok || (birth != nil && (birth.After(time.Now().AddDate(-15, 0, 0)) || birth.Before(time.Now().AddDate(-80, 0, 0)))) {
		f["birthDate"] = "Tanggal lahir tidak valid."
	}
	if len(req.City) > 100 {
		f["city"] = "Kota maksimal 100 karakter."
	}
	if len(req.Address) > 500 {
		f["address"] = "Alamat maksimal 500 karakter."
	}
	if len(req.Summary) > 2000 {
		f["summary"] = "Ringkasan maksimal 2000 karakter."
	}
	if len(req.Education) > 10 || len(req.Experiences) > 20 || len(req.Certifications) > 20 {
		f["education"] = "Jumlah riwayat melebihi batas."
	}
	for i, e := range req.Education {
		if !slices.Contains(eduLevels, e.Level) {
			f["education."+strconv.Itoa(i)+".level"] = "Pilih jenjang."
		}
		if l := len(strings.TrimSpace(e.Institution)); l < 2 || l > 200 {
			f["education."+strconv.Itoa(i)+".institution"] = "Nama institusi wajib diisi."
		}
		if e.StartYear != nil && e.EndYear != nil && *e.EndYear < *e.StartYear {
			f["education."+strconv.Itoa(i)+".endYear"] = "Tahun lulus tidak boleh sebelum tahun masuk."
		}
		if e.GPA != nil && (*e.GPA < 0 || *e.GPA > 4) {
			f["education."+strconv.Itoa(i)+".gpa"] = "IPK 0 sampai 4."
		}
	}
	exps := make([][2]*time.Time, len(req.Experiences))
	for i, e := range req.Experiences {
		s, ok1 := parseDate(&e.StartDate)
		en, ok2 := parseDate(e.EndDate)
		if !ok1 || s == nil {
			f["experiences."+strconv.Itoa(i)+".startDate"] = "Tanggal mulai wajib diisi."
		}
		if !ok2 || (s != nil && en != nil && en.Before(*s)) {
			f["experiences."+strconv.Itoa(i)+".endDate"] = "Tanggal selesai tidak valid."
		}
		if len(strings.TrimSpace(e.Company)) < 2 || len(strings.TrimSpace(e.Title)) < 2 {
			f["experiences."+strconv.Itoa(i)+".company"] = "Perusahaan dan jabatan wajib diisi."
		}
		exps[i] = [2]*time.Time{s, en}
	}
	prec := func(s string) string {
		if s == "month" || s == "year" {
			return s
		}
		return "day"
	}
	if req.ContactEmail != nil {
		v := strings.ToLower(strings.TrimSpace(*req.ContactEmail))
		if v != "" && (len(v) > 254 || !contactEmailRe.MatchString(v)) {
			f["contactEmail"] = "Format email kontak tidak valid."
		}
		req.ContactEmail = &v
	}
	if req.Skills != nil && len(*req.Skills) > 100 || req.Languages != nil && len(*req.Languages) > 20 || req.Links != nil && len(*req.Links) > 20 || req.Projects != nil && len(*req.Projects) > 30 {
		f["skills"] = "Jumlah data melebihi batas."
	}
	if req.Links != nil {
		for i, l := range *req.Links {
			if !strings.HasPrefix(l.URL, "https://") && !strings.HasPrefix(l.URL, "http://") || len(l.URL) > 500 {
				f["links."+strconv.Itoa(i)+".url"] = "Tautan harus diawali http:// atau https://."
			}
		}
	}
	certs := make([][2]*time.Time, len(req.Certifications))
	for i, e := range req.Certifications {
		a, ok1 := parseDate(e.IssuedOn)
		b, ok2 := parseDate(e.ExpiresOn)
		if !ok1 || !ok2 || len(strings.TrimSpace(e.Name)) < 2 {
			f["certifications."+strconv.Itoa(i)+".name"] = "Nama dan tanggal sertifikasi tidak valid."
		}
		certs[i] = [2]*time.Time{a, b}
	}
	if len(f) > 0 {
		platform.Fail(c, platform.ErrValidation(f))
		return
	}
	ctx := c.Request.Context()
	var out profileResp
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		_, err := q.UpdateCandidateProfile(ctx, sqlc.UpdateCandidateProfileParams{UserID: p.UserID, Version: req.Version,
			FullName: req.FullName, Phone: req.Phone, BirthDate: birth, City: strings.TrimSpace(req.City),
			Address: strings.TrimSpace(req.Address), Summary: strings.TrimSpace(req.Summary)})
		if platform.IsNoRows(err) {
			return platform.ErrConflict("stale_version", "Profil sudah diubah di tab atau perangkat lain. Muat ulang untuk melihat versi terbaru.")
		}
		if err != nil {
			return err
		}
		if err := q.DeleteEducationForUser(ctx, p.UserID); err != nil {
			return err
		}
		for _, e := range req.Education {
			if err := q.InsertEducation(ctx, sqlc.InsertEducationParams{UserID: p.UserID, Level: e.Level, Institution: strings.TrimSpace(e.Institution),
				Major: strings.TrimSpace(e.Major), StartYear: e.StartYear, EndYear: e.EndYear, Gpa: e.GPA}); err != nil {
				return err
			}
		}
		if err := q.DeleteExperiencesForUser(ctx, p.UserID); err != nil {
			return err
		}
		for i, e := range req.Experiences {
			end := exps[i][1]
			if e.IsCurrent {
				end = nil
			}
			if _, err := q.InsertExperienceFull(ctx, sqlc.InsertExperienceFullParams{UserID: p.UserID, Company: strings.TrimSpace(e.Company),
				Title: strings.TrimSpace(e.Title), Location: strings.TrimSpace(e.Location), StartDate: *exps[i][0], StartPrecision: prec(e.StartPrecision),
				EndDate: end, EndPrecision: prec(e.EndPrecision), IsCurrent: e.IsCurrent, Description: e.Description}); err != nil {
				return err
			}
		}
		if req.ContactEmail != nil {
			if err := q.SetContactEmail(ctx, sqlc.SetContactEmailParams{UserID: p.UserID, ContactEmail: *req.ContactEmail}); err != nil {
				return err
			}
		}
		if req.Skills != nil {
			if err := q.DeleteSkillsForUser(ctx, p.UserID); err != nil {
				return err
			}
			for _, v := range *req.Skills {
				if v = strings.TrimSpace(v); v != "" && len(v) <= 80 {
					if err := q.InsertSkill(ctx, sqlc.InsertSkillParams{UserID: p.UserID, Name: v}); err != nil {
						return err
					}
				}
			}
		}
		if req.Languages != nil {
			if err := q.DeleteLanguagesForUser(ctx, p.UserID); err != nil {
				return err
			}
			for _, v := range *req.Languages {
				if n := strings.TrimSpace(v.Name); n != "" && len(n) <= 60 && len(v.Level) <= 60 {
					if err := q.InsertLanguage(ctx, sqlc.InsertLanguageParams{UserID: p.UserID, Name: n, Level: strings.TrimSpace(v.Level)}); err != nil {
						return err
					}
				}
			}
		}
		if req.Links != nil {
			if err := q.DeleteLinksForUser(ctx, p.UserID); err != nil {
				return err
			}
			for _, v := range *req.Links {
				if err := q.InsertLink(ctx, sqlc.InsertLinkParams{UserID: p.UserID, Label: strings.TrimSpace(v.Label), Url: strings.TrimSpace(v.URL)}); err != nil {
					return err
				}
			}
		}
		if req.Projects != nil {
			if err := q.DeleteProjectsForUser(ctx, p.UserID); err != nil {
				return err
			}
			for _, v := range *req.Projects {
				if n := strings.TrimSpace(v.Name); len(n) >= 2 && len(n) <= 200 && len(v.Description) <= 2000 {
					if err := q.InsertProject(ctx, sqlc.InsertProjectParams{UserID: p.UserID, Name: n, Description: v.Description}); err != nil {
						return err
					}
				}
			}
		}
		if err := q.DeleteCertificationsForUser(ctx, p.UserID); err != nil {
			return err
		}
		for i, e := range req.Certifications {
			if err := q.InsertCertification(ctx, sqlc.InsertCertificationParams{UserID: p.UserID, Name: strings.TrimSpace(e.Name),
				Issuer: strings.TrimSpace(e.Issuer), IssuedOn: certs[i][0], ExpiresOn: certs[i][1]}); err != nil {
				return err
			}
		}
		if err := platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "candidate",
			Action: "candidate.profile_update", TargetType: "user", TargetID: p.UserID.String(), RequestID: platform.RequestID(c)}); err != nil {
			return err
		}
		out, err = h.loadProfile(c, q, p)
		return err
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}
