# ATS: Sistem Rekrutmen dan Ujian Daring

Aplikasi rekrutmen yang dipasang di infrastruktur pelanggan (buyout, tanpa akun cloud pengembang): portal karier, akun kandidat, lamaran berbasis workflow berversi, ujian daring tahan putus koneksi, back office per peran, laporan, dan audit. Data contoh memakai organisasi fiktif **PT Arunika Daya Persada**.

Status implementasi, hasil uji aktual, dan keterbatasan: [`docs/STATUS.md`](docs/STATUS.md).

## Isi repository

| Path | Isi |
|---|---|
| `backend/` | Go 1.24 + Gin, modular monolith. `cmd/{api,worker,migrate,seed,loadseed}`, `internal/<modul>`, `db/{migrations,queries,sqlc}` |
| `frontend/` | React 19 + Vite + TypeScript strict + Tailwind 4 + Radix. Bundle terpisah: karier, akun, ujian, back office |
| `contracts/openapi.yaml` | Kontrak API; tipe frontend di-generate darinya dan diperiksa saat build |
| `tests/e2e` | Playwright (desktop 1440 px dan ponsel 390 px) |
| `tests/load` | Skenario k6, orkestrator, rekonsiliasi SQL |
| `ops/windows` | Setup dan operasi pengembangan di Windows (PowerShell 5.1, tanpa Docker) |
| `ops/linux`, `ops/nginx`, `ops/systemd`, `ops/monitoring`, `ops/backup` | Rilis dan operasi produksi Linux |
| `docs/` | Arsitektur, produk, keamanan, operasi, serah terima |

## Mulai cepat (Windows, tanpa Docker)

Prasyarat: Go 1.24+, Node.js 20.19+ (diuji 24), PostgreSQL 17 (psql di PATH). Redis opsional.

```powershell
# 1. Buat .env dengan kunci acak baru (ganti USER/SANDI sesuai PostgreSQL Anda)
powershell -ExecutionPolicy Bypass -File ops\windows\new-env.ps1 -DatabaseUrl 'postgres://USER:SANDI@localhost:5432/ats_dev?sslmode=disable' -SeedPassword 'Dev-Sandi-2026'
# 2. Periksa prasyarat; setiap dependency yang gagal disebut namanya
powershell -ExecutionPolicy Bypass -File ops\windows\preflight.ps1
# 3. Dependency, build, buat database, migrasi, seed data fiktif
powershell -ExecutionPolicy Bypass -File ops\windows\setup.ps1
# 4. Jalankan API (:8080), worker (:8081), frontend dev (:5173)
powershell -ExecutionPolicy Bypass -File ops\windows\start.ps1
# 5. Uji asap, lalu hentikan
powershell -ExecutionPolicy Bypass -File ops\windows\smoke.ps1
powershell -ExecutionPolicy Bypass -File ops\windows\stop.ps1
```

Buka http://localhost:5173. Akun seed (sandi = `SEED_PASSWORD`):

| Email | Peran |
|---|---|
| admin@arunika.example | Super Admin |
| hr.ops@arunika.example / hr.ti@arunika.example | HR Admin unit Operasional Pabrik / Teknologi Informasi |
| rekruter.ops@arunika.example | Recruiter unit Operasional Pabrik |
| rekruter.ti@arunika.example | Recruiter lowongan Backend Engineer |
| hm.ops@arunika.example | Hiring Manager lowongan Operator Produksi dan Supervisor QC |
| penguji@arunika.example | Examiner |
| sari.kandidat@contoh.test, budi.kandidat@contoh.test | Kandidat dengan profil lengkap |

Email (verifikasi, reset, notifikasi) di mode dev ditulis sebagai berkas `.eml` di `.data\mail`.

## Pemeriksaan lengkap

```powershell
powershell -File ops\windows\check-all.ps1 -TestDatabaseUrl 'postgres://USER:SANDI@localhost:5432/ats_test?sslmode=disable'
```

Menjalankan gofmt, go vet, drift sqlc, go test terhadap PostgreSQL sungguhan, build backend, generate kontrak + typecheck, lint, Vitest, build frontend, dan Playwright. Database `*_test` dihapus isinya saat test.

Uji lain: `ops\windows\test-upgrade.ps1` (upgrade skema dari rilis lama), `ops\windows\restore-drill.ps1` (restore ke lingkungan kosong), `tests\load\run-load.ps1` (k6 + rekonsiliasi).

## Produksi (Linux)

`ops/linux/build-release.sh` menghasilkan tarball berisi binary statis, aset frontend, dan konfigurasi ops. Lihat [`docs/operations/deployment-linux.md`](docs/operations/deployment-linux.md).

## Dokumen penting

- Arsitektur dan ADR: [`docs/architecture/`](docs/architecture/)
- Asumsi dan keputusan yang menunggu pelanggan: [`docs/product/assumptions.md`](docs/product/assumptions.md)
- Spesifikasi modul dan matriks otorisasi: [`docs/product/modules.md`](docs/product/modules.md), [`docs/product/authorization.md`](docs/product/authorization.md)
- Keamanan: [`docs/security/`](docs/security/)
- Privasi: [`docs/product/privacy.md`](docs/product/privacy.md)
- Operasi, backup, laporan uji beban: [`docs/operations/`](docs/operations/)
- Serah terima, panduan pengguna, UAT, dukungan: [`docs/handover/`](docs/handover/)
