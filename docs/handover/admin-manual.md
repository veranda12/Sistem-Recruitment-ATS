# Manual Admin

## Akun Super Admin pertama (produksi)

Produksi tidak memakai seed dan tidak memiliki sandi default. Prosedur (DBA + calon admin, didokumentasikan di register serah terima):
1. Jalankan migrasi (`bin/migrate up`).
2. Buat organisasi, unit, lokasi, katalog permission, peran, dan workflow awal dengan skrip SQL yang ditinjau bersama (templat: `backend/internal/app/seed.go` tanpa data fiktif).
3. Buat baris user staf dengan `password_hash` acak yang tidak dapat dipakai, beri penugasan `super_admin` seluruh organisasi.
4. Kirim tautan atur sandi: minta calon admin membuka *Lupa kata sandi* di `/lupa-sandi` dengan emailnya.
5. Admin login, mengaktifkan verifikasi dua langkah (wajib untuk peran ini di produksi), lalu membuat akun staf lain dari UI.

## Pengguna dan peran (`/staf/pengguna`)

- *Tambah pengguna*: akun dibuat tanpa sandi; pengguna menerima email untuk mengatur sandi.
- *Beri peran*: pilih peran dan unit (kosong = seluruh organisasi). Peran Super Admin hanya dapat diberikan pemegang `roles.manage` dan selalu se-organisasi.
- *Cabut*: berlaku pada permintaan berikutnya pengguna tersebut. Super Admin aktif terakhir tidak dapat dicabut atau dinonaktifkan.
- *Nonaktifkan*: mencabut semua sesi.
- *Reset MFA* (perangkat hilang): `POST /api/v1/admin/users/{id}/mfa-reset` dengan alasan verifikasi identitas; tidak untuk akun sendiri; mencabut sesi target. Tombol UI tersedia di halaman pengguna.
- *Matriks peran dan izin* menampilkan izin tiap peran.

## Pengaturan (`/staf/pengaturan`)

- *Profil organisasi dan brand*: nama, nama tampilan portal, tagline, tentang, warna utama (#RRGGBB). Tampil di portal segera.
- *Kebijakan*: retensi, batas idle staf, kesempatan ujian, batas akomodasi. Setiap perubahan butuh alasan, dapat diberi waktu berlaku, tercatat versi dan riwayatnya. Nilai "belum ditetapkan" menunggu keputusan pemilik kebijakan.
- Master data unit/lokasi dinonaktifkan (bukan dihapus) agar histori tetap utuh: `POST /api/v1/admin/units|locations/{id}/active`.

## Komunikasi dan antrean (`/staf/komunikasi`)

Riwayat email, event gagal permanen (dead-letter) dengan *Coba lagi* (teraudit). Status *Perlu rekonsiliasi* = pengiriman ambigu; cek log SMTP sebelum mengirim ulang manual.

## Audit (`/staf/audit`)

Read-only, saring berdasarkan awalan tindakan (contoh `application.`, `identity.reveal`, `admin.`, `exam.`). Tidak dapat diubah atau dihapus (dijaga trigger database).
