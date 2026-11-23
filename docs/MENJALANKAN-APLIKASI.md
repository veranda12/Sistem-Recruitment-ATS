# Menjalankan aplikasi ATS dari awal (Windows, tanpa Docker)

Panduan ini membawa Anda dari komputer yang belum berisi apa pun sampai aplikasi berjalan di
http://localhost:5173, termasuk membuat database, menjalankan migrasi skema, dan mengisi data contoh.
Semua perintah dijalankan di **PowerShell dari folder root repository** (folder yang berisi `backend`, `frontend`, dan `ops`).

Ringkasan urutannya:

1. Pasang prasyarat (Go, Node.js, PostgreSQL).
2. Buat berkas `.env`.
3. Periksa prasyarat (`preflight.ps1`).
4. Build, buat database, migrasi, dan seed (`setup.ps1`, atau langkah manual di bagian 5).
5. Jalankan aplikasi (`start.ps1`) dan buka di browser.
6. Hentikan aplikasi (`stop.ps1`).

---

## 1. Prasyarat

| Tool | Versi minimal | Teruji | Wajib? |
|---|---|---|---|
| Go | 1.24 | 1.24.5 | Ya |
| Node.js dan npm | 20.19 / 10 | 24.22.1 / 11.19.1 | Ya |
| PostgreSQL (server dan `psql`) | 17 | 17.5 (installer EDB) | Ya |
| Poppler | 24.08 | 24.08.0 | Hanya untuk fitur isi profil dari CV |
| Tesseract OCR | 5.5 | 5.5.0 (installer UB Mannheim) | Hanya untuk CV hasil scan atau foto |
| LibreOffice | 26.2 | 26.2.5.2 | Hanya untuk CV Word (`.doc`, `.docx` bergambar) |
| Redis | 8.x | 8.10.3 | Tidak. Tanpa Redis aplikasi tetap jalan (mode terbatas) |

Setelah memasang PostgreSQL, tambahkan `C:\Program Files\PostgreSQL\17\bin` ke PATH, lalu pastikan layanannya berjalan:

```powershell
Start-Service postgresql-x64-17
psql --version
```

Bila PowerShell menolak menjalankan skrip, setiap perintah di bawah sudah memakai `-ExecutionPolicy Bypass` sehingga
pengaturan sistem tidak perlu diubah.

---

## 2. Buat berkas `.env`

`.env` berisi alamat database, kunci enkripsi, dan pengaturan lain. Berkas ini **tidak boleh masuk Git**.

Ganti `USER` dan `SANDI` dengan akun PostgreSQL Anda, dan `Dev-Sandi-2026` dengan kata sandi yang akan dipakai
semua akun contoh (minimal 10 karakter, ada huruf dan angka):

```powershell
powershell -ExecutionPolicy Bypass -File ops\windows\new-env.ps1 `
  -DatabaseUrl 'postgres://USER:SANDI@localhost:5432/ats_dev?sslmode=disable' `
  -SeedPassword 'Dev-Sandi-2026'
```

Skrip ini membuat tiga kunci acak (`SESSION_KEY`, `NIK_ENC_KEY`, `NIK_INDEX_KEY`). Simpan salinan `NIK_ENC_KEY` dan
`NIK_INDEX_KEY` di tempat aman: tanpa kunci itu, NIK yang sudah tersimpan tidak bisa dibuka lagi. Karena itu,
`new-env.ps1` menolak menimpa `.env` yang sudah ada kecuali dengan `-Force`.

**Port 8080 sudah dipakai aplikasi lain** (mis. Java/Tomcat)? Jangan matikan aplikasinya. Tambahkan `-ApiPort`, misalnya
`-ApiPort 8090`, saat membuat `.env`. Bila `.env` sudah ada, cukup ubah baris `HTTP_ADDR=127.0.0.1:8090`. `start.ps1`,
`preflight.ps1`, `smoke.ps1`, dan proxy frontend otomatis mengikuti port ini; alamat yang dibuka di browser tetap
`http://localhost:5173`.

### 2a. Pemindai berkas

Dokumen dan CV yang diunggah dipindai dulu sebelum diproses. `new-env.ps1` mengisi `SCAN_COMMAND` dengan pemindai
khusus pengembangan (`ops\windows\dev-scan.ps1`, hanya mengenali berkas uji EICAR). Bila `.env` Anda dibuat sebelum
pembaruan ini dan `SCAN_COMMAND` masih kosong, CV akan tertahan di "Menunggu pemeriksaan file". Tambahkan barisnya
secara manual, dengan path yang sesuai lokasi repository Anda (tanpa spasi):

```
SCAN_COMMAND="powershell -NoProfile -ExecutionPolicy Bypass -File D:\Portfoliots\ops\windows\dev-scan.ps1"
```

Di server produksi, isi dengan pemindai sungguhan, misalnya `clamdscan --no-summary`.

### 2b. Opsional: mengirim email sungguhan

Tanpa `SMTP_ADDR`, email ditulis ke `.data\mail` (lihat bagian 6). Untuk mengirim ke kotak masuk sungguhan, misalnya
lewat Gmail dengan App Password (Akun Google, Keamanan, Sandi aplikasi):

```
SMTP_ADDR=smtp.gmail.com:587
SMTP_USERNAME=akun-anda@gmail.com
SMTP_PASSWORD=sandi-aplikasi-16-karakter
SMTP_TLS=starttls
MAIL_FROM="Rekrutmen <akun-anda@gmail.com>"
```

Gunakan `SMTP_TLS=implicit` untuk server yang memakai port 465. Restart dengan `stop.ps1` lalu `start.ps1`.

### 2c. Opsional: alat pembaca CV

Lewati bagian ini bila belum memerlukan fitur "Isi profil dari CV". Aplikasi lain tetap berjalan, dan
`start.ps1` hanya akan memberi peringatan bahwa worker CV berhenti.

`new-env.ps1` dan `setup.ps1` **mengisi `CV_*` di `.env` secara otomatis**:

- Poppler, Tesseract, dan LibreOffice dicari di PATH dan lokasi pemasangan umum (`C:\poppler-*`,
  `C:\Program Files\Tesseract-OCR`, `C:\Program Files\LibreOffice`, paket winget, `D:\Tools\...`).
- Data bahasa Tesseract (Indonesia, Inggris, deteksi orientasi) diunduh ke `tools\tessdata` dan diverifikasi
  checksum-nya bila Tesseract ditemukan.
- Nilai yang sudah Anda isi tidak diubah.

Bila alatnya belum terpasang, pasang sekaligus lewat winget lalu isi `.env`:

```powershell
powershell -ExecutionPolicy Bypass -File ops\windows\detect-tools.ps1 -Install
# tambahkan -WithLibreOffice untuk CV Word lama (.doc), unduhan ~350 MB
```

Setelah memasang alat baru secara manual, cukup jalankan ulang `detect-tools.ps1` (tanpa parameter). Hasil yang
ditulis ke `.env` kira-kira seperti ini (path berspasi diberi tanda kutip):

```
CV_PDFTOPPM=C:/poppler-24.08.0/Library/bin/pdftoppm.exe
CV_TESSERACT="C:/Program Files/Tesseract-OCR/tesseract.exe"
CV_TESSDATA=D:/Portfolio/ats/tools/tessdata
CV_SOFFICE="C:/Program Files/LibreOffice/program/soffice.com"
```

Redis portabel (opsional) tetap dipasang terpisah: `powershell -ExecutionPolicy Bypass -File ops\windows\cv-tools.ps1`.

---

## 3. Periksa prasyarat

```powershell
powershell -ExecutionPolicy Bypass -File ops\windows\preflight.ps1
```

Skrip ini memeriksa versi tool, isi `.env`, koneksi dan login PostgreSQL, Redis, dan port 8080, 8081, serta 5173.
Setiap masalah disebut namanya beserta cara memperbaikinya. Lanjutkan setelah muncul **"Preflight lulus."**

Peringatan kuning tentang `sqlc` atau `REDIS_URL` kosong boleh diabaikan untuk menjalankan aplikasi.

---

## 4. Build, buat database, migrasi, dan seed (cara cepat)

```powershell
powershell -ExecutionPolicy Bypass -File ops\windows\setup.ps1
```

Satu perintah ini menjalankan, berurutan:

1. preflight;
2. `go mod download` di `backend`;
3. build semua program backend ke `backend\bin` (`api`, `worker`, `cvworker`, `migrate`, `seed`, dan alat lain);
4. `npm ci` di `frontend`;
5. membuat database dari `DATABASE_URL` bila belum ada (encoding UTF8);
6. **migrasi skema** dengan `backend\bin\migrate.exe up`;
7. seed data contoh PT Arunika Daya Persada (organisasi fiktif), bila `SEED_PASSWORD` terisi.

Pilihan:

- `-SkipSeed`: tanpa data contoh. Wajib untuk instalasi yang bukan pengembangan atau staging.
- `-SkipFrontend`: lewati `npm ci`.

Bila semua berhasil, muncul **"Setup selesai."** Lanjut ke bagian 6.

---

## 5. Langkah manual (bila ingin menjalankan tahap demi tahap)

Bagian ini melakukan hal yang sama dengan `setup.ps1`, tetapi satu per satu, sehingga Anda bisa melihat setiap tahap,
terutama migrasi.

### 5.1 Muat `.env` ke sesi PowerShell

Program backend membaca konfigurasi dari environment, bukan langsung dari `.env`. Jalankan ini sekali per jendela
PowerShell:

```powershell
. .\ops\windows\common.ps1
Import-DotEnv
```

### 5.2 Build backend

```powershell
cd backend
go mod download
go build -o bin ./cmd/...
cd ..
```

### 5.3 Pasang dependency frontend

```powershell
cd frontend
npm ci --no-audit --no-fund
cd ..
```

### 5.4 Buat database

Contoh untuk database `ats_dev`. Nama database harus sama dengan yang ada di `DATABASE_URL`:

```powershell
createdb -h localhost -p 5432 -U USER -E UTF8 -T template0 ats_dev
```

### 5.5 Migrasi skema database

```powershell
.\backend\bin\migrate.exe up
```

Hasil yang benar, contohnya:

```
versi skema: 9 dirty=false
```

Catatan tentang migrasi:

- Berkas migrasi ada di `backend\db\migrations` dan sudah tertanam di dalam program `migrate.exe` serta `api.exe`.
- `migrate up` aman dijalankan berulang kali. Bila skema sudah terbaru, perintah ini tidak mengubah apa pun.
- Tidak ada perintah `down`. Pembatalan rilis memakai binary versi sebelumnya yang kompatibel, bukan menghapus skema.
- Bila hasilnya `dirty=true`, migrasi terakhir gagal di tengah jalan. Periksa pesan error di atasnya dan jangan
  jalankan aplikasi sebelum diperbaiki.

### 5.6 Isi data contoh (seed)

```powershell
.\backend\bin\seed.exe
```

Seed menolak berjalan bila `APP_ENV=production`, dan dilewati bila organisasi contoh sudah ada.

Untuk **mengosongkan database pengembangan lalu mengisinya ulang dari nol** (skema dihapus, migrasi dijalankan
ulang, lalu seed):

```powershell
.\backend\bin\seed.exe --reset
```

`--reset` hanya mau berjalan bila `APP_ENV` bernilai `development` atau `test` dan nama database berakhiran `_dev`,
`_test`, `_e2e`, atau `_load`, sehingga database dengan data nyata tidak ikut terhapus.

---

## 6. Jalankan aplikasi

(Opsional) Jalankan Redis lebih dulu bila sudah dipasang lewat `cv-tools.ps1`, lalu isi
`REDIS_URL=redis://127.0.0.1:6379/0` di `.env`. Redis tidak berjalan otomatis saat komputer menyala.

```powershell
powershell -ExecutionPolicy Bypass -File ops\windows\redis-start.ps1
```

Jalankan aplikasi:

```powershell
powershell -ExecutionPolicy Bypass -File ops\windows\start.ps1
```

`start.ps1` menjalankan semua proses di latar belakang. Log ada di `.data\logs`, dan PID dicatat di `.data\pids.json`.

| Proses | Alamat | Fungsi |
|---|---|---|
| API | http://127.0.0.1:8080 | Backend REST, termasuk pemeriksaan kesiapan `/readyz` |
| Worker | 127.0.0.1:8081 | Email, notifikasi, pemindaian dokumen, perpindahan tahap otomatis |
| Worker CV | 127.0.0.1:8082 | Pembacaan CV (berhenti dengan peringatan bila alat CV belum dipasang) |
| Frontend | http://localhost:5173 | Portal karier, akun kandidat, ruang ujian, back office |

Buka **http://localhost:5173** di browser.

### Akun contoh

Kata sandi semua akun sama dengan `SEED_PASSWORD` di `.env`.

| Email | Peran |
|---|---|
| admin@arunika.example | Super Admin |
| hr.ops@arunika.example | HR Admin unit Operasional Pabrik |
| hr.ti@arunika.example | HR Admin unit Teknologi Informasi |
| rekruter.ops@arunika.example | Recruiter unit Operasional Pabrik |
| rekruter.ti@arunika.example | Recruiter lowongan Backend Engineer |
| hm.ops@arunika.example | Hiring Manager lowongan Operator Produksi dan Supervisor QC |
| penguji@arunika.example | Examiner (penguji) |
| sari.kandidat@contoh.test, budi.kandidat@contoh.test | Kandidat dengan profil lengkap |

Halaman masuk ada di http://localhost:5173/masuk dan back office staf di http://localhost:5173/staf. Kandidat baru bisa mendaftar sendiri di http://localhost:5173/daftar.

### Email di mode pengembangan

Tanpa `SMTP_ADDR`, email tidak benar-benar dikirim. Email ditulis sebagai berkas `.eml` di `.data\mail`. Untuk
memverifikasi akun kandidat baru, buka email terbaru di folder itu (misalnya dengan Notepad), lalu salin tautan
verifikasinya ke browser.

---

## 7. Periksa bahwa aplikasi berjalan

```powershell
powershell -ExecutionPolicy Bypass -File ops\windows\smoke.ps1
```

Uji asap ini memeriksa `/readyz`, daftar lowongan publik, login staf, daftar lamaran, CSRF, dan logout. Semua baris
harus `[OK]`.

Pemeriksaan cepat secara manual:

```powershell
Invoke-RestMethod http://127.0.0.1:8080/readyz
```

`database` harus bernilai `ok`. `redis` bernilai `ok` bila Redis dipakai, atau `disabled`/`degraded` bila tidak.

---

## 8. Hentikan aplikasi

```powershell
powershell -ExecutionPolicy Bypass -File ops\windows\stop.ps1
powershell -ExecutionPolicy Bypass -File ops\windows\redis-stop.ps1   # bila Redis dijalankan
```

`stop.ps1` hanya menghentikan proses yang dicatat oleh `start.ps1`.

---

## 9. Menjalankan lagi di hari berikutnya

Build, migrasi, dan seed tidak perlu diulang. Cukup:

```powershell
Start-Service postgresql-x64-17   # bila PostgreSQL belum berjalan
powershell -ExecutionPolicy Bypass -File ops\windows\start.ps1
```

Setelah menarik kode baru dari Git, build ulang dan jalankan migrasi sebelum `start.ps1`:

```powershell
. .\ops\windows\common.ps1; Import-DotEnv
cd backend; go build -o bin ./cmd/...; cd ..
cd frontend; npm ci --no-audit --no-fund; cd ..
.\backend\bin\migrate.exe up
```

---

## 10. Masalah umum

| Gejala | Penyebab | Solusi |
|---|---|---|
| `.env sudah ada` saat `new-env.ps1` | `.env` lama tidak ditimpa demi melindungi kunci | Pakai `.env` yang ada, atau `-Force` bila memang ingin membuat ulang (NIK lama tidak bisa dibuka lagi) |
| `PostgreSQL tidak dapat dihubungi` | Layanan PostgreSQL mati | `Start-Service postgresql-x64-17` |
| `Login PostgreSQL ... gagal` | User atau sandi di `DATABASE_URL` salah | Perbaiki `DATABASE_URL` di `.env` |
| `Proses sebelumnya mungkin masih berjalan` | `.data\pids.json` masih ada | Jalankan `stop.ps1`, lalu `start.ps1` |
| `Port 8080/8081/5173 sudah dipakai` | Instance lama atau aplikasi lain | `stop.ps1`; bila masih terpakai, cari prosesnya dengan `netstat -ano \| findstr :8080` |
| `API tidak siap dalam 30 detik` | Error konfigurasi atau migrasi | Baca `.data\logs\api.log` dan `api.err.log` |
| `Worker CV berhenti` saat `start.ps1` | Alat CV belum dipasang atau path di `.env` salah | Ikuti bagian 2c, lalu periksa dengan `ops\windows\cv-check.ps1 -Mode preflight` |
| CV tertahan di "Menunggu pemeriksaan file" | `SCAN_COMMAND` kosong | Ikuti bagian 2a, lalu `stop.ps1` dan `start.ps1` |
| Email tidak terkirim setelah SMTP diisi | Login atau TLS ditolak server | Baca `.data\logs\worker.log`; untuk Gmail pakai App Password, bukan sandi akun |
| Kandidat tidak menerima email verifikasi | Mode dev menulis email ke berkas | Buka `.data\mail\*.eml` |
| `migrate` menampilkan `dirty=true` | Migrasi terakhir gagal di tengah | Baca pesan error, perbaiki penyebabnya; untuk database dev, `seed.exe --reset` |

---

## 11. Langkah lanjutan

- Demo untuk calon pengguna dengan data contoh lengkap dan akun siap pakai, termasuk akses lewat ngrok:
  [`docs/DEMO.md`](DEMO.md)
- Pemeriksaan lengkap (format, lint, test Go dengan PostgreSQL, Vitest, build, dan Playwright) memakai database uji
  terpisah:
  ```powershell
  powershell -ExecutionPolicy Bypass -File ops\windows\check-all.ps1 -TestDatabaseUrl 'postgres://USER:SANDI@localhost:5432/ats_test?sslmode=disable'
  ```
- Detail pengembangan di Windows: [`docs/operations/windows-dev.md`](operations/windows-dev.md)
- Pembaca CV: [`docs/cv-parsing.md`](cv-parsing.md)
- Produksi di Linux: [`docs/operations/deployment-linux.md`](operations/deployment-linux.md)
- Video demo (Remotion): [`video/ats-demo/README.md`](../video/ats-demo/README.md)
