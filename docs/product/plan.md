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

| Requirement | Modul | Test (path) | Bukti |
|---|---|---|---|
| GET /jobs hanya terbit, filter allowlist, keyset pagination, draft 404, tutup berstatus jelas | recruitment | `backend/internal/app/flow_test.go::TestCandidateToRecruiterSlice`, `tests/e2e/slice.spec.ts` | STATUS |
| Apply balapan dengan penutupan | recruitment | `flow_test.go::TestApplyRacesClose` | STATUS |
| Dua registrasi bersamaan NIK sama -> satu master | identity, recruitment | `flow_test.go::TestConcurrentSameNIKRegistration`, `internal/identity/nik_test.go` | STATUS |
| Snapshot lamaran tidak berubah; dua lamaran; /me milik sendiri | recruitment | `TestCandidateToRecruiterSlice` | STATUS |
| Transisi: izin, expected version, guard, histori + audit | workflow, recruitment | `TestCandidateToRecruiterSlice`, `workflow_test.go` | STATUS |
| Workflow: validasi graph, rule DSL, simulasi, publish immutable, pin versi, rollback, publish serentak | workflow | `internal/workflow/definition_test.go`, `workflow_test.go::TestWorkflowPublishVersioningAndPin` | STATUS |
| Approval: kuorum, self-approval, dua approver serentak, approval basi setelah withdrawal | workflow | `workflow_test.go::TestApprovalGuardsOnRealDB` | STATUS |
| Login/logout, CSRF, revoke, MFA (TOTP, replay, recovery, reset, wajib per peran) | auth | `flow_test.go`, `internal/auth/mfa_test.go`, `mfa_test.go` | STATUS |
| RBAC scope, HM miliknya, examiner tanpa NIK, admin terakhir, pencabutan efektif | auth, settings | `TestCandidateToRecruiterSlice`, `admin_reports_test.go::TestAdminAccessSettingsAudit` | STATUS |
| Ujian: start idempoten, DTO tanpa kunci, receipt, konflik dua tab, submit sekali, deadline, expiry vs submit | exam | `exam_test.go` | STATUS |
| Skoring fixture (kosong, jamak, esai pending), esai buta, regrade tanpa transisi kedua, rilis -> cabang workflow | exam, workflow | `exam_test.go` | STATUS |
| Bank soal impor + pratinjau, versi soal/paket, ujian berjalan tidak berubah | exam | `bank_test.go`, `internal/exam/bank_test.go` | STATUS |
| Autosave klien: debounce, batas tunda, serialisasi, retry opId sama, konflik, draft IndexedDB, kalibrasi waktu | frontend exam | `frontend/src/features/exam/*.test.ts` | STATUS |
| Ujian lewat browser: offline/reconnect/refresh/receipt, ponsel | e2e | `tests/e2e/exam.spec.ts`, `mobile.spec.ts` | STATUS |
| Interview: bentrok (serentak), reschedule zona waktu, batal otomatis, scorecard; keputusan & pemberitahuan | recruitment | `interviews_test.go`, `tests/e2e/decision.spec.ts` | STATUS |
| Requisition: approval tanpa self-approval, lowongan dari requisition | recruitment | `interviews_test.go::TestRequisitionApprovalAndJob` | STATUS |
| Dokumen: signature, ukuran, karantina, pindai, unduh terscope | documents | `documents_test.go` | STATUS |
| Laporan = fixture (dua lamaran + withdrawal), ekspor terscope, formula injection, tautan pendek | reporting | `admin_reports_test.go::TestReportsMatchFixtureAndExport` | STATUS |
| Audit append-only, /metrics tidak publik | platform | `TestAdminAccessSettingsAudit`, `internal/platform/metrics_test.go` | STATUS |
| Kontras warna aktual | frontend ui | `frontend/src/ui/tokens.test.ts` | STATUS |
| Migrasi dari kosong dan upgrade dari rilis lama | db | `ops/windows/setup.ps1`, `ops/windows/test-upgrade.ps1` | STATUS |
| Restore ke lingkungan kosong | ops | `ops/windows/restore-drill.ps1` | `docs/operations/backup-restore.md` |
| Beban + integritas (oracle independen) | exam | `tests/load/exam.js`, `tests/load/reconcile.sql` | `docs/operations/load-test-report.md` |
