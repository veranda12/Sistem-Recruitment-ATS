# Demo untuk calon pengguna

Stack demo memakai database tersendiri (`ats_demo_dev`) berisi data contoh organisasi fiktif PT Arunika Daya Persada.
Database pengembangan di `.env` tidak disentuh. Semua akun sudah terverifikasi, jadi tidak ada yang perlu menunggu email.

## Menjalankan

Prasyarat: setup biasa sudah pernah dijalankan (lihat [MENJALANKAN-APLIKASI.md](MENJALANKAN-APLIKASI.md)) dan
`SEED_PASSWORD` terisi di `.env`. Semua perintah dari folder root repository.

```powershell
# Pertama kali, atau untuk mengembalikan data ke kondisi awal sebelum demo berikutnya
powershell -ExecutionPolicy Bypass -File ops\windows\demo.ps1 start -Reset

# Menjalankan lagi dengan data terakhir (perubahan selama demo tetap ada)
powershell -ExecutionPolicy Bypass -File ops\windows\demo.ps1 start

# Menghentikan
powershell -ExecutionPolicy Bypass -File ops\windows\demo.ps1 stop
```

Buka http://localhost:4173. Stack demo memakai port 4173 (web), 8100 (API), 8101 (worker), dan 8102 (worker CV),
sehingga bisa berjalan bersamaan dengan stack pengembangan (5173/8080).

## Membuka dari internet dengan ngrok

1. Pasang ngrok, login dengan authtoken Anda, lalu klaim satu domain statis gratis di dashboard ngrok.
2. Jalankan demo dengan alamat publik tersebut:
   ```powershell
   powershell -ExecutionPolicy Bypass -File ops\windows\demo.ps1 start -PublicOrigin https://NAMA-ANDA.ngrok-free.app
   ngrok http --url=NAMA-ANDA.ngrok-free.app 4173
   ```
3. Bagikan `https://NAMA-ANDA.ngrok-free.app`.

Catatan:

- Selama dijalankan dengan `-PublicOrigin`, login hanya diterima dari alamat itu. Di laptop Anda sendiri, buka juga
  lewat alamat ngrok, bukan `localhost:4173`.
- Ngrok gratis menampilkan halaman peringatan sekali sebelum aplikasi terbuka. Penonton cukup klik "Visit Site".
- Aplikasi terbuka ke internet selama tunnel berjalan. Semua akun memakai `SEED_PASSWORD` yang sama, jadi gunakan
  sandi yang kuat, jangan unggah data atau CV asli, dan hentikan ngrok setelah demo selesai.
- Email tidak benar-benar dikirim. Isinya tersimpan di `.data\demo\mail`. Untuk penonton yang ingin mencoba sebagai
  kandidat, pakai akun tamu di bawah.

## Akun

Kata sandi semua akun = `SEED_PASSWORD` di `.env`.

### Staf

| Email | Nama | Peran |
|---|---|---|
| admin@arunika.example | Rangga Admin | Super Admin |
| hr.ops@arunika.example | Dewi Lestari | HR Admin unit Operasional Pabrik |
| hr.ti@arunika.example | Yusuf Pratama | HR Admin unit Teknologi Informasi |
| rekruter.ops@arunika.example | Maya Sari | Recruiter unit Operasional Pabrik |
| rekruter.ti@arunika.example | Andi Wijaya | Recruiter lowongan Backend Engineer |
| hm.ops@arunika.example | Bambang Hartono | Hiring Manager lowongan Operator Produksi dan Supervisor QC |
| penguji@arunika.example | Ratna Kusuma | Examiner (penguji) |

### Kandidat dengan profil kosong (untuk mencoba dari awal)

| Email | Kegunaan |
|---|---|
| nadia.pratama@example.com | Peragaan "Isi profil dari CV". Unggah `.data\demo\CV-Nadia-Pratama.pdf` |
| tamu1@example.com, tamu2@example.com, tamu3@example.com | Dipinjamkan ke penonton yang ingin mencoba melamar sendiri |

### Kandidat dengan riwayat lamaran

Email kandidat contoh mengikuti nama, misalnya `rina.amelia@contoh.test` atau `yoga.pratama@contoh.test`.

| Kandidat | Lowongan | Kondisi saat data baru diisi |
|---|---|---|
| Rina Amelia | Operator Produksi | Diterima, pemberitahuan keputusan sudah dikirim |
| Andi Prasetyo | Operator Produksi | Menunggu persetujuan hiring manager |
| Siti Nurhaliza | Operator Produksi | Interview terjadwal |
| Hendra Wijaya | Operator Produksi | Review Penilaian (esai belum dinilai) |
| Yoga Pratama | Operator Produksi | Diundang ujian, belum mulai |
| Rizky Firmansyah | Operator Produksi | Screening |
| Lestari Handayani | Operator Produksi | Menarik lamaran sendiri |
| Bagus Santoso, Dimas Saputra | Operator Produksi | Ditolak |
| Dewi Kartika | Supervisor Quality Control | Interview terjadwal |
| Lestari Handayani | Supervisor Quality Control | Sedang mengerjakan ujian |
| Intan Permatasari | Supervisor Quality Control | Diundang ujian, belum mulai |
| Fajar Nugroho | Backend Engineer | Interview terjadwal |
| Maya Putri | Backend Engineer | Sedang mengerjakan ujian |
| Nadia Rahma | Backend Engineer | Screening |

## Alur yang bisa diperagakan langsung

| Peragaan | Masuk sebagai | Langkah |
|---|---|---|
| Kandidat mengisi profil dari CV | nadia.pratama@example.com | Profil, pilih CV di "Isi profil dari CV", periksa usulan dan "Lihat sumber", "Simpan pilihan ke profil", lengkapi tanggal lahir, lalu lamar Operator Produksi |
| Recruiter memindahkan tahap | rekruter.ops@arunika.example | Lamaran, Rizky Firmansyah, "Lolos screening: ke Ujian" |
| Kandidat mengerjakan ujian | yoga.pratama@contoh.test | Ujian, "Mulai ujian", jawab, "Selesai dan kirim", bukti penerimaan |
| Penguji menilai esai tanpa identitas | penguji@arunika.example | Ujian, Gelombang November, "Penilaian esai" |
| HR merilis nilai | hr.ops@arunika.example | Ujian, Gelombang November, "Rilis nilai" |
| Hiring manager menyetujui | hm.ops@arunika.example | Lamaran, Andi Prasetyo, "Setujui" |
| Kandidat melihat hasil | rina.amelia@contoh.test | Lamaran saya dan Kotak masuk |

Jalankan `demo.ps1 start -Reset` sebelum demo berikutnya agar data kembali ke kondisi di atas.
