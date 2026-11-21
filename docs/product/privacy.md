# Privasi dan Pemrosesan Data Pribadi

Dokumen kerja teknis untuk pemilik kebijakan pelanggan. **Bukan nasihat hukum dan bukan klaim kepatuhan UU No. 27 Tahun 2022 (UU PDP).** Butir bertanda *keputusan* wajib ditetapkan pelanggan (Legal/HR/DPO) sebelum go-live.

## Tujuan pemrosesan dan aliran data

| Tujuan | Data | Sumber -> tujuan | Dasar pemrosesan (*keputusan*) |
|---|---|---|---|
| Akun dan komunikasi | Email, nama, sandi (hash) | Kandidat -> DB -> email | Usulan: persetujuan saat mendaftar / langkah pra-kontrak |
| Verifikasi identitas internal | NIK (terenkripsi), nama | Kandidat -> DB (AES-GCM) | *keputusan* |
| Seleksi | Profil, pendidikan, pengalaman, dokumen, jawaban ujian, nilai, interview, scorecard | Kandidat/staf -> DB, object storage | *keputusan* |
| Keamanan dan audit | IP, user agent (sesi), log audit | Sistem -> DB | Kepentingan sah keamanan (*keputusan*) |
| Pelaporan agregat | Hitungan per tahap | DB -> laporan | Data agregat, tanpa identitas pada funnel |
| Ekspor data pelamar | Nama, email, tahap | DB -> CSV (24 jam) | Izin khusus `pii.export`, teraudit |

Tidak ada pengiriman data ke pihak ketiga oleh aplikasi selain relay SMTP dan pemindai malware milik pelanggan. Tidak ada analitik pihak ketiga, CDN, atau font eksternal.

## Pemberitahuan privasi

Versi yang disetujui kandidat dicatat per pendaftaran (`audit_logs.detail.privacy_notice_version`) dan dalam snapshot lamaran. Nilai saat ini `2026-11-draft`; **teks pemberitahuan final adalah *keputusan* pelanggan** dan versinya diganti di `PrivacyNoticeVersion`.

## Retensi (usulan, *keputusan*)

| Kategori | Usulan | Pengaturan |
|---|---|---|
| Akun kandidat tanpa lamaran aktif | 24 bulan sejak aktivitas terakhir | `retention.candidate_months` (berversi, belum dieksekusi otomatis) |
| Lamaran, snapshot, histori, nilai | 24 bulan setelah keputusan akhir | sama |
| Dokumen kandidat | Sama dengan lamaran; dihapus lunak segera atas permintaan | `documents.deleted_at` |
| Ekspor | 24 jam | otomatis (worker) |
| Log audit | 5 tahun (usulan) | *keputusan* |
| Sesi | Idle 30 menit staf / 2 jam kandidat, absolut 12 jam | env |

Eksekusi retensi otomatis **belum diimplementasikan**: pengaturan tersimpan dan berversi, tetapi penghapusan/anonimisasi terjadwal menunggu keputusan nilai dan legal hold.

## Permintaan subjek data (akses, koreksi, penghapusan)

1. Kandidat mengajukan lewat kanal resmi pelanggan; verifikasi pemohon minimal: login ke akun + konfirmasi melalui email terdaftar. Untuk koreksi NIK atau akun tanpa akses email: verifikasi manual oleh HR dengan dokumen, dicatat alasan di audit.
2. **Akses**: profil, lamaran, timeline, notifikasi tersedia di akun. Salinan lengkap dibuat oleh HR melalui ekspor terscope (teraudit).
3. **Koreksi**: profil dapat diubah kandidat (snapshot lamaran lama tidak berubah, sebagai bukti proses). Koreksi NIK lewat jalur klaim manual (`identity_claims`), tanpa merge otomatis.
4. **Penghapusan**: anonimisasi kolom identitas (nama, email, telepon, alamat, NIK, dokumen) sambil mempertahankan baris bukti tanpa identitas, kecuali ada legal hold. Prosedur teknis untuk dieksekusi DBA dengan audit; perkakas otomatis belum dibuat.
5. Bukti eksekusi: entri audit `privacy.*` dengan pelaku, alasan, request ID.

### Backup dan salinan

Data yang dihapus masih ada di backup sampai kedaluwarsa retensi backup. Saat restore, daftar penghapusan (dari audit) harus diterapkan ulang sebelum sistem dibuka (lihat `docs/operations/backup-restore.md`). Ekspor CSV kedaluwarsa 24 jam. Object storage: hapus juga versi lama bila versioning aktif.

## Keputusan legal/HR yang dibutuhkan

- Dasar pemrosesan per tujuan dan teks pemberitahuan privasi final.
- Masa retensi per kategori, legal hold, dan masa simpan backup.
- Siapa pejabat/fungsi pelindung data pribadi dan kanal permintaan subjek data.
- Apakah NIK diperlukan sama sekali untuk tahap rekrutmen awal (minimisasi data).
- Kebijakan pemberitahuan nilai ujian kepada kandidat.
- Prosedur pemberitahuan insiden kebocoran (waktu, pihak yang dihubungi).
