# Rencana Fase dan Matriks Requirement

Status aktual tiap fase dicatat di `docs/STATUS.md`. Dokumen ini adalah rencana; jangan menganggap butir selesai tanpa bukti di STATUS.

## Fase

| Fase | Isi | Gerbang keluar |
|---|---|---|
| 1 | Inventaris, arsitektur, ADR, asumsi, ERD awal, rencana | Dokumen ada di `docs/` |
| 2 | Foundation: config tervalidasi, logger JSON, `/healthz` `/readyz`, migrasi, sqlc, sesi, CSRF, RBAC + scope, organisasi, seed, design system | Login/logout dan penolakan lintas scope lulus di API test dan UI |
| 3 | Vertical slice: karier -> registrasi/verifikasi -> profil + NIK -> apply (snapshot + pin workflow) -> recruiter membuka lamaran -> transisi manual teraudit. Lalu requisition, interview, keputusan, builder, evaluator, versioning | Integration test DB riil; alur Playwright kandidat + recruiter |
| 4 | Ujian: undangan, start, resume, autosave, submit, receipt, expiry, skoring, esai, rilis, monitoring, event workflow | Tes race/idempotensi lulus |
| 5 | Komunikasi, laporan, admin, retensi, observability, seed lengkap, seed k6 | Dashboard cocok fixture |
| 6 | Verifikasi menyeluruh: gofmt, vet, test, race, migrasi kosong + upgrade, drift sqlc, Vitest, Playwright, build | Semua hijau atau tercatat |
| 7 | Setup Windows PowerShell + preflight | Checkout bersih jalan |
| 8 | Artefak Linux, Nginx, systemd, monitoring | Konfigurasi tervalidasi |
| 9 | Backup/restore runbook + drill | Restore teruji |
| 10 | Handover | Dokumen lengkap |

## Matriks requirement -> modul -> test -> bukti

| Requirement | Modul | Test | Bukti |
|---|---|---|---|
| GET /jobs hanya versi terbit, filter, pagination stabil | recruitment | `backend/internal/recruitment/jobs_test.go` | STATUS |
| Apply balapan dengan penutupan mengikuti urutan transaksi | recruitment | `apply_test.go::TestApplyRacesClose` | STATUS |
| Dua registrasi bersamaan NIK sama -> satu master | identity | `identity_test.go::TestConcurrentNIK` | STATUS |
| Snapshot lamaran tidak berubah saat profil diperbarui | recruitment | `apply_test.go::TestSnapshotImmutable` | STATUS |
| /me/* dibatasi pemilik | recruitment | `apply_test.go::TestOwnerScope` | STATUS |
| Transisi memeriksa izin, expected version, guard | workflow, recruitment | `transition_test.go` | STATUS |
| Workflow: validasi graph, publish immutable, versi pin | workflow | `validate_test.go`, `engine_test.go` | STATUS |
| Login/logout, sesi revoke, CSRF | auth | `auth_test.go` | STATUS |
| RBAC scope unit/job, HM hanya miliknya, Examiner tanpa NIK | auth | `authz_test.go` | STATUS |
| Cegah hapus admin aktif terakhir | auth | `admin_test.go` | STATUS |
| Ujian: start idempoten, save receipt, submit sekali | exam | `exam_test.go` | STATUS |
| Autosave client: debounce, serialisasi, konflik | frontend exam | `autosave.test.ts` | STATUS |
| Audit actor/action/target/requestId | platform | `audit_test.go` | STATUS |
| Alur UI kandidat & recruiter | e2e | `tests/e2e/*.spec.ts` | STATUS |
