// Seluruh keputusan editorial video ada di sini: urutan shot, framing, kursor, caption, dan penutup.
// Satuan waktu: frame pada FPS di bawah. Koordinat: px CSS viewport capture (1440 x 900), sama dengan kotak
// elemen di public/footage/manifest.json, sehingga framing tetap tepat bila footage direkam ulang.

export const FPS = 30;
export const WIDTH = 1920;
export const HEIGHT = 1080;
export const DURATION = 1800; // 60 detik

// Area layar aplikasi dan pita caption di bawahnya.
export const STAGE = { w: WIDTH, h: 880 };
export const CAPTION_BAND = { y: 880, h: HEIGHT - 880, padX: 72 };
export const MAX_ZOOM = 2.4; // footage deviceScaleFactor 2: di atas ~2,4x teks mulai lunak

export type Rect = [x: number, y: number, w: number, h: number];
// Framing: persegi eksplisit, atau gabungan kotak elemen bernama dari manifest ditambah padding.
export type Focus = { rect: Rect } | { boxes: string[]; pad?: number };
export type Point = [x: number, y: number] | string; // koordinat atau nama kotak (pusatnya)

export type Shot = {
  still: string; // id di manifest = nama file tanpa .png
  from: number;
  dur: number;
  focus: Focus;
  // Satu gerakan reframing halus (10 sampai 16 frame), tidak bersamaan dengan kursor.
  reframe?: { to: Focus; at: number; dur: number };
  // Kursor bergerak ke target nyata lalu klik; aksinya terlihat pada shot berikutnya (hard cut).
  cursor?: { from: Point; to: Point; moveAt: number; moveDur: number; clickAt: number; fadeIn?: boolean };
};

export type Caption = { from: number; dur: number; text: string };

const DIALOG_FRAME: Focus = { rect: [160, 90, 830, 500] };
const DIALOG_DETAIL: Focus = { rect: [175, 230, 960, 440] };
const EVIDENCE_FRAME: Focus = { rect: [185, 5, 1070, 270] };
const RECRUITER_FRAME: Focus = { rect: [233, 80, 1120, 520] };

export const SHOTS: Shot[] = [
  // Kandidat: melamar, ditolak karena profil belum lengkap.
  { still: "c01-lowongan", from: 0, dur: 96, focus: DIALOG_FRAME,
    cursor: { from: [1060, 690], to: "apply", moveAt: 24, moveDur: 28, clickAt: 58, fadeIn: true } },
  { still: "c02-dialog-lamar", from: 96, dur: 40, focus: DIALOG_DETAIL,
    cursor: { from: [600, 600], to: "send", moveAt: 0, moveDur: 18, clickAt: 24 } },
  { still: "c03-profil-belum-lengkap", from: 136, dur: 90, focus: DIALOG_DETAIL,
    cursor: { from: [872, 483], to: "link", moveAt: 46, moveDur: 20, clickAt: 72 } },
  // Unggah CV.
  { still: "c04-profil-pilih-cv", from: 226, dur: 154, focus: { boxes: ["incomplete", "panel"], pad: 28 },
    cursor: { from: [1060, 690], to: "submit", moveAt: 70, moveDur: 28, clickAt: 106, fadeIn: true } },
  { still: "c05-review-siap", from: 380, dur: 90, focus: { rect: [180, 90, 1080, 500] } },
  // Bukti: jabatan dan periode beserta sumbernya di CV (porsi terpanjang).
  { still: "c06-review-pengalaman", from: 470, dur: 110, focus: { rect: [185, 230, 600, 470] },
    cursor: { from: [1000, 560], to: "sourceTitle", moveAt: 40, moveDur: 26, clickAt: 74, fadeIn: true } },
  { still: "c07-sumber-jabatan", from: 580, dur: 165, focus: { rect: [180, 0, 1080, 600] },
    reframe: { to: EVIDENCE_FRAME, at: 26, dur: 14 } },
  { still: "c08-sumber-periode", from: 745, dur: 170, focus: EVIDENCE_FRAME },
  // Simpan ke profil, lalu kirim lamaran.
  { still: "c09-review-diterima", from: 915, dur: 100, focus: { rect: [180, 500, 1080, 400] },
    cursor: { from: [900, 560], to: "save", moveAt: 30, moveDur: 28, clickAt: 66, fadeIn: true } },
  { still: "c10-profil-diperbarui", from: 1015, dur: 75, focus: { rect: [180, 90, 1080, 260] } },
  { still: "c13-dialog-kirim", from: 1090, dur: 60, focus: DIALOG_DETAIL,
    cursor: { from: [600, 600], to: "send", moveAt: 6, moveDur: 24, clickAt: 38, fadeIn: true } },
  { still: "c14-lamaran-terkirim", from: 1150, dur: 100, focus: { rect: [160, 90, 700, 420] } },
  // Recruiter: lamaran yang sama, tinjau, pindah tahap, riwayat.
  { still: "r01-daftar-lamaran", from: 1250, dur: 95, focus: { rect: [228, 60, 1100, 500] },
    cursor: { from: [700, 400], to: "link", moveAt: 34, moveDur: 26, clickAt: 66, fadeIn: true } },
  { still: "r02-detail-lamaran", from: 1345, dur: 135, focus: RECRUITER_FRAME,
    cursor: { from: [292, 330], to: "action", moveAt: 72, moveDur: 28, clickAt: 108 } },
  { still: "r03-dialog-pindah", from: 1480, dur: 90, focus: RECRUITER_FRAME,
    cursor: { from: [344, 207], to: "confirm", moveAt: 30, moveDur: 26, clickAt: 64 } },
  { still: "r05-riwayat", from: 1570, dur: 230, focus: RECRUITER_FRAME },
];

// Kalimat sederhana yang menjelaskan aksi dan akibat yang tampak pada shot.
export const CAPTIONS: Caption[] = [
  { from: 0, dur: 136, text: "Nadia melamar posisi Staf Administrasi Operasional." },
  { from: 136, dur: 90, text: "Profilnya belum lengkap, jadi lamaran belum bisa dikirim." },
  { from: 226, dur: 154, text: "Nadia mengunggah CV untuk mengisi profilnya." },
  { from: 380, dur: 90, text: "Isi CV tampil sebagai usulan yang bisa diperiksa." },
  { from: 470, dur: 445, text: "Jabatan dan periode kerja bisa diperiksa langsung pada CV." },
  { from: 915, dur: 175, text: "Nadia menerima usulan dan menyimpannya ke profil." },
  { from: 1090, dur: 160, text: "Setelah datanya sesuai, Nadia mengirim lamaran." },
  { from: 1250, dur: 95, text: "Recruiter Maya Sari menemukan lamaran Nadia." },
  { from: 1345, dur: 225, text: "Maya meninjau profil Nadia, lalu memindahkannya ke tahap Ujian." },
  { from: 1570, dur: 230, text: "Lamaran Nadia kini berada di tahap Ujian." },
];

// Nama yang tampil di aplikasi (portal kandidat dan back office). Ganti di sini bila perlu.
export const CLOSING = { from: 1680, text: "Arunika Karier · Back office rekrutmen" };
