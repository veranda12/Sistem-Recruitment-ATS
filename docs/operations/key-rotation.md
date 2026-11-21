# Rotasi Kunci

## SESSION_KEY

Dipakai untuk token CSRF dan enkripsi rahasia MFA. Rotasi membatalkan token CSRF (pengguna memuat ulang halaman) **dan membuat rahasia MFA tidak dapat dibuka**: sebelum rotasi, catat bahwa semua staf ber-MFA harus didaftarkan ulang (reset MFA oleh admin), atau tunda rotasi. Langkah: ganti nilai di env file, restart semua replika.

## NIK_ENC_KEY (enkripsi NIK)

Kolom `key_version` per baris memungkinkan rotasi bertahap. Perkakas otomatis **belum dibuat**; prosedur:
1. Tambahkan dukungan multi-kunci (kunci lama + baru, `NIK_ENC_KEY_VERSION` baru) di konfigurasi.
2. Baris baru ditulis dengan versi baru; backfill batch kecil: dekripsi versi lama -> enkripsi versi baru -> update per baris dengan audit jumlah.
3. Setelah semua baris versi baru dan backup lama melewati retensi, hapus kunci lama.

## NIK_INDEX_KEY (blind index, kunci pencocokan)

Rotasi harus menjaga keunikan NIK sepanjang migrasi:
1. **Expand**: tambah kolom `nik_blind_index_next` + unique index parsial `(org_id, nik_blind_index_next) WHERE nik_blind_index_next IS NOT NULL`.
2. **Dual-write**: aplikasi menulis kedua indeks untuk pendaftaran baru; pengecekan duplikat memakai keduanya.
3. **Backfill tervalidasi**: batch per 1.000 baris (dekripsi NIK -> HMAC kunci baru). Bentrok unique = data ganda yang sebelumnya tidak terdeteksi: hentikan dan tangani manual.
4. **Verifikasi**: `count(*) WHERE nik_blind_index_next IS NULL` = 0.
5. **Cutover**: aplikasi membaca kolom baru; lalu **contract**: ganti nama kolom, hapus indeks lama pada rilis berikutnya.

## Kredensial database dan layanan

Role aplikasi, role migrasi, kredensial SMTP, dan kata sandi restic dirotasi lewat env file + restart. Kredensial tidak pernah masuk Git. Catat tanggal rotasi dan pelaku di register serah terima.
