# Architecture Decision Records

| ID | Keputusan | Status |
|---|---|---|
| 0001 | Modular monolith Go + Gin, proses api/worker terpisah | Diterima |
| 0002 | sqlc + pgxpool, migrasi golang-migrate tanpa down destruktif | Diterima |
| 0003 | Outbox PostgreSQL dikonsumsi worker langsung; Asynq ditunda | Diterima |
| 0004 | Redis opsional, mode degraded eksplisit | Diterima |
| 0005 | Sesi opaque di PostgreSQL + CSRF terikat sesi | Diterima |
| 0006 | NIK: AES-256-GCM + blind index HMAC-SHA256 | Diterima |
| 0007 | Definisi workflow JSONB berversi immutable + stage dimaterialisasi | Diterima |
| 0008 | Pembaca CV: Poppler + Tesseract + LibreOffice lokal, parser deterministik Go, antrean CV PostgreSQL + Asynq | Diterima |

## 0001 Modular monolith
Satu binary per peran (`cmd/api`, `cmd/worker`) dari satu modul Go. Batas modul dijaga lewat paket `internal/<modul>`. Alasan: tim pelanggan kecil, deploy on-premise, satu artefak lebih mudah diserahkan.

## 0002 sqlc + pgxpool
SQL eksplisit di `backend/db/queries`, dihasilkan ke `backend/db/sqlc`. Migrasi `NNNN_nama.up.sql`; rilis berisiko memakai expand/backfill/contract. Tidak ada down migration destruktif otomatis.

## 0003 Outbox tanpa Asynq (untuk saat ini)
Prompt menyarankan Asynq di atas Redis. Lingkungan dev Windows tidak menyediakan Redis, dan kegagalan Redis tidak boleh membatalkan pekerjaan yang sudah diakui. Worker mengonsumsi tabel `outbox` langsung dengan `FOR UPDATE SKIP LOCKED`, `available_at` untuk backoff, `attempts` berbatas, status `dead` untuk dead-letter, dan tabel `processed_events(consumer, event_id)` untuk deduplikasi. Konsekuensi: latensi dispatch dibatasi interval polling (default 1 detik). Asynq dapat ditambahkan kemudian sebagai lapisan dispatch tanpa mengubah kontrak outbox. **Perlu konfirmasi pelanggan** bila volume event diperkirakan sangat tinggi.

## 0004 Redis opsional
`REDIS_URL` kosong atau tidak terjangkau: `/readyz` melaporkan `redis: degraded`, cache publik mati, limiter lokal per instance dengan kuota lebih ketat. Login/registrasi/reset tetap melayani dengan limiter lokal. Ekspor PII menolak (503) bila limiter global tidak tersedia dan `STRICT_SENSITIVE_LIMITS=true`.

## 0005 Sesi
Token sesi 32 byte acak di cookie `ats_session` (HttpOnly, SameSite=Lax, Secure di produksi). DB menyimpan SHA-256 digest. Idle expiry dapat diatur (default 30 menit staf, 120 menit kandidat), absolute 12 jam. Rotasi saat login, revoke saat logout/reset sandi. Token CSRF = HMAC(session token, CSRF_KEY) dikirim via header `X-CSRF-Token`; mutasi juga memeriksa `Origin` terhadap `PUBLIC_ORIGIN`.

## 0006 NIK
Normalisasi: buang spasi/titik/strip, wajib tepat 16 digit ASCII. Ciphertext AES-256-GCM, nonce 12 byte acak per baris, `key_version`. Blind index HMAC-SHA256 dengan kunci terpisah + `blind_key_version`. `UNIQUE(org_id, nik_blind_index)` mencegah dua master dari registrasi simultan. Rotasi kunci pencocokan: kolom `nik_blind_index_next`, dual-write, backfill tervalidasi dengan unique index, cutover, lalu contract.

## 0007 Workflow
`workflows.draft_definition` dapat diubah. Publish memvalidasi graph lalu menulis `workflow_versions.definition` (immutable, dijaga trigger) dan memateri `workflow_stages`. Lamaran menyimpan `workflow_version_id` saat apply. Rule DSL adalah AST JSON dengan field/operator allowlist; tidak ada eval/SQL/URL.

## 0008 Pembaca CV lokal dan deterministik
**Konteks:** kandidat mengunggah CV PDF/foto/Word; hasil hanya boleh menjadi saran berbukti, tanpa API berbayar dan tanpa mengirim CV ke layanan luar.
**Keputusan:**
- Ekstraksi teks + koordinat: Poppler 24.08 (`pdftotext -bbox-layout`, `pdfinfo`, `pdfimages -list`, `pdftoppm`). Lisensi GPL-2.0/3.0; dipanggil sebagai executable terpisah (tidak ditautkan ke binary ATS), sehingga distribusi buyout ATS tidak menjadi karya turunan. Biner Poppler dipasang dari paket OS (Linux) atau build Windows terpisah.
- OCR: Tesseract 5.5 (Apache-2.0) dengan `tessdata_fast` ind/eng/osd (Apache-2.0) yang dikunci ke commit `87416418` + SHA-256.
- Word lawas dan DOCX bergambar/ambigu: LibreOffice headless 26.2 (MPL-2.0), profil sementara per job, makro dimatikan. DOCX terstruktur dibaca langsung di Go (archive/zip + encoding/xml) tanpa konverter.
- Interpretasi: baseline deterministik di Go (urutan baca berbasis gutter, section ID/EN, entri berbasis jangkar tanggal). Tidak ada model generatif di jalur utama; adapter model lokal opsional belum dibuat karena baseline sudah memenuhi target pada fixture (lihat docs/cv-parsing.md), dan manfaatnya belum terbukti.
- Antrean: PostgreSQL `cv_parse_runs` adalah sumber kebenaran (lease, fencing token, retry/backoff, dead-letter). Asynq (Redis) hanya sinyal pada antrean `cv`; dispatcher memulihkan sinyal hilang dan memproses langsung saat Redis mati. Worker CV adalah proses terpisah (`cmd/cvworker`) dari worker ujian/outbox.
**Alternatif ditolak:** layanan OCR/parsing cloud (biaya, data pribadi keluar), rewrite backend ke Python (di luar batas), Asynq sebagai sumber kebenaran (Redis opsional di arsitektur ATS).
**Konsekuensi:** host produksi wajib memasang Poppler + Tesseract (+ LibreOffice bila .doc didukung); `cvworker` menolak start bila preflight gagal.
