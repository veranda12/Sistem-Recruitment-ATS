# Pengembangan di Windows (tanpa Docker)

## Prasyarat dan versi teruji

| Tool | Teruji | Catatan |
|---|---|---|
| Go | 1.24.5 | `go version` |
| Node.js / npm | 24.22.1 / 11.19.1 | Minimal 20.19 |
| PostgreSQL | 17.5 (installer EDB) | Tambahkan `C:\Program Files\PostgreSQL\17\bin` ke PATH |
| sqlc | 1.29.0 | Hanya untuk mengubah query. Unduh `sqlc_1.29.0_windows_amd64.zip` |
| k6 | 1.0.0 | Hanya untuk uji beban |
| Redis | 8.10.3 (redis-windows, msys2) di `tools
edis` | Tidak wajib; lihat bagian Redis |
| ClamAV | 1.5.5 (zip resmi Cisco-Talos) di `D:\Tools\ClamAV` | Tidak wajib; pemindai dokumen sungguhan, lihat bagian Pemindai |
| GCC (MinGW-w64, WinLibs UCRT) | 16.2.0 di `D:\Tools\WinLibs` | Hanya untuk `go test -race` (cgo) |

`ops\windows\preflight.ps1` memeriksa semuanya dan menyebut dependency yang gagal.

## Skrip

| Skrip | Fungsi |
|---|---|
| `new-env.ps1 -DatabaseUrl ... -SeedPassword ...` | Membuat `.env` dari `.env.example` dengan kunci acak baru |
| `preflight.ps1` | Memeriksa tool, konfigurasi, koneksi DB, Redis, port |
| `setup.ps1 [-SkipSeed] [-SkipFrontend]` | go mod download, build binary, npm ci, buat DB, migrasi, seed |
| `start.ps1` / `stop.ps1` | Menjalankan / menghentikan API, worker, Vite (PID di `.data\pids.json`) |
| `smoke.ps1` | readyz, lowongan, login, lamaran terscope, CSRF, logout |
| `check-all.ps1 -TestDatabaseUrl ...` | Seluruh pemeriksaan termasuk Playwright |
| `test-upgrade.ps1 -FromRef <commit>` | Uji upgrade skema dari rilis lama |
| `backup.ps1` / `restore-drill.ps1` | Backup dan drill restore |
| `dev-scan.ps1` | Pemindai **khusus dev** (hanya string uji EICAR) untuk `SCAN_COMMAND` |
| `redis-start.ps1` / `redis-stop.ps1` | Menjalankan / menghentikan Redis lokal (tidak sebagai service) |
| `clamav-start.ps1 [-Update]` / `clamav-stop.ps1` | Menjalankan / menghentikan clamd di 127.0.0.1:3310; `-Update` memperbarui signature |

Reset database dev: `backend\bin\seed.exe --reset` (hanya untuk APP_ENV development/test dan nama DB berakhiran `_dev`, `_test`, `_e2e`, `_load`).

## Redis (opsional)

Tanpa `REDIS_URL` aplikasi berjalan dalam mode degraded: cache publik mati, limiter lokal per instance (kuota dibagi dua), ekspor data pribadi ditolak 503 bila `STRICT_SENSITIVE_LIMITS=true`, dan worker CV memproses langsung dari PostgreSQL. Untuk mode penuh:

```powershell
powershell -File ops\windows\cv-tools.ps1      # sekali: memasang Redis portabel ke tools\redis
powershell -File ops\windows\redis-start.ps1   # setiap kali dibutuhkan (bukan service)
# .env: REDIS_URL=redis://127.0.0.1:6379/0, lalu restart start.ps1
```

Pemeriksaan: `Invoke-RestMethod http://127.0.0.1:8080/readyz` menampilkan `redis: ok`. Diuji di dev dengan backlog CV (`docs/cv-parsing.md`); dua instance API dengan Redis bersama belum diuji.

## Pemindai malware (ClamAV, opsional di dev)

`dev-scan.ps1` hanya mengenali string uji EICAR. Untuk pemindai sungguhan di Windows:

1. Unduh `clamav-<versi>.win.x64.zip` dari rilis GitHub Cisco-Talos/clamav. Cocokkan SHA-256 dengan digest aset rilis, lalu ekstrak ke folder tanpa spasi, mis. `D:\Tools\ClamAV`. Zip ini tidak berisi database signature.
2. Buat `freshclam.conf` dan `clamd.conf` di folder itu:

   ```
   # freshclam.conf
   DatabaseDirectory D:\Tools\ClamAV\db
   DatabaseMirror database.clamav.net
   ReceiveTimeout 0
   # clamd.conf
   DatabaseDirectory D:\Tools\ClamAV\db
   TCPSocket 3310
   TCPAddr 127.0.0.1
   MaxFileSize 30M
   MaxScanSize 60M
   ```

3. `powershell -File ops\windows\clamav-start.ps1 -Update`. Unduhan pertama sekitar 110 MB (main + daily), dan memuat signature butuh 20–60 detik. Pada koneksi lambat `ReceiveTimeout 0` mencegah unduhan `main.cvd` diputus.
4. Di `.env`: `SCAN_COMMAND=D:\Tools\ClamAV\clamdscan.exe --no-summary --config-file=D:\Tools\ClamAV\clamd.conf`, lalu restart `start.ps1`.

Kode keluar ClamAV sama dengan kontrak `SCAN_COMMAND`: 0 bersih, 1 terinfeksi, lainnya galat (dokumen tetap tertahan dan dicoba ulang). Selama clamd mati, unggahan tertahan "Menunggu pemeriksaan file". Folder ClamAV dapat diganti lewat `-ClamDir` atau `CLAMAV_DIR`.

Catatan pengujian di Windows (2026-11-23):

- **Defender dan EICAR.** Windows Defender memblokir pembukaan berkas EICAR (galat Windows 225) sebelum ClamAV membacanya. Akibatnya `clamdscan` keluar 2 dan dokumen tetap tertahan, bukan ditandai terinfeksi.
- **Uji jalur terinfeksi.** Gunakan signature lokal sementara: `sigtool --md5 berkas > dbts-uji.hdb`, lalu `clamdscan --reload`. Hapus `.hdb` itu setelah selesai.
- **Gunakan `clamdscan`, bukan `clamscan`.** `clamscan` keluar 0 (hanya peringatan) bila berkas tidak dapat dibuka.

## Race detector

`go test -race` butuh cgo dan GCC. Dengan WinLibs (`winget install BrechtSanders.WinLibs.POSIX.UCRT --location D:\Tools\WinLibs`):

```bash
export PATH="/d/Tools/WinLibs/mingw64/bin:$PATH" CGO_ENABLED=1
go test -race ./...
```

## Email dan berkas

- Tanpa `SMTP_ADDR`, email ditulis ke `MAIL_DIR` (`.data\mail`) sebagai `.eml`. Untuk SMTP lokal, jalankan server SMTP uji (mis. MailHog/Mailpit) dan isi `SMTP_ADDR=127.0.0.1:1025`.
- Dokumen disimpan di `OBJECT_DIR` (`.data\objects`), subfolder `quarantine/` dan `clean/`. Tanpa `SCAN_COMMAND` dokumen tetap `pending`. Untuk dev: `SCAN_COMMAND=powershell -NoProfile -ExecutionPolicy Bypass -File D:\path\ats\ops\windows\dev-scan.ps1` (path tanpa spasi), atau ClamAV (bagian Pemindai).

## Mengubah skema atau query

1. Tambah `backend\db\migrations\NNNN_nama.up.sql` (pola expand/backfill/contract).
2. Ubah `backend\db\queries\*.sql`, jalankan `sqlc generate` di `backend`.
3. `check-all.ps1` memastikan tidak ada drift (`sqlc diff`).
4. Bila kontrak API berubah, ubah `contracts\openapi.yaml`; `npm run build` gagal bila tipe frontend tidak cocok.
