# Prosedur Dukungan Pasca Go-Live

Durasi garansi, jam dukungan, dan waktu respons **menunggu kesepakatan** (lihat `handover.md`). Prosedur di bawah berlaku apa pun nilai yang disepakati.

## Pelaporan

Pelapor (admin pelanggan) mengirim: ringkasan, waktu kejadian (WIB), peran dan akun terdampak (tanpa sandi), langkah reproduksi, **kode permintaan** dari pesan error (`requestId`), screenshot. Jangan mengirim NIK, CV, atau jawaban ujian lewat kanal dukungan.

## Severity (usulan)

| Severity | Definisi | Contoh |
|---|---|---|
| S1 Kritis | Layanan utama mati atau data berisiko hilang/bocor | Ujian berlangsung tidak bisa menyimpan; kebocoran data |
| S2 Tinggi | Fungsi utama terganggu tanpa jalan pintas | Rilis nilai gagal; login staf gagal massal |
| S3 Sedang | Fungsi terganggu dengan jalan pintas | Ekspor gagal, bisa ulang manual |
| S4 Rendah | Kosmetik, pertanyaan | Salah ketik label |

## Kontak dan eskalasi

| Peran | Nama | Kontak | Jam |
|---|---|---|---|
| Admin aplikasi pelanggan (L1) | | | |
| TI infrastruktur pelanggan | | | |
| Vendor (L2/L3) | | | sesuai kontrak |
| Pemilik bisnis (eskalasi S1) | | | |

## Diagnosis

1. Cocokkan `requestId` di log: `journalctl -u 'ats-api@*' -o cat | grep <requestId>`.
2. Kesehatan: `/readyz`, health worker, dashboard Grafana (error, latensi, pool, outbox).
3. Audit untuk perubahan data: `/staf/audit` atau tabel `audit_logs` (read-only).
4. Ujian: `exam_save_receipts`, `exam_submissions`, `reconcile.sql` (read-only).

## Verifikasi perbaikan

Perbaikan disertai test otomatis yang gagal sebelum perbaikan dan lulus sesudahnya, dijalankan `check-all` di CI, diuji di staging oleh pelapor, lalu dirilis dengan `deploy.sh` (rollback otomatis bila smoke gagal). Catat di register perubahan: nomor tiket, commit, versi rilis, penguji, tanggal.
