# Panduan Pengguna per Peran

Semua layar berbahasa Indonesia, mendukung keyboard penuh dan layar ponsel. Waktu ditampilkan dalam zona organisasi (WIB).

## Kandidat

1. **Daftar** di `/daftar`: nama sesuai KTP, email, NIK (16 digit, disimpan terenkripsi, tampil tersamar), kata sandi minimal 10 karakter berisi huruf dan angka, persetujuan pemberitahuan privasi. Respons selalu "Pendaftaran diproses"; buka tautan verifikasi di email.
2. **Lengkapi profil** di *Profil*: telepon, tanggal lahir, kota, minimal satu pendidikan. Unggah CV/dokumen (PDF, DOCX, JPG, PNG, maks 5 MB). Dokumen bisa dibuka setelah lolos pemindaian keamanan.
3. **Melamar**: buka lowongan, tekan *Lamar posisi ini*, konfirmasi. Profil saat itu disalin ke lamaran; perubahan profil berikutnya tidak mengubah lamaran yang sudah terkirim. Satu lamaran per lowongan.
4. **Pantau** di *Lamaran saya* dan *Kotak masuk*. Lamaran aktif dapat *ditarik*; interview terjadwal ikut dibatalkan.
5. **Ujian** di menu *Ujian*: tekan *Mulai ujian* saat jadwal terbuka. Selama ujian:
   - Jawaban tersimpan otomatis. Label *Tersimpan pukul ...* berarti server sudah menerima.
   - *Menunggu koneksi* / *Jawaban hanya di perangkat*: jawaban belum diterima server. Tetap kerjakan; jawaban dikirim otomatis saat tersambung. Jangan menutup peramban.
   - Waktu dihitung server. Mengubah jam perangkat tidak menambah waktu.
   - Bila ada jawaban berbeda dari tab/perangkat lain, Anda diminta memilih versi yang dipakai.
   - *Selesai dan kirim* menampilkan jumlah soal belum dijawab dan ditandai ragu. Setelah dikirim, Anda mendapat **bukti penerimaan** (kode, waktu, jumlah dijawab). Simpan kodenya.
   - Bila waktu habis saat offline, jawaban terakhir yang diterima server yang dinilai. Ajukan keberatan ke tim rekrutmen dengan menyebut kode bukti.
   - Di perangkat bersama: keluar (logout) setelah selesai; draft lokal dihapus saat logout.

## Recruiter

- *Lamaran*: daftar lamaran dalam unit/lowongan yang ditugaskan (filter status). Lamaran di luar akses tidak terlihat.
- *Detail lamaran*: data saat melamar, dokumen, histori tahap (pelaku, waktu, alasan), interview, scorecard, lamaran lain kandidat yang masih dalam akses Anda.
- Tindakan: pindah tahap sesuai workflow (dengan catatan), *Tolak* (alasan wajib), *Jadwalkan interview* (zona waktu, interviewer; bentrok jadwal ditolak), *Jadwalkan ulang* / *Batalkan* (alasan wajib, kandidat diberi tahu), *Kirim pemberitahuan keputusan* setelah keputusan akhir (sekali).
- Bila muncul "sudah diubah orang lain", muat ulang lalu ulangi.

## HR Admin

Semua kemampuan recruiter dalam unitnya, ditambah:
- *Lowongan*: buat draft (boleh dari permintaan tenaga kerja yang disetujui), terbitkan, tutup.
- *Permintaan tenaga kerja*: setujui/tolak (tidak untuk permintaan sendiri).
- *Ujian*: undang peserta dari lamaran di tahap Ujian (akomodasi waktu beralasan), pantau, *Rilis nilai* (memicu perpindahan tahap otomatis sesuai workflow).
- *Workflow*: ubah draft, validasi, simulasi, terbitkan, aktifkan kembali versi lama (untuk lamaran baru).
- *Laporan*: funnel per kohort, ekspor funnel; ekspor data pelamar (izin khusus, tidak tersedia saat Redis mati).
- *Tampilkan NIK* di detail lamaran: wajib alasan, tercatat di audit.
- *Audit*: catatan dalam unitnya.

## Hiring Manager

- Melihat lamaran lowongan yang ditugaskan, mengisi *penilaian interview* bila menjadi interviewer, *Setujui / Tidak setujui* pada tahap Persetujuan (tidak dapat menyetujui lamaran yang ia pindahkan sendiri ke tahap tersebut).
- Mengajukan *permintaan tenaga kerja*.

## Examiner

- *Bank soal*: buat bank, impor CSV dengan pratinjau (baris bermasalah ditandai), terbitkan versi paket. Perubahan soal membuat versi baru; ujian berjalan tidak terpengaruh.
- *Ujian*: monitoring jadwal; *Penilaian esai* memakai kode peserta (identitas disembunyikan). Regrade setelah rilis wajib alasan dan tidak memindahkan tahap lagi.

## Super Admin

Lihat `admin-manual.md`.
