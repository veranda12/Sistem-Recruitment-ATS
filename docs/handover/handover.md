# Serah Terima

## Paket yang diserahkan

| Komponen | Lokasi |
|---|---|
| Source code + riwayat Git | Repository ini (belum di-push ke remote mana pun; tidak ada deploy eksternal) |
| Kontrak API | `contracts/openapi.yaml` |
| Arsitektur, ADR, ERD | `docs/architecture/` |
| Spesifikasi modul, otorisasi, asumsi, privasi | `docs/product/` |
| Keamanan (threat model, pemetaan ASVS) | `docs/security/` |
| Operasi (Windows dev, Linux, runbook, backup/restore, rotasi kunci, uji beban) | `docs/operations/` |
| Panduan pengguna, manual admin, UAT, dukungan, VAPT, known issues, dependency | `docs/handover/` |
| Status dan hasil uji aktual | `docs/STATUS.md` |

## Prosedur serah terima akses dan kredensial

Tidak ada secret di Git (`.env` diabaikan; `.env.example` tanpa nilai). Saat serah terima:
1. Pelanggan membuat **kunci baru** (`SESSION_KEY`, `NIK_ENC_KEY`, `NIK_INDEX_KEY`) di lingkungannya sendiri; kunci pengembangan tidak pernah dipakai di produksi.
2. Pelanggan membuat role database aplikasi dan migrasi sendiri, kredensial SMTP, Redis, MinIO/restic.
3. Akun Super Admin pertama dibuat dengan prosedur di `admin-manual.md`; pengembang tidak memegang akun produksi.
4. Bila pengembang pernah diberi akses staging/produksi: cabut akun, rotasi semua kredensial yang pernah dibagikan (lihat `docs/operations/key-rotation.md`), catat di register di bawah.

### Register

| Item | Pemilik pelanggan | Tanggal dibuat/dirotasi | Catatan |
|---|---|---|---|
| Kunci aplikasi (3) | | | Brankas terpisah dari backup |
| Role DB aplikasi / migrasi | | | |
| SMTP relay | | | |
| Redis / MinIO / restic | | | |
| Sertifikat TLS | | | |
| Akun Super Admin pertama | | | MFA aktif |

## Keputusan yang masih terbuka dan pemiliknya

| Keputusan | Pemilik usulan | Referensi |
|---|---|---|
| Teks pemberitahuan privasi, dasar pemrosesan, retensi, legal hold, kanal permintaan subjek data | Legal + HR (DPO) | `docs/product/privacy.md` |
| Kesempatan retake, akomodasi waktu, pemberitahuan nilai, proses keberatan | HR | `docs/product/assumptions.md` |
| RPO/RTO (contoh 15 menit / 4 jam), lokasi backup, masa simpan backup | TI pelanggan | `docs/operations/backup-restore.md` |
| SLA dukungan, jam layanan, waktu respons, durasi garansi | Manajemen + vendor | `support.md` |
| Versi acuan OWASP untuk audit, jadwal VAPT independen | Keamanan TI | `docs/security/asvs-mapping.md`, `vapt-readiness.md` |
| Perlu/tidaknya NIK di tahap awal (minimisasi data) | Legal + HR | `privacy.md` |
| Kapasitas target (500 peserta serentak) di hardware produksi | TI pelanggan | `docs/operations/load-test-report.md` |
