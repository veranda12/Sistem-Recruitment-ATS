# Template UAT

Isi oleh penguji pelanggan di lingkungan staging. Hasil: Lulus / Gagal / Tidak diuji. Untuk Gagal, cantumkan langkah, hasil aktual, kode permintaan (requestId dari pesan error), dan screenshot.

| No | Peran | Skenario | Langkah ringkas | Hasil diharapkan | Hasil | Catatan / requestId |
|---|---|---|---|---|---|---|
| 1 | HR Admin | Terbitkan lowongan | Lowongan -> Buat -> Terbitkan | Tampil di portal, filter unit/lokasi bekerja | | |
| 2 | Kandidat | Daftar dan verifikasi | /daftar -> email -> tautan | Email terverifikasi, NIK tampil tersamar | | |
| 3 | Kandidat | Profil sekali, dua lamaran | Lengkapi profil, lamar 2 lowongan | 2 lamaran, snapshot tidak berubah saat profil diubah | | |
| 4 | Recruiter | Proses lamaran | Pindah ke Ujian dengan catatan | Histori mencatat pelaku dan alasan | | |
| 5 | HR Admin | Undang ujian | Ujian -> jadwal -> Undang | Kandidat menerima undangan | | |
| 6 | Kandidat | Ujian di ponsel dengan koneksi putus | Mulai, jawab, mode pesawat, jawab, sambung lagi, kirim | Label "hanya di perangkat" lalu "tersimpan"; bukti penerimaan | | |
| 7 | Examiner | Nilai esai | Penilaian esai | Identitas tersembunyi, nilai akhir dihitung | | |
| 8 | HR Admin | Rilis nilai | Rilis nilai | Lamaran berpindah: lulus -> Interview, kurang -> Ditolak, esai pending -> Review | | |
| 9 | Recruiter | Interview | Jadwalkan, jadwal ulang (zona WITA), batalkan | Bentrok ditolak; kandidat diberi tahu waktu lama/baru | | |
| 10 | Hiring Manager | Persetujuan | Isi penilaian, Setujui | Status Diterima | | |
| 11 | Recruiter | Pemberitahuan keputusan | Kirim pemberitahuan | Kandidat menerima "Hasil seleksi", tidak bisa dikirim dua kali | | |
| 12 | Kandidat | Tarik lamaran | Tarik lamaran | Status Ditarik, interview dibatalkan | | |
| 13 | HR Admin | Audit | Audit -> application. | Pelaku, tindakan, alasan, waktu tercatat | | |
| 14 | Super Admin | Akses | Cabut peran recruiter | Recruiter ditolak pada permintaan berikutnya | | |
| 15 | Super Admin | MFA | Aktifkan verifikasi dua langkah | Login meminta kode; kode pemulihan bekerja sekali | | |
| 16 | HR Admin | Laporan | Laporan + ekspor funnel | Angka sama dengan dashboard; tautan unduh kedaluwarsa | | |
| 17 | Semua | Aksesibilitas | Navigasi keyboard, zoom 200%, ponsel 360 px | Semua tindakan terjangkau, tanpa scroll horizontal | | |

Penandatanganan: nama, peran, tanggal, keputusan (diterima / diterima dengan catatan / ditolak).
