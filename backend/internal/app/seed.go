package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"ats/db/sqlc"
	"ats/internal/auth"
	"ats/internal/identity"
	"ats/internal/platform"
	"ats/internal/workflow"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Data fiktif PT Arunika Daya Persada. Dipakai cmd/seed dan integration test.
type seedUser struct {
	Email, Name, Kind, Label string
}

var SeedUsers = []seedUser{
	{"admin@arunika.example", "Rangga Admin", "staff", "Super Admin"},
	{"hr.ops@arunika.example", "Dewi Lestari", "staff", "HR Admin unit Operasional Pabrik"},
	{"hr.ti@arunika.example", "Yusuf Pratama", "staff", "HR Admin unit Teknologi Informasi"},
	{"rekruter.ops@arunika.example", "Maya Sari", "staff", "Recruiter unit Operasional Pabrik"},
	{"rekruter.ti@arunika.example", "Andi Wijaya", "staff", "Recruiter lowongan Backend Engineer"},
	{"hm.ops@arunika.example", "Bambang Hartono", "staff", "Hiring Manager lowongan Operator Produksi"},
	{"penguji@arunika.example", "Ratna Kusuma", "staff", "Examiner"},
	{"sari.kandidat@contoh.test", "Sari Wulandari", "candidate", "Kandidat (profil lengkap)"},
	{"budi.kandidat@contoh.test", "Budi Santoso", "candidate", "Kandidat (profil lengkap)"},
}

var rolePerms = map[string][]string{
	"super_admin": {auth.PermUsersManage, auth.PermRolesManage, auth.PermSettingsManage, auth.PermAuditRead, auth.PermCredentialsManage,
		auth.PermWorkflowsRead, auth.PermWorkflowsManage, auth.PermWorkflowsPublish, auth.PermJobsRead, auth.PermReportsRead, auth.PermCVReplay, auth.PermPrivacyManage},
	"hr_admin": {auth.PermJobsRead, auth.PermJobsManage, auth.PermJobsPublish, auth.PermRequisitionsRead, auth.PermRequisitionsApprove,
		auth.PermApplicationsRead, auth.PermApplicationsMove, auth.PermApplicationsAssign, auth.PermIdentityReveal, auth.PermPIIExport,
		auth.PermWorkflowsRead, auth.PermWorkflowsManage, auth.PermWorkflowsPublish, auth.PermReportsRead, auth.PermReportsExport,
		auth.PermResultsRelease, auth.PermExamsManage, auth.PermAuditRead},
	"recruiter":      {auth.PermJobsRead, auth.PermApplicationsRead, auth.PermApplicationsMove, auth.PermRequisitionsRead},
	"hiring_manager": {auth.PermApplicationsRead, auth.PermApprovalsDecide, auth.PermRequisitionsRead, auth.PermRequisitionsManage},
	"examiner":       {auth.PermExamsManage, auth.PermExamsGrade},
}

var roleNames = map[string]string{"super_admin": "Super Admin", "hr_admin": "HR Admin", "recruiter": "Recruiter",
	"hiring_manager": "Hiring Manager", "examiner": "Examiner"}

func Seed(ctx context.Context, tx pgx.Tx, slug, pwHash string, keys identity.Keys) error {
	q := sqlc.New(tx)
	var orgID uuid.UUID
	profile, _ := json.Marshal(map[string]any{
		"tagline": "Produsen komponen industri dengan pabrik di Karawang dan kantor di Jakarta serta Surabaya.",
		"about":   "PT Arunika Daya Persada (organisasi fiktif untuk demonstrasi) membuka kesempatan kerja di bidang produksi, mutu, dan teknologi informasi.",
	})
	brand, _ := json.Marshal(map[string]any{"primaryColor": "#1557B0", "displayName": "Arunika Karier"})
	if err := tx.QueryRow(ctx, `INSERT INTO organizations (slug, name, timezone, profile, brand) VALUES ($1, 'PT Arunika Daya Persada', 'Asia/Jakarta', $2, $3) RETURNING id`,
		slug, profile, brand).Scan(&orgID); err != nil {
		return err
	}
	units := map[string]uuid.UUID{}
	for code, name := range map[string]string{"OPS": "Operasional Pabrik", "TI": "Teknologi Informasi"} {
		var id uuid.UUID
		if err := tx.QueryRow(ctx, `INSERT INTO units (org_id, code, name) VALUES ($1, $2, $3) RETURNING id`, orgID, code, name).Scan(&id); err != nil {
			return err
		}
		units[code] = id
	}
	locs := map[string]uuid.UUID{}
	for _, l := range [][3]string{{"JKT", "Kantor Pusat Jakarta", "Jakarta Selatan"}, {"KRW", "Pabrik Karawang", "Karawang"}, {"SBY", "Kantor Cabang Surabaya", "Surabaya"}} {
		var id uuid.UUID
		if err := tx.QueryRow(ctx, `INSERT INTO locations (org_id, code, name, city) VALUES ($1, $2, $3, $4) RETURNING id`, orgID, l[0], l[1], l[2]).Scan(&id); err != nil {
			return err
		}
		locs[l[0]] = id
	}
	for k, d := range auth.PermissionCatalog {
		if _, err := tx.Exec(ctx, `INSERT INTO permissions (key, description) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET description = EXCLUDED.description`, k, d); err != nil {
			return err
		}
	}
	roles := map[string]uuid.UUID{}
	for key, perms := range rolePerms {
		var id uuid.UUID
		if err := tx.QueryRow(ctx, `INSERT INTO roles (org_id, key, name, system) VALUES ($1, $2, $3, true) RETURNING id`, orgID, key, roleNames[key]).Scan(&id); err != nil {
			return err
		}
		roles[key] = id
		for _, p := range perms {
			if _, err := tx.Exec(ctx, `INSERT INTO role_permissions (role_id, permission_key) VALUES ($1, $2)`, id, p); err != nil {
				return err
			}
		}
	}
	users := map[string]uuid.UUID{}
	now := time.Now()
	for _, su := range SeedUsers {
		u, err := q.CreateUser(ctx, sqlc.CreateUserParams{OrgID: orgID, Kind: su.Kind, Email: su.Email, DisplayName: su.Name, PasswordHash: pwHash, EmailVerifiedAt: &now})
		if err != nil {
			return err
		}
		users[su.Email] = u.ID
	}

	// Workflow default diterbitkan sebagai versi 1.
	def, _ := json.Marshal(workflow.DefaultDefinition())
	wf, err := q.CreateWorkflow(ctx, sqlc.CreateWorkflowParams{OrgID: orgID, Key: "standar", Name: "Rekrutmen Standar", DraftDefinition: def, UpdatedBy: ptr(users["admin@arunika.example"])})
	if err != nil {
		return err
	}
	if _, err := workflow.Publish(ctx, q, wf.ID, 0, ptr(users["admin@arunika.example"]), "Versi awal dari seed", "seed"); err != nil {
		return err
	}

	type job struct {
		slug, title, unit, loc, etype, status, desc, req string
		closesInDays                                     int
	}
	jobs := []job{
		{"operator-produksi-karawang", "Operator Produksi", "OPS", "KRW", "contract", "published",
			"Menjalankan mesin produksi sesuai SOP, mencatat hasil produksi per shift, dan menjaga kebersihan area kerja.",
			"Minimal SMA/SMK sederajat. Bersedia bekerja shift. Diutamakan berpengalaman di manufaktur.", 30},
		{"supervisor-quality-control", "Supervisor Quality Control", "OPS", "KRW", "full_time", "published",
			"Memimpin tim QC, menyusun laporan mutu harian, dan menindaklanjuti ketidaksesuaian produk.",
			"Minimal D3 Teknik. Pengalaman 3 tahun di bidang mutu. Memahami ISO 9001.", 21},
		{"backend-engineer", "Backend Engineer", "TI", "JKT", "full_time", "published",
			"Mengembangkan dan memelihara layanan internal perusahaan, termasuk integrasi data produksi.",
			"Minimal S1 Informatika atau setara. Menguasai Go atau Java dan PostgreSQL.", 45},
		{"admin-gudang-surabaya", "Admin Gudang", "OPS", "SBY", "contract", "closed",
			"Mencatat keluar-masuk barang dan menyusun laporan stok mingguan.", "Minimal SMA. Teliti dan terbiasa dengan spreadsheet.", 0},
		{"data-analyst", "Data Analyst", "TI", "JKT", "full_time", "draft",
			"Menyusun dasbor produksi dan analisis efisiensi.", "Minimal S1. Menguasai SQL.", 30},
	}
	jobIDs := map[string]uuid.UUID{}
	for i, j := range jobs {
		var closes *time.Time
		if j.closesInDays > 0 {
			t := now.AddDate(0, 0, j.closesInDays)
			closes = &t
		}
		created, err := q.CreateJob(ctx, sqlc.CreateJobParams{OrgID: orgID, UnitID: units[j.unit], LocationID: locs[j.loc], WorkflowID: wf.ID,
			Slug: j.slug, Title: j.title, EmploymentType: j.etype, Description: j.desc, Requirements: j.req, ClosesAt: closes,
			CreatedBy: users["hr.ops@arunika.example"]})
		if err != nil {
			return err
		}
		jobIDs[j.slug] = created.ID
		if j.status != "draft" {
			// published_at berurutan agar urutan daftar publik deterministik.
			if _, err := tx.Exec(ctx, `UPDATE job_openings SET status = 'published', published_at = now() - make_interval(hours => $2) WHERE id = $1`, created.ID, len(jobs)-i); err != nil {
				return err
			}
		}
		if j.status == "closed" {
			if _, err := tx.Exec(ctx, `UPDATE job_openings SET status = 'closed', closed_at = now() WHERE id = $1`, created.ID); err != nil {
				return err
			}
		}
	}

	assign := func(email, role string, unit, job *uuid.UUID) error {
		_, err := tx.Exec(ctx, `INSERT INTO scoped_role_assignments (org_id, user_id, role_id, unit_id, job_id, granted_by) VALUES ($1, $2, $3, $4, $5, $6)`,
			orgID, users[email], roles[role], unit, job, users["admin@arunika.example"])
		return err
	}
	ops, ti := units["OPS"], units["TI"]
	for _, a := range []struct {
		email, role string
		unit, job   *uuid.UUID
	}{
		{"admin@arunika.example", "super_admin", nil, nil},
		{"hr.ops@arunika.example", "hr_admin", &ops, nil},
		{"hr.ti@arunika.example", "hr_admin", &ti, nil},
		{"rekruter.ops@arunika.example", "recruiter", &ops, nil},
		{"rekruter.ti@arunika.example", "recruiter", nil, ptr(jobIDs["backend-engineer"])},
		{"hm.ops@arunika.example", "hiring_manager", nil, ptr(jobIDs["operator-produksi-karawang"])},
		{"hm.ops@arunika.example", "hiring_manager", nil, ptr(jobIDs["supervisor-quality-control"])},
		{"penguji@arunika.example", "examiner", nil, nil},
	} {
		if err := assign(a.email, a.role, a.unit, a.job); err != nil {
			return fmt.Errorf("assign %s: %w", a.email, err)
		}
	}

	// Profil kandidat sintetis. NIK fiktif (bukan milik orang nyata).
	cands := []struct {
		email, nik, phone, city, birth string
	}{
		{"sari.kandidat@contoh.test", "3216014507980001", "081200000001", "Bekasi", "1998-07-05"},
		{"budi.kandidat@contoh.test", "3578010101950002", "081200000002", "Surabaya", "1995-01-01"},
	}
	for _, c := range cands {
		uid := users[c.email]
		name := ""
		for _, su := range SeedUsers {
			if su.Email == c.email {
				name = su.Name
			}
		}
		if err := q.CreateCandidateProfile(ctx, sqlc.CreateCandidateProfileParams{UserID: uid, OrgID: orgID, FullName: name}); err != nil {
			return err
		}
		bd, _ := time.Parse("2006-01-02", c.birth)
		if _, err := q.UpdateCandidateProfile(ctx, sqlc.UpdateCandidateProfileParams{UserID: uid, Version: 1, FullName: name, Phone: c.phone,
			BirthDate: &bd, City: c.city, Summary: "Profil contoh untuk demonstrasi."}); err != nil {
			return err
		}
		y1, y2 := int32(2013), int32(2016)
		if err := q.InsertEducation(ctx, sqlc.InsertEducationParams{UserID: uid, Level: "SMK", Institution: "SMK Negeri Contoh", Major: "Teknik Mesin", StartYear: &y1, EndYear: &y2}); err != nil {
			return err
		}
		s, err := keys.Seal(c.nik, orgID[:])
		if err != nil {
			return err
		}
		if _, err := q.InsertCandidateIdentity(ctx, sqlc.InsertCandidateIdentityParams{OrgID: orgID, UserID: uid, NikCiphertext: s.Ciphertext,
			NikNonce: s.Nonce, KeyVersion: int32(s.KeyVersion), NikBlindIndex: s.BlindIndex, BlindKeyVersion: int32(s.IndexVersion), NikLast4: s.Last4}); err != nil {
			return err
		}
	}
	if err := seedExam(ctx, q, orgID, users["penguji@arunika.example"]); err != nil {
		return fmt.Errorf("seed ujian: %w", err)
	}
	return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: orgID, ActorKind: "system", Action: "seed.run", TargetType: "organization", TargetID: orgID.String()})
}

func ptr[T any](v T) *T { return &v }

// ExamFixture: kunci jawaban fixture. Bobot: pilihan tunggal 1, pilihan jamak 2, esai 2 (maks 8 poin).
// Dipakai seed dan test untuk menghitung nilai yang diharapkan secara independen.
type ExamFixtureQuestion struct {
	Kind    string
	Stem    string
	Options [][2]string // {id, teks}
	Key     []string
	Weight  float64
	Rubric  string
}

var ExamFixture = []ExamFixtureQuestion{
	{"single", "Berapakah hasil 12 x 3?", [][2]string{{"a", "32"}, {"b", "36"}, {"c", "38"}, {"d", "34"}}, []string{"b"}, 1, ""},
	{"single", "Alat pelindung diri yang wajib saat bekerja di dekat mesin press adalah", [][2]string{{"a", "Sarung tangan dan kacamata pelindung"}, {"b", "Topi kain"}, {"c", "Sandal"}, {"d", "Jam tangan"}}, []string{"a"}, 1, ""},
	{"single", "Langkah pertama dalam metode 5S adalah", [][2]string{{"a", "Seiso (bersihkan)"}, {"b", "Seiri (pilah)"}, {"c", "Shitsuke (rawat)"}, {"d", "Seiketsu (standarkan)"}}, []string{"b"}, 1, ""},
	{"single", "Jika produksi 480 unit per 8 jam, berapa unit per jam?", [][2]string{{"a", "50"}, {"b", "60"}, {"c", "70"}, {"d", "80"}}, []string{"b"}, 1, ""},
	{"multiple", "Pilih semua tindakan yang benar saat terjadi kebocoran oli di lantai produksi.", [][2]string{{"a", "Pasang tanda peringatan"}, {"b", "Lapor ke supervisor"}, {"c", "Biarkan sampai shift berikutnya"}, {"d", "Bersihkan dengan bahan penyerap"}}, []string{"a", "b", "d"}, 2, ""},
	{"essay", "Jelaskan langkah Anda bila menemukan produk cacat di lini produksi.", nil, nil, 2,
		"2 poin: hentikan/pisahkan produk, lapor, catat. 1 poin: sebagian langkah. 0: tidak relevan."},
}

func seedExam(ctx context.Context, q *sqlc.Queries, orgID, examiner uuid.UUID) error {
	bank, err := q.CreateQuestionBank(ctx, sqlc.CreateQuestionBankParams{OrgID: orgID, Name: "Tes Dasar Operasional", Category: "Operasional", CreatedBy: &examiner})
	if err != nil {
		return err
	}
	var ids []uuid.UUID
	for _, f := range ExamFixture {
		qu, err := q.CreateQuestion(ctx, sqlc.CreateQuestionParams{OrgID: orgID, BankID: bank.ID})
		if err != nil {
			return err
		}
		opts := []map[string]string{}
		for _, o := range f.Options {
			opts = append(opts, map[string]string{"id": o[0], "text": o[1]})
		}
		ob, _ := json.Marshal(opts)
		key := f.Key
		if key == nil {
			key = []string{}
		}
		kb, _ := json.Marshal(key)
		v, err := q.InsertQuestionVersion(ctx, sqlc.InsertQuestionVersionParams{QuestionID: qu.ID, VersionNo: 1, Kind: f.Kind, Stem: f.Stem,
			Options: ob, AnswerKey: kb, Weight: f.Weight, Rubric: f.Rubric, CreatedBy: &examiner})
		if err != nil {
			return err
		}
		ids = append(ids, v.ID)
	}
	pkg, err := q.CreateExamPackage(ctx, sqlc.CreateExamPackageParams{OrgID: orgID, Key: "tes-dasar", Name: "Tes Dasar Operasional"})
	if err != nil {
		return err
	}
	pv, err := q.InsertPackageVersion(ctx, sqlc.InsertPackageVersionParams{OrgID: orgID, PackageID: pkg.ID, VersionNo: 1, QuestionVersionIds: ids,
		DurationMinutes: 30, PassScore: 75, ShuffleItems: true, ShuffleOptions: true, PublishedBy: &examiner})
	if err != nil {
		return err
	}
	now := time.Now()
	_, err = q.CreateSchedule(ctx, sqlc.CreateScheduleParams{OrgID: orgID, PackageVersionID: pv.ID, Name: "Gelombang November",
		OpensAt: now.Add(-time.Hour), ClosesAt: now.AddDate(0, 0, 14), MaxAttempts: 1, CreatedBy: &examiner})
	return err
}
