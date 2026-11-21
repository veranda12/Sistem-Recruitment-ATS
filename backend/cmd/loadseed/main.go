// loadseed menyiapkan peserta sintetis untuk uji beban k6: N kandidat unik, masing-masing satu lamaran di tahap
// ujian dan satu undangan valid pada jadwal khusus run. Deterministik per run ID dan hanya berjalan di database
// yang namanya berakhiran _load atau _test. Pembersihan (-cleanup) hanya menyentuh data berprefiks run ID.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"ats/db/sqlc"
	"ats/internal/app"
	"ats/internal/auth"
	"ats/internal/identity"
	"ats/internal/platform"
	"ats/internal/workflow"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type participant struct {
	Email        string `json:"email"`
	InvitationID string `json:"invitationId"`
}

func main() {
	run := flag.String("run", "", "ID run (huruf kecil/angka), mis. r20261121a")
	n := flag.Int("n", 500, "jumlah peserta")
	out := flag.String("out", "", "berkas fixture JSON untuk k6")
	hours := flag.Int("hours", 6, "lama jadwal terbuka")
	minutes := flag.Int("minutes", 60, "durasi ujian per peserta")
	cleanup := flag.Bool("cleanup", false, "nonaktifkan data run ini (akun dinonaktifkan, undangan dibatalkan)")
	flag.Parse()
	if *run == "" || strings.ContainsAny(*run, " -_@") {
		fail("isi -run dengan huruf kecil/angka tanpa spasi")
	}
	cfg, err := platform.LoadConfig()
	if err != nil {
		fail(err.Error())
	}
	u, _ := url.Parse(cfg.DatabaseURL)
	dbName := strings.TrimPrefix(u.Path, "/")
	if cfg.IsProduction() || !(strings.HasSuffix(dbName, "_load") || strings.HasSuffix(dbName, "_test")) {
		fail("loadseed hanya untuk database *_load atau *_test di luar produksi")
	}
	ctx := context.Background()
	pool, err := platform.NewPool(ctx, cfg.DatabaseURL, 8, "ats-loadseed")
	if err != nil {
		fail(err.Error())
	}
	defer pool.Close()
	prefix := "k6-" + *run + "-"

	if *cleanup {
		tag, err := pool.Exec(ctx, `UPDATE exam_participants SET status = 'cancelled' WHERE candidate_user_id IN (SELECT id FROM users WHERE email LIKE $1) AND status IN ('invited','started')`, prefix+"%")
		if err != nil {
			fail(err.Error())
		}
		tag2, err := pool.Exec(ctx, `UPDATE users SET status = 'disabled' WHERE email LIKE $1`, prefix+"%")
		if err != nil {
			fail(err.Error())
		}
		fmt.Printf("run %s dibersihkan: %d undangan dibatalkan, %d akun dinonaktifkan (audit dan jawaban tetap untuk rekonsiliasi)\n", *run, tag.RowsAffected(), tag2.RowsAffected())
		return
	}

	pw := os.Getenv("SEED_PASSWORD")
	if auth.ValidatePassword(pw) != "" {
		fail("SEED_PASSWORD wajib (sandi semua peserta sintetis)")
	}
	hash, err := auth.HashPassword(ctx, pw)
	if err != nil {
		fail(err.Error())
	}
	q := sqlc.New(pool)
	orgID, err := app.ResolveOrg(ctx, pool, cfg.OrgSlug)
	if err != nil {
		fail("organisasi belum ada; jalankan migrate + seed dulu")
	}
	keys := identity.Keys{EncKey: cfg.NIKEncKey, EncVersion: cfg.NIKEncKeyVer, IndexKey: cfg.NIKIndexKey, IndexVersion: cfg.NIKIndexKeyVer}
	job, err := q.GetJobBySlug(ctx, sqlc.GetJobBySlugParams{OrgID: orgID, Slug: "operator-produksi-karawang"})
	if err != nil {
		fail("lowongan seed tidak ditemukan")
	}
	wf, _ := q.GetWorkflow(ctx, job.WorkflowID)
	pkg, err := q.GetExamPackageByKey(ctx, sqlc.GetExamPackageByKeyParams{OrgID: orgID, Key: "tes-dasar"})
	if err != nil {
		fail("paket tes-dasar tidak ditemukan")
	}
	pv, _ := q.LatestPackageVersion(ctx, pkg.ID)
	hrUser, _ := q.GetUserByEmail(ctx, sqlc.GetUserByEmailParams{OrgID: orgID, Email: "hr.ops@arunika.example"})
	now := time.Now()
	sched, err := q.CreateSchedule(ctx, sqlc.CreateScheduleParams{OrgID: orgID, PackageVersionID: pv.ID, Name: "Uji beban " + *run,
		OpensAt: now.Add(-time.Minute), ClosesAt: now.Add(time.Duration(*hours) * time.Hour), MaxAttempts: 1, CreatedBy: &hrUser.ID})
	if err != nil {
		fail(err.Error())
	}
	// Durasi ujian khusus run memakai akomodasi waktu agar plateau dapat menahan semua attempt aktif.
	extra := int32(max(*minutes-int(pv.DurationMinutes), 0))

	// NIK sintetis deterministik per run+indeks, diawali 99 (bukan kode wilayah nyata).
	runHash := sha256.Sum256([]byte(*run))
	runNum := binary.BigEndian.Uint32(runHash[:4]) % 100000
	fixture := make([]participant, 0, *n)
	start := time.Now()
	for batch := 0; batch < *n; batch += 50 {
		err := platform.WithTx(ctx, pool, func(tx pgx.Tx) error {
			qt := q.WithTx(tx)
			for i := batch; i < min(batch+50, *n); i++ {
				email := fmt.Sprintf("%s%04d@load.test", prefix, i)
				nt := now
				usr, err := qt.CreateUser(ctx, sqlc.CreateUserParams{OrgID: orgID, Kind: "candidate", Email: email, DisplayName: fmt.Sprintf("Peserta Beban %04d", i), PasswordHash: hash, EmailVerifiedAt: &nt})
				if err != nil {
					return fmt.Errorf("user %d: %w", i, err)
				}
				if err := qt.CreateCandidateProfile(ctx, sqlc.CreateCandidateProfileParams{UserID: usr.ID, OrgID: orgID, FullName: usr.DisplayName}); err != nil {
					return err
				}
				nik := fmt.Sprintf("99%05d%09d", runNum, i)
				s, err := keys.Seal(nik, orgID[:])
				if err != nil {
					return err
				}
				if _, err := qt.InsertCandidateIdentity(ctx, sqlc.InsertCandidateIdentityParams{OrgID: orgID, UserID: usr.ID, NikCiphertext: s.Ciphertext, NikNonce: s.Nonce,
					KeyVersion: int32(s.KeyVersion), NikBlindIndex: s.BlindIndex, BlindKeyVersion: int32(s.IndexVersion), NikLast4: s.Last4}); err != nil {
					return err
				}
				def, err := workflow.LoadVersion(ctx, qt, *wf.ActiveVersionID)
				if err != nil {
					return err
				}
				a, err := qt.InsertApplication(ctx, sqlc.InsertApplicationParams{OrgID: orgID, JobID: job.ID, CandidateUserID: usr.ID, UnitID: job.UnitID, WorkflowVersionID: *wf.ActiveVersionID, StageKey: def.Start})
				if err != nil {
					return err
				}
				if err := qt.InsertApplicationSnapshot(ctx, sqlc.InsertApplicationSnapshotParams{ApplicationID: a.ID, Profile: []byte(`{"sintetis":true}`)}); err != nil {
					return err
				}
				if _, err := workflow.Execute(ctx, qt, workflow.Command{ExecutionID: uuid.New(), ApplicationID: a.ID, Kind: "manual", ToStage: "ujian",
					Actor: workflow.Actor{UserID: &hrUser.ID, Kind: "staff"}, Reason: "Persiapan uji beban " + *run}); err != nil {
					return err
				}
				p, err := qt.InsertParticipant(ctx, sqlc.InsertParticipantParams{OrgID: orgID, ScheduleID: sched.ID, ApplicationID: a.ID, CandidateUserID: usr.ID, ExtraMinutes: extra, InvitedBy: &hrUser.ID})
				if err != nil {
					return err
				}
				fixture = append(fixture, participant{Email: email, InvitationID: p.ID.String()})
			}
			return nil
		})
		if err != nil {
			fail(err.Error())
		}
	}
	// Event outbox persiapan tidak relevan untuk pengukuran; tandai selesai agar worker tidak membebani run.
	if _, err := pool.Exec(ctx, `UPDATE outbox SET status = 'done', processed_at = now() WHERE status = 'pending' AND aggregate_type = 'application'
		AND aggregate_id IN (SELECT a.id::text FROM job_applications a JOIN users u ON u.id = a.candidate_user_id WHERE u.email LIKE $1)`, prefix+"%"); err != nil {
		fail(err.Error())
	}
	b, _ := json.MarshalIndent(map[string]any{"run": *run, "scheduleId": sched.ID, "packageVersion": pv.VersionNo, "participants": fixture}, "", "  ")
	if err := os.WriteFile(*out, b, 0o600); err != nil {
		fail(err.Error())
	}
	fmt.Printf("run %s: %d peserta, jadwal %s, %.1f s -> %s\n", *run, len(fixture), sched.ID, time.Since(start).Seconds(), *out)
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "loadseed:", msg)
	os.Exit(2)
}
