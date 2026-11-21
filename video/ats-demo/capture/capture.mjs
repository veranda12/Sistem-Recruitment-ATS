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
  hr: "hr.ops@arunika.example",
  recruiter: "rekruter.ops@arunika.example",
  recruiterName: "Maya Sari",
  transitionNote: "Pengalaman administrasi sesuai kebutuhan posisi.",
};

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

// 1. HR Admin membuat dan menerbitkan lowongan lewat back office.
console.log("HR Admin: buat lowongan");
{
  const ctx = await newContext();
  const page = await login(ctx, DEMO.hr, STAFF_PW, "hr");
  await page.goto(BASE + "/staf/lowongan");
  await page.getByRole("button", { name: "Buat lowongan" }).click();
  const dlg = page.getByRole("dialog", { name: "Buat lowongan" });
  await dlg.getByLabel("Judul").fill(DEMO.job.title);
  await dlg.getByLabel("Slug URL").fill(DEMO.job.slug);
  await dlg.getByLabel("Unit").selectOption({ label: DEMO.job.unit });
  await dlg.getByLabel("Lokasi").selectOption({ label: DEMO.job.location });
  await dlg.getByLabel("Deskripsi").fill(DEMO.job.description);
  await dlg.getByLabel("Kualifikasi").fill(DEMO.job.requirements);
  const wf = dlg.getByLabel("Workflow");
  if (!(await wf.inputValue())) await wf.fill("standar");
  await dlg.getByRole("button", { name: "Simpan draft" }).click();
  await dlg.waitFor({ state: "hidden" });
  const row = page.getByRole("listitem").filter({ hasText: DEMO.job.title });
  await row.getByRole("button", { name: "Terbitkan" }).click();
  await page.getByRole("dialog", { name: "Terbitkan lowongan?" }).getByRole("button", { name: "Terbitkan" }).click();
  await row.getByText("Lihat di portal").waitFor();
  await ctx.close();
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
await cand.close();

// 9. Recruiter unit Operasional Pabrik menemukan lamaran yang sama dan memindahkan tahapnya.
console.log("Recruiter: rekam");
{
  const ctx = await newContext();
  const rp = await login(ctx, DEMO.recruiter, STAFF_PW, "recruiter");
  await rp.goto(BASE + "/staf/lamaran");
  const link = rp.getByRole("link", { name: DEMO.candidate.name });
  await link.waitFor();
  await shot(rp, "r01-daftar-lamaran", {
    actor: "recruiter",
    state: "Daftar lamaran unit, lamaran Nadia di tahap Screening",
    boxes: { row: link.locator("xpath=ancestor::tr[1]"), link, table: rp.locator("table").first() },
  });
  await link.click();
  await rp.getByRole("heading", { level: 1, name: DEMO.candidate.name }).waitFor();
  const action = rp.getByRole("region", { name: "Tindakan" }).getByRole("button").first();
  const dataHeading = rp.getByRole("heading", { name: "Data saat melamar" });
  const detailBoxes = () => ({
    header: rp.getByRole("heading", { level: 1 }).locator(".."),
    action,
    data: dataHeading.locator(".."),
    experience: rp.getByText(/Staf Administrasi di PT Bumi Lestari Niaga/),
    history: rp.getByRole("heading", { name: "Riwayat tahap" }).locator(".."),
  });
  await shot(rp, "r02-detail-lamaran", { actor: "recruiter", state: "Detail lamaran Nadia, tahap Screening", boxes: detailBoxes() });
  await action.click();
  const moveDlg = rp.getByRole("dialog");
  await moveDlg.waitFor();
  await moveDlg.getByLabel("Catatan").fill(DEMO.transitionNote);
  await shot(rp, "r03-dialog-pindah", {
    actor: "recruiter",
    state: "Dialog pindah tahap dengan catatan",
    boxes: { dialog: moveDlg, confirm: moveDlg.getByRole("button", { name: "Konfirmasi" }), title: moveDlg.getByRole("heading").first() },
  });
  await moveDlg.getByRole("button", { name: "Konfirmasi" }).click();
  await moveDlg.waitFor({ state: "hidden" });
  await rp.getByText("Tahap: Ujian").waitFor();
  await shot(rp, "r04-tahap-ujian", { actor: "recruiter", state: "Lamaran dipindah ke Ujian, notifikasi tersimpan tampil", boxes: detailBoxes() });
  await rp.getByText("Perubahan tersimpan.").waitFor({ state: "hidden", timeout: 15000 }).catch(() => {});
  await shot(rp, "r05-riwayat", { actor: "recruiter", state: "Riwayat tahap memuat perpindahan oleh recruiter", boxes: detailBoxes() });
  await ctx.close();
}
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
