# Asumsi Produk dan Keputusan Terbuka

## Asumsi awal (berlaku sampai dikonfirmasi)

- Satu organisasi per instalasi, dengan beberapa unit dan lokasi.
- UI berbahasa Indonesia. Tanggal ditampilkan menurut zona organisasi (awal `Asia/Jakarta`); penyimpanan memakai `timestamptz`.
- Satu akun kandidat per identitas (NIK) terverifikasi secara internal.
- Verifikasi email hanya membuktikan akses ke email, bukan identitas.
- Satu lamaran per kandidat per publikasi lowongan (`unique(job_id, candidate_id)`). Bila posisi dibuka ulang, HR membuat publikasi baru; lamaran lama tetap di histori.
- Batas waktu ujian ketat, tanpa grace period.
- Pilihan jamak dinilai benar hanya bila seluruh himpunan jawaban cocok.
- Keputusan hiring tidak otomatis; workflow hanya memindahkan tahap, keputusan akhir butuh persetujuan manusia.

## Di luar asumsi dasar

Integrasi Dukcapil, SSO perusahaan, WhatsApp, proctoring video, dan keputusan hiring otomatis tidak termasuk. Tidak ada janji integrasi atau kontrak yang belum tersedia.

## Keputusan yang harus dikonfirmasi pelanggan

| Topik | Nilai sementara di sistem | Pemilik keputusan |
|---|---|---|
| Retensi data pelamar | Belum dihapus otomatis; pengaturan `retention.candidate_months` = 24 (belum dieksekusi) | HR + Legal |
| Kesempatan retake ujian | `max_attempts` per jadwal = 1 | HR |
| Akomodasi waktu | Extra time menit per peserta, diberikan examiner, teraudit | HR |
| Pemberitahuan nilai | Nilai dirilis manual, kandidat melihat status lulus/tidak + skor | HR |
| SLA dukungan | Belum ditetapkan | Manajemen + vendor |
| RPO / RTO | Contoh sasaran: RPO 15 menit, RTO 4 jam | TI pelanggan |
| Dasar pemrosesan data pribadi (UU PDP) | Belum ditetapkan | Legal |
| Volume event / kebutuhan Asynq | Outbox polling 1 detik | TI pelanggan |
| Peran yang wajib MFA | `MFA_REQUIRED_ROLES`, default produksi `super_admin,hr_admin` | Keamanan TI |
| Batas idle sesi | Staf 30 menit, kandidat 2 jam, absolut 12 jam | Keamanan TI |
| Batas laju (login 20/menit/IP, registrasi 10, umum 600) | Konfigurasi kode + Nginx `limit_req` | TI pelanggan (perhatikan NAT kantor) |
| Pemindai malware | `SCAN_COMMAND` (mis. ClamAV); tanpa pemindai dokumen tidak dapat diunduh | TI pelanggan |
| Kebijakan buka ulang posisi | Publikasi lowongan baru; lamaran lama tetap di histori | HR |

## Organisasi fiktif seed

**PT Arunika Daya Persada** (fiktif) dengan unit *Operasional Pabrik* dan *Teknologi Informasi*, lokasi Jakarta, Karawang, Surabaya. Seluruh nama orang, NIK, dan soal dalam seed adalah sintetis.
