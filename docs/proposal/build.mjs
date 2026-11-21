// Menyusun dokumen "Preview produk dan proposal implementasi" (HTML -> PDF A4 lanskap).
// Prasyarat: screenshot di docs/proposal/img (node docs/proposal/screenshots.mjs). Jalankan: node docs/proposal/build.mjs
import { createRequire } from "node:module";
import { writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const require = createRequire(join(here, "../../frontend/package.json"));
const { chromium } = require("playwright");

const BRAND = '<span class="b1">Rekrut</span><span class="b2">Flow</span>';
const FOOT = "Preview produk dan proposal implementasi · November 2026";
const ORG = "PT Arunika Daya Persada";

const esc = (s) => s; // konten ditulis tangan di file ini, sudah berupa HTML aman

// ---------- Konten modul ----------
const modules = [
  // Kandidat
  { eyebrow: "KANDIDAT · PORTAL KARIER", title: "Lowongan terbuka tampil di portal karier dan bisa dicari tanpa login.", img: "01-portal", cap: "Portal karier dengan pencarian lowongan. Data demo.",
    blocks: [["YANG TERLIHAT", "Judul lowongan, unit, lokasi, jenis kerja, dan batas lamaran. Hanya lowongan berstatus terbit yang tampil."], ["PENCARIAN", "Kata kunci, unit, lokasi, dan jenis kerja. Di data demo tercatat 3 lowongan terbuka."], ["MANFAAT", "Kandidat menemukan posisi yang sesuai lebih dulu, lalu membuat akun hanya saat ingin melamar."]] },
  { eyebrow: "KANDIDAT · DETAIL LOWONGAN", title: "Detail lowongan menjelaskan pekerjaan, kualifikasi, dan batas lamaran.", img: "02-lowongan", cap: "Detail lowongan Operator Produksi. Data demo.",
    blocks: [["ISI", "Deskripsi pekerjaan, kualifikasi, unit, lokasi kerja, dan jenis kontrak."], ["STATUS", "Label Menerima lamaran tampil selama lowongan terbuka. Setelah ditutup, tombol lamar hilang dan statusnya tertulis jelas."], ["MELAMAR", "Satu klik dari profil yang sudah lengkap. Data profil disalin ke lamaran saat dikirim."]] },
  { eyebrow: "KANDIDAT · PENDAFTARAN", title: "Satu akun kandidat untuk semua lamaran, dengan NIK yang disimpan terenkripsi.", img: "03-daftar", cap: "Form pendaftaran akun kandidat. Data contoh.",
    blocks: [["YANG DIINPUT", "Nama sesuai KTP, email, NIK 16 digit, kata sandi, dan persetujuan pemberitahuan privasi."], ["VERIFIKASI EMAIL", "Tautan verifikasi dikirim ke email. Akun baru bisa dipakai setelah tautan dibuka."], ["NIK", "Disimpan terenkripsi dan ditampilkan tersamar. Satu NIK hanya untuk satu akun, sehingga pendaftaran ganda tertahan."]] },
  { eyebrow: "KANDIDAT · PROFIL", title: "Profil diisi sekali dan dipakai untuk setiap lamaran.", img: "04-profil", cap: "Profil kandidat, bagian identitas. Data demo.",
    blocks: [["IDENTITAS", "Nama, nomor telepon, tanggal lahir, kota domisili, alamat, dan ringkasan diri."], ["PENDIDIKAN DAN PENGALAMAN", "Riwayat pendidikan dengan IPK opsional, serta pengalaman kerja dengan periode dan uraian tugas."], ["SALINAN LAMARAN", "Lamaran yang sudah terkirim menyimpan salinan profil saat itu. Perubahan profil berikutnya tidak mengubah lamaran lama."]] },
  { eyebrow: "KANDIDAT · DOKUMEN", title: "Dokumen diunggah ke profil dan diperiksa sebelum bisa dibuka.", img: "05-dokumen", cap: "Bagian pengalaman kerja dan dokumen pada profil. Data demo.",
    blocks: [["FORMAT", "PDF, DOCX, JPG, atau PNG, maksimal 5 MB per berkas. Jenis dokumen dipilih saat unggah, misalnya CV."], ["PEMERIKSAAN", "Jenis asli berkas dicek, lalu berkas dipindai. Selama belum lolos, berkas tidak dapat diunduh siapa pun."], ["AKSES", "Recruiter hanya dapat mengunduh dokumen kandidat pada lowongan yang ditugaskan, lewat tautan yang berlaku singkat."]] },
  { eyebrow: "KANDIDAT · LAMARAN SAYA", title: "Kandidat melihat status setiap lamaran dari satu halaman.", img: "06-lamaran-saya", cap: "Daftar lamaran milik kandidat. Data demo.",
    blocks: [["YANG TERLIHAT", "Lowongan, tanggal kirim, tahap saat ini, dan status: Aktif, Diterima, Tidak lolos, atau Ditarik."], ["TARIK LAMARAN", "Lamaran aktif dapat ditarik dengan alasan. Jadwal interview yang terkait dibatalkan otomatis."], ["MANFAAT", "Kandidat tidak perlu menanyakan status ke HR satu per satu."]] },
  { eyebrow: "KANDIDAT · RIWAYAT LAMARAN", title: "Riwayat lamaran menunjukkan tahap yang sudah dilalui beserta waktunya.", img: "07-timeline", cap: "Riwayat lamaran Operator Produksi milik Rina Amelia, status Diterima. Data demo.",
    blocks: [["RIWAYAT", "Setiap tahap tampil dengan tanggal dan jam, dari Screening sampai keputusan akhir."], ["CATATAN INTERNAL", "Alasan perpindahan dan nama petugas hanya terlihat oleh staf, tidak ditampilkan ke kandidat."], ["CONTOH DATA DEMO", "Screening, Ujian, Interview, Persetujuan, lalu Diterima."]] },
  { eyebrow: "KANDIDAT · KOTAK MASUK", title: "Setiap pemberitahuan email juga tersimpan di kotak masuk akun.", img: "08-kotak-masuk", cap: "Kotak masuk kandidat. Data demo.",
    blocks: [["ISI", "Jadwal interview, perubahan jadwal atau pembatalan, perubahan tahap, rilis nilai, dan hasil seleksi."], ["PENGIRIMAN", "Email dikirim lewat antrean dan dicoba ulang bila gagal. Status pengiriman terlihat oleh admin di menu Komunikasi."], ["MANFAAT", "Kandidat tetap bisa membaca pemberitahuan walau email masuk ke folder spam."]] },
  // Ujian
  { eyebrow: "UJIAN ONLINE · UNDANGAN", title: "Undangan ujian menampilkan paket, durasi, dan jendela waktu pengerjaan.", img: "09-undangan-ujian", cap: "Daftar undangan ujian kandidat. Data demo.",
    blocks: [["YANG TERLIHAT", "Nama paket, lowongan, gelombang, durasi, jendela waktu dibuka sampai ditutup, dan status."], ["MULAI", "Waktu dihitung oleh server sejak tombol Mulai ditekan. Membuka ulang melanjutkan ujian yang sama, bukan membuat ujian baru."], ["AKOMODASI", "Tambahan waktu dapat diberikan per peserta dengan alasan, dan tercatat di audit."]] },
  { eyebrow: "UJIAN ONLINE · PONSEL", title: "Ujian dapat dikerjakan di ponsel, dengan daftar soal di panel terpisah.", imgs: ["14-ujian-ponsel", "15-ujian-ponsel-navigator"], cap: "Ruang ujian di layar ponsel 390 px dan panel daftar soal. Data demo.",
    blocks: [["STATUS SIMPAN", "Bagian atas menampilkan waktu tersisa dan status, misalnya Tersimpan pukul 13.40. Jawaban disimpan otomatis tanpa tombol simpan."], ["SOAL ACAK", "Urutan soal dan pilihan diacak per peserta dan tetap sama saat halaman dibuka ulang. Kunci jawaban tidak pernah dikirim ke perangkat peserta."], ["NAVIGASI", "Soal yang sudah dijawab dan yang ditandai ragu-ragu terlihat di daftar soal."]] },
  { eyebrow: "UJIAN ONLINE · KONEKSI TERPUTUS", title: "Saat koneksi terputus, jawaban tersimpan di perangkat dan dikirim saat tersambung.", img: "11-ujian-offline", cap: "Ruang ujian ketika koneksi terputus. Data demo.",
    blocks: [["TANDA JELAS", "Bagian atas menampilkan Menunggu koneksi, dan soal yang belum diterima server ditandai Hanya di perangkat."], ["KIRIM ULANG OTOMATIS", "Jawaban disimpan di browser lalu dikirim ulang dengan kode yang sama, sehingga tidak tercatat ganda di server."], ["WAKTU", "Waktu ujian tetap berjalan menurut server, sehingga tidak bisa ditambah dengan memutus koneksi."]] },
  { eyebrow: "UJIAN ONLINE · KIRIM JAWABAN", title: "Sebelum mengirim, peserta melihat jumlah soal yang sudah dan belum dijawab.", img: "12-konfirmasi-kirim", cap: "Konfirmasi pengiriman jawaban. Data demo.",
    blocks: [["KONFIRMASI", "Jumlah soal dijawab dan belum dijawab tampil sebelum peserta menekan Kirim jawaban."], ["SEKALI KIRIM", "Pengiriman hanya diterima satu kali. Klik ganda atau pengiriman ulang tidak membuat jawaban kedua."], ["WAKTU HABIS", "Bila peserta tidak mengirim, jawaban yang sudah diterima server sebelum batas waktu tetap dinilai."]] },
  { eyebrow: "UJIAN ONLINE · BUKTI PENERIMAAN", title: "Peserta menerima bukti penerimaan jawaban dengan kode dan sidik jawaban.", img: "13-bukti-penerimaan", cap: "Bukti penerimaan jawaban setelah dikirim. Data demo.",
    blocks: [["ISI", "Kode bukti, waktu diterima, jumlah soal dijawab, dan sidik jawaban."], ["SIDIK JAWABAN", "Dihitung dari jawaban akhir yang tersimpan di database, dan dapat dicocokkan ulang, termasuk setelah data dipulihkan dari backup."], ["MANFAAT", "Menjadi pegangan bersama bila ada keberatan peserta tentang jawabannya."]] },
  // Recruiter
  { eyebrow: "RECRUITER · DAFTAR LAMARAN", title: "Recruiter hanya melihat lamaran pada unit atau lowongan yang ditugaskan.", img: "16-daftar-lamaran", cap: "Daftar lamaran untuk recruiter unit Operasional Pabrik. Data demo.",
    blocks: [["KOLOM", "Kandidat, lowongan, tahap, status, dan tanggal kirim, dengan filter status."], ["CAKUPAN", "Recruiter unit Operasional Pabrik tidak melihat lamaran Backend Engineer di unit Teknologi Informasi. Alamat lamaran di luar cakupan juga tidak dapat dibuka langsung."], ["MANFAAT", "Data pelamar hanya terbuka bagi petugas yang menanganinya."]] },
  { eyebrow: "RECRUITER · DETAIL LAMARAN", title: "Detail lamaran memuat data saat melamar, riwayat tahap, dokumen, dan interview.", img: "17-detail-lamaran", cap: "Detail lamaran Andi Prasetyo pada tahap Persetujuan. Data demo.",
    blocks: [["DATA SAAT MELAMAR", "Salinan profil saat kandidat melamar. NIK tampil tersamar."], ["RIWAYAT TAHAP", "Setiap perpindahan mencatat petugas, waktu, dan alasan. Perpindahan otomatis dari hasil ujian tercatat sebagai Sistem."], ["INTERVIEW", "Jadwal, interviewer, tombol jadwal ulang, batal, dan isi penilaian, serta ringkasan penilaian interviewer."]] },
  { eyebrow: "RECRUITER · JADWAL INTERVIEW", title: "Jadwal interview dibuat dengan zona waktu dan pengecekan bentrok interviewer.", img: "19-jadwal-interview", cap: "Form jadwal interview. Data demo.",
    blocks: [["YANG DIINPUT", "Tanggal dan jam, zona waktu, durasi, jenis (tatap muka atau video), lokasi atau tautan, dan interviewer."], ["BENTROK", "Interviewer yang sudah terjadwal di jam yang sama ditolak, termasuk bila dua recruiter menyimpan pada saat bersamaan."], ["PEMBERITAHUAN", "Kandidat menerima email dan pesan di kotak masuk, juga saat jadwal diubah atau dibatalkan."]] },
  // HR Admin
  { eyebrow: "HR ADMIN · LOWONGAN", title: "Lowongan dibuat, diterbitkan, dan ditutup dari satu daftar.", img: "20-lowongan-staf", cap: "Daftar lowongan untuk HR Admin. Data demo.",
    blocks: [["YANG TERLIHAT", "Judul, unit, lokasi, jenis kerja, jumlah lamaran, batas lamaran, dan status Terbit atau Ditutup."], ["BUAT LOWONGAN", "Workflow seleksi dan paket ujian dipilih saat membuat lowongan. Lowongan dapat dibuat dari permintaan tenaga kerja yang disetujui."], ["TUTUP", "Lowongan yang ditutup hilang dari portal. Lamaran yang sudah masuk tetap diproses."]] },
  { eyebrow: "HR ADMIN · PERMINTAAN TENAGA KERJA", title: "Permintaan tenaga kerja diajukan hiring manager dan disetujui HR.", img: "21-permintaan", cap: "Daftar permintaan tenaga kerja. Data demo.",
    blocks: [["YANG DIINPUT", "Posisi, unit, lokasi, jumlah orang, jenis kerja, dan alasan kebutuhan."], ["PERSETUJUAN", "HR menyetujui atau menolak dengan catatan. Pengaju tidak dapat menyetujui permintaannya sendiri."], ["CONTOH DATA DEMO", "Operator Forklift 1 orang sudah disetujui. Teknisi Perawatan Mesin 2 orang menunggu persetujuan."]] },
  { eyebrow: "HR ADMIN · MONITORING UJIAN", title: "Monitoring ujian memisahkan jawaban yang diterima server dari sinyal perangkat.", img: "22-monitoring-ujian", cap: "Monitoring gelombang ujian November. Data demo.",
    blocks: [["KOLOM", "Peserta, status, jawaban diterima server, terakhir aktif, nilai, dan status rilis. Peserta dengan tambahan waktu ditandai."], ["TERAKHIR AKTIF", "Berasal dari sinyal perangkat dan bersifat perkiraan. Jumlah jawaban hanya menghitung yang sudah tersimpan di server."], ["RILIS NILAI", "Nilai dirilis oleh HR. Setelah rilis, lamaran berpindah tahap otomatis sesuai aturan workflow."]] },
  { eyebrow: "HR ADMIN · LAPORAN", title: "Laporan funnel per lowongan dihitung dari riwayat tahap sesuai periode.", img: "23-laporan", cap: "Laporan rekrutmen. Data demo dibuat dalam beberapa menit, sehingga waktu per tahap tercatat 0 jam.",
    blocks: [["RINGKASAN", "Jumlah lamaran, kandidat unik, aktif, ditarik, dan diterima dalam periode yang dipilih."], ["FUNNEL DAN WAKTU", "Jumlah lamaran yang pernah mencapai tiap tahap, serta median dan rata-rata lama per tahap, menurut zona waktu perusahaan."], ["EKSPOR CSV", "Funnel dan data pelamar, hanya untuk data dalam cakupan pengguna. Isi sel yang berisiko dinetralkan agar aman dibuka di Excel."]] },
  { eyebrow: "HR ADMIN · BUKA NIK", title: "NIK lengkap hanya dibuka dengan alasan, dan setiap aksesnya tercatat.", img: "24-reveal-nik", cap: "Dialog Tampilkan NIK dengan alasan. Data demo.",
    blocks: [["TERSAMAR", "Di seluruh layar, NIK tampil tersamar dengan empat digit terakhir."], ["BUKA DENGAN ALASAN", "Hanya peran yang diberi izin. Alasan minimal 10 karakter wajib diisi."], ["TERCATAT", "Setiap pembukaan NIK masuk ke audit bersama nama petugas, waktu, dan alasannya."]] },
  { eyebrow: "HR ADMIN · AUDIT", title: "Audit mencatat siapa melakukan apa, kapan, pada data apa, dan alasannya.", img: "29-audit", cap: "Catatan audit. Data demo.",
    blocks: [["KOLOM", "Waktu, pelaku, tindakan, target, hasil, dan alasan."], ["HANYA BACA", "Catatan audit tidak dapat diubah atau dihapus dari aplikasi, dan database menolak perubahan pada tabel audit."], ["PENCARIAN", "Saring berdasarkan awalan tindakan, misalnya application, identity.reveal, atau admin."]] },
  // Hiring manager
  { eyebrow: "HIRING MANAGER · PERSETUJUAN", title: "Hiring manager menyetujui kandidat dari halaman lamaran.", img: "25-persetujuan-hm", cap: "Persetujuan kandidat Andi Prasetyo oleh hiring manager. Data demo.",
    blocks: [["AKSES", "Hanya lamaran pada lowongan yang menjadi tanggung jawabnya."], ["KONFIRMASI", "Setujui atau Tidak setujui, dengan catatan. Tindakan tercatat bersama nama dan alasannya."], ["ATURAN", "Jumlah persetujuan minimum diatur di workflow. Persetujuan untuk lamaran yang kemudian ditarik tidak dihitung."]] },
  // Examiner
  { eyebrow: "EXAMINER · BANK SOAL", title: "Bank soal dikelola per kategori, dan soal dapat diimpor dari CSV dengan pratinjau.", img: "26-bank-soal", cap: "Bank soal, impor CSV, dan penerbitan versi paket. Data demo.",
    blocks: [["JENIS SOAL", "Pilihan tunggal, pilihan jamak, dan esai, masing-masing dengan bobot. Esai memakai rubrik."], ["IMPOR CSV", "Pratinjau menandai baris yang salah sebelum soal disimpan, misalnya kunci di luar pilihan."], ["VERSI PAKET", "Paket yang sudah terbit tidak dapat diubah. Ujian yang sedang berjalan tetap memakai versinya."]] },
  { eyebrow: "EXAMINER · PENILAIAN ESAI", title: "Esai dinilai tanpa melihat identitas peserta, sesuai rubrik.", img: "27-penilaian-esai", cap: "Penilaian esai dengan kode peserta. Data demo.",
    blocks: [["PENILAIAN BUTA", "Peserta tampil sebagai kode, misalnya P-6184EB68. Nama dan data kontak tidak terlihat."], ["PENILAIAN ULANG", "Nilai yang sudah dirilis dapat dinilai ulang dengan alasan, tanpa memindahkan tahap lamaran untuk kedua kalinya."], ["PILIHAN GANDA", "Dinilai otomatis. Pilihan jamak dianggap benar bila seluruh pilihan cocok dengan kunci."]] },
  // Super admin
  { eyebrow: "SUPER ADMIN · WORKFLOW", title: "Tahapan rekrutmen diatur sebagai workflow yang divalidasi sebelum terbit.", img: "28-workflow", cap: "Editor workflow Rekrutmen Standar. Data demo.",
    blocks: [["TAHAPAN", "Screening, ujian dengan paket tertentu, review penilaian, interview, persetujuan, lalu diterima atau ditolak."], ["VALIDASI", "Workflow diperiksa sebelum terbit, misalnya tahap yang tidak dapat dicapai atau transisi tanpa jalur default."], ["VERSI", "Lamaran memakai versi workflow saat kandidat melamar. Versi baru hanya berlaku untuk lamaran baru."]] },
  { eyebrow: "SUPER ADMIN · SIMULASI", title: "Simulasi menunjukkan jalur lamaran sebelum aturan diterbitkan.", img: "28b-simulasi", cap: "Transisi bersyarat dan simulasi nilai ujian 68. Data demo.",
    blocks: [["SYARAT TRANSISI", "Contoh: hasil persetujuan sama dengan approved menuju Diterima, selain itu ke Ditolak sebagai jalur default."], ["SIMULASI", "Nilai 68 di bawah ambang 75 menghasilkan jalur Screening, Ujian, lalu Ditolak. Data tidak berubah."], ["VERSI TERBIT", "Daftar versi yang sudah terbit, dengan penerbit, waktu, dan catatan."]] },
  { eyebrow: "SUPER ADMIN · PENGGUNA", title: "Akun staf diberi peran dengan cakupan unit atau lowongan.", img: "30-pengguna", cap: "Daftar pengguna dan penugasan peran. Data demo.",
    blocks: [["PERAN DAN CAKUPAN", "Contoh: Hiring Manager untuk lowongan Operator Produksi dan Supervisor Quality Control, Recruiter untuk unit Operasional Pabrik."], ["TINDAKAN", "Beri peran, cabut peran, reset verifikasi dua langkah, dan nonaktifkan akun. Sesi akun yang dinonaktifkan langsung berakhir."], ["PENGAMAN", "Admin terakhir tidak dapat dinonaktifkan atau dicabut perannya."]] },
  { eyebrow: "SUPER ADMIN · PENGATURAN", title: "Kebijakan perusahaan disimpan berversi, dan yang belum diputuskan ditandai.", img: "31-pengaturan", cap: "Pengaturan profil organisasi dan kebijakan. Data demo.",
    blocks: [["PROFIL ORGANISASI", "Nama perusahaan, nama portal karier, dan deskripsi yang tampil di portal."], ["KEBIJAKAN", "Batas akomodasi waktu, kesempatan ujian, retensi data pelamar, dan batas sesi. Nilai yang belum diputuskan tertulis belum ditetapkan."], ["RIWAYAT", "Setiap perubahan dicatat beserta versi, waktu efektif, dan alasannya."]] },
  { eyebrow: "SUPER ADMIN · KOMUNIKASI", title: "Antrean email dan riwayat pengiriman terlihat oleh admin.", img: "32-komunikasi", cap: "Antrean dan riwayat pengiriman email. Data demo.",
    blocks: [["ANTREAN", "Jumlah pesan menunggu, gagal permanen, dan umur event tertua."], ["RIWAYAT", "Waktu, penerima, template, status, dan jumlah percobaan untuk setiap email."], ["MANFAAT", "Bila kandidat mengaku tidak menerima email, admin dapat memeriksa status pengirimannya."]] },
  { eyebrow: "SEMUA STAF · KEAMANAN AKUN", title: "Verifikasi dua langkah memakai aplikasi autentikator di ponsel.", img: "33-mfa", cap: "Aktivasi verifikasi dua langkah. Kunci pada gambar adalah data demo.",
    blocks: [["AKTIVASI", "Masukkan kunci ke aplikasi autentikator, lalu ketik kode 6 digit untuk memastikan perangkat sudah benar."], ["KODE CADANGAN", "Kode cadangan sekali pakai diberikan setelah aktivasi, untuk masuk bila ponsel hilang."], ["WAJIB PER PERAN", "Peran tertentu dapat diwajibkan memakai verifikasi dua langkah, misalnya Super Admin dan HR Admin."]] },
];

// ---------- Halaman ----------
const pages = [];
const add = (section, html, cls = "") => pages.push({ section, html, cls });

const header = (section) => `<header><div class="brand">${BRAND}</div><div class="sec">${section}</div></header>`;

// Hitung nomor halaman modul untuk daftar isi
const P_OVERVIEW = 3, P_FLOW = 4, P_ACCESS = 5, P_MOD0 = 6;
const P_MODN = P_MOD0 + modules.length - 1;
const P_RELI = P_MODN + 1, P_EXAMPLE = P_RELI + 1, P_PROP0 = P_EXAMPLE + 1, P_PROPN = P_PROP0 + 2;
const firstOf = (prefix) => P_MOD0 + modules.findIndex((m) => m.eyebrow.startsWith(prefix));
const lastOf = (prefix) => P_MOD0 + modules.length - 1 - [...modules].reverse().findIndex((m) => m.eyebrow.startsWith(prefix));
const range = (p) => { const a = firstOf(p), b = lastOf(p); return a === b ? `${a}` : `${a} sampai ${b}`; };

// 1. Sampul
pages.push({ cover: true, html: `
<div class="cover">
  <div class="cv-left">
    <div class="brand big">${BRAND}</div>
    <div class="bar"></div>
    <h1 class="hero">Rekrutmen tertata. Keputusan lebih cepat.</h1>
    <p class="lead">Satu sistem untuk lowongan, lamaran kandidat, ujian online, interview, persetujuan, dan laporan rekrutmen.</p>
  </div>
  <div class="cv-right"><div class="shot"><img src="img/00-sampul.png"></div></div>
  <div class="cv-foot"><b>Preview produk dan proposal implementasi</b><br>Disiapkan oleh Deni Maulana Shobri<br>November 2026</div>
</div>` });

// 2. Daftar isi
const toc = [
  ["Gambaran singkat sistem", `${P_OVERVIEW}`],
  ["Alur kerja antar peran", `${P_FLOW}`],
  ["Hak akses per peran", `${P_ACCESS}`],
  ["Modul per peran", `${P_MOD0} sampai ${P_MODN}`],
  ["Keandalan ujian dan perlindungan data", `${P_RELI}`],
  ["Contoh alur satu lamaran hingga keputusan", `${P_EXAMPLE}`],
  ["Proposal dan implementasi", `${P_PROP0} sampai ${P_PROPN}`],
];
add("Tentang dokumen ini", `
<div class="eyebrow">RINGKASAN</div>
<h2>Dokumen ini menjelaskan cara tiap peran memakai RekrutFlow, dari permintaan tenaga kerja sampai keputusan akhir.</h2>
<table class="toc">${toc.map(([t, p], i) => `<tr><td class="n">${i + 1}</td><td>${t}</td><td class="p">${p}</td></tr>`).join("")}</table>
<p class="tocnote">Kandidat ${range("KANDIDAT")}, Ujian online ${range("UJIAN")}, Recruiter ${range("RECRUITER")}, HR Admin ${range("HR ADMIN")}, Hiring Manager ${range("HIRING")}, Examiner ${range("EXAMINER")}, Super Admin ${firstOf("SUPER")} sampai ${lastOf("SEMUA")}.</p>
<p class="body">Setiap bagian menampilkan layar aplikasi yang sudah diperiksa langsung dengan data demo ${ORG}, organisasi fiktif untuk demonstrasi. Nama kandidat, NIK, nilai, dan jadwal pada screenshot adalah data demo, bukan data nyata. Screenshot diambil pada 21 November 2026.</p>`);

// 3. Gambaran singkat
add("1. Gambaran singkat sistem", `
<div class="eyebrow">GAMBARAN SINGKAT</div>
<h2>Satu sistem untuk portal karier, seleksi, ujian online, dan keputusan rekrutmen.</h2>
<p class="body w">RekrutFlow membantu tim HR mengelola rekrutmen dari permintaan tenaga kerja sampai keputusan akhir. Kandidat melamar dan mengerjakan ujian dari portal karier, sementara recruiter, hiring manager, dan examiner bekerja dari back office sesuai perannya. Setiap perpindahan tahap tercatat dengan petugas dan alasannya.</p>
<div class="cols3">
  <div class="col"><h3>Portal karier dan kandidat</h3><ul>
    <li>Lowongan terbuka dengan pencarian per unit, lokasi, dan jenis kerja</li>
    <li>Satu akun kandidat per NIK, dengan verifikasi email</li>
    <li>Profil dan dokumen diisi sekali untuk semua lamaran</li>
    <li>Status, riwayat lamaran, dan kotak masuk pemberitahuan</li></ul></div>
  <div class="col"><h3>Seleksi dan ujian online</h3><ul>
    <li>Workflow tahapan yang dapat diatur, disimulasikan, dan diberi versi</li>
    <li>Bank soal, impor CSV, dan paket ujian berversi</li>
    <li>Ujian di laptop atau ponsel dengan simpan otomatis dan bukti penerimaan</li>
    <li>Penilaian otomatis dan penilaian esai tanpa identitas peserta</li></ul></div>
  <div class="col"><h3>Kontrol dan laporan</h3><ul>
    <li>Peran dengan cakupan unit atau lowongan</li>
    <li>Jadwal interview dengan pengecekan bentrok interviewer</li>
    <li>Persetujuan hiring manager dan permintaan tenaga kerja</li>
    <li>Laporan funnel, ekspor CSV, audit, dan verifikasi dua langkah</li></ul></div>
</div>
<div class="stats">
  <div><b>6 peran</b> dengan menu dan cakupan data berbeda</div>
  <div>Jawaban ujian <b>tersimpan otomatis</b>, juga saat koneksi sempat terputus</div>
  <div>NIK <b>terenkripsi</b> dan hanya dibuka dengan alasan yang tercatat</div>
  <div>Uji beban <b>500 peserta</b> serentak tanpa jawaban hilang</div>
</div>`);

// 4. Alur kerja
const steps = [
  ["Hiring Manager", "Mengajukan permintaan tenaga kerja: posisi, unit, lokasi, jumlah orang, dan alasan.", "Permintaan yang disetujui HR."],
  ["HR Admin", "Membuat lowongan, memilih workflow dan paket ujian, lalu menerbitkannya ke portal.", "Lowongan tampil di portal karier."],
  ["Kandidat", "Mendaftar dengan NIK, melengkapi profil dan dokumen, lalu melamar.", "Lamaran dengan salinan profil saat melamar."],
  ["Recruiter", "Membuka lamaran dalam cakupannya, memeriksa berkas, lalu memindahkan ke tahap ujian.", "Kandidat terpilih diundang ujian."],
  ["Ujian", "Kandidat mengerjakan ujian. Examiner menilai esai, HR merilis nilai.", "Lamaran berpindah otomatis sesuai nilai."],
  ["Interview dan keputusan", "Recruiter menjadwalkan interview, interviewer mengisi penilaian, hiring manager menyetujui.", "Diterima atau ditolak, dengan pemberitahuan."],
];
add("2. Alur kerja antar peran", `
<div class="eyebrow">ALUR KERJA</div>
<h2>Dari permintaan tenaga kerja sampai keputusan dalam satu alur.</h2>
<div class="flow">${steps.map(([t, d, r], i) => `<div class="step"><div class="num">${i + 1}</div><h4>${t}</h4><p>${d}</p><div class="lbl">HASIL</div><p>${r}</p></div>${i < steps.length - 1 ? '<div class="arrow">&rarr;</div>' : ""}`).join("")}</div>
<div class="note"><b>Yang berjalan otomatis:</b> perpindahan tahap setelah nilai dirilis sesuai aturan workflow, penilaian soal pilihan, penilaian jawaban tersimpan saat waktu habis, pembatalan interview saat lamaran ditarik atau diputuskan, serta email dan pesan kotak masuk untuk setiap perubahan penting. <b>Yang dikerjakan pengguna:</b> persetujuan permintaan tenaga kerja, screening berkas, undangan ujian, penilaian esai, rilis nilai, jadwal dan penilaian interview, persetujuan kandidat, dan pengiriman pemberitahuan keputusan.</div>`);

// 5. Hak akses
const roles = [
  ["Super Admin", "7 menu: Lowongan (lihat), Workflow, Laporan, Pengguna, Pengaturan, Komunikasi, dan Audit", "Workflow, pengguna dan peran, pengaturan organisasi. Tidak membuka isi lamaran kandidat"],
  ["HR Admin", "8 menu: Lamaran, Lowongan, Permintaan tenaga kerja, Workflow, Ujian, Bank soal, Laporan, dan Audit", "Lowongan, tahap lamaran, workflow, jadwal ujian dan rilis nilai, persetujuan permintaan, buka NIK dengan alasan, dan ekspor data pelamar"],
  ["Recruiter", "3 menu: Lamaran, Lowongan (lihat), dan Permintaan tenaga kerja (lihat)", "Tahap lamaran dan jadwal interview, hanya untuk unit atau lowongan yang ditugaskan"],
  ["Hiring Manager", "2 menu: Lamaran dan Permintaan tenaga kerja", "Permintaan tenaga kerja, penilaian interview, dan persetujuan kandidat pada lowongannya"],
  ["Examiner", "2 menu: Ujian dan Bank soal", "Bank soal, paket ujian, dan penilaian esai. Tanpa akses NIK dan data kontak kandidat"],
  ["Kandidat", "Portal karier: Lowongan, Lamaran saya, Ujian, Profil, dan Kotak masuk", "Profil, dokumen, dan lamaran milik sendiri, serta jawaban ujian selama waktu berjalan"],
];
add("3. Hak akses per peran", `
<div class="eyebrow">HAK AKSES</div>
<h2>Setiap peran melihat dan mengubah data sesuai tugasnya.</h2>
<table class="grid"><thead><tr><th style="width:16%">PERAN</th><th style="width:40%">MENU YANG TAMPIL</th><th>BISA MENGUBAH</th></tr></thead>
<tbody>${roles.map(([a, b, c]) => `<tr><td class="strong">${a}</td><td>${b}</td><td>${c}</td></tr>`).join("")}</tbody></table>
<div class="note">Menu yang tidak diizinkan tidak tampil di navigasi. Bila alamat lamaran di luar cakupan dibuka langsung, sistem menjawab data tidak ditemukan. Semua staf memiliki menu Keamanan akun untuk verifikasi dua langkah. Pembagian peran dan cakupan dapat disesuaikan saat implementasi.</div>`);

// 6. Modul
for (const m of modules) {
  const visual = m.imgs
    ? `<div class="phones">${m.imgs.map((i) => `<div class="phone"><img src="img/${i}.png"></div>`).join("")}</div>`
    : `<div class="shot"><img src="img/${m.img}.png"></div>`;
  add("4. Modul per peran", `
<div class="eyebrow">${m.eyebrow}</div>
<h2>${esc(m.title)}</h2>
<div class="mod"><figure>${visual}<figcaption>${m.cap}</figcaption></figure>
<div class="side">${m.blocks.map(([h, t]) => `<div class="blk"><div class="eyebrow sm">${h}</div><p>${t}</p></div>`).join("")}</div></div>`);
}

// 7. Keandalan
const lt = [
  ["Masuk (login)", "500", "53 ms"], ["Mulai ujian", "500", "10 ms"], ["Melanjutkan ujian", "4.192", "3 ms"],
  ["Simpan jawaban otomatis", "62.080", "6 ms"], ["Sinyal aktif", "23.674", "2 ms"], ["Kirim jawaban", "500", "9 ms"],
];
add("5. Keandalan dan perlindungan data", `
<div class="eyebrow">KEANDALAN UJIAN</div>
<h2>Diuji dengan 500 peserta serentak, lalu dicocokkan langsung dengan database.</h2>
<div class="two">
  <div>
    <table class="grid"><thead><tr><th>OPERASI</th><th class="r">JUMLAH</th><th class="r">95% SELESAI DALAM</th></tr></thead>
    <tbody>${lt.map(([a, b, c]) => `<tr><td>${a}</td><td class="r">${b}</td><td class="r">${c}</td></tr>`).join("")}</tbody></table>
    <p class="small">Simulasi 500 peserta mengerjakan ujian 25 menit: 91.446 permintaan, 0 gagal. Setiap jawaban yang dinyatakan tersimpan (62.393) ditemukan di database, setiap peserta tepat satu pengiriman, dan sidik jawaban seluruh bukti penerimaan cocok.</p>
  </div>
  <div class="side">
    <div class="blk"><div class="eyebrow sm">PERLINDUNGAN DATA</div><p>NIK dienkripsi dan dicari lewat indeks tersendiri tanpa menyimpan NIK terbuka. Sandi disimpan dengan Argon2id. Dokumen dipindai sebelum bisa diunduh.</p></div>
    <div class="blk"><div class="eyebrow sm">CAKUPAN AKSES</div><p>Akses ditolak secara bawaan dan dibuka per peran serta unit atau lowongan. Setiap tindakan penting masuk audit yang tidak dapat diubah.</p></div>
    <div class="blk"><div class="eyebrow sm">BACKUP DAN PEMULIHAN</div><p>Prosedur backup dan latihan pemulihan sudah dijalankan: dokumen, riwayat workflow, dan sidik bukti penerimaan ujian tetap cocok setelah dipulihkan.</p></div>
  </div>
</div>
<div class="note"><b>Catatan:</b> uji beban dijalankan di satu laptop pengembang (Intel Core i5 6 core, RAM 16 GB) dengan aplikasi, database, dan generator beban pada mesin yang sama. Angka ini menunjukkan perilaku sistem, bukan jaminan kapasitas produksi. Kapasitas untuk server Anda diukur ulang saat implementasi. Dokumen ini tidak menyatakan sertifikasi atau kepatuhan standar tertentu.</div>`);

// 8. Contoh alur
const ex = [
  ["1. Permintaan", "Hiring manager mengajukan 1 Operator Produksi untuk lini 2. HR menyetujui dengan catatan anggaran."],
  ["2. Lowongan", "HR membuat lowongan Operator Produksi Karawang dengan workflow Rekrutmen Standar dan paket Tes Dasar Operasional, lalu menerbitkannya."],
  ["3. Melamar", "Rina mendaftar dengan NIK, memverifikasi email, melengkapi profil, mengunggah CV, lalu melamar. Profil disalin ke lamaran."],
  ["4. Screening", "Recruiter Operasional Pabrik memeriksa berkas dan memindahkan lamaran ke tahap Ujian dengan alasan."],
  ["5. Ujian", "Rina diundang ke gelombang November, mengerjakan 6 soal dalam 30 menit, lalu menerima kode bukti penerimaan."],
  ["6. Penilaian", "Soal pilihan dinilai otomatis. Examiner menilai esai tanpa melihat nama. HR merilis nilai, dan lamaran berpindah ke Interview karena nilai di atas ambang 75."],
  ["7. Interview", "Recruiter menjadwalkan interview video dengan hiring manager. Rina menerima email jadwal. Interviewer mengisi penilaian 5 dari 5, rekomendasi lanjut."],
  ["8. Keputusan", "Hiring manager menyetujui. Lamaran berstatus Diterima, pemberitahuan keputusan dikirim, dan seluruh langkah tercatat di audit dan laporan funnel."],
];
const half = Math.ceil(ex.length / 2);
const exTable = (rows) => `<table class="grid"><thead><tr><th style="width:30%">TAHAP</th><th>YANG TERJADI</th></tr></thead><tbody>${rows.map(([a, b]) => `<tr><td class="strong">${a}</td><td>${b}</td></tr>`).join("")}</tbody></table>`;
add("6. Contoh alur satu lamaran", `
<div class="eyebrow">ILUSTRASI</div>
<h2>Satu lamaran Operator Produksi, dari permintaan sampai keputusan.</h2>
<p class="body">Nama kandidat, nilai, dan jadwal pada contoh ini adalah ilustrasi.</p>
<div class="two eq">${exTable(ex.slice(0, half))}${exTable(ex.slice(half))}</div>
<div class="note"><b>Catatan:</b> contoh ini ilustrasi. Nama dan angka adalah contoh, bukan data nyata. Screening, undangan ujian, penilaian esai, rilis nilai, jadwal interview, dan persetujuan dilakukan pengguna lewat tombol dan form.</div>`);

// 9-11. Proposal
const scope = [
  ["Pemetaan alur rekrutmen", "Pemetaan tahapan seleksi per jenis posisi, syarat perpindahan tahap, dan persetujuan, lalu dituangkan menjadi workflow."],
  ["Organisasi dan peran", "Penyiapan unit, lokasi, akun staf, peran, dan cakupan unit atau lowongan untuk setiap pengguna."],
  ["Portal karier", "Profil perusahaan dan nama portal, isi pemberitahuan privasi, serta format lowongan sesuai kebutuhan."],
  ["Ujian dan bank soal", "Migrasi soal dari dokumen yang ada lewat impor CSV, penyusunan paket, durasi, ambang lulus, dan kebijakan akomodasi waktu."],
  ["Email dan pemberitahuan", "Penyesuaian isi email dan koneksi ke server email perusahaan."],
  ["Kebijakan data", "Penetapan masa simpan data pelamar, peran yang boleh membuka NIK dan mengekspor data, serta peran yang wajib verifikasi dua langkah."],
  ["Server dan keamanan", "Pemasangan di server Anda atau cloud pilihan Anda, HTTPS, backup terjadwal, latihan pemulihan, dan pemantauan."],
  ["Uji kapasitas", "Uji beban ulang di server Anda sesuai jumlah peserta ujian serentak yang direncanakan."],
  ["Pengujian bersama pengguna", "Tim Anda menguji alur dari permintaan tenaga kerja sampai keputusan dengan data uji sebelum dipakai untuk rekrutmen nyata."],
  ["Setup dan pelatihan", "Pembuatan akun tiap peran, panduan pengguna dan admin, serta sesi pelatihan per peran."],
];
add("7. Proposal dan implementasi", `
<div class="eyebrow">PROPOSAL IMPLEMENTASI</div>
<h2>Ruang lingkup yang disesuaikan dengan cara kerja Anda.</h2>
<table class="grid tight"><thead><tr><th style="width:24%">BAGIAN</th><th>CAKUPAN</th></tr></thead>
<tbody>${scope.map(([a, b]) => `<tr><td class="strong">${a}</td><td>${b}</td></tr>`).join("")}</tbody></table>`);

const next = [
  ["Editor template email", "Ubah dan pratinjau isi email dari aplikasi.", "Saat ini isi email diatur oleh tim implementasi."],
  ["Penghapusan data otomatis", "Penghapusan atau anonimisasi data pelamar sesuai masa simpan.", "Perlu keputusan kebijakan dan legal perusahaan."],
  ["Tampilan papan dan kalender", "Papan kanban lamaran per tahap dan kalender interview.", "Saat ini tersedia dalam bentuk daftar."],
  ["Masuk dengan akun kantor (SSO)", "Login staf memakai akun perusahaan, misalnya Microsoft atau Google.", "Membutuhkan akses ke penyedia identitas perusahaan."],
  ["Pemberitahuan WhatsApp", "Jadwal interview dan hasil seleksi lewat WhatsApp.", "Membutuhkan akun WhatsApp Business API."],
  ["Pengawasan ujian", "Pengawasan peserta, misalnya foto berkala selama ujian.", "Perlu kebijakan privasi dan persetujuan peserta."],
];
add("7. Proposal dan implementasi", `
<div class="eyebrow">PENGEMBANGAN BERIKUTNYA</div>
<h2>Fitur yang sudah direncanakan, di luar cakupan yang sudah berjalan.</h2>
<table class="grid"><thead><tr><th style="width:22%">FITUR</th><th style="width:42%">YANG DIBUTUHKAN</th><th>CATATAN</th></tr></thead>
<tbody>${next.map(([a, b, c]) => `<tr><td class="strong">${a}</td><td>${b}</td><td>${c}</td></tr>`).join("")}</tbody></table>
<div class="note">Cakupan, jadwal, dan biaya pengembangan ini ditentukan setelah kebutuhan dan prioritasnya disepakati bersama.</div>`);

add("7. Proposal dan implementasi", `
<div class="eyebrow">USULAN KERJA SAMA</div>
<h2>Mulai dari kebutuhan yang paling penting.</h2>
<div class="boxes">
  <div><h3>Pemetaan kebutuhan</h3><p>Diskusi alur seleksi, jenis posisi, jumlah pengguna dan unit, volume pelamar, jumlah peserta ujian serentak, dan kebutuhan integrasi. Hasilnya menjadi acuan cakupan serta penawaran final.</p></div>
  <div><h3>Pilihan implementasi</h3><p><b>Beli putus:</b> cakupan lisensi, source code, lokasi server, pemeliharaan, dan dukungan ditetapkan dalam kesepakatan kerja sama.</p><p><b>Langganan bulanan:</b> biaya mengikuti modul, pengguna, kapasitas server, dan kebutuhan dukungan. Ketentuan layanan dituangkan dalam penawaran final.</p></div>
  <div><h3>Tahapan pelaksanaan</h3><p>Pemetaan kebutuhan, konfigurasi dan penyesuaian, uji penerimaan, pelatihan, lalu penerapan. Jadwal dan biaya ditentukan setelah ruang lingkup disepakati.</p></div>
  <div><h3>Demo dan diskusi</h3><p>Demo dan pembahasan kebutuhan dapat dilakukan melalui Google Meet, termasuk mencoba ujian langsung dari ponsel.</p></div>
</div>
<h3 class="closing">Mari lihat alur rekrutmen Anda melalui sesi demo.</h3>
<p class="body">Kami siap menjadwalkan sesi demo dan diskusi kebutuhan sesuai jadwal tim Anda.</p>`);

// ---------- HTML ----------
const css = `
@page { size: 297mm 210mm; margin: 0 }
:root { --ink:#1b2430; --muted:#5b6675; --line:#dfe3e8; --blue:#1f6fd1; --navy:#14448f; --soft:#f3f5f7 }
* { box-sizing: border-box }
html,body { margin:0; background:#fff; color:var(--ink); font-family:"Source Sans 3", "Segoe UI", sans-serif; -webkit-print-color-adjust:exact; print-color-adjust:exact }
.page { width:297mm; height:210mm; padding:11mm 18mm 0; position:relative; overflow:hidden; page-break-after:always }
header { display:flex; justify-content:space-between; align-items:center; padding-bottom:4.5mm; border-bottom:1px solid var(--line); margin-bottom:9mm }
.brand { font-weight:700; font-size:11pt; letter-spacing:-.1px } .b1 { color:var(--ink) } .b2 { color:var(--blue) }
.brand.big { font-size:15pt }
.sec { font-size:8.5pt; color:var(--muted) }
footer { position:absolute; left:18mm; right:18mm; bottom:7mm; border-top:1px solid var(--line); padding-top:3mm; display:flex; justify-content:space-between; font-size:7.5pt; color:var(--muted) }
.eyebrow { color:var(--blue); font-size:8pt; font-weight:600; letter-spacing:1.4px; margin-bottom:2.5mm }
.eyebrow.sm { font-size:7.8pt; margin-bottom:1.5mm }
h2 { font-family:"Source Serif 4", Georgia, serif; font-weight:600; font-size:23pt; line-height:1.17; margin:0 0 6mm; max-width:235mm; letter-spacing:-.2px }
h3 { font-family:"Source Serif 4", Georgia, serif; font-weight:600; font-size:14pt; margin:0 0 2.5mm }
h4 { font-family:"Source Serif 4", Georgia, serif; font-weight:600; font-size:13pt; margin:0 0 2mm; line-height:1.15 }
p { margin:0 }
.body { font-size:11pt; line-height:1.55; color:#2b3542; max-width:200mm }
.body.w { margin-bottom:8mm }
.small { font-size:9pt; color:var(--muted); line-height:1.5; margin-top:4mm }
/* sampul */
.cover { position:relative; height:210mm; padding:16mm 18mm }
.cv-left { position:absolute; left:18mm; top:16mm; width:122mm }
.bar { width:22mm; height:2mm; background:var(--blue); margin:10mm 0 15mm }
.hero { font-family:"Source Serif 4", Georgia, serif; font-weight:700; font-size:42pt; line-height:1.08; margin:0 0 8mm; letter-spacing:-.5px }
.lead { font-size:13pt; line-height:1.55; color:#2b3542; width:112mm }
.cv-right { position:absolute; left:150mm; top:14mm; width:129mm }
.cv-foot { position:absolute; left:18mm; bottom:14mm; font-size:10pt; line-height:1.9; color:#2b3542 }
.cv-foot b { color:var(--ink) }
/* daftar isi */
.toc { width:220mm; border-collapse:collapse; margin-top:2mm }
.toc td { padding:3.4mm 0; border-bottom:1px solid var(--line); font-size:11.5pt }
.toc .n { width:7mm; color:var(--blue); font-weight:600 } .toc .p { text-align:right; color:var(--muted); font-size:10pt }
.tocnote { width:220mm; font-size:8.5pt; color:var(--muted); line-height:1.5; margin:2.5mm 0 8mm }
/* gambaran */
.cols3 { display:grid; grid-template-columns:repeat(3,1fr); gap:9mm }
.col { border-top:1.5px solid var(--blue); padding-top:4mm }
.col ul { margin:0; padding-left:5mm; font-size:10pt; line-height:1.5 } .col li { margin-bottom:1.6mm }
.stats { margin-top:9mm; background:var(--soft); border-radius:2mm; padding:5mm 6mm; display:grid; grid-template-columns:repeat(4,1fr); gap:6mm; font-size:10pt; line-height:1.5 }
/* alur */
.flow { display:flex; align-items:stretch; gap:0 }
.step { flex:1; border:1px solid var(--line); border-radius:2mm; padding:5mm 4mm; min-height:94mm }
.step .num { font-family:"Source Serif 4", Georgia, serif; color:var(--blue); font-size:20pt; font-weight:600; margin-bottom:2mm }
.step p { font-size:9.5pt; line-height:1.45; color:#2b3542 }
.step .lbl { font-size:7.5pt; letter-spacing:1.2px; color:var(--muted); font-weight:600; margin:4mm 0 1mm }
.arrow { align-self:center; color:var(--blue); font-size:14pt; padding:0 1.2mm }
.note { margin-top:6mm; background:var(--soft); border-radius:2mm; padding:4.5mm 5.5mm; font-size:9.5pt; line-height:1.55; color:#2b3542 }
/* tabel */
table.grid { width:100%; border-collapse:collapse }
.grid th { text-align:left; font-size:7.5pt; letter-spacing:1.1px; color:var(--muted); font-weight:600; padding:2mm 2.5mm; border-bottom:1px solid var(--line) }
.grid td { font-size:9.8pt; line-height:1.45; padding:3mm 2.5mm; border-bottom:1px solid var(--line); vertical-align:top; color:#2b3542 }
.grid.tight td { padding:2.3mm 2.5mm; font-size:9.5pt }
.grid .strong { font-weight:700; color:var(--ink) } .grid .r { text-align:right }
/* modul */
.mod { display:grid; grid-template-columns:150mm 1fr; gap:10mm; align-items:start }
figure { margin:0 }
.shot { border:1px solid #d5dae1; border-radius:2mm; overflow:hidden; height:96mm; background:#fff }
.shot img { width:100%; display:block }
.cv-right .shot { height:auto } .cv-right .shot img { width:100% }
.phones { display:flex; gap:8mm; justify-content:center; height:96mm }
.phone { border:1px solid #d5dae1; border-radius:4mm; overflow:hidden; height:96mm; width:44.4mm }
.phone img { width:100%; display:block }
figcaption { font-size:8.3pt; color:var(--muted); margin-top:2.5mm }
.side .blk { border-bottom:1px solid var(--line); padding:0 0 4mm; margin-bottom:4mm }
.side .blk:last-child { border-bottom:none }
.side p { font-size:10.3pt; line-height:1.5; color:#2b3542 }
.two { display:grid; grid-template-columns:150mm 1fr; gap:10mm; align-items:start }
.two.eq { grid-template-columns:1fr 1fr; gap:8mm; margin-top:5mm }
/* usulan */
.boxes { display:grid; grid-template-columns:1fr 1fr; gap:7mm 10mm }
.boxes > div { border-top:1px solid var(--navy); padding-top:3.5mm }
.boxes p { font-size:9.8pt; line-height:1.5; color:#2b3542; margin-bottom:1.8mm }
.closing { font-size:16pt; margin:9mm 0 3mm }
`;

const fonts = '<link rel="preconnect" href="https://fonts.googleapis.com"><link href="https://fonts.googleapis.com/css2?family=Source+Sans+3:wght@400;600;700&family=Source+Serif+4:opsz,wght@8..60,600;8..60,700&display=swap" rel="stylesheet">';
const body = pages.map((p, i) => p.cover
  ? `<section class="page" style="padding:0">${p.html}</section>`
  : `<section class="page">${header(p.section)}${p.html}<footer><span>${FOOT}</span><span>${i + 1}</span></footer></section>`).join("\n");
const html = `<!doctype html><html lang="id"><head><meta charset="utf-8"><title>RekrutFlow Preview Proposal</title>${fonts}<style>${css}</style></head><body>${body}</body></html>`;
const htmlPath = join(here, "proposal.html");
writeFileSync(htmlPath, html);

// Periksa nomor halaman yang dipakai daftar isi
if (pages.length !== P_PROPN) throw new Error(`jumlah halaman ${pages.length} != perkiraan ${P_PROPN}`);

const browser = await chromium.launch();
const page = await browser.newPage();
await page.goto(pathToFileURL(htmlPath).href, { waitUntil: "networkidle" });
await page.evaluate(() => document.fonts.ready);
// Peringatan bila isi halaman melewati footer
const overflow = await page.evaluate(() => [...document.querySelectorAll("section.page")].map((s, i) => {
  const f = s.querySelector("footer"); if (!f) return null;
  const limit = f.getBoundingClientRect().top; let max = 0;
  for (const el of s.querySelectorAll("*")) { if (el.closest("footer") || el.closest("header")) continue; max = Math.max(max, el.getBoundingClientRect().bottom); }
  return max > limit - 2 ? i + 1 : null;
}).filter(Boolean));
const out = join(here, "RekrutFlow-Preview-Proposal.pdf");
await page.pdf({ path: out, width: "297mm", height: "210mm", printBackground: true, preferCSSPageSize: true });
await browser.close();
console.log(`${pages.length} halaman -> ${out}`);
if (overflow.length) console.log("PERINGATAN isi menabrak footer di halaman:", overflow.join(", "));
