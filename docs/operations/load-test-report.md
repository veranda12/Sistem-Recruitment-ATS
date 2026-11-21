# Laporan Uji Beban Ujian

Semua angka di bawah adalah hasil run nyata yang tersimpan di `tests/load/results/<run>/` (k6 summary JSON, stdout, rekonsiliasi SQL, sampel sumber daya, metadata). **Lingkungan bukan staging**: API, worker, PostgreSQL, dan generator k6 berjalan di satu laptop pengembang, sehingga generator bersaing CPU dengan sistem yang diuji. Hasil ini **tidak** menjadi klaim kapasitas produksi; klaim 500 peserta memerlukan run ulang di staging dengan hardware produksi.

## Lingkungan

| Item | Nilai |
|---|---|
| Mesin | Laptop, Intel Core i5-10500H (6 core / 12 thread), RAM 15,8 GB, Windows 11 Home, SSD lokal |
| PostgreSQL | 17.5 (layanan Windows, konfigurasi default installer, `max_connections=100`) |
| API / worker | 1 replika API (`DB_MAX_CONNS=20`), 1 worker (pool 6), build commit tercantum per run |
| Redis | Tidak ada (mode degraded; tidak di jalur durability) |
| Limiter | `RATE_LIMIT_SCALE=100` karena semua VU dari satu IP generator (konfigurasi uji, bukan produksi) |
| Generator | k6 v1.0.0, cookie jar per VU, satu peserta unik + satu undangan valid per VU (fixture `loadseed`, dibuat terpisah sebelum pengukuran) |
| Skenario | Login -> start -> load/resume -> navigasi + menjawab/mengubah -> autosave batch (rata-rata tiap 10 s) -> heartbeat (~30 s, jitter) -> resume acak (2% per langkah) -> submit dengan batch terakhir. Seed PRNG dan think time dapat diulang (`SEED=20261121`) |
| Oracle | `tests/load/reconcile.sql` (read-only, independen dari respons aplikasi) + counter k6 `acked_ops` |

Perintah: `powershell -File tests\load\run-load.ps1 -Profile <profil> -Participants <n> -ExamSeconds <detik>`

## Ringkasan hasil

| Run | Profil | VU / peserta | Durasi mengerjakan | Hasil utama |
|---|---|---|---|---|
| r1121130116 | smoke | 5 | 60 s | Oracle menemukan **bug**: jawaban yang dikosongkan tercatat revisi 0 di snapshot final (1 butir). Diperbaiki + test regresi |
| r1121130352 | smoke | 5 | 60 s | 53 request, 0 gagal, semua cek integritas 0 selisih |
| r1121130521 | baseline | 100 | 180 s | 2.489 request, 0 gagal, 0 retry, 0 penolakan; semua cek integritas 0 selisih |
| r1121130857 | ramp 500 + plateau | 500 | 1.500 s | lihat bagian 500 VU |

### Baseline 100 VU (r1121130521, commit db25a69)

| Operasi | n | p95 | p99 | maks | Target p95 |
|---|---|---|---|---|---|
| login | 100 | 52 ms | 109 ms | 171 ms | <= 1,5 s |
| start | 100 | 18 ms | 37 ms | 43 ms | <= 1,5 s |
| resume | 177 | 3 ms | 4 ms | 33 ms | <= 800 ms |
| save (batch) | 1.484 | 4 ms | 7 ms | 17 ms | <= 400 ms (p99 <= 1 s) |
| heartbeat | 528 | 2 ms | 3 ms | 8 ms | - |
| submit | 100 | 8 ms | 15 ms | 19 ms | <= 1 s (p99 <= 2 s) |

Integritas: 1.549 ACK = 1.549 receipt persist; 100 submit = 100 submission = 100 hasil; peserta dengan >1 attempt 0; digest receipt tidak cocok 0; snapshot beda dari revisi committed 0; tulis lintas peserta 0; revisi attempt != jumlah receipt 0; konflik 0; error tak terduga 0.

## 500 VU dengan plateau

Run r1121130857: 500 VU naik bertahap lalu plateau, durasi mengerjakan 1.500 s, lingkungan laptop yang sama.

| Operasi | n | p95 | p99 | maks | Target p95 |
|---|---|---|---|---|---|
| login | 500 | 53 ms | 87 ms | 157 ms | <= 1,5 s |
| start | 500 | 10 ms | 31 ms | 41 ms | <= 1,5 s |
| resume | 4.192 | 3 ms | 9 ms | 279 ms | <= 800 ms |
| save (batch) | 62.080 | 6 ms | 22 ms | 893 ms | <= 400 ms (p99 <= 1 s) |
| heartbeat | 23.674 | 2 ms | 12 ms | 808 ms | - |
| submit | 500 | 9 ms | 22 ms | 70 ms | <= 1 s (p99 <= 2 s) |

Total 91.446 request, 0 gagal. Integritas (`reconcile.txt`): 500 peserta = 500 attempt = 500 submission, 0 expired; 62.393 ACK k6 = 62.393 receipt persist; peserta dengan >1 attempt 0; attempt final tanpa submission 0; hasil != submission 0; digest receipt tidak cocok 0; snapshot beda dari revisi committed 0; tulis lintas peserta 0; revisi attempt != jumlah receipt 0.

Catatan: CPU proses PostgreSQL tidak tertangkap sampler `Get-Process` (`resources.csv`); gunakan `counters.csv` (counter sistem) untuk CPU total. Hasil tetap bukan klaim kapasitas produksi.
