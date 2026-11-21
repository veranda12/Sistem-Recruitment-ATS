# Status Implementasi

Terakhir diperbarui: 2026-11-21. Dokumen ini mencatat hasil **aktual** yang dijalankan, bukan rencana.

## Lingkungan yang ditemukan (inventaris fase 1)

| Komponen | Versi / kondisi |
|---|---|
| OS | Windows 11 Home 10.0.26200 |
| Go | 1.24.5 windows/amd64 |
| Node / npm | 24.22.1 / 11.19.1 |
| PostgreSQL | 17.5 (instance lokal port 5432, role `pos`) ; database proyek: `ats_dev`, `ats_test` |
| sqlc | 1.29.0 (binary rilis Windows) |
| golang-migrate CLI | v4.18.3 (`go install -tags postgres`); binary `cmd/migrate` memakai migrasi tertanam |
| Redis | 2026-11-19: tidak tersedia. 2026-11-21: Redis 8.10.3 (redis-windows, msys2, tanpa service) di `tools\redis`, dijalankan manual `ops\windows\redis-start.ps1`; tidak berjalan otomatis saat boot |
| Poppler | 24.08.0 (`C:\poppler-24.08.0`) |
| Tesseract | 5.5.0.20241111 + tessdata_fast ind/eng/osd commit 87416418 (`tools\tessdata`, SHA-256 terverifikasi) |
| LibreOffice | 26.2.5.2 (winget; 26.8.3.1 gagal diunduh 404) |
| Git | Repo belum ada; diinisialisasi lokal (`main`), tanpa remote |

## Fase

| Fase | Status | Catatan |
|---|---|---|
| 1 Inventaris & rencana | Selesai | `docs/architecture/architecture.md`, `docs/architecture/adr/README.md`, `docs/product/assumptions.md`, `docs/product/plan.md` |
| 2 Foundation | Backend selesai; design system + 3 layout frontend selesai | config tervalidasi, logger JSON, `/healthz` `/readyz`, sesi + CSRF, RBAC scope, seed |
| 3 Vertical slice | Selesai: backend, UI, dan E2E Playwright lulus | karier -> registrasi -> profil -> apply -> recruiter -> transisi teraudit; workflow publish/simulate/rollback/approval |
| 4 Ujian | Selesai: protokol ujian, skoring, esai, rilis, monitoring, ruang ujian web, bank soal + impor CSV + versi paket. Belum: uji race lintas dua instance API (tidak ada staging) | commit 39cb841, b0b79f2, 90c3b4e |
| 5 Komunikasi, laporan, admin | Selesai | 3f10478, 1d287fe |
| 6-7 Verifikasi, setup Windows | Selesai (check-all 11/11, clean checkout, uji upgrade skema) | 4b7de38, 7f70641 |
| 8 Artefak Linux | Artefak dibangun; belum dijalankan di server Linux | 78df906 |
| 9 Backup/restore | Drill restore lulus | 7fc530c |
| 10 Handover | Dokumen selesai | e4fbb7e |
| Tambahan: MFA staf | Selesai + test integrasi | 8f416cd |
| Tambahan: uji beban 500 VU | Selesai, 0 selisih rekonsiliasi | `docs/operations/load-test-report.md` |
| Tambahan: pembacaan CV | Selesai: parser semua format, antrean + worker khusus, API review/apply, UI, fixture + evaluasi, benchmark | d64360d, 1a21dff, a218118; `docs/cv-parsing.md` |

## Migrasi terakhir

`0009_cv_parse_timing` (versi skema 9). `ats_dev` dimigrasi ke 9 pada 2026-11-21.

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

| Frontend `tsc -b`, `eslint .`, `vite build` (bundle per area: career/auth/candidate/backoffice) | lulus | 2026-11-20 |
| Vitest kontras token (15 pasangan warna aktual, WCAG 4.5/3) | lulus | 2026-11-20 |
| `seed --reset` (hanya APP_ENV development/test dan DB *_dev/_test/_e2e) + seluruh `go test ./...` ulang | lulus | 2026-11-20 |
| Playwright run 1 | gagal sebelum tes: webServer API start sebelum globalSetup, DB e2e belum dimigrasi | 2026-11-20 |
| Playwright `cd frontend && npx playwright test` (desktop 1440: portal+filter URL+tutup; registrasi->verifikasi email->profil->apply 2 lowongan->apply ulang ditolak; recruiter OPS pindah tahap beralasan, histori setelah refresh, recruiter TI 404; HR reveal NIK; kandidat lihat tahap + inbox; keyboard + skip link + menu sesuai peran. Mobile 390: tanpa scroll horizontal, CTA terlihat, menu drawer staf) | **8 lulus**, diulang 3x stabil | 2026-11-20 |
| Tinjauan screenshot (`frontend/test-results/screens/`) | temuan: sidebar staf tidak setinggi halaman, peran tampil sebagai kunci mentah; keduanya diperbaiki | 2026-11-20 |

| `TestExamProtocolOnRealDB`: start retry = attempt sama; DTO tanpa answerKey/weight/seed/rubric/ID opsi asli; Cache-Control private,no-store; kandidat lain 404; retry opId -> ACK byte-identik; opId sama payload beda 409; revisi basi -> 409 + state server, batch tidak diterapkan; dua tab serentak -> tepat 1 diterima; submit revisi basi 409; retry submit identik -> receipt sama; key lain 409; save setelah final 409; retry ACK lama setelah final tetap dikembalikan; 1 submission; nilai objektif 6/8 + esai pending -> score null; snapshot = revisi committed (rekonsiliasi SQL); review examiner tanpa nama/NIK; nilai > bobot 422; esai 1.5 -> 93.75; examiner tidak bisa rilis; rilis HR -> worker -> interview; regrade wajib alasan; replay event rilis tidak menambah transisi; 1 notifikasi rilis | lulus | 2026-11-20 |
| `TestExamBranchesDeadlineAndExpiry`: semua salah -> ditolak; esai belum dinilai saat rilis -> review; submit membawa batch terakhir dalam satu transaksi; save lewat deadline 409; ACK sebelum deadline tetap bisa diambil ulang; submit lewat deadline -> finalisasi expired + receipt; expiry worker vs submit serentak -> 1 submission, 1 hasil; nilai 1 benar = 12.5; monitor membedakan submitted/expired | lulus | 2026-11-20 |
| Vitest `autosave.test.ts` (10): debounce 800 ms, batas 3 detik saat mengetik terus, navigasi tanpa perubahan tanpa write, serialisasi + penggabungan edit, tersimpan hanya setelah ACK, retry opId+payload sama, konflik eksplisit (pakai milik saya / server), flush sebelum submit, final menolak write, pemulihan draft | lulus | 2026-11-20 |
| Vitest `draftStore.test.ts` (5): kunci user+attempt, TTL, hapus saat finalisasi/logout, kalibrasi offset jam server (jam klien mundur tidak menambah waktu) | lulus | 2026-11-20 |
| Playwright penuh (13 tes, desktop 1440 + mobile 390): ditambah ujian lewat browser (simpan -> offline "hanya di perangkat" -> reconnect "tersimpan di server" -> refresh memulihkan draft -> tandai ragu -> konfirmasi -> bukti penerimaan, refresh menampilkan bukti sama), monitoring staf + rilis memicu transisi event, examiner tanpa identitas, ruang ujian ponsel (drawer navigator, tanpa scroll horizontal, opsi terakhir dapat diklik di atas action bar) | **13 lulus**, 2 kali berturut-turut | 2026-11-20 |
| Run E2E pertama setelah ujian ditambah | 3 gagal: 429 rate limit login (seluruh suite dari satu IP). Solusi: `RATE_LIMIT_SCALE` (default 1, ditolak bila bukan 1 di produksi), E2E memakai 20 | 2026-11-20 |

| **2026-11-21** | | |
| `go test ./...` termasuk integrasi PostgreSQL (`DATABASE_URL_TEST`) dan tool CV sungguhan | lulus setelah migrasi 0009: app 55,6 s, auth, cvparse, cvparse/eval 188,8 s, exam, identity, platform, workflow | 2026-11-21 |
| `TestMFAEnrollmentLoginRecoveryAndReset`, `TestMFAEnforcedForPrivilegedRoles` | lulus | 2026-11-21 |
| `cvworker preflight` (fixture kecil sungguhan: PDF native+render, OCR ind+eng, konversi DOCX) | semua OK; dengan tessdata tanpa `ind` -> GAGAL exit 1 (terdeteksi) | 2026-11-21 |
| Unit `internal/cvparse` (tanggal berpresisi, periode, telepon E.164, URL aman, alias skill, dua kolom, data sensitif + instruksi palsu diabaikan, bukti = baris sama, DOCX: XXE/DOCTYPE, makro, bukan Word, batas terurai, tabel tahun-di-kolom, CFB, deteksi format) | lulus | 2026-11-21 |
| `TestFixtureQuality` (88 fixture, binary sungguhan, ambang email/telepon >=95%/90%, F1 pengalaman >=85%/75%, 0 rekaan, bukti valid) | lulus | 2026-11-21 |
| `TestCVParseReviewApplyFlow`, `TestCVParseProfileChangedAndPartial`, `TestCVParseFormatsAndFailures`, `TestCVParseFencingAndDeletion` | lulus (30,9 s) | 2026-11-21 |
| Vitest (36, termasuk `cv/model.test.ts`), `tsc -b`, `tsc -p tsconfig.e2e.json`, `eslint .` | lulus | 2026-11-21 |
| Playwright penuh (22 tes; termasuk `cv.spec.ts`: unggah -> siap diperiksa -> lihat sumber -> koreksi telepon -> tolak email -> refresh -> simpan -> profil) | **22 lulus**. Run pertama: 2 gagal karena ekspektasi lama (menu "Keamanan akun" sejak MFA, label "Berkas" ambigu) -> test diperbarui | 2026-11-21 |
| `cveval` | 88/88 hasil sesuai; lihat tabel di `docs/cv-parsing.md`; laporan `docs/cv-parsing-eval.json` | 2026-11-21 |
| Backlog 20 unggahan CV (`tests/load/cv-backlog.mjs`) | 20/20 siap diperiksa; antre p95 19,0 s; proses p95 10,7 s; API p95 3,1 -> 3,3 ms | 2026-11-21 |
| Uji beban ujian 500 VU (r1121130857) | 91.446 request, 0 gagal, 0 selisih rekonsiliasi | 2026-11-21 |
| Bug ditemukan saat uji di Chrome | layar kosong: `useEffect` mengembalikan Promise dari `scrollTo` (Chrome baru); Vite dev mati karena EBUSY saat Playwright menulis report. Keduanya diperbaiki | 2026-11-21 |

## Blocker / keterbatasan saat ini


- Race detector Go (`go test -race`) tidak dapat dijalankan: butuh cgo + gcc, tidak terpasang di mesin Windows ini.
- Redis kini tersedia (manual). Antrean CV via Asynq diuji di dev (backlog); jalur tanpa Redis diuji di Playwright. Cache publik + limiter global lintas instance belum diuji dengan dua instance API.
- Pembacaan CV: koreksi perspektif foto, OCR per wilayah, batas memori per proses di Windows, UI staf untuk hasil CV, dan korpus CV nyata belum ada/diuji (detail `docs/cv-parsing.md`).
- Unit systemd (termasuk `ats-cvworker.service`) belum dijalankan di server Linux.

## Langkah konkret berikutnya

1. Uji pembacaan CV dengan sampel CV nyata yang disetujui pemiliknya (tidak di-commit); kalibrasi heading/penanda dari kegagalan nyata.
2. UI staf untuk melihat hasil pembacaan CV dari detail lamaran (API sudah ada).
3. Jalankan unit systemd + preflight CV di server Linux staging; ulangi `cveval` dan backlog di host produksi.
4. Koreksi perspektif foto bila kegagalan nyata menunjukkan kebutuhan.

Reproduksi pembacaan CV: `powershell -File ops\windows\cv-tools.ps1`, isi `CV_*` di `.env`, `powershell -File ops\windows\cv-check.ps1 -Mode all`, `start.ps1`, buka `/akun/profil` -> "Isi profil dari CV".
