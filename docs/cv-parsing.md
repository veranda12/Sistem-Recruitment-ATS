# Pembacaan CV

Kandidat mengunggah CV, sistem membaca dan menampilkan **usulan** isian profil beserta bagian CV yang menjadi buktinya. Profil hanya berubah setelah kandidat menyetujui. Fitur ini tidak memberi skor kecocokan, tidak meranking, dan tidak menyimpulkan kelayakan dari foto, nama, alamat, atau atribut pribadi.

Status uji aktual: bagian [Hasil evaluasi](#hasil-evaluasi) dan `docs/STATUS.md`. Keputusan teknologi: ADR 0008 (`docs/architecture/adr/README.md`).

## Alur

```
Kandidat ──unggah (kind=cv, ≤15 MiB)──▶ documents (karantina) ──outbox──▶ worker: pindai
   │                                                               │ bersih
   └─POST /me/documents/{id}/parse (202, idempoten)                ▼
        cv_parse_runs: waiting_scan ──▶ queued ──(Asynq "cv" / dispatcher PostgreSQL)──▶ cvworker
                                                                                          │ klaim lease + fence
                                          ready_for_review | partial | failed ◀── Process + Extract
   PATCH /review (terima/ubah/tolak) ─▶ POST /apply (1 transaksi, cek versi profil) ─▶ applied
```

| Komponen | Path |
|---|---|
| Parser (format, OCR, tata letak, ekstraksi, normalisasi) | `backend/internal/cvparse/` |
| Run, antrean, worker, API review/apply | `backend/internal/cvreview/` |
| Proses worker CV (+ `preflight`) | `backend/cmd/cvworker/` |
| Migrasi | `backend/db/migrations/0008_cv_parsing.up.sql`, `0009_cv_parse_timing.up.sql` |
| Query | `backend/db/queries/cvparse.sql` |
| UI | `frontend/src/features/candidate/cv/` (`CvImport.tsx`, `CvReview.tsx`, `model.ts`) |
| Fixture + golden | `backend/internal/cvfixture/`, `go run ./cmd/cvfixtures` |
| Evaluasi | `backend/internal/cvparse/eval/`, `go run ./cmd/cveval` |
| Test | `internal/cvparse/*_test.go`, `internal/cvparse/eval/eval_test.go`, `internal/app/cvparse_test.go`, `frontend/src/features/candidate/cv/model.test.ts`, `tests/e2e/cv.spec.ts` |
| Benchmark backlog | `tests/load/cv-backlog.mjs` |

## Jalur per format

| Input | Jalur | Alasan tercatat di manifest |
|---|---|---|
| PDF berteks | `pdftotext -bbox-layout` (kata + koordinat), mutu per halaman, render preview JPEG 110 DPI | `text_layer_good`, `text_layer_sparse_no_images`, `dedupe_duplicate_layer:N` |
| PDF scan / campuran | halaman bermutu rendah atau berupa gambar penuh dirender 300 DPI (PGM) lalu OCR; layer teks di atas gambar dibandingkan kosakatanya dengan OCR; native diutamakan bila cocok | `no_text_layer`, `text_layer_unmapped_glyphs`, `text_layer_low_letter_rate`, `image_page_with_text_layer`, `text_layer_matches_image:0.92`, `text_layer_mismatch_image:0.01`, `blank_page` |
| JPG/PNG/WebP | cek dimensi sebelum decode, orientasi EXIF, cek buram, normalisasi latar (bayangan), OSD rotasi 90/180, deskew terverifikasi, OCR; bila keyakinan rendah coba threshold Sauvola, satu kolom, lalu orientasi alternatif (maks. 3 + 1) | `image_input`, langkah di `attempts`, transformasi di `transforms` |
| DOCX | OOXML dibaca langsung: header, paragraf, tabel (baris data digabung "tahun \| jabatan"; tabel tata letak dibaca per sel), text box, footer; locator `document.xml#t1.r2.c0.p0` | `ooxml_structured`; render ke PDF bila `docx_image_only` atau `docx_many_textboxes_order_ambiguous` |
| DOC (Word 97-2003) | validasi CFB + stream `WordDocument` + FIB 0xA5EC, tolak terenkripsi/bermakro, konversi LibreOffice ke PDF privat lalu jalur PDF | `cfb_worddocument` |

Keputusan OCR tidak hanya dari ada/tidaknya teks: jumlah kata, rasio karakter pengganti/area privat, rasio huruf/angka, kata ganda bertumpuk, gambar halaman (`pdfimages -list`), dan tinta pada preview.

**Urutan baca** (`layout.go`): header/footer berulang dibuang; gutter kolom = pita x terlebar yang (hampir) tidak dilintasi baris; baris yang melintas (nama, heading lebar) memisah wilayah; di tiap wilayah kolom kiri lalu kanan; heading hanya di sisi kiri tanpa heading di kanan = tabel (baris sejajar digabung). Section diingat per kolom dan berlanjut antarhalaman.

**Ekstraksi** (`extract.go`): heading Indonesia/Inggris; entri pengalaman dijangkarkan pada rentang tanggal; jabatan/perusahaan dari penanda (PT, Tbk, Engineer, Staf...) lalu urutan, dengan keraguan "ditentukan dari urutan"; beberapa jabatan di bawah satu perusahaan mewarisi perusahaan induk. Baris data pribadi sensitif (agama, jenis kelamin, status perkawinan, NIK, tanggal lahir, kewarganegaraan) dan teks yang menyuruh "sistem" berbuat sesuatu (beserta baris lanjutannya) diabaikan.

**Normalisasi** (`normalize.go`): tanggal berpresisi `day|month|year` ("2021" tetap tahun, bukan 1 Januari; "Feb 2022 – sekarang" = 2022-02 + current); telepon E.164 dengan `assumedCountry: ID` untuk awalan 0; URL hanya http/https dengan label terpisah dari target, tidak pernah dikunjungi; alias skill eksplisit ("Postgres" → PostgreSQL, "SQL" bukan PostgreSQL). Tidak menebak jabatan dari nama berkas, universitas dari domain email, atau skill dari gelar.

**Saran** menyimpan path stabil, nilai asli, nilai ternormalisasi, ID bukti (baris IR dari versi dokumen yang sama), status review, dan alasan keraguan terpisah: `readDoubts` (pembacaan karakter, mis. OCR) dan `assocDoubts` (pemasangan ke entri, mis. "Ada dua nomor telepon"). UI tidak menampilkan persentase.

## Konsistensi

- Run idempoten per `(kandidat, dokumen, parser_config_version)`; dokumen orang lain = 404 yang sama dengan tidak ada.
- Klaim = lease 45 s + `fence` naik; heartbeat tiap 2 s membawa tahap/halaman; hasil hanya ditulis bila `fence` masih sama **dan** dokumen belum dihapus (job terlambat tidak menghidupkan dokumen terhapus).
- Gagal sementara (timeout, tool crash) = kembali ke antrean dengan backoff 30 s × 2ⁿ (maks. 10 menit); setelah 3 percobaan = dead-letter (`dead_lettered_at`), replay oleh pemegang `cv_parses.replay` (`POST /admin/cv-parses/{id}/replay`). Gagal permanen (rusak, terkunci, buram) = `failed` dengan alasan stabil, tanpa profil kosong "sukses".
- Redis/Asynq hanya sinyal (TaskID = run+fence). Dispatcher (3 s) mengirim ulang sinyal yang hilang, mengambil alih lease kedaluwarsa, dan memproses langsung dari PostgreSQL bila Redis mati.
- Review: `PATCH` dengan `reviewVersion` (optimistic). Apply: satu transaksi mengunci run, cek `profileVersion`; konflik = 409 `profile_changed` tanpa kehilangan review; ulang/klik ganda = hasil sama (`alreadyApplied`), tanpa entri ganda. Entri mirip default "lewati", pilihan "tambah" atau "ganti" (ID entri). Entri lama tidak pernah dihapus karena tidak ada di CV baru.
- Email dari CV hanya menjadi **email kontak**; email akun (login) tidak pernah berubah dari CV.
- Hapus dokumen: run dibatalkan (fence naik), saran/bukti/IR dihapus, direktori preview dihapus, URL preview 404.

## Batas (Limits)

| Batas | Nilai | Perilaku |
|---|---|---|
| Ukuran CV | 15 MiB (dokumen lain 5 MiB) | 413 saat unggah; `too_large` di parser |
| Halaman | 20 | `too_many_pages` (tidak dipotong diam-diam); tepat 20 = diproses (fixture `limit.20pages.pdf`) |
| Gambar | sisi ≤ 12.000 px, ≤ 40 MP, dicek sebelum decode | `image_too_large` |
| Render | ≤ 36 MP per halaman, ≤ 220 MP per dokumen; DPI turun bertahap ke min. 150 | `image_too_large` per halaman |
| DOCX | ≤ 3.000 entri, ≤ 120 MiB terurai, rasio ≤ 150 untuk entri > 1 MiB; DOCTYPE ditolak; makro ditolak | `archive_limits_exceeded`, `macro_not_allowed`, `corrupt_file` |
| Waktu per tahap | info 20 s, teks 40 s, render 60 s, OCR 120 s, konversi 150 s | `stage_timeout` (sementara, dicoba ulang) |
| Keluaran tool | 64 MiB | `archive_limits_exceeded` |
| Percobaan OCR | 3 per halaman (+1 orientasi alternatif) | tercatat di `attempts` |
| Concurrency | `CV_WORKER_CONCURRENCY` (default 2) | antrean `cv` terpisah dari ujian |

Tool dijalankan dengan argumen terpisah (tanpa shell), direktori kerja sementara per job (dihapus saat selesai, timeout, atau crash), nama berkas pengguna tidak pernah dipakai sebagai path, env minimal, `HOME/TEMP` diarahkan ke direktori job, `OMP_THREAD_LIMIT=1`. Timeout mematikan seluruh pohon proses (`taskkill /T` di Windows, process group di Linux). LibreOffice memakai profil sementara dengan `MacroSecurityLevel=3`, `DisableMacrosExecution`, dan tidak memuat tautan eksternal. Di Linux, unit `ops/systemd/ats-cvworker.service` membatasi memori (1,5 GB), CPU (2 core), proses (64), jaringan (hanya localhost), dan filesystem (hanya-baca kecuali `objects/cvparse` dan `cvwork`) untuk worker **dan** semua proses turunannya. Di Windows dev, batas memori/CPU per proses tidak diterapkan (hanya timeout dan concurrency).

## Versi tool yang diuji

| Tool | Versi | Lisensi | Catatan |
|---|---|---|---|
| Poppler (pdftotext, pdfinfo, pdfimages, pdftoppm) | 24.08.0 | GPL-2.0/3.0 | dipanggil sebagai executable terpisah; render OCR memakai PGM (encoder PNG Windows ~11 s/halaman pada 300 DPI) |
| Tesseract | 5.5.0.20241111 | Apache-2.0 | `--oem 1`, TSV via `-c tessedit_create_tsv=1` |
| tessdata_fast ind/eng/osd | commit 87416418, SHA-256 `69786901…` / `7d4322bd…` / `9cf5d576…` | Apache-2.0 | `ops/windows/cv-tools.ps1` mengunduh + verifikasi |
| LibreOffice | 26.2.5.2 | MPL-2.0 | opsional (DOC, DOCX bergambar) |
| Redis (Windows dev) | 8.10.3 msys2, redis-windows/redis-windows | BSD-3/RSALv2/SSPLv1 sesuai upstream | tanpa service; produksi memakai paket Linux |
| Asynq | v0.26.0 | MIT | |

## Pemasangan

### Windows (dev, tanpa Docker)

1. Tesseract 5.5 (installer UB Mannheim) dan Poppler 24.08 (mis. `C:\poppler-24.08.0`).
2. LibreOffice (opsional): `winget install --id TheDocumentFoundation.LibreOffice --exact --version 26.2.5.2`.
3. `powershell -File ops\windows\cv-tools.ps1` → `tools\tessdata` (ind, eng, osd) dan `tools\redis` (terverifikasi SHA-256).
4. Isi `.env` (lihat `.env.example`, bagian "Pembaca CV"): `CV_PDFINFO`, `CV_PDFTOTEXT`, `CV_PDFTOPPM`, `CV_PDFIMAGES`, `CV_TESSERACT`, `CV_TESSDATA`, `CV_SOFFICE`, `CV_WORK_DIR`. Path berspasi diberi tanda kutip.
5. `powershell -File ops\windows\cv-check.ps1 -Mode preflight` harus menampilkan `[OK]` untuk `pdf_native` dan `ocr_ind_eng`.
6. `setup.ps1` membangun `cvworker`; `start.ps1` menjalankannya bersama API dan worker.

### Menjalankan Redis (manual, tidak otomatis saat komputer menyala)

```powershell
powershell -File ops\windows\redis-start.ps1   # Redis 127.0.0.1:6379, data .data\redis, log .data\logs\redis.log
# .env: REDIS_URL=redis://127.0.0.1:6379/0
powershell -File ops\windows\start.ps1         # API + worker + worker CV memakai Asynq
powershell -File ops\windows\stop.ps1
powershell -File ops\windows\redis-stop.ps1    # hentikan Redis
```

Redis tidak dipasang sebagai service dan tidak ada entri startup. Bila Redis mati atau `REDIS_URL` kosong, aplikasi tetap berjalan: API menandai `redis: degraded/disabled`, dan worker CV mengambil job langsung dari PostgreSQL (jalur ini diuji di Playwright). Setelah komputer restart, jalankan `redis-start.ps1` lalu `start.ps1` bila ingin memakai Redis lagi.

### Linux (produksi, systemd)

```bash
apt-get install poppler-utils tesseract-ocr tesseract-ocr-ind tesseract-ocr-eng libreoffice-writer-nogui redis-server
useradd --system --gid ats --home /var/lib/ats/cvwork ats-cv
install -d -o ats-cv -g ats /var/lib/ats/cvwork /var/lib/ats/objects/cvparse
cp ops/systemd/ats-cvworker.service /etc/systemd/system/
sudo -u ats-cv /opt/ats/current/bin/cvworker preflight     # harus lolos
systemctl enable --now ats-cvworker
```

Periksa versi paket distribusi dan ulangi `cveval` di host produksi; angka di dokumen ini dari host Windows dev.

## Troubleshooting

| Gejala | Penyebab | Tindakan |
|---|---|---|
| Preflight `tessdata_ind` gagal | `ind.traineddata` tidak ada di `CV_TESSDATA` | jalankan `cv-tools.ps1` / `apt install tesseract-ocr-ind`. Tesseract tetap berjalan tanpa bahasa Indonesia (hanya peringatan), karena itu worker menolak start |
| OCR selalu 0 kata | config `tsv` tidak ditemukan bila `--tessdata-dir` khusus | sudah diatasi (`-c tessedit_create_tsv=1`); periksa versi Tesseract ≥ 5 |
| Render PDF sangat lambat (≥10 s/halaman) di Windows | encoder PNG Poppler | render memakai PGM/JPEG; pastikan binary terbaru terpasang |
| `.doc` gagal `tool_unavailable` | `CV_SOFFICE` kosong / LibreOffice tidak terpasang | pasang LibreOffice atau minta kandidat menyimpan sebagai PDF/DOCX |
| `.doc` lambat (~8–12 s) | startup LibreOffice per job (profil sementara) | wajar; naikkan concurrency bila backlog |
| Font berbeda pada DOC/DOCX hasil render | substitusi font LibreOffice | pasang font yang umum dipakai CV (Carlito/Caladea, Liberation) di server; teks tetap terbaca karena memakai layer teks PDF hasil konversi |
| Foto `image_too_blurry` | varians Laplacian < 60 | kandidat diminta foto ulang (pesan sudah menjelaskan) |
| Run tertahan `waiting_scan` | pemindai dokumen belum menilai berkas | periksa worker outbox dan `SCAN_COMMAND` |
| Worker CV berhenti saat start | preflight gagal | `cvworker preflight` / log `.data\logs\cvworker.log` |

## Retensi dan proteksi data

- Berkas asli tetap di storage privat sebagai dokumen kandidat (immutable per unggahan; unggah ulang = versi/dokumen baru) sampai kebijakan retensi organisasi menghapusnya. Eksekusi retensi otomatis belum ada di ATS (lihat `docs/handover/known-issues.md`).
- IR (`cv_parse_artifacts`), bukti (`cv_evidence_refs`), saran, dan preview halaman (`objects/cvparse/<run>/f<fence>/`) hanya dapat dibaca pemilik, atau staf dengan `applications.read` pada salah satu lamaran kandidat (dicatat di audit `cv_parse.view`). Dihapus saat dokumen dihapus.
- Log, metrik (`ats_cv_parse_*`), dan trace (`CVPARSE_TRACE=1`) hanya memuat format, tahap, durasi, dan kode alasan; tidak ada teks CV, nama, email, telepon, crop, atau path pribadi. Audit apply hanya berisi nama field dan jumlah entri.
- Default tidak mengirim CV ke layanan eksternal; tidak ada model generatif.

## Hasil evaluasi

Commit kode parser saat evaluasi: lihat `docs/cv-parsing-eval.json` (`parser: cvparse-1`). Mesin: Intel Core i5-10500H 6c/12t, RAM 15,8 GB, Windows 11, SSD; PostgreSQL 17.5 lokal. Perintah: `powershell -File ops\windows\cv-check.ps1 -Mode all` (atau `go run ./cmd/cvfixtures` lalu `go run ./cmd/cveval -fixtures ../tests/cv-fixtures/out -report ../docs/cv-parsing-eval.json`).

**Korpus:** 14 CV sintetis unik (dev 10, holdout 4; semua render satu persona masuk split yang sama), 88 berkas: PDF native 1/2 kolom 38 (termasuk layer teks ganda 4, batas halaman), scan 6 (2 di antaranya dua halaman), campuran 4 (termasuk halaman 2 gagal), foto 9 (miring+bayangan, rotasi 90/180, EXIF 6, buram, WebP, PNG), DOCX 16 (header, tabel, text box, satu berupa gambar tertanam), DOC asli 14 (CFB hasil LibreOffice "MS Word 97"), kasus gagal 7. Golden ditulis per persona, tidak diturunkan dari aturan parser.

| Strata | Field | TP | FP | FN | Precision | Recall | F1 |
|---|---|---|---|---|---|---|---|
| dev / digital | email | 44 | 0 | 0 | 1,000 | 1,000 | 1,000 |
| dev / digital | telepon | 48 | 0 | 0 | 1,000 | 1,000 | 1,000 |
| dev / digital | tuple pengalaman | 77 | 0 | 0 | 1,000 | 1,000 | 1,000 |
| dev / digital | tuple pendidikan | 48 | 0 | 0 | 1,000 | 1,000 | 1,000 |
| dev / digital | skill | 161 | 1 | 1 | 0,994 | 0,994 | 0,994 |
| dev / OCR | email | 13 | 0 | 0 | 1,000 | 1,000 | 1,000 |
| dev / OCR | telepon | 14 | 0 | 0 | 1,000 | 1,000 | 1,000 |
| dev / OCR | tuple pengalaman | 25 | 0 | 2 | 1,000 | 0,926 | 0,962 |
| dev / OCR | skill | 47 | 1 | 6 | 0,979 | 0,887 | 0,931 |
| holdout / digital | email | 21 | 0 | 0 | 1,000 | 1,000 | 1,000 |
| holdout / digital | telepon | 17 | 0 | 0 | 1,000 | 1,000 | 1,000 |
| holdout / digital | tuple pengalaman | 30 | 0 | 0 | 1,000 | 1,000 | 1,000 |
| holdout / OCR | email / telepon | 6 / 5 | 0 | 0 | 1,000 | 1,000 | 1,000 |
| holdout / OCR | tuple pengalaman | 9 | 0 | 0 | 1,000 | 1,000 | 1,000 |

- Per format (dev): tuple pengalaman DOC 17/17, DOCX 19/19, foto 10/10, scan 10/10, campuran 5/7 (2 FN = pengalaman di halaman 2 yang sengaja tidak terbaca; tetap masuk denominator).
- Hasil per berkas sesuai harapan: 88/88 (ok, partial, atau kode gagal yang benar).
- Bukti: 1.874 saran bernilai, semuanya memiliki bukti yang teksnya memuat nilai (0 tanpa bukti, 0 bukti tidak cocok).
- Nilai rekaan pada fixture yang sengaja menghilangkan/menyisipkan field: 0 (NIK, agama, status, tahun lahir, skill "Kubernetes" dan email dari teks instruksi palsu tidak muncul).
- CER OCR (scan + foto terhadap teks native persona yang sama): 0,80% (72/9.006 karakter). Error digit email/telepon pada subset OCR: 0 nilai salah setelah normalisasi; satu kasus "+62" terbaca "t62" dikoreksi dengan tanda "Tanda + mungkin salah baca".
- Target spesifikasi (email/telepon ≥95% digital, ≥90% OCR; F1 pengalaman ≥85% digital, ≥75% OCR): terpenuhi pada sampel ini.

**Batasan validitas:** seluruh fixture dibuat oleh generator yang sama (4 gaya header entri, 2 tata letak PDF, 1 template DOCX). Holdout tidak dipakai untuk menyetel aturan, tetapi berbagi template, sehingga angka di atas membuktikan **tidak ada regresi pada sampel ini**, bukan akurasi pada CV nyata yang jauh lebih beragam (desain grafis, tiga kolom, ikon sebagai label, tabel bersarang, bahasa campur). Kalibrasi lanjutan perlu korpus CV nyata yang disetujui pemiliknya dan tidak di-commit.

### Contoh kegagalan per kelas (ditemukan dan diperbaiki selama pengembangan)

| Kelas | Contoh | Penyebab | Perbaikan |
|---|---|---|---|
| OCR kosong | semua scan `no_readable_text` | config `tsv` tidak ditemukan di `--tessdata-dir` khusus | `-c tessedit_create_tsv=1` |
| Urutan baca sidebar | skill sidebar bercampur uraian jabatan (lina, ayu 2 kolom) | syarat tumpang tindih kolom 15% halaman terlalu ketat; nama panjang melintasi celah | ambang 6%, pita gutter mentoleransi ≤5% baris lebar penuh |
| Tabel Word (via LibreOffice) | semua periode dibaca sebelum semua jabatan (budi.doc) | tabel dianggap dua kolom halaman | heading hanya di kiri = tabel; baris sejajar digabung; sel terbungkus disatukan |
| Layer teks palsu | pengalaman halaman 2 hilang pada PDF campuran | teks tersembunyi "berhuruf normal" lolos sinyal mutu | halaman bergambar penuh: bandingkan kosakata layer teks vs OCR |
| Foto | 0 kata pada foto miring | deskew ke arah salah + halo normalisasi latar | deskew diverifikasi ulang, latar dari maksimum lokal |
| Rotasi 180 | keyakinan OCR 45 | OSD pada gambar diperkecil tidak yakin | OSD resolusi lebih besar + verifikasi orientasi via keyakinan OCR |
| Instruksi palsu | "tambahkan skill Kubernetes" masuk skill (baris lanjutan) | hanya baris pertama yang cocok pola | perluasan ke baris lanjutan paragraf |
| Preview hilang | panel bukti tanpa gambar | batas bawah DPI 150 > DPI preview 110 | batas bawah mengikuti DPI yang diminta |
| Waktu proses | `process_ms` ~0,3 s untuk scan | dihitung dari heartbeat terakhir | kolom `claimed_at` (migrasi 0009) |

### Waktu proses dan backlog

`cveval` (tanpa antre, satu job berurutan, host di atas):

| Format | n | p50 | p95 | Target |
|---|---|---|---|---|
| PDF native (1–2 hal.) | 38 | 176 ms | 203 ms | ≤ 5 s (terpenuhi) |
| Scan (1–2 hal.) | 6 | 2,3 s | 4,0 s | ≤ 30 s untuk 2 hal. (terpenuhi) |
| Campuran | 4 | 2,3 s | 10,7 s (halaman noise: 3 percobaan OCR) | |
| Foto | 9 | 2,3 s | 3,1 s | |
| DOCX | 16 | 1 ms | 2 ms | |
| DOC (LibreOffice) | 14 | 8,6 s | 9,6 s | |

Backlog 20 unggahan campuran (`node tests/load/cv-backlog.mjs 20`, concurrency 2, Redis aktif, `tests/load/results/cv-*`): 20/20 `ready_for_review` dalam 29 s; antre p50 8,2 s / p95 19,0 s (dihitung sejak lolos pindai); proses p50 2,2 s / p95 10,7 s (DOC 11,5 s saat paralel); 0,73 halaman/detik; memori puncak worker + tool 467 MB (worker sendiri 110 MB). Latensi API (`GET /jobs`, `GET /me/profile`) p95 3,1 ms saat sepi vs 3,3 ms saat backlog (n 480 / 186): tidak ada dampak terukur pada jalur API. Sampler CPU hanya menangkap proses yang hidup saat sampel (Tesseract berumur pendek terlewat), jadi angka CPU tidak dilaporkan sebagai akurat. Target kapasitas ujian ATS tidak berlaku untuk OCR; untuk lonjakan unggahan naikkan `CV_WORKER_CONCURRENCY` sesuai core (OCR memakai 1 thread/job) dan memori (~200 MB/job OCR).

## Belum tersedia / belum teruji

- Koreksi perspektif foto (kertas trapesium) belum diimplementasikan; foto miring dan berputar ditangani.
- OCR per wilayah (region) dalam halaman native: saat ini satu halaman memakai native, OCR, atau gabungan seluruh halaman (baris OCR yang tidak bertumpuk dengan native ditambahkan).
- Adapter model layout/LLM lokal opsional tidak dibuat (baseline memenuhi target pada fixture; tanpa bukti manfaat).
- Batas memori/CPU per proses di Windows dev (Job Object) belum diterapkan; di Linux lewat systemd.
- UI staf untuk melihat hasil pembacaan CV kandidat belum ada (API `GET /cv-parses/{id}` berscope dan teraudit sudah ada).
- Unit systemd `ats-cvworker.service` dan paket Linux belum dijalankan di server sungguhan (tidak ada host Linux).
- Korpus CV nyata belum diuji (lihat batasan validitas).
