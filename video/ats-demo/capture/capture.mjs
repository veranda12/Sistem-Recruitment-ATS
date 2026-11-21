// Merekam footage video demo dari stack demo lokal (lihat capture/demo-stack.ps1).
//
//   powershell -File video\ats-demo\capture\demo-stack.ps1 start   (reset DB demo + jalankan)
//   cd video\ats-demo; npm run capture
//
// Hasil: public/footage/*.png (viewport 1440x900, deviceScaleFactor 2) dan public/footage/manifest.json yang
// memetakan setiap still ke route, aktor, state, dan kotak elemen (px CSS viewport) untuk framing dan kursor.
// Tidak ada kredensial di manifest. Kata sandi staf dibaca dari SEED_PASSWORD di .env; kata sandi Nadia dibuat acak.
import { chromium } from "playwright";
import { randomBytes } from "node:crypto";
import { existsSync, mkdirSync, readdirSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "../../..");
const BASE = "http://localhost:5174";
const OUT = join(here, "../public/footage");
const AUTH = join(here, ".auth");
const MAIL = join(root, ".data/video-demo/mail");
const VIEW = { width: 1440, height: 900 };

// Identitas demo (fiktif). Dipakai konsisten di CV, akun, lowongan, dan caption.
const DEMO = {
  candidate: { name: "Nadia Pratama", email: "nadia.pratama@example.com", nik: "3275015403029901", birthDate: "2002-03-14" },
  job: {
    title: "Staf Administrasi Operasional",
    slug: "staf-administrasi-operasional",
    unit: "Operasional Pabrik",
    location: "Kantor Pusat Jakarta",
    description:
      "Mengelola administrasi operasional harian: arsip surat jalan dan faktur, laporan pengiriman, serta jadwal tim operasional.",
    requirements: "Minimal D3 Administrasi atau setara. Pengalaman administrasi minimal satu tahun. Terbiasa dengan Microsoft Excel.",
  },
  admin: "admin@arunika.example",
  hr: "hr.ops@arunika.example",
  recruiter: "rekruter.ops@arunika.example",
  recruiterName: "Maya Sari",
  hm: "hm.ops@arunika.example",
  hmName: "Bambang Hartono",
  examiner: "penguji@arunika.example",
  transitionNote: "Pengalaman administrasi sesuai kebutuhan posisi.",
  schedule: "Gelombang November",
  interviewLink: "https://meet.example.com/arunika-hr",
  interviewNotes: "Teliti, terbiasa dengan arsip dan laporan harian.",
  approvalNote: "Sesuai kebutuhan administrasi operasional.",
};

// Jawaban Nadia untuk paket "Tes Dasar Operasional" dari seed fiktif (backend/internal/app/seed.go). Dipilih
// berdasarkan teks soal dan opsi yang tampil, karena urutan soal dan opsi diacak. Aplikasi tidak mengirim kunci.
const EXAM_ANSWERS = [
  ["Berapakah hasil 12 x 3?", ["36"]],
  ["Alat pelindung diri yang wajib", ["Sarung tangan dan kacamata pelindung"]],
  ["Langkah pertama dalam metode 5S", ["Seiri (pilah)"]],
  ["Jika produksi 480 unit per 8 jam", ["60"]],
  ["Pilih semua tindakan yang benar saat terjadi kebocoran oli", ["Pasang tanda peringatan", "Lapor ke supervisor", "Bersihkan dengan bahan penyerap"]],
  ["Jelaskan langkah Anda bila menemukan produk cacat", "Hentikan proses, pisahkan produk cacat ke area karantina, laporkan ke supervisor, lalu catat di formulir ketidaksesuaian."],
];

function seedPassword() {
  if (process.env.SEED_PASSWORD) return process.env.SEED_PASSWORD;
  const env = readFileSync(join(root, ".env"), "utf8");
  const m = /^SEED_PASSWORD=(.*)$/m.exec(env);
  if (!m) throw new Error("SEED_PASSWORD tidak ditemukan di .env");
  return m[1].trim().replace(/^["']|["']$/g, "");
}

async function preflight() {
  try {
    const r = await fetch(BASE + "/api/v1/jobs");
    if (!r.ok) throw new Error(String(r.status));
  } catch (e) {
    throw new Error(`Stack demo tidak terjangkau di ${BASE} (${e.message}). Jalankan dulu: powershell -File video\\ats-demo\\capture\\demo-stack.ps1 start`);
  }
  const jobs = await (await fetch(BASE + "/api/v1/jobs")).json();
  if (JSON.stringify(jobs).includes(DEMO.job.slug)) {
    throw new Error("Data demo sudah berisi lowongan rekaman sebelumnya. Reset dulu: demo-stack.ps1 stop, lalu demo-stack.ps1 start");
  }
}

await preflight();
rmSync(OUT, { recursive: true, force: true });
mkdirSync(OUT, { recursive: true });
mkdirSync(AUTH, { recursive: true });
const STAFF_PW = seedPassword();
const CAND_PW = `Nadia-${randomBytes(9).toString("base64url")}9`;

// Chromium penuh (headless baru) + --lang: kontrol bawaan browser (input tanggal, pemilih berkas) berbahasa
// Indonesia. Headless shell bawaan Playwright mengabaikan --lang.
const browser = await chromium.launch({ channel: "chromium", args: ["--lang=id-ID"] });
const shots = [];

async function newContext() {
  const ctx = await browser.newContext({ viewport: VIEW, deviceScaleFactor: 2, locale: "id-ID", timezoneId: "Asia/Jakarta" });
  // Scrollbar disembunyikan agar tidak muncul garis ganda di frame; tampilan produk lain tidak diubah.
  await ctx.addInitScript(() => {
    const s = document.createElement("style");
    s.textContent = "html{scrollbar-width:none}::-webkit-scrollbar{display:none}";
    document.addEventListener("DOMContentLoaded", () => document.head.appendChild(s));
    // Garis merah pemeriksa ejaan browser bukan bagian dari produk.
    document.addEventListener("focusin", (e) => {
      if (e.target instanceof HTMLElement) e.target.spellcheck = false;
    });
  });
  return ctx;
}

async function login(ctx, email, password, stateName) {
  const page = await ctx.newPage();
  await page.goto(BASE + "/masuk");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Kata sandi").fill(password);
  await page.getByRole("button", { name: "Masuk" }).click();
  await page.waitForURL((u) => !u.pathname.startsWith("/masuk"));
  await ctx.storageState({ path: join(AUTH, `${stateName}.json`) });
  return page;
}

async function box(locator) {
  const b = await locator.boundingBox();
  if (!b) throw new Error("elemen tidak terlihat untuk kotak framing");
  const r = (v) => Math.round(v * 10) / 10;
  return { x: r(b.x), y: r(b.y), w: r(b.width), h: r(b.height) };
}

// Simpan still viewport beserta kotak elemen yang dipakai komposisi untuk crop dan target kursor.
async function shot(page, id, { actor, state, boxes = {} }) {
  await page.waitForLoadState("networkidle");
  await page.mouse.move(VIEW.width - 2, VIEW.height - 2); // hindari hover/tooltip kebetulan
  await page.waitForTimeout(350);
  const measured = {};
  for (const [k, loc] of Object.entries(boxes)) measured[k] = await box(loc);
  const file = `${id}.png`;
  await page.screenshot({ path: join(OUT, file), animations: "disabled", caret: "hide" });
  const u = new URL(page.url());
  shots.push({ id, file, route: u.pathname + u.search, actor, state, boxes: measured });
  console.log(`  ${id}  ${u.pathname}  (${state})`);
}

async function scrollTo(page, locator, top = 120) {
  await locator.evaluate((e, t) => window.scrollTo(0, e.getBoundingClientRect().top + window.scrollY - t), top);
  await page.waitForTimeout(250);
}

// 0. CV sintetis Nadia (PDF teks dari HTML di repo).
const cvPdf = join(here, "out/CV-Nadia-Pratama.pdf");
mkdirSync(dirname(cvPdf), { recursive: true });
{
  const p = await browser.newPage();
  await p.goto("file:///" + join(here, "cv-nadia-pratama.html").replace(/\\/g, "/"));
  await p.pdf({ path: cvPdf, format: "A4", printBackground: true });
  await p.close();
}

// Persiapan (tidak direkam): Super Admin menjadikan Bambang Hartono Hiring Manager unit Operasional Pabrik,
// karena di seed ia hanya ditugaskan ke lowongan tertentu. Lewat menu Pengguna di back office.
console.log("Super Admin: peran hiring manager");
{
  const ctx = await newContext();
  const page = await login(ctx, DEMO.admin, STAFF_PW, "admin");
  await page.goto(BASE + "/staf/pengguna");
  const row = page.getByRole("listitem").filter({ hasText: DEMO.hmName }).first();
  await row.getByRole("button", { name: "Beri peran" }).click();
  const dlg = page.getByRole("dialog", { name: `Beri peran: ${DEMO.hmName}` });
  await dlg.getByLabel("Peran").selectOption({ label: "Hiring Manager" });
  await dlg.getByLabel("Unit").selectOption({ label: DEMO.job.unit });
  await dlg.getByLabel("Alasan").fill("Hiring manager untuk lowongan unit Operasional Pabrik.");
  await dlg.getByRole("button", { name: "Simpan" }).click();
  await dlg.waitFor({ state: "hidden" });
  await ctx.close();
}

// 1. HR Admin membuat dan menerbitkan lowongan lewat back office.
console.log("HR Admin: buat lowongan");
const hrCtx = await newContext();
const hr = await login(hrCtx, DEMO.hr, STAFF_PW, "hr");
{
  await hr.goto(BASE + "/staf/lowongan");
  await hr.getByRole("button", { name: "Buat lowongan" }).click();
  const dlg = hr.getByRole("dialog", { name: "Buat lowongan" });
  await dlg.getByLabel("Judul").fill(DEMO.job.title);
  await dlg.getByLabel("Slug URL").fill(DEMO.job.slug);
  await dlg.getByLabel("Unit").selectOption({ label: DEMO.job.unit });
  await dlg.getByLabel("Lokasi").selectOption({ label: DEMO.job.location });
  await dlg.getByLabel("Deskripsi").fill(DEMO.job.description);
  await dlg.getByLabel("Kualifikasi").fill(DEMO.job.requirements);
  const wf = dlg.getByLabel("Workflow");
  if (!(await wf.inputValue())) await wf.fill("standar");
  await shot(hr, "h01-buat-lowongan", {
    actor: "hr",
    state: "Form Buat lowongan terisi, belum disimpan",
    boxes: { dialog: dlg, title: dlg.getByLabel("Judul"), save: dlg.getByRole("button", { name: "Simpan draft" }) },
  });
  await dlg.getByRole("button", { name: "Simpan draft" }).click();
  await dlg.waitFor({ state: "hidden" });
  const row = hr.getByRole("listitem").filter({ hasText: DEMO.job.title });
  await row.getByRole("button", { name: "Terbitkan" }).click();
  const pub = hr.getByRole("dialog", { name: "Terbitkan lowongan?" });
  await shot(hr, "h02-terbitkan", { actor: "hr", state: "Konfirmasi terbitkan lowongan draft", boxes: { dialog: pub, confirm: pub.getByRole("button", { name: "Terbitkan" }) } });
  await pub.getByRole("button", { name: "Terbitkan" }).click();
  await row.getByText("Lihat di portal").waitFor();
  await shot(hr, "h03-lowongan-terbit", {
    actor: "hr",
    state: "Lowongan terbit dan tampil di portal",
    boxes: { row, link: row.getByText("Lihat di portal"), header: hr.getByRole("heading", { level: 1 }) },
  });
}

// 2. Nadia mendaftar (API pendaftaran aplikasi + tautan verifikasi dari kotak surat dev), lalu masuk.
console.log("Kandidat: daftar dan verifikasi email");
const cand = await newContext();
{
  const since = Date.now() - 1000;
  const h = { Origin: BASE };
  const r = await cand.request.post(BASE + "/api/v1/auth/register", {
    headers: h,
    data: { email: DEMO.candidate.email, password: CAND_PW, fullName: DEMO.candidate.name, nik: DEMO.candidate.nik, acceptPrivacy: true },
  });
  if (r.status() !== 202) throw new Error(`pendaftaran Nadia gagal: ${r.status()} ${await r.text()}`);
  let token = null;
  for (let i = 0; i < 60 && !token; i++) {
    await new Promise((x) => setTimeout(x, 300));
    if (!existsSync(MAIL)) continue;
    for (const f of readdirSync(MAIL).filter((f) => f.endsWith(".eml") && statSync(join(MAIL, f)).mtimeMs >= since)) {
      const b = readFileSync(join(MAIL, f), "utf8");
      if (b.includes(`To: ${DEMO.candidate.email}`)) token = /token=([A-Za-z0-9_-]+)/.exec(b)?.[1] ?? null;
    }
  }
  if (!token) throw new Error("email verifikasi Nadia tidak ditemukan di " + MAIL);
  const v = await cand.request.post(BASE + "/api/v1/auth/verify-email", { headers: h, data: { token } });
  if (!v.ok()) throw new Error("verifikasi email gagal: " + v.status());
}
const page = await login(cand, DEMO.candidate.email, CAND_PW, "kandidat");

console.log("Kandidat: rekam");
// 3. Detail lowongan, lalu mencoba melamar dengan profil yang belum lengkap.
await page.goto(`${BASE}/lowongan/${DEMO.job.slug}`);
await page.getByRole("heading", { level: 1, name: DEMO.job.title }).waitFor();
await shot(page, "c01-lowongan", {
  actor: "kandidat",
  state: "Detail lowongan terbit, belum melamar",
  boxes: { title: page.getByRole("heading", { level: 1 }), apply: page.getByRole("button", { name: "Lamar posisi ini" }) },
});
await page.getByRole("button", { name: "Lamar posisi ini" }).click();
const applyDlg = page.getByRole("dialog", { name: `Lamar ${DEMO.job.title}` });
await applyDlg.waitFor();
await shot(page, "c02-dialog-lamar", {
  actor: "kandidat",
  state: "Dialog lamar terbuka",
  boxes: { dialog: applyDlg, send: applyDlg.getByRole("button", { name: "Kirim lamaran" }) },
});
await applyDlg.getByRole("button", { name: "Kirim lamaran" }).click();
await applyDlg.getByText("Lengkapi profil Anda sebelum melamar.").waitFor();
await shot(page, "c03-profil-belum-lengkap", {
  actor: "kandidat",
  state: "Lamaran ditolak sistem karena profil belum lengkap",
  boxes: { dialog: applyDlg, alert: applyDlg.getByText("Lengkapi profil Anda sebelum melamar."), link: applyDlg.getByRole("link", { name: "Lengkapi profil" }) },
});

// 4. Profil: unggah CV.
await applyDlg.getByRole("link", { name: "Lengkapi profil" }).click();
await page.waitForURL(/\/akun\/profil$/);
const cvHeading = page.getByRole("heading", { name: "Isi profil dari CV" });
await cvHeading.waitFor();
await page.getByLabel("Berkas CV").setInputFiles(cvPdf);
await scrollTo(page, cvHeading, 260);
await shot(page, "c04-profil-pilih-cv", {
  actor: "kandidat",
  state: "Profil belum lengkap, berkas CV-Nadia-Pratama.pdf dipilih",
  boxes: {
    incomplete: page.getByText("Profil belum lengkap"),
    panel: cvHeading.locator(".."),
    file: page.getByLabel("Berkas CV"),
    submit: page.getByRole("button", { name: "Isi profil dari CV" }),
  },
});
await page.getByRole("button", { name: "Isi profil dari CV" }).click();
await page.waitForURL(/\/akun\/profil\/cv\//);
await page.getByText("Siap diperiksa").first().waitFor({ timeout: 120000 });
await page.evaluate(() => window.scrollTo(0, 0));
await shot(page, "c05-review-siap", {
  actor: "kandidat",
  state: "Hasil pembacaan CV siap diperiksa, belum ada keputusan",
  boxes: { heading: page.getByRole("heading", { name: "Periksa data dari CV" }), status: page.getByText("Siap diperiksa").first() },
});

// 5. Bukti: jabatan dan periode, lalu sumbernya di CV.
const exp = page.getByRole("region", { name: /Staf Administrasi di PT Bumi Lestari Niaga/ });
await scrollTo(page, exp, 96);
const expBoxes = () => ({
  region: exp,
  title: exp.getByRole("group", { name: "Keputusan untuk Jabatan" }).locator(".."),
  period: exp.getByRole("group", { name: "Keputusan untuk Periode" }).locator(".."),
  sourceTitle: exp.getByRole("button", { name: "Lihat sumber Jabatan" }),
  sourcePeriod: exp.getByRole("button", { name: "Lihat sumber Periode" }),
});
await shot(page, "c06-review-pengalaman", { actor: "kandidat", state: "Usulan pengalaman dari CV: jabatan dan periode", boxes: expBoxes() });
// Panel sumber menempel di atas kolom kanan. Baris yang diperiksa digulir sejajar panel, sehingga nilai usulan dan
// sorotan di CV terbaca dalam satu bingkai.
const crop = page.locator("img[alt^='Potongan']").first();
for (const [field, id, label] of [
  ["Jabatan", "c07-sumber-jabatan", "jabatan"],
  ["Periode", "c08-sumber-periode", "periode"],
]) {
  await exp.getByRole("button", { name: `Lihat sumber ${field}` }).click();
  await crop.waitFor();
  const row = exp.getByRole("group", { name: `Keputusan untuk ${field}` }).locator("..");
  await scrollTo(page, row, 70);
  await page.waitForTimeout(600);
  await shot(page, id, {
    actor: "kandidat",
    state: `Sumber ${label} disorot pada potongan halaman CV`,
    boxes: { row, panel: crop.locator("xpath=ancestor::*[.//p or .//h3][1]"), crop, mark: crop.locator("xpath=following-sibling::span[1]") },
  });
}

// 6. Terima usulan lalu simpan ke profil.
await page.getByRole("button", { name: "Terima semua yang tidak perlu diperiksa" }).click();
await page.getByText(/sudah diputuskan/).waitFor();
await page.waitForTimeout(500);
const save = page.getByRole("button", { name: "Simpan pilihan ke profil" });
await shot(page, "c09-review-diterima", {
  actor: "kandidat",
  state: "Semua usulan diterima, belum disimpan",
  boxes: { region: exp, save, decided: page.getByText(/sudah diputuskan/).first() },
});
await save.click();
await page.getByText("Profil diperbarui").waitFor();
await page.evaluate(() => window.scrollTo(0, 0));
await shot(page, "c10-profil-diperbarui", {
  actor: "kandidat",
  state: "Usulan CV diterapkan ke profil",
  boxes: { alert: page.getByText("Profil diperbarui").locator("xpath=ancestor::*[@role='status' or @role='alert'][1]") },
});

// 7. Lengkapi tanggal lahir (tidak diusulkan dari CV karena data pribadi sensitif), simpan profil.
await page.goto(BASE + "/akun/profil");
const birth = page.getByLabel("Tanggal lahir");
await birth.waitFor();
await birth.fill(DEMO.candidate.birthDate);
await birth.blur();
await scrollTo(page, page.getByRole("heading", { name: "Identitas" }), 120);
await shot(page, "c11-profil-tanggal-lahir", {
  actor: "kandidat",
  state: "Tanggal lahir diisi manual, belum disimpan",
  boxes: { birth, phone: page.getByLabel("Nomor telepon"), city: page.getByLabel("Kota domisili") },
});
await page.getByRole("button", { name: "Simpan profil" }).click();
await page.getByText("Profil tersimpan.").waitFor();
const expHeading = page.getByRole("heading", { name: "Pengalaman kerja" });
await scrollTo(page, expHeading, 120);
await shot(page, "c12-profil-pengalaman", {
  actor: "kandidat",
  state: "Profil tersimpan, pengalaman dari CV tampil di form",
  boxes: { heading: expHeading, company: page.getByLabel("Perusahaan").first(), title: page.getByLabel("Jabatan").first() },
});

// 8. Kirim lamaran.
await page.goto(`${BASE}/lowongan/${DEMO.job.slug}`);
await page.getByRole("button", { name: "Lamar posisi ini" }).click();
await applyDlg.waitFor();
await shot(page, "c13-dialog-kirim", { actor: "kandidat", state: "Dialog lamar, profil lengkap", boxes: { dialog: applyDlg, send: applyDlg.getByRole("button", { name: "Kirim lamaran" }) } });
await applyDlg.getByRole("button", { name: "Kirim lamaran" }).click();
await page.waitForURL(/\/akun\/lamaran\/[^/]+\?baru=1/);
await page.getByText("Lamaran terkirim").waitFor();
const appUrl = page.url();
await shot(page, "c14-lamaran-terkirim", {
  actor: "kandidat",
  state: "Lamaran terkirim",
  boxes: { alert: page.getByText("Lamaran terkirim").locator("xpath=ancestor::*[@role='status' or @role='alert'][1]"), heading: page.getByRole("heading", { level: 1 }) },
});
// Kotak elemen dari halaman detail lamaran staf (dipakai beberapa shot recruiter dan hiring manager).
const detailBoxes = (p) => ({
  header: p.getByRole("heading", { level: 1 }).locator(".."),
  data: p.getByRole("heading", { name: "Data saat melamar" }).locator(".."),
  history: p.getByRole("heading", { name: "Riwayat tahap" }).locator(".."),
});
// Muat ulang halaman sampai teks muncul (perpindahan tahap otomatis diproses worker).
async function reloadUntil(p, text, timeout = 60000) {
  const until = Date.now() + timeout;
  for (;;) {
    await p.reload();
    await p.getByRole("heading", { level: 1 }).waitFor();
    if (await p.getByText(text).first().isVisible().catch(() => false)) return;
    if (Date.now() > until) throw new Error(`Menunggu "${text}" di ${p.url()} melewati batas waktu`);
    await p.waitForTimeout(1500);
  }
}
const firstAction = (p) => p.getByRole("region", { name: "Tindakan" }).getByRole("button").first();

// 9. Recruiter unit Operasional Pabrik menemukan lamaran yang sama dan memindahkannya ke Ujian.
console.log("Recruiter: screening");
const recCtx = await newContext();
const rp = await login(recCtx, DEMO.recruiter, STAFF_PW, "recruiter");
await rp.goto(BASE + "/staf/lamaran");
{
  const link = rp.getByRole("link", { name: DEMO.candidate.name });
  await link.waitFor();
  await shot(rp, "r01-daftar-lamaran", {
    actor: "recruiter",
    state: "Daftar lamaran unit, lamaran Nadia di tahap Screening",
    boxes: { row: link.locator("xpath=ancestor::tr[1]"), link, table: rp.locator("table").first() },
  });
  await link.click();
}
await rp.getByRole("heading", { level: 1, name: DEMO.candidate.name }).waitFor();
const staffAppUrl = rp.url();
await shot(rp, "r02-detail-lamaran", {
  actor: "recruiter",
  state: "Detail lamaran Nadia, tahap Screening",
  boxes: { ...detailBoxes(rp), action: firstAction(rp), experience: rp.getByText(/Staf Administrasi di PT Bumi Lestari Niaga/) },
});
await firstAction(rp).click();
{
  const dlg = rp.getByRole("dialog");
  await dlg.getByLabel("Catatan").fill(DEMO.transitionNote);
  await shot(rp, "r03-dialog-pindah", {
    actor: "recruiter",
    state: "Dialog pindah tahap ke Ujian dengan catatan",
    boxes: { dialog: dlg, confirm: dlg.getByRole("button", { name: "Konfirmasi" }), title: dlg.getByRole("heading").first() },
  });
  await dlg.getByRole("button", { name: "Konfirmasi" }).click();
  await dlg.waitFor({ state: "hidden" });
}
await rp.getByText("Tahap: Ujian").waitFor();
await rp.getByText("Perubahan tersimpan.").waitFor({ state: "hidden", timeout: 15000 }).catch(() => {});
await shot(rp, "r05-riwayat", { actor: "recruiter", state: "Lamaran di tahap Ujian, riwayat mencatat recruiter", boxes: detailBoxes(rp) });

// 10. HR Admin mengundang Nadia ke jadwal ujian.
console.log("HR Admin: undang ujian");
await hr.goto(BASE + "/staf/ujian");
await hr.getByRole("link", { name: DEMO.schedule }).click();
await hr.getByRole("heading", { level: 1, name: DEMO.schedule }).waitFor();
const monitorUrl = hr.url();
const nadiaRow = (p) => p.getByRole("row").filter({ hasText: DEMO.candidate.name });
await hr.getByRole("button", { name: "Undang peserta" }).click();
{
  const dlg = hr.getByRole("dialog", { name: "Undang peserta" });
  const opt = dlg.getByLabel(new RegExp(`${DEMO.candidate.name} · ${DEMO.job.title}`));
  await opt.check();
  await shot(hr, "h04-undang-peserta", {
    actor: "hr",
    state: "Dialog Undang peserta, lamaran Nadia dipilih",
    boxes: { dialog: dlg, option: opt.locator(".."), invite: dlg.getByRole("button", { name: /^Undang/ }) },
  });
  await dlg.getByRole("button", { name: /^Undang/ }).click();
  await dlg.waitFor({ state: "hidden" });
}
await nadiaRow(hr).waitFor();
await hr.waitForTimeout(4000); // tunggu notifikasi hasil undangan hilang
await shot(hr, "h05-peserta-diundang", { actor: "hr", state: "Nadia terdaftar sebagai peserta jadwal ujian", boxes: { row: nadiaRow(hr), table: hr.locator("table").first() } });

// 11. Nadia mengerjakan ujian online.
console.log("Kandidat: ujian");
await page.goto(BASE + "/akun/ujian");
const startBtn = page.getByRole("button", { name: "Mulai ujian" });
await startBtn.waitFor();
await shot(page, "e01-undangan-ujian", {
  actor: "kandidat",
  state: "Undangan ujian terbuka",
  boxes: { start: startBtn, card: startBtn.locator("xpath=ancestor::li[1]"), heading: page.getByRole("heading", { level: 1 }) },
});
await startBtn.click();
await page.waitForURL(/\/ujian\//);
const saved = page.getByText("Jawaban tersimpan di server");
let shotChoice = false;
for (let i = 0; i < EXAM_ANSWERS.length; i++) {
  const stemEl = page.locator("#soal p.mt-4");
  await stemEl.waitFor();
  const stem = (await stemEl.innerText()).trim();
  const ans = EXAM_ANSWERS.find(([k]) => stem.startsWith(k));
  if (!ans) throw new Error(`Soal tidak dikenal: ${stem}`);
  if (typeof ans[1] === "string") {
    await page.getByLabel("Jawaban esai").fill(ans[1]);
  } else {
    for (const t of ans[1]) {
      const esc = t.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
      await page.locator("main fieldset label").filter({ hasText: new RegExp(`^[A-Z]\\.\\s*${esc}$`) }).click();
    }
  }
  await saved.waitFor({ timeout: 15000 });
  await page.waitForTimeout(800);
  if (typeof ans[1] === "string") {
    await shot(page, "e03-soal-esai", {
      actor: "kandidat",
      state: "Jawaban esai tersimpan di server",
      boxes: { heading: page.locator("#judul-soal"), status: saved, essay: page.getByLabel("Jawaban esai"), header: page.locator("header").first() },
    });
  } else if (!shotChoice && ans[1].length === 1) {
    shotChoice = true;
    await shot(page, "e02-soal-pilihan", {
      actor: "kandidat",
      state: "Soal pilihan dijawab dan tersimpan di server",
      boxes: { heading: page.locator("#judul-soal"), status: saved, options: page.locator("main fieldset"), header: page.locator("header").first() },
    });
  }
  if (i < EXAM_ANSWERS.length - 1) await page.getByRole("button", { name: "Berikutnya" }).click();
}
await page.getByRole("button", { name: "Selesai dan kirim" }).click();
{
  const dlg = page.getByRole("dialog", { name: "Kirim jawaban sekarang?" });
  await dlg.getByText("Dijawab: 6 dari 6").waitFor();
  await shot(page, "e04-kirim-jawaban", { actor: "kandidat", state: "Konfirmasi kirim, 6 dari 6 dijawab", boxes: { dialog: dlg, send: dlg.getByRole("button", { name: "Kirim jawaban" }) } });
  await dlg.getByRole("button", { name: "Kirim jawaban" }).click();
}
await page.getByRole("heading", { name: "Bukti penerimaan jawaban" }).waitFor();
await shot(page, "e05-bukti-penerimaan", {
  actor: "kandidat",
  state: "Bukti penerimaan jawaban",
  boxes: { heading: page.getByRole("heading", { name: "Bukti penerimaan jawaban" }), list: page.locator("dl").first() },
});

// 12. Examiner menilai esai tanpa identitas peserta.
console.log("Examiner: penilaian esai");
{
  const ctx = await newContext();
  const xp = await login(ctx, DEMO.examiner, STAFF_PW, "examiner");
  await xp.goto(monitorUrl);
  await xp.getByRole("link", { name: "Penilaian esai" }).first().click();
  await xp.getByRole("heading", { level: 1, name: /Penilaian esai/ }).waitFor();
  const sec = xp.locator("section").filter({ hasText: "Rubrik" }).first();
  await sec.getByLabel("Nilai").fill("2");
  await sec.getByLabel("Catatan").fill("Langkah lengkap dan runtut.");
  await shot(xp, "x01-penilaian-esai", {
    actor: "examiner",
    state: "Penguji mengisi nilai esai, identitas disembunyikan",
    boxes: { header: xp.getByRole("heading", { level: 1 }).locator(".."), section: sec, save: sec.getByRole("button", { name: "Simpan nilai" }) },
  });
  await sec.getByRole("button", { name: "Simpan nilai" }).click();
  await xp.getByText(/Nilai saat ini: 100/).waitFor({ timeout: 15000 });
  await xp.getByText("Nilai esai tersimpan.").waitFor({ state: "hidden", timeout: 15000 }).catch(() => {});
  await shot(xp, "x02-esai-dinilai", {
    actor: "examiner",
    state: "Nilai esai tersimpan, nilai total 100",
    boxes: { header: xp.getByRole("heading", { level: 1 }).locator(".."), section: xp.locator("section").filter({ hasText: "Rubrik" }).first() },
  });
  await ctx.close();
}

// 13. HR Admin merilis nilai; workflow memindahkan lamaran ke Interview secara otomatis.
console.log("HR Admin: rilis nilai");
await hr.goto(monitorUrl);
await nadiaRow(hr).getByText("Siap dirilis").waitFor();
await hr.getByRole("button", { name: "Rilis nilai" }).click();
{
  const dlg = hr.getByRole("dialog", { name: "Rilis nilai?" });
  await shot(hr, "h06-rilis-nilai", {
    actor: "hr",
    state: "Konfirmasi rilis nilai (isi halaman di balik dialog tidak dapat diukur)",
    boxes: { dialog: dlg, confirm: dlg.getByRole("button", { name: "Rilis nilai" }) },
  });
  await dlg.getByRole("button", { name: "Rilis nilai" }).click();
  await dlg.waitFor({ state: "hidden" });
}
await hr.getByRole("button", { name: "Perbarui" }).click();
await nadiaRow(hr).getByText("Dirilis").waitFor();
await hr.getByText(/hasil dirilis/).waitFor({ state: "hidden", timeout: 15000 }).catch(() => {});
await shot(hr, "h07-nilai-dirilis", { actor: "hr", state: "Nilai Nadia 100 dirilis", boxes: { row: nadiaRow(hr), table: hr.locator("table").first() } });
await hrCtx.close();

// 14. Recruiter melihat lamaran pindah ke Interview, lalu menjadwalkan interview dengan hiring manager.
console.log("Recruiter: interview");
await rp.goto(staffAppUrl);
await reloadUntil(rp, "Tahap: Interview");
await shot(rp, "r06-tahap-interview", { actor: "recruiter", state: "Lamaran otomatis pindah ke Interview setelah nilai dirilis", boxes: detailBoxes(rp) });
const ivHeading = (p) => p.getByRole("heading", { name: "Interview", exact: true });
const ivItem = (p) => ivHeading(p).locator("xpath=ancestor::section[1]").getByRole("listitem").first();
// Interview dijadwalkan pada menit berikutnya (aplikasi menolak waktu lampau). Capture menunggu sampai jam
// interview tiba sebelum interviewer menilai, sehingga penilaian tidak bertanggal lebih awal dari interview.
const ivDuration = 15;
const ivStart = new Date(Math.ceil((Date.now() + 60000) / 60000) * 60000);
const jkt = (d) =>
  new Intl.DateTimeFormat("sv-SE", { timeZone: "Asia/Jakarta", year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hourCycle: "h23" })
    .format(d)
    .replace(" ", "T");
await rp.getByRole("button", { name: "Jadwalkan interview" }).click();
{
  const dlg = rp.getByRole("dialog", { name: "Jadwalkan interview" });
  await dlg.getByLabel("Tanggal dan jam").fill(jkt(ivStart));
  await dlg.getByLabel("Durasi (menit)").fill(String(ivDuration));
  await dlg.getByLabel("Jenis").selectOption({ label: "Video" });
  await dlg.getByLabel("Lokasi atau tautan").fill(DEMO.interviewLink);
  await dlg.getByLabel(DEMO.hmName).check();
  await dlg.getByLabel(DEMO.recruiterName).check();
  await shot(rp, "r07-jadwal-interview", {
    actor: "recruiter",
    state: "Form Jadwalkan interview terisi dengan dua interviewer",
    boxes: { dialog: dlg, save: dlg.getByRole("button", { name: "Simpan" }) },
  });
  await dlg.getByRole("button", { name: "Simpan" }).click();
  await dlg.waitFor({ state: "hidden" });
}
await ivItem(rp).waitFor();
await rp.getByText(/Tersimpan\. Kandidat menerima/).waitFor({ state: "hidden", timeout: 15000 }).catch(() => {});
await scrollTo(rp, ivHeading(rp), 300);
await shot(rp, "r08-interview-terjadwal", { actor: "recruiter", state: "Interview terjadwal", boxes: { item: ivItem(rp), heading: ivHeading(rp) } });

const waitUntil = ivStart.getTime() + 15000;
console.log(`  menunggu jam interview (${new Date(waitUntil).toLocaleTimeString("id-ID", { timeZone: "Asia/Jakarta" })} WIB)`);
while (Date.now() < waitUntil) {
  await new Promise((r) => setTimeout(r, 5000));
}

// 15. Hiring manager mengisi penilaian interview.
console.log("Hiring manager: penilaian interview");
const hmCtx = await newContext();
const hp = await login(hmCtx, DEMO.hm, STAFF_PW, "hm");
await hp.goto(staffAppUrl);
await hp.getByRole("heading", { level: 1, name: DEMO.candidate.name }).waitFor();
await scrollTo(hp, ivHeading(hp), 300);
await ivItem(hp).getByRole("button", { name: "Isi penilaian" }).click();
{
  const dlg = hp.getByRole("dialog", { name: "Penilaian interview" });
  await dlg.getByLabel("Nilai (1-5)").selectOption("5");
  await dlg.getByLabel("Rekomendasi").selectOption({ label: "Lanjut" });
  await dlg.getByLabel("Catatan").fill(DEMO.interviewNotes);
  await shot(hp, "m01-penilaian-interview", { actor: "hm", state: "Form penilaian interview terisi", boxes: { dialog: dlg, save: dlg.getByRole("button", { name: "Simpan" }) } });
  await dlg.getByRole("button", { name: "Simpan" }).click();
  await dlg.waitFor({ state: "hidden" });
}
const evalList = (p) => p.getByRole("heading", { name: "Penilaian interviewer" }).locator("..");
await evalList(hp).waitFor();
await hp.getByText(/Tersimpan\. Kandidat menerima/).waitFor({ state: "hidden", timeout: 15000 }).catch(() => {});
await scrollTo(hp, ivHeading(hp), 300);
await shot(hp, "m02-penilaian-tersimpan", { actor: "hm", state: "Penilaian interviewer tercatat", boxes: { item: ivItem(hp), evals: evalList(hp), heading: ivHeading(hp) } });

// 16. Recruiter mengajukan persetujuan.
console.log("Recruiter: ajukan persetujuan");
await rp.goto(staffAppUrl);
await rp.getByRole("heading", { level: 1, name: DEMO.candidate.name }).waitFor();
await shot(rp, "r09-detail-interview", { actor: "recruiter", state: "Tahap Interview dengan tombol Ajukan persetujuan", boxes: { ...detailBoxes(rp), action: firstAction(rp) } });
await firstAction(rp).click();
{
  const dlg = rp.getByRole("dialog");
  await dlg.getByLabel("Catatan").fill("Rekomendasi interviewer: lanjut.");
  await shot(rp, "r10-dialog-persetujuan", { actor: "recruiter", state: "Dialog pindah ke Persetujuan", boxes: { dialog: dlg, confirm: dlg.getByRole("button", { name: "Konfirmasi" }) } });
  await dlg.getByRole("button", { name: "Konfirmasi" }).click();
  await dlg.waitFor({ state: "hidden" });
}
await rp.getByText("Tahap: Persetujuan").waitFor();

// 17. Hiring manager menyetujui; workflow menutup lamaran sebagai Diterima.
console.log("Hiring manager: persetujuan");
await hp.goto(staffAppUrl);
await reloadUntil(hp, "Tahap: Persetujuan");
const approve = hp.getByRole("region", { name: "Tindakan" }).getByRole("button", { name: "Setujui", exact: true });
await shot(hp, "m03-detail-persetujuan", { actor: "hm", state: "Hiring manager melihat tombol Setujui", boxes: { ...detailBoxes(hp), action: approve } });
await approve.click();
{
  const dlg = hp.getByRole("dialog");
  await dlg.getByLabel("Catatan").fill(DEMO.approvalNote);
  await shot(hp, "m04-dialog-setujui", { actor: "hm", state: "Dialog Setujui dengan catatan", boxes: { dialog: dlg, confirm: dlg.getByRole("button", { name: "Konfirmasi" }) } });
  await dlg.getByRole("button", { name: "Konfirmasi" }).click();
  await dlg.waitFor({ state: "hidden" });
}
await reloadUntil(hp, "Tahap: Diterima");
await shot(hp, "m05-diterima", { actor: "hm", state: "Lamaran berstatus Diterima", boxes: detailBoxes(hp) });
await hmCtx.close();

// 18. Recruiter mengirim pemberitahuan keputusan.
console.log("Recruiter: pemberitahuan keputusan");
await rp.goto(staffAppUrl);
const notify = rp.getByRole("button", { name: "Kirim pemberitahuan keputusan" });
await notify.waitFor();
await shot(rp, "r11-kirim-keputusan", { actor: "recruiter", state: "Diterima, pemberitahuan belum dikirim", boxes: { ...detailBoxes(rp), action: notify } });
await notify.click();
await rp.getByText(/Pemberitahuan keputusan sudah dikirim/).waitFor();
await rp.getByText("Pemberitahuan keputusan dikirim ke kandidat.").waitFor({ state: "hidden", timeout: 15000 }).catch(() => {});
await shot(rp, "r12-keputusan-terkirim", {
  actor: "recruiter",
  state: "Pemberitahuan keputusan terkirim",
  boxes: { ...detailBoxes(rp), sent: rp.getByText(/Pemberitahuan keputusan sudah dikirim/) },
});
await recCtx.close();

// 19. Nadia melihat hasilnya.
console.log("Kandidat: hasil");
await page.goto(appUrl.replace(/\?baru=1$/, ""));
await page.getByRole("heading", { level: 1 }).waitFor();
await shot(page, "n01-lamaran-diterima", {
  actor: "kandidat",
  state: "Lamaran Nadia berstatus Diterima dengan riwayat lengkap",
  boxes: { heading: page.getByRole("heading", { level: 1 }), status: page.getByText(/Tahap saat ini/).first(), history: page.getByRole("heading", { name: "Riwayat" }).locator("..") },
});
await page.goto(BASE + "/akun/notifikasi");
await page.getByRole("heading", { level: 1, name: "Kotak masuk" }).waitFor();
await shot(page, "n02-kotak-masuk", { actor: "kandidat", state: "Kotak masuk kandidat", boxes: { heading: page.getByRole("heading", { level: 1 }), list: page.locator("main ul").first() } });
await cand.close();
await browser.close();

const manifest = {
  generatedAt: new Date().toISOString(),
  source: { app: BASE, database: "ats_video_dev (seed fiktif PT Arunika Daya Persada)", viewport: VIEW, deviceScaleFactor: 2 },
  identity: {
    candidate: { name: DEMO.candidate.name, email: DEMO.candidate.email, cv: "capture/cv-nadia-pratama.html" },
    job: { title: DEMO.job.title, slug: DEMO.job.slug, unit: DEMO.job.unit },
    recruiter: { name: DEMO.recruiterName, role: "Recruiter unit Operasional Pabrik" },
    candidateApplication: new URL(appUrl).pathname,
  },
  shots,
};
writeFileSync(join(OUT, "manifest.json"), JSON.stringify(manifest, null, 2));
console.log(`${shots.length} still -> ${OUT}`);
