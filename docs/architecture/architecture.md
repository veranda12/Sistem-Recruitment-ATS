# Arsitektur Sistem Rekrutmen

Dokumen ini menjelaskan bentuk sistem, batas modul, dan alasan pilihan teknis. Keputusan individual dicatat di `docs/architecture/adr/`.

## Ringkasan

- **Bentuk:** modular monolith Go (Gin) dalam satu repo, dua proses yang bisa diskalakan terpisah: `api` dan `worker`. Proses `seed` dan `migrate` adalah alat operator.
- **Frontend:** satu aplikasi React 19 + Vite + TypeScript strict. Bundle dipisah per area melalui lazy route: `career` (publik), `candidate` (ruang kandidat), `exam` (ruang ujian), `backoffice` (staf).
- **Data:** PostgreSQL 17 sebagai sumber kebenaran. Akses via `pgxpool` + query bernama `sqlc`. Skema via `golang-migrate` (file SQL berurutan, tanpa down migration destruktif otomatis).
- **Redis (opsional):** rate limiting lintas instance dan cache daftar lowongan publik. Bila Redis tidak tersedia, API masuk mode degraded: cache mati, limiter lokal konservatif aktif. Redis tidak pernah berada di jalur durability.
- **Pekerjaan asinkron:** transactional outbox di PostgreSQL. Worker mengklaim baris outbox dengan `FOR UPDATE SKIP LOCKED`, retry berbatas dengan backoff + jitter, dead-letter, dan deduplikasi per consumer berdasarkan event ID. Lihat ADR-0003.
- **Berkas:** bucket privat S3-compatible (MinIO). Di dev tanpa MinIO, adaptor filesystem privat di `.data/objects` dengan kontrak yang sama.
- **Edge:** Nginx melayani aset frontend dan meneruskan `/api` ke API pada origin yang sama.

## Batas modul (backend/internal)

| Modul | Tanggung jawab | Boleh bergantung pada |
|---|---|---|
| platform | config, logger, db, http error/response, middleware, audit, outbox, idempotensi, rate limit | (tidak ada modul bisnis) |
| auth | akun, sandi Argon2id, sesi, CSRF, verifikasi email, reset, RBAC + scope | platform |
| identity | NIK: normalisasi, AEAD, blind index, reveal teraudit | platform, auth |
| settings | organisasi, unit, lokasi, pengaturan berversi | platform, auth |
| workflow | definisi graph, validasi, evaluator rule, engine transisi | platform, auth |
| recruitment | requisition, lowongan, profil kandidat, lamaran, snapshot, interview, keputusan | platform, auth, identity, workflow |
| exam | bank soal, paket, jadwal, attempt, autosave, submit, skoring | platform, auth, workflow (via event) |
| documents | upload karantina, pemindaian, unduh bertanda | platform, auth |
| notify | template, inbox, pengiriman email | platform |
| reporting | dashboard, ekspor | platform, auth |

Dependensi satu arah: `workflow` tidak mengimpor `recruitment`; exam memicu workflow lewat outbox event, bukan panggilan langsung.

Setiap modul: service (aturan bisnis, menerima transaksi dari caller untuk operasi yang harus commit bersama), handler tipis (parsing, pemetaan error), query SQL di `backend/db/queries/<modul>.sql`.

## Alur request

```
Browser -> Nginx -> API (Gin)
                     |- RequestID, recover, body limit, deadline per kelas operasi
                     |- Sesi (cookie HttpOnly, digest di DB) -> Principal
                     |- CSRF (token terikat sesi + cek Origin) untuk mutasi
                     |- Rate limit (Redis bila ada, lokal bila tidak)
                     '- Handler -> Service -> Tx PostgreSQL (+ outbox, audit di tx yang sama)
Worker -> klaim outbox -> handler topik (email, workflow event, ekspor) -> selesai / retry / dead
```

## Kelas operasi dan deadline

| Kelas | Contoh | Deadline handler |
|---|---|---|
| critical-write | save/submit ujian, apply, transisi | 5 s |
| interactive | login, baca profil, daftar lamaran | 10 s |
| background-read | dashboard, laporan | 30 s |

Body limit default 1 MiB, upload dokumen 5 MiB (endpoint khusus).

## Anggaran koneksi PostgreSQL

Contoh untuk `max_connections = 100`:

| Konsumen | Jumlah | Pool max | Total |
|---|---|---|---|
| API replica | 2 | 20 | 40 |
| Worker | 1 | 10 | 10 |
| Migrasi (sekali per rilis) | 1 | 2 | 2 |
| Cadangan admin/superuser | - | - | 10 |
| **Sisa** | | | 38 |

Naikkan pool hanya setelah mengukur pool wait, slow query, lock wait, dan panjang antrean. Dikonfigurasi lewat `DB_MAX_CONNS`.

## Zona waktu

Semua waktu disimpan `timestamptz`. Tampilan memakai zona organisasi (`organizations.timezone`, awal `Asia/Jakarta`). API mengembalikan RFC 3339; frontend memformat dengan `Intl.DateTimeFormat` dan zona organisasi.
