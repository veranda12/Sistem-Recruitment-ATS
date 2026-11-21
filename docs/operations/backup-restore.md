# Backup dan Restore

## Apa yang di-backup

| Komponen | Cara | Frekuensi contoh | Catatan |
|---|---|---|---|
| PostgreSQL (sumber kebenaran) | Base backup fisik `ops/backup/pg-basebackup.sh` + archive WAL (`postgresql-archive.conf`) | Base harian, WAL kontinu (`archive_timeout=300`) | Memungkinkan PITR. Contoh sasaran RPO 15 menit, RTO 4 jam **menunggu persetujuan pelanggan**. |
| PostgreSQL (logis, tambahan) | `pg_dump -Fc` (`ops/windows/backup.ps1` di Windows dev) | Harian | Untuk migrasi lintas versi dan drill cepat. |
| Object storage (dokumen) | MinIO versioning + object lock, atau `restic` (`ops/backup/objects-backup.sh`) | Harian | Folder `exports/` tidak di-backup (berumur 24 jam). |
| Kunci (`NIK_ENC_KEY`, `NIK_INDEX_KEY`, `SESSION_KEY`, kata sandi restic) | Brankas terpisah (mis. password manager perusahaan / HSM) | Saat berubah | **Jangan** disimpan bersama backup data. Tanpa `NIK_ENC_KEY` NIK di backup tidak dapat dibuka. |

Volume backup wajib terenkripsi dan berada di host berbeda dari server DB.

## Prosedur restore (lingkungan kosong)

1. Siapkan PostgreSQL 17 kosong. Untuk PITR: ekstrak base backup, isi `restore_command = 'cp /backup/ats/wal/%f %p'` dan `recovery_target_time = '<waktu>'`, buat `recovery.signal`, start.
   Untuk dump logis: `createdb` lalu `pg_restore --no-owner --exit-on-error`.
2. Pulihkan object storage ke `OBJECT_DIR` (restic restore / MinIO versi pada waktu yang sama).
3. Pasang kunci dari brankas ke environment file.
4. Jalankan `bin/migrate up` (aman: hanya menerapkan migrasi yang belum ada).
5. Verifikasi dengan `ops/windows/restore-drill.ps1` (atau padanan SQL di bawah).
6. Rekonsiliasi data yang mungkin tidak konsisten antar komponen (lihat bagian berikut).

### Titik pemulihan konsisten dan rekonsiliasi

- DB dan object storage di-backup terpisah, sehingga titik waktunya bisa berbeda. Setelah restore:
  - Baris `documents` tanpa berkas: tandai `scan_status='error'` dan minta kandidat mengunggah ulang.
  - Berkas tanpa baris `documents`: hapus (yatim).
- Event outbox `pending` ikut dipulihkan dan akan dikirim ulang. Consumer idempoten (dedup per event ID, unique notifikasi, `email_deliveries`), tetapi email yang terkirim setelah titik backup dapat terkirim ulang: periksa `email_deliveries` status `sent` setelah titik pemulihan.
- Attempt ujian `in_progress` yang deadline-nya terlewat selama downtime akan difinalisasi worker expiry sebagai `expired` dengan jawaban committed terakhir. Kandidat terdampak diarahkan ke proses keberatan.
- Data yang dihapus atas permintaan subjek data setelah titik backup harus dihapus ulang sebelum sistem dibuka (lihat `docs/product/privacy.md`, daftar penghapusan tercatat di audit).

## Hasil drill (aktual)

Drill dijalankan 2026-11-21 di laptop pengembang Windows 11, PostgreSQL 17.5 lokal, sumber `ats_e2e` setelah suite E2E (14 user, 6 lamaran, 87 baris audit).

| Langkah | Hasil |
|---|---|
| `backup.ps1` (pg_dump -Fc + objek + manifest SHA-256) | 15 berkas, 0,7 s |
| Verifikasi manifest | 15/15 checksum cocok |
| Restore DB ke `ats_restore_test` kosong | 2,5 s, skema versi 6 |
| Dokumen: berkas ada dan SHA-256 = sidik saat unggah | lulus |
| Receipt ujian: digest jawaban = SHA-256 snapshot, revisi final sama | lulus |
| Histori workflow: tahap terakhir histori = tahap lamaran | lulus (6 transisi) |
| Total | 3,5 s |

Temuan dari drill pertama: digest receipt semula dihitung atas byte JSON aplikasi sehingga tidak dapat diverifikasi ulang dari `jsonb` setelah restore. Diperbaiki: digest kini dihitung PostgreSQL atas teks kanonis `jsonb` dan diperiksa di test integrasi dan drill.

**Belum diuji:** PITR dengan base backup + WAL di server Linux, restore object storage MinIO/restic, dan RTO pada volume data produksi. Angka di atas adalah dataset kecil dan **bukan** bukti RTO 4 jam.

## Rilis berisiko

- Migrasi dengan pola expand -> backfill -> contract. Kolom baru ditambahkan nullable/berdefault, backfill bertahap dalam batch kecil, constraint dipasang `NOT VALID` lalu `VALIDATE`, kolom lama dihapus pada rilis berikutnya setelah binary lama tidak dipakai.
- Tidak ada down migration otomatis. Rollback = kembali ke binary sebelumnya (`deploy.sh` melakukannya otomatis bila smoke gagal), skema tetap kompatibel mundur.
- Uji upgrade dari rilis lama: `ops/windows/test-upgrade.ps1` (aktual: skema 3 -> 5 dengan data utuh, 2026-11-21).
