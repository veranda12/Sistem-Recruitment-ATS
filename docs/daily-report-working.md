# Daily report working

Catatan kerja bersama untuk semua sesi dan agent yang mengerjakan repo ats. Setiap pengerjaan punya bagian sendiri, berurutan dari atas ke bawah. Pengerjaan 1 ditulis oleh sesi Claude pada 2026-11-27. Pengerjaan 2 dan seterusnya diisi sesi atau agent berikutnya.

## Indeks

| No | Pengerjaan | Status | Tanggal | Dikerjakan oleh |
|---|---|---|---|---|
| 1 | Paket 1 BRD: track, field PTK, batas kuota, template workflow default | Selesai di kode dan diuji. Review: 7 dari 7 temuan diperbaiki. Di-commit di branch `feat/paket-1-track-ptk`, belum di-merge ke `main` | 2026-11-27 | Claude (sesi ini) |
| 2 | (belum diisi) | | | |

## Cara menambah pengerjaan

- Tambahkan bagian baru di bawah pengerjaan terakhir dengan judul `## Pengerjaan N: <apa yang dikerjakan> (<tanggal>, <sesi atau agent>)`.
- Isi minimal: status, file baru dan yang diubah, migrasi atau endpoint baru, hasil verifikasi yang benar-benar dijalankan, keputusan atau deviasi, dan yang belum selesai.
- Tambahkan barisnya di indeks. Jangan mengubah atau menghapus bagian pengerjaan lain.
- Jangan tulis kata sandi, DSN, atau kunci ke file ini.
- Sebelum menulis migrasi, cek `backend/db/migrations`. Per 2026-11-27 migrasi terakhir adalah `0016`.

---

## Pengerjaan 1: Paket 1 BRD, track, field PTK, batas kuota, template workflow default

Dikerjakan: 2026-11-27, Claude (sesi ini). Sumber: `docs/proposal/Prompt-Paket-Ringan-BRD.md`, bagian Paket 1.

### Status

Selesai di kode dan diuji, termasuk perbaikan setelah code review. Di-commit di branch `feat/paket-1-track-ptk` (hash ada di `git log`), belum di-merge ke `main`. Langkah berikutnya: pemilik menjalankan `check-all.ps1` dan `/code-review` pada branch ini.

### Kronologi

1. Membaca prompt Paket 1 dan Bagian A. Eksplorasi repo: migrasi terakhir 0015, tabel `requisitions` dan `job_openings`, rute, handler, pola test, dan Lampiran B BRD.
2. Rencana disusun. Pemilik menyetujui lima usulan. Usulan 2 diubah (kuota baris lama). Tahap MCU dan Offering/PKWT dari Lampiran B diminta ditambahkan ke template.
3. Implementasi berurutan: migrasi `0016`, query dan `sqlc generate`, handler dan router, seed, test Go, OpenAPI dan `gen:api`, frontend, E2E, lalu dokumen.
4. Koreksi saat implementasi: kunci workflow harus huruf kecil dan tanda hubung karena constraint database. Kunci template karena itu memakai `track-...`.
5. Verifikasi dan uji upgrade (lihat bagian hasil).
6. Pembaruan `docs/STATUS.md` dan `docs/product/modules.md`.

### Yang dibuat dan diubah

| File | Jenis | Isi |
|---|---|---|
| `backend/db/migrations/0016_track_ptk.up.sql` | Baru | Track, field PTK, kuota, backfill, FK komposit |
| `backend/db/queries/requisitions.sql` | Diubah | `CreateRequisition` dengan kolom baru |
| `backend/db/queries/recruitment.sql` | Diubah | Filter track, kolom baru, `SumActiveJobQuota`, `UpdateJobQuota` |
| `backend/db/sqlc/models.go`, `recruitment.sql.go`, `requisitions.sql.go` | Generate | Hasil `sqlc generate`, jangan diedit manual |
| `backend/internal/recruitment/requisitions.go` | Diubah | Validasi track, alasan, line, tanggal; cek tanggal saat submit; filter `unitCode` pada daftar |
| `backend/internal/recruitment/jobs.go` | Diubah | Track warisan, kuota di transaksi, `UpdateJobQuota`, filter track publik |
| `backend/internal/recruitment/applications.go` | Diubah | Filter `track` pada daftar lamaran staf |
| `backend/internal/app/router.go` | Diubah | Satu rute: `PATCH /staff/jobs/:id` |
| `backend/internal/app/seed.go` | Diubah | Tiga lowongan contoh, track dan kuota lowongan lama, pemanggilan template |
| `backend/internal/app/seed_track.go` | Baru | Tiga template workflow: Operator, Staff, MT S1/D4 |
| `backend/internal/app/track_test.go` | Baru | 20 test: 14 dari Paket 1, 2 dari perbaikan (PATCH kuota sama, batas line), 4 untuk filter `unitCode` |
| `backend/internal/app/interviews_test.go` | Diubah | Hanya field baru di empat pemanggilan, asersi tidak berubah |
| `contracts/openapi.yaml` | Diubah | Rute, field, schema `Track` dan `ReasonCategory`, `PATCH` |
| `frontend/src/lib/api-types.ts` | Generate | Hasil `npm run gen:api`, +96 baris, tidak ada yang hilang |
| `frontend/src/lib/format.ts` | Diubah | Label track dan alasan |
| `frontend/src/features/backoffice/jobs.tsx` | Diubah | Ditulis ulang: track, kuota, pilih permintaan, ubah kuota |
| `frontend/src/features/backoffice/interviews.tsx` | Diubah | Form dan daftar permintaan: track, alasan, line, target tanggal |
| `frontend/src/features/backoffice/applications.tsx` | Diubah | Filter track |
| `frontend/src/features/career/routes.tsx` | Diubah | Filter track di URL, label track di kartu dan detail |
| `tests/e2e/track.spec.ts` | Baru | 5 test E2E: tiga dari rencana awal, dua dari perbaikan |
| `tests/demo/demo-data.mjs` | Diubah | Track dan alasan pada dua permintaan demo |
| `docs/product/modules.md` | Diubah | Layar, API, data, validasi, dan tes |
| `docs/STATUS.md` | Diubah | Bagian hasil uji 2026-11-27 dan 2026-11-28, migrasi terakhir |
| `docs/daily-report-working.md` | Baru | Indeks dan bagian pengerjaan (dokumen ini) |

Total: 21 file lacak diubah, +864 dan -97 baris (termasuk file generate dan perbaikan setelah review). File baru: empat file kode dan tes (781 baris) serta laporan ini.

### Endpoint

| Rute | Perubahan |
|---|---|
| `PATCH /staff/jobs/{id}` | Baru. Hanya kuota, dengan `expectedVersion` dan `quota`. 200; 404 di luar scope; 409 jika versi basi; 422 jika lowongan closed atau kuota melebihi kebutuhan. Dicatat di audit `job.quota_update`. |
| `POST /requisitions` | `track` dan `reasonCategory` wajib. `line` (maks 100) dan `targetJoinDate` opsional. |
| `POST /requisitions/{id}/submit` | 422 jika `targetJoinDate` sudah lewat, dihitung dari tanggal hari ini di zona organisasi. |
| `POST /staff/jobs` | `quota` wajib, minimal 1. `track` wajib bila `requisitionId` kosong. Bila diisi, track diwarisi; nilai berbeda dapat 422. |
| `GET /jobs?track=` | Baru. Allowlist; nilai lain 422. |
| `GET /applications?track=` | Baru. Allowlist; nilai lain 422. |
| `GET /jobs` dan `GET /jobs/{slug}` | Respons menambah `track`. |
| `GET /staff/jobs` | Respons menambah `track` dan `quota`. |
| `GET /requisitions` | Respons menambah `track`, `line`, `targetJoinDate`, dan `reasonCategory`. |
| `GET /requisitions?unitCode=` | Baru (perbaikan review). Filter kode unit, dicocokkan lewat ID unit; scope tetap berlaku. Format salah atau kode tidak dikenal: 422. |

Rute yang tidak disebut di atas tidak berubah.

### Test

| # | Test | Yang diperiksa | Hasil |
|---|---|---|---|
| 1 | `TestTrackRequisitionCreateRead` | Tiga requisition tersimpan; GET /requisitions dan DB memuat track | lulus |
| 2 | `TestTrackValidation` | `MANAGER`, kosong, dan `staff` menghasilkan 422 dengan field `track`, di requisition dan lowongan | lulus |
| 3 | `TestJobInheritsRequisitionTrack` | Track STAFF pada requisition MT menghasilkan 422; track sama menghasilkan 201; tanpa track diwarisi MT | lulus |
| 4 | `TestPublicJobsTrackFilter` | `?track=OPERATOR` hanya published OPERATOR, dicocokkan dengan DB; `XYZ` menghasilkan 422; tanpa filter tidak berubah | lulus |
| 5 | `TestDraftJobTrackHiddenFromPublic` | Draft tidak muncul di daftar dan `/jobs/{slug}` menghasilkan 404 | lulus |
| 6 | `TestRequisitionTargetJoinDate` | Submit tanggal kemarin menghasilkan 422 dan status tetap draft; besok dan kosong menghasilkan 200 | lulus |
| 7 | `TestReasonCategoryEnum` | Di luar enum dan kosong menghasilkan 422; tiga nilai valid menghasilkan 201 | lulus |
| 8 | `TestJobQuotaWithinHeadcount` | Headcount 10: 6 menghasilkan 201, 5 menghasilkan 422, 4 menghasilkan 201; ubah ke 7 menghasilkan 422 dan DB tetap; versi basi menghasilkan 409 | lulus |
| 9 | `TestJobQuotaRace` | Dua create paralel kuota 6: tepat satu 201, satu 422, satu baris di DB | lulus |
| 10 | `TestClosedJobExcludedFromQuota` | Lowongan closed tidak dihitung; ubah kuota closed menghasilkan 422 | lulus |
| 11 | `TestTrackScopeNotFound` | HR TI dan recruiter TI tidak melihat data OPS; PATCH, approve, dan submit di luar scope menghasilkan 404 | lulus |
| 12 | `TestTrackQuotaAudit` | Audit `requisition.create`, `job.create`, dan `job.quota_update` memuat track, kuota, dan nilai lama-baru | lulus |
| 13 | `TestSeedWorkflowTemplates` | Tiga template lolos `workflow.Validate`; MT tepat tiga interview berurutan; Staff tanpa cek fisik | lulus |
| 14 | `TestCandidateAppliesTwoTracks` | Melamar OPERATOR dan MT menghasilkan dua lamaran dengan `workflow_version_id` berbeda | lulus |
| E2E-1 | `track.spec.ts`: portal | Filter track di URL, desktop: lowongan OPERATOR tampil, Backend Engineer tidak | lulus |
| E2E-2 | `track.spec.ts`: ponsel | Filter MT pada 390 px tanpa scroll horizontal | lulus |
| E2E-3 | `track.spec.ts`: alur staf | HM membuat permintaan MT, HR menyetujui, lowongan dibuat dari permintaan itu, kuota melebihi kebutuhan ditolak | lulus |

Di luar daftar paket, test menambah pengecekan: 409 pada versi basi, PATCH lowongan closed, dan urutan workflow pertama. Test lama lulus tanpa asersi yang diubah.

### Hasil verifikasi

Putaran pertama, saat Paket 1 selesai. Semua hasil berikut dari perintah yang dijalankan langsung. Hasil putaran kedua, setelah code review, ada di bagian Perbaikan.

| Pemeriksaan | Perintah | Hasil |
|---|---|---|
| Format dan statis Go | `gofmt -l .`, `go vet ./...`, `go build ./...`, `sqlc diff` | Kosong; exit 0; exit 0; bersih |
| Test Go lengkap | `go test ./... -count=1 -p 1` dengan `ats_test` | Exit 0. `internal/app` 69,7 detik; paket lain `ok` |
| Race | `go test -race` untuk tiga test kuota dan scope | PASS, tanpa DATA RACE |
| Kontrak dan tipe | `npm run gen:api`, `typecheck`, `lint` | Exit 0 semua |
| Unit frontend | `npm test` (Vitest) | 36 passed, 4 file |
| Build frontend | `npm run build` | Exit 0 |
| E2E paket | `npx playwright test track.spec.ts` | 3 passed |
| E2E penuh | `npx playwright test` dengan `E2E_DATABASE_URL` ke `ats_test` | 33 passed (desktop 30, mobile 3) |

### Uji upgrade data lama

Skema HEAD (0001–0015) dibangun dan di-seed, lalu lima requisition dan lowongan uji ditambahkan. Setelah migrasi 0016: versi 16, tidak dirty.

| Lowongan | Status | Kuota | Kebutuhan requisition | Alasan |
|---|---|---|---|---|
| `uji-r1-satu` | published | 3 | 3 | Satu-satunya lowongan aktif |
| `uji-r2-draft` | draft | 1 | 5 | Ada dua lowongan aktif di requisition |
| `uji-r2-pub` | published | 1 | 5 | Ada dua lowongan aktif di requisition |
| `uji-r3-draft` | draft | 2 | 2 | Satu-satunya aktif; lowongan closed tidak dihitung |
| `uji-r3-tutup` | closed | 1 | 2 | Closed, jadi 1 |

Pada uji ini, lima lowongan seed lama mendapat backfill `STAFF` dan kuota 1. Seed baru memberi Operator Produksi dan Admin Gudang track OPERATOR. Semua requisition lama mendapat track `STAFF` dan alasan kosong. Database uji sementara `ats_mig_req_test` sudah dihapus.

### Keputusan dan deviasi

1. Kunci workflow memakai tanda hubung: `track-operator`, `track-staff`, `track-mt-s1-d4`. Constraint `workflows.key` hanya menerima `[a-z0-9-]`. Kunci tahap tetap memakai garis bawah.
2. Template Staff memuat "MCU (bila perlu)", karena Lampiran B mencantumkannya di alur Staff. Test 13 hanya memeriksa tidak ada tahap "Cek Fisik". Untuk menghapusnya, hapus satu tahap di `seed_track.go`.
3. MCU memakai tipe `screening`, Offering memakai `decision`. Engine tidak memberi perilaku khusus pada keduanya. Modul MCU dan offering (dokumen, data, penawaran) belum dibuat.
4. `PATCH` memakai `expectedVersion` sesuai keputusan pemilik. Publish dan close yang sudah ada memakai `version`, sehingga keduanya tidak seragam. Yang lama tidak diubah.
5. `PATCH` mengembalikan 404 di luar scope. Publish dan close yang ada tetap 403 dan tidak diubah.
6. Ada tambahan UI "Ubah kuota" di halaman Lowongan, agar kuota bisa diubah dari antarmuka. Edit penuh tetap di luar scope.
7. Pengecekan requisition di `CreateJob` dipindah ke dalam transaksi yang mengunci requisition, agar test paralel benar. Pengguna tanpa izin unit sekarang mendapat 403 sebelum 422.
8. Lima lowongan lama tetap di workflow standar karena E2E bergantung padanya. Operator Produksi dan Admin Gudang mendapat track OPERATOR, sisanya STAFF.
9. E2E memakai `E2E_DATABASE_URL` ke `ats_test`, bukan default `ats_e2e`, sesuai aturan database `_test`.
10. Tiga lowongan contoh baru: Operator Forklift Gudang, Staf Perencanaan Produksi, dan Management Trainee Teknologi.
11. Commit dibuat di branch `feat/paket-1-track-ptk`, bukan langsung di `main`, sesuai aturan git sesi ini. Merge ke `main` menunggu keputusan pemilik.

### Aturan untuk sesi dan agent lain

| Area | Aturan atau risiko |
|---|---|
| Migrasi | Jangan ubah 0001–0016. Migrasi berikutnya 0017. |
| Test lama | Hanya boleh menambah field. Asersi dan logika tidak diubah. |
| Database uji | `ats_test` sudah di-reset dan berisi skema 0016 serta seed Paket 1. Setiap test dan E2E me-reset ulang. Jangan pakai database tanpa akhiran `_test`. |
| Kredensial | DSN PostgreSQL dikirim lewat environment per perintah. Jangan tulis DSN atau kata sandi ke file. |
| Binari E2E | Setelah mengubah Go, bangun ulang `backend/bin` dengan `go build -o bin/ ./cmd/...` sebelum Playwright. |
| File generate | `backend/db/sqlc/*` dan `frontend/src/lib/api-types.ts` dihasilkan. Ubah sumbernya, lalu generate ulang. Jangan diedit manual. |
| Rawan konflik | `contracts/openapi.yaml`, `backend/internal/app/router.go`, `backend/internal/app/seed.go`, `backend/internal/recruitment/jobs.go`, `docs/STATUS.md`, `docs/product/modules.md`. Paket 2 sampai 5 juga menyentuh openapi dan router. |
| Bukan milik Paket 1 | `docs/proposal/build.mjs`, `docs/proposal/screenshots.mjs`, `ops/windows/demo.ps1`, dan tiga file untracked di `docs/` dan `docs/proposal/` sudah ada sebelum sesi ini. Jangan ikut di-commit. |
| Line ending | `frontend/src/features/backoffice/interviews.tsx` tetap CRLF di working copy. Git menormalisasi ke LF sesuai `.gitattributes`, jadi isi commit tidak terpengaruh. |
| Commit | Paket 1 ada di branch `feat/paket-1-track-ptk`. Jangan ikut commit file di "Bukan milik Paket 1". |

### Belum selesai dan keterbatasan

- Belum di-merge ke `main`. `check-all.ps1` dan `/code-review` belum dijalankan oleh sesi ini. Perintah intinya sudah.
- Backfill diuji dengan data uji dan seed, bukan data produksi.
- `ops\windows\test-upgrade.ps1` belum dijalankan. Yang dijalankan adalah varian manual dengan pola yang sama.
- `docs/architecture/erd.md` belum diperbarui karena tidak disebut di paket.
- Tidak ada test case Paket 1 yang belum ditulis.
- Versi laporan di Claude Docs (tautan privat) sudah tidak dipakai. File ini yang berlaku.

### Perbaikan setelah code review

Sumber: temuan `/code-review` atas Paket 1, dikerjakan sesi ini pada 2026-11-27 sampai 2026-11-28 sesuai instruksi pemilik. Hanya temuan berikut yang diperbaiki. Perilaku lain tidak diubah.

| # | Temuan | Status | Perbaikan |
|---|---|---|---|
| 1 | Form "Buat lowongan" tidak di-reset | Selesai | Form kembali ke nilai awal saat dialog dibuka dan setelah lowongan berhasil dibuat (`jobs.tsx`). |
| 2 | Ganti unit tidak membuang requisition yang tidak cocok; field Track tetap terkunci | Selesai | Daftar requisition dimuat server lewat `unitCode`. Saat unit diganti, requisition yang dipilih dibuang beserta track warisannya, dan field Track terbuka lagi (`gantiUnit`). |
| 3 | Requisition dicocokkan dengan unit memakai nama, bukan ID | Selesai, opsi C | `GET /requisitions` menerima filter `unitCode`. Server mencocokkan lewat ID unit, dan scope tetap berlaku. Klien tidak lagi membandingkan nama unit. |
| 4 | Simpan kuota sama tetap menaikkan versi dan menulis audit | Selesai | `UpdateJobQuota` mengembalikan 200 dengan data lowongan apa adanya bila kuota sama. Tidak ada versi baru dan tidak ada audit. |
| 5 | Ubah kuota tidak memeriksa org dan status requisition | Selesai | Pemeriksaan requisition dipindah ke `kunciRequisition`, dipakai oleh pembuatan dan ubah kuota. |
| 6 | Batas 100 karakter `line` dihitung sebelum TrimSpace | Selesai | Batas dihitung setelah TrimSpace, sama dengan nilai yang disimpan (`requisitions.go`). |
| 7 | Pengecekan kuota ditulis ganda | Selesai | `cekKuotaRequisition` dipakai pembuatan dan ubah kuota, dengan pesan error yang sama. |

Test tambahan:

- `TestPatchKuotaSamaTanpaPerubahan`: kuota sama menghasilkan 200, versi tetap 1, dan tidak ada audit `job.quota_update`. Lulus.
- `TestRequisitionLineTrimSebelumBatas`: line 98 karakter dengan spasi di awal dan akhir diterima (201) dan tersimpan tanpa spasi. Line 101 karakter ditolak (422). Lulus.
- `TestFilterUnitCodeHanyaUnitTerpilih` (a): filter `unitCode` hanya mengembalikan requisition unit tersebut. Lulus.
- `TestFilterUnitCodeTidakValid422` (b): format salah (`ops`, `TI!`, `A`, 21 karakter) dan kode tidak dikenal (`XYZ`) menghasilkan 422. Lulus.
- `TestFilterUnitCodeTidakMembukaScope` (c): recruiter OPS dengan `unitCode=TI` mendapat daftar kosong. `unitCode=OPS` tetap menampilkan requisition OPS. Lulus.
- `TestFilterUnitCodeNamaSamaKodeBeda` (d): unit kedua bernama sama, `OPS2`, dibuat di test. Filter `OPS` dan `OPS2` tidak tercampur. Lulus.
- E2E `form lowongan kosong lagi setelah lowongan dibuat`: lulus.
- E2E `ganti unit membuang permintaan tenaga kerja yang tidak lagi cocok`: lulus.

Hasil verifikasi setelah perbaikan (putaran kedua):

- `gofmt -l .` kosong. `go vet ./...`, `go build ./...`, dan `sqlc diff` exit 0.
- `go test ./... -count=1 -p 1` exit 0. `ok` untuk `internal/app`, `auth`, `cvparse`, `cvparse/eval`, `exam`, `identity`, `notify`, `platform`, `workflow`.
- `npm run gen:api`, `typecheck`, dan `lint` exit 0. `npm test`: 36 passed. `npm run build` exit 0.
- Playwright `track.spec.ts`: 5 passed. Suite penuh: 35 passed (desktop 32, mobile 3).
- `go test -race` tidak dijalankan ulang di putaran ini. Jalur kuota tidak berubah; hasil race putaran pertama tetap berlaku.
- Hasil putaran ini juga dicatat di `docs/STATUS.md` pada bagian 2026-11-28.

Catatan selama perbaikan:

- Pembuatan OpenAPI pertama gagal karena ringkasan PATCH mengandung `: `. Sudah diperbaiki sebelum verifikasi.
- E2E form kosong pertama gagal karena test tidak memilih track untuk lowongan tanpa permintaan. Yang salah adalah test, bukan aplikasi. Sudah diperbaiki.
- Pada putaran kedua, E2E menunggu opsi requisition muncul sebelum memilihnya, karena daftar sekarang dimuat dari server setelah unit dipilih.
- Diagnostik editor sempat menunjukkan `UnitCode` tidak dikenal. `go build ./...` lulus dan field itu ada di struct hasil `sqlc generate`, jadi diagnostik itu tidak dianggap error.
- Di laporan sebelumnya, blok `go vet ./...` tampil bercampur dengan log test. Yang benar: vet tidak menghasilkan output dan exit 0. Output asli dijalankan ulang di putaran ini.

Keputusan temuan 3: opsi C (dipilih pemilik). Implementasi:

- `GET /requisitions` menerima `unitCode` (opsional). Format salah menghasilkan 422 pada `unitCode`. Kode yang tidak ada di organisasi juga 422. Unit nonaktif tetap bisa difilter agar riwayat requisition bisa dicari.
- Filter ditambahkan sebagai `AND` di query yang sudah memuat scope, sehingga tidak bisa memperluas akses.
- Form lowongan hanya memuat permintaan untuk unit yang dipilih. Field unit harus dipilih dulu (`Pilih unit dulu.`).
- `gantiUnit` tidak lagi membandingkan nama. Mengganti unit membuang permintaan yang dipilih beserta track warisannya. Track yang dipilih manual tetap.
- OpenAPI: parameter `unitCode` dan respons 422 pada `GET /requisitions`. `gen:api` dijalankan.

---

## Pengerjaan 2: (belum diisi)

_Sesi atau agent yang mengerjakan pengerjaan berikutnya mengganti judul ini dengan judul pengerjaannya, mengisi indeks, dan menulis bagiannya di sini._
