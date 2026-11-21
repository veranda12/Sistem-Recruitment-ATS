// Seluruh keputusan editorial video ada di sini: urutan shot, framing, kursor, caption, dan penutup.
// Satuan waktu: frame pada FPS di bawah. Koordinat: px CSS viewport capture (1440 x 900), sama dengan kotak
// elemen di public/footage/manifest.json, sehingga framing tetap tepat bila footage direkam ulang.

export const FPS = 30;
export const WIDTH = 1920;
export const HEIGHT = 1080;
export const DURATION = 3600; // 120 detik

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
const STAFF_FRAME: Focus = { rect: [233, 80, 1120, 520] };
const STAFF_TABLE: Focus = { rect: [228, 60, 1100, 330] };
const dialog = (pad = 48): Focus => ({ boxes: ["dialog"], pad });

// Urutan shot. `from` dihitung otomatis dari durasi; ubah `dur` untuk memperpanjang atau memendekkan shot.
type ShotSpec = Omit<Shot, "from">;
const SPECS: ShotSpec[] = [
  // HR Admin menerbitkan lowongan.
  { still: "h01-buat-lowongan", dur: 105, focus: { rect: [460, 20, 520, 400] } },
  { still: "h02-terbitkan", dur: 45, focus: dialog(60),
    cursor: { from: [720, 640], to: "confirm", moveAt: 0, moveDur: 18, clickAt: 28, fadeIn: true } },
  { still: "h03-lowongan-terbit", dur: 75, focus: { boxes: ["header", "row"], pad: 30 } },
  // Kandidat: melamar, ditolak karena profil belum lengkap, lalu mengisi profil dari CV.
  { still: "c01-lowongan", dur: 96, focus: DIALOG_FRAME,
    cursor: { from: [1060, 690], to: "apply", moveAt: 24, moveDur: 28, clickAt: 58, fadeIn: true } },
  { still: "c02-dialog-lamar", dur: 40, focus: DIALOG_DETAIL,
    cursor: { from: [600, 600], to: "send", moveAt: 0, moveDur: 18, clickAt: 24 } },
  { still: "c03-profil-belum-lengkap", dur: 90, focus: DIALOG_DETAIL,
    cursor: { from: [872, 483], to: "link", moveAt: 46, moveDur: 20, clickAt: 72 } },
  { still: "c04-profil-pilih-cv", dur: 130, focus: { boxes: ["incomplete", "panel"], pad: 28 },
    cursor: { from: [1060, 690], to: "submit", moveAt: 56, moveDur: 28, clickAt: 92, fadeIn: true } },
  { still: "c05-review-siap", dur: 90, focus: { rect: [180, 90, 1080, 500] } },
  // Bukti: jabatan dan periode beserta sumbernya di CV.
  { still: "c06-review-pengalaman", dur: 110, focus: { rect: [185, 230, 600, 470] },
    cursor: { from: [1000, 560], to: "sourceTitle", moveAt: 40, moveDur: 26, clickAt: 74, fadeIn: true } },
  { still: "c07-sumber-jabatan", dur: 150, focus: { rect: [180, 0, 1080, 600] }, reframe: { to: EVIDENCE_FRAME, at: 24, dur: 14 } },
  { still: "c08-sumber-periode", dur: 150, focus: EVIDENCE_FRAME },
  { still: "c09-review-diterima", dur: 100, focus: { rect: [180, 500, 1080, 400] },
    cursor: { from: [900, 560], to: "save", moveAt: 30, moveDur: 28, clickAt: 66, fadeIn: true } },
  { still: "c10-profil-diperbarui", dur: 75, focus: { rect: [180, 90, 1080, 260] } },
  { still: "c13-dialog-kirim", dur: 60, focus: DIALOG_DETAIL,
    cursor: { from: [600, 600], to: "send", moveAt: 6, moveDur: 24, clickAt: 38, fadeIn: true } },
  { still: "c14-lamaran-terkirim", dur: 80, focus: { rect: [160, 90, 700, 420] } },
  // Recruiter: screening ke Ujian.
  { still: "r01-daftar-lamaran", dur: 75, focus: { rect: [228, 60, 1100, 500] },
    cursor: { from: [700, 400], to: "link", moveAt: 20, moveDur: 26, clickAt: 52, fadeIn: true } },
  { still: "r02-detail-lamaran", dur: 105, focus: STAFF_FRAME,
    cursor: { from: [292, 330], to: "action", moveAt: 50, moveDur: 26, clickAt: 82 } },
  { still: "r03-dialog-pindah", dur: 75, focus: STAFF_FRAME,
    cursor: { from: [344, 207], to: "confirm", moveAt: 20, moveDur: 24, clickAt: 52 } },
  { still: "r05-riwayat", dur: 75, focus: STAFF_FRAME },
  // HR Admin mengundang ke ujian.
  { still: "h04-undang-peserta", dur: 90, focus: dialog(40),
    cursor: { from: [600, 700], to: "invite", moveAt: 36, moveDur: 24, clickAt: 66, fadeIn: true } },
  { still: "h05-peserta-diundang", dur: 60, focus: STAFF_TABLE },
  // Kandidat mengerjakan ujian.
  { still: "e01-undangan-ujian", dur: 75, focus: { boxes: ["heading", "card"], pad: 40 },
    cursor: { from: [700, 500], to: "start", moveAt: 20, moveDur: 24, clickAt: 52, fadeIn: true } },
  { still: "e02-soal-pilihan", dur: 105, focus: { rect: [240, 60, 1000, 420] } },
  { still: "e04-kirim-jawaban", dur: 70, focus: dialog(60),
    cursor: { from: [900, 700], to: "send", moveAt: 14, moveDur: 24, clickAt: 46, fadeIn: true } },
  { still: "e05-bukti-penerimaan", dur: 90, focus: { boxes: ["heading", "list"], pad: 40 } },
  // Penguji menilai esai.
  { still: "x01-penilaian-esai", dur: 90, focus: { boxes: ["header", "section"], pad: 24 },
    cursor: { from: [900, 700], to: "save", moveAt: 36, moveDur: 24, clickAt: 66, fadeIn: true } },
  { still: "x02-esai-dinilai", dur: 70, focus: { boxes: ["header", "section"], pad: 24 } },
  // HR Admin merilis nilai.
  { still: "h06-rilis-nilai", dur: 70, focus: dialog(60),
    cursor: { from: [700, 700], to: "confirm", moveAt: 14, moveDur: 24, clickAt: 46, fadeIn: true } },
  { still: "h07-nilai-dirilis", dur: 80, focus: STAFF_TABLE },
  // Interview.
  { still: "r06-tahap-interview", dur: 90, focus: STAFF_FRAME },
  // Dialog tinggi: mulai dari tanggal dan jenis, lalu turun ke interviewer dan tombol Simpan.
  { still: "r07-jadwal-interview", dur: 110, focus: { rect: [470, 125, 500, 330] },
    reframe: { to: { rect: [470, 440, 500, 330] }, at: 40, dur: 14 },
    cursor: { from: [600, 560], to: "save", moveAt: 64, moveDur: 22, clickAt: 94, fadeIn: true } },
  { still: "r08-interview-terjadwal", dur: 70, focus: { boxes: ["heading", "item"], pad: 40 } },
  { still: "m01-penilaian-interview", dur: 80, focus: dialog(36),
    cursor: { from: [700, 700], to: "save", moveAt: 30, moveDur: 24, clickAt: 62, fadeIn: true } },
  { still: "m02-penilaian-tersimpan", dur: 75, focus: { boxes: ["heading", "item", "evals"], pad: 36 } },
  // Persetujuan dan keputusan.
  { still: "r10-dialog-persetujuan", dur: 75, focus: dialog(60),
    cursor: { from: [700, 700], to: "confirm", moveAt: 14, moveDur: 24, clickAt: 48, fadeIn: true } },
  { still: "m04-dialog-setujui", dur: 75, focus: dialog(60),
    cursor: { from: [700, 700], to: "confirm", moveAt: 14, moveDur: 24, clickAt: 48, fadeIn: true } },
  // Status Diterima di judul, lalu turun ke akhir riwayat tahap.
  { still: "m05-diterima", dur: 90, focus: STAFF_FRAME, reframe: { to: { rect: [800, 400, 420, 330] }, at: 36, dur: 14 } },
  { still: "r11-kirim-keputusan", dur: 60, focus: STAFF_FRAME,
    cursor: { from: [700, 500], to: "action", moveAt: 8, moveDur: 22, clickAt: 36, fadeIn: true } },
  { still: "r12-keputusan-terkirim", dur: 75, focus: STAFF_FRAME },
  // Kandidat melihat hasil.
  { still: "n01-lamaran-diterima", dur: 150, focus: { rect: [180, 80, 560, 470] } },
  { still: "n02-kotak-masuk", dur: 124, focus: { rect: [180, 70, 760, 230] } },
];

export const SHOTS: Shot[] = SPECS.reduce<Shot[]>((acc, s) => {
  const prev = acc[acc.length - 1];
  acc.push({ ...s, from: prev ? prev.from + prev.dur : 0 });
  return acc;
}, []);

// Caption mulai pada shot tertentu dan bertahan sampai caption berikutnya. Kalimat sederhana yang menjelaskan
// aksi dan akibat yang tampak.
const CAPTION_AT: [still: string, text: string][] = [
  ["h01-buat-lowongan", "HR menerbitkan lowongan Staf Administrasi Operasional."],
  ["c01-lowongan", "Nadia melamar posisi tersebut dari portal karier."],
  ["c03-profil-belum-lengkap", "Profilnya belum lengkap, jadi lamaran belum bisa dikirim."],
  ["c04-profil-pilih-cv", "Nadia mengunggah CV untuk mengisi profilnya."],
  ["c05-review-siap", "Isi CV tampil sebagai usulan yang bisa diperiksa."],
  ["c06-review-pengalaman", "Jabatan dan periode kerja bisa diperiksa langsung pada CV."],
  ["c09-review-diterima", "Nadia menerima usulan dan menyimpannya ke profil."],
  ["c13-dialog-kirim", "Setelah datanya sesuai, Nadia mengirim lamaran."],
  ["r01-daftar-lamaran", "Recruiter Maya Sari menemukan lamaran Nadia."],
  ["r02-detail-lamaran", "Maya meninjau profil Nadia, lalu memindahkannya ke tahap Ujian."],
  ["h04-undang-peserta", "HR mengundang Nadia ke ujian online."],
  ["e01-undangan-ujian", "Nadia mengerjakan ujian, dan setiap jawaban tersimpan di server."],
  ["e04-kirim-jawaban", "Setelah dikirim, Nadia menerima bukti penerimaan jawaban."],
  ["x01-penilaian-esai", "Penguji menilai esai tanpa melihat identitas peserta."],
  ["h06-rilis-nilai", "HR merilis nilai, dan Nadia mendapat nilai 100."],
  ["r06-tahap-interview", "Lamaran otomatis pindah ke tahap Interview."],
  ["r07-jadwal-interview", "Maya menjadwalkan interview video dengan hiring manager."],
  ["m01-penilaian-interview", "Hiring manager mengisi penilaian interview."],
  ["r10-dialog-persetujuan", "Maya mengajukan persetujuan ke hiring manager."],
  ["m04-dialog-setujui", "Hiring manager menyetujui, dan lamaran berstatus Diterima."],
  ["r11-kirim-keputusan", "Maya mengirim pemberitahuan keputusan ke Nadia."],
  ["n01-lamaran-diterima", "Nadia melihat lamarannya diterima."],
];

const fromOf = (still: string) => {
  const s = SHOTS.find((x) => x.still === still);
  if (!s) throw new Error(`Caption merujuk shot yang tidak ada: ${still}`);
  return s.from;
};
export const CAPTIONS: Caption[] = CAPTION_AT.map(([still, text], i) => {
  const from = fromOf(still);
  const next = CAPTION_AT[i + 1];
  return { from, dur: (next ? fromOf(next[0]) : DURATION) - from, text };
});

// Nama yang tampil di aplikasi (portal kandidat dan back office). Ganti di sini bila perlu.
export const CLOSING = { from: DURATION - 120, text: "Arunika Karier · Back office rekrutmen" };
