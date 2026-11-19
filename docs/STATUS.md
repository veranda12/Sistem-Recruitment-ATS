# Status Implementasi

Terakhir diperbarui: 2026-11-19. Dokumen ini mencatat hasil **aktual** yang dijalankan, bukan rencana.

## Lingkungan yang ditemukan (inventaris fase 1)

| Komponen | Versi / kondisi |
|---|---|
| OS | Windows 11 Home 10.0.26200 |
| Go | 1.24.5 windows/amd64 |
| Node / npm | 24.22.1 / 11.19.1 |
| PostgreSQL | 17.5 (instance lokal port 5432, role `pos`) ; database proyek: `ats_dev`, `ats_test` |
| sqlc | 1.29.0 (binary rilis Windows) |
| golang-migrate CLI | v4.18.3 (`go install -tags postgres`); binary `cmd/migrate` memakai migrasi tertanam |
| Redis | **tidak tersedia** (tidak terpasang, Docker daemon mati, WSL hanya distro docker-desktop) -> API berjalan mode `redis: disabled` |
| Git | Repo belum ada; diinisialisasi lokal (`main`), tanpa remote |

## Fase

| Fase | Status | Catatan |
|---|---|---|
| 1 Inventaris & rencana | Selesai | `docs/architecture/architecture.md`, `docs/architecture/adr/README.md`, `docs/product/assumptions.md`, `docs/product/plan.md` |
| 2 Foundation | Backend selesai; design system frontend sedang dikerjakan | config tervalidasi, logger JSON, `/healthz` `/readyz`, sesi + CSRF, RBAC scope, seed |
| 3 Vertical slice | Backend + test DB riil lulus; UI sedang dikerjakan | karier -> registrasi -> profil -> apply -> recruiter -> transisi teraudit; workflow publish/simulate/rollback/approval |
| 4-10 | Belum dimulai | |

## Migrasi terakhir

`0003_workflow_recruitment` (versi skema 3).

## Hasil uji aktual

Perintah (dari `backend/`):

```
go build ./... && go vet ./...
go test ./internal/workflow/ ./internal/identity/
DATABASE_URL_TEST="postgres://pos:pos@localhost:5432/ats_test?sslmode=disable" go test ./internal/app/ -count=1 -p 1 -v
```

| Uji | Hasil | Tanggal |
|---|---|---|
| `go build ./...`, `go vet ./...` | lulus | 2026-11-19 |
| Unit workflow (validasi graph: siklus, target hilang, tanpa fallback, tak terjangkau, terminal keluar, approval kosong, field rule terlarang; null semantics; simulasi 5 cabang) | lulus | 2026-11-19 |
| Unit identity (normalisasi NIK termasuk digit non-ASCII ditolak, AEAD nonce unik, AAD org, blind index berkunci) | lulus | 2026-11-19 |
| Migrasi dari database kosong (`bin/migrate up` di `ats_dev` baru) + seed | lulus, versi 3 dirty=false | 2026-11-19 |
| `TestCandidateToRecruiterSlice` (publik hanya published, draft 404, tutup berstatus jelas, keyset pagination, filter allowlist, registrasi netral email ganda, verifikasi email via worker, profil wajib lengkap, edit basi 409, CSRF 403, Idempotency-Key replay, already_applied, job_closed, 2 lamaran, snapshot tidak berubah, scope recruiter OPS vs TI 404, HM baca saja, examiner 403, reveal NIK beralasan + audit, withdraw, kandidat lain 404, replay outbox tidak menggandakan notifikasi, logout mencabut sesi) | lulus | 2026-11-19 |
| `TestConcurrentSameNIKRegistration` (6 registrasi paralel NIK sama -> 1 master, 5 klaim manual, akun klaim tidak bisa melamar) | lulus | 2026-11-19 |
| `TestApplyRacesClose` (8 apply paralel vs close: semua 201 atau job_closed, jumlah tersimpan = jumlah sukses) | lulus (pada run ini close menang 8/8) | 2026-11-19 |
| `TestWorkflowEventBranchesOnRealDB` (lulus/di bawah ambang/manual/paket lain diabaikan, event duplikat no-op, rule_ref + versi tercatat) | lulus | 2026-11-19 |
| `TestApprovalGuardsOnRealDB` (self-approval, peran bukan penyetuju, 2 approver serentak -> tepat 1, approval basi setelah withdrawal, event untuk lamaran ditarik diabaikan, approve via API) | lulus | 2026-11-19 |
| `TestWorkflowPublishVersioningAndPin` (draft invalid tidak bisa publish, draft basi 409, simulasi, publish serentak nomor unik, versi immutable di DB, lamaran lama pakai v1, lamaran baru pakai versi aktif, rollback, recruiter 403) | lulus | 2026-11-19 |

## Blocker / keterbatasan saat ini

- Redis tidak tersedia di mesin ini: cache publik dan limiter global belum teruji end-to-end; jalur degraded (limiter lokal) yang dipakai.
- Frontend belum ada.

## Langkah konkret berikutnya

Bangun frontend (Vite + React 19 + TS strict + Tailwind) dengan design token, layout karier/kandidat/back office, lalu alur UI registrasi -> apply -> recruiter membuka lamaran, dan uji Playwright.
