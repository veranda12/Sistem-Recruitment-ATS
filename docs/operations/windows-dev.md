# Pengembangan di Windows (tanpa Docker)

## Prasyarat dan versi teruji

| Tool | Teruji | Catatan |
|---|---|---|
| Go | 1.24.5 | `go version` |
| Node.js / npm | 24.22.1 / 11.19.1 | Minimal 20.19 |
| PostgreSQL | 17.5 (installer EDB) | Tambahkan `C:\Program Files\PostgreSQL\17\bin` ke PATH |
| sqlc | 1.29.0 | Hanya untuk mengubah query. Unduh `sqlc_1.29.0_windows_amd64.zip` |
| k6 | 1.0.0 | Hanya untuk uji beban |
| Redis | tidak wajib | Lihat bagian Redis |

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

Reset database dev: `backend\bin\seed.exe --reset` (hanya untuk APP_ENV development/test dan nama DB berakhiran `_dev`, `_test`, `_e2e`, `_load`).

## Redis (opsional)

Tanpa `REDIS_URL` aplikasi berjalan dalam mode degraded: cache publik mati, limiter lokal per instance (kuota dibagi dua), ekspor data pribadi ditolak 503 bila `STRICT_SENSITIVE_LIMITS=true`. Untuk mencoba mode penuh:

```powershell
wsl --install -d Ubuntu          # sekali, butuh restart
wsl -d Ubuntu -- sudo apt-get update
wsl -d Ubuntu -- sudo apt-get install -y redis-server
wsl -d Ubuntu -- sudo service redis-server start
# WSL2 meneruskan localhost; di .env:
# REDIS_URL=redis://127.0.0.1:6379/0
```

Pemeriksaan: `Invoke-RestMethod http://127.0.0.1:8080/readyz` harus menampilkan `redis: ok`. Mode dengan Redis **belum diuji** di repository ini (lingkungan pengembangan tidak memiliki Redis).

## Email dan berkas

- Tanpa `SMTP_ADDR`, email ditulis ke `MAIL_DIR` (`.data\mail`) sebagai `.eml`. Untuk SMTP lokal, jalankan server SMTP uji (mis. MailHog/Mailpit) dan isi `SMTP_ADDR=127.0.0.1:1025`.
- Dokumen disimpan di `OBJECT_DIR` (`.data\objects`), subfolder `quarantine/` dan `clean/`. Tanpa `SCAN_COMMAND` dokumen tetap `pending`. Untuk dev: `SCAN_COMMAND=powershell -NoProfile -ExecutionPolicy Bypass -File D:\path\ats\ops\windows\dev-scan.ps1` (path tanpa spasi).

## Mengubah skema atau query

1. Tambah `backend\db\migrations\NNNN_nama.up.sql` (pola expand/backfill/contract).
2. Ubah `backend\db\queries\*.sql`, jalankan `sqlc generate` di `backend`.
3. `check-all.ps1` memastikan tidak ada drift (`sqlc diff`).
4. Bila kontrak API berubah, ubah `contracts\openapi.yaml`; `npm run build` gagal bila tipe frontend tidak cocok.
