// Mengambil screenshot layar aplikasi (data demo) untuk dokumen proposal.
// Prasyarat: start.ps1 berjalan dan data demo terisi (tests/demo/demo-data.mjs). Jalankan: node docs/proposal/screenshots.mjs
import { createRequire } from "node:module";
import { mkdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const require = createRequire(join(here, "../../frontend/package.json"));
const { chromium } = require("playwright");
const BASE = "http://localhost:5173";
const PW = process.env.SEED_PASSWORD || "Arunika-Dev-2026";
const OUT = join(here, "img");
mkdirSync(OUT, { recursive: true });

const browser = await chromium.launch();
const done = [];
// ONLY=admin,examiner menjalankan bagian tertentu saja.
const only = (process.env.ONLY || "").split(",").filter(Boolean);
const want = (n) => only.length === 0 || only.includes(n);

async function session(email, opts = {}) {
  const ctx = await browser.newContext({ viewport: opts.viewport || { width: 1440, height: 900 }, deviceScaleFactor: 1.5, locale: "id-ID", timezoneId: "Asia/Jakarta", ...(opts.mobile ? { isMobile: true, hasTouch: true } : {}) });
  const page = await ctx.newPage();
  if (email) {
    await page.goto(BASE + "/masuk");
    await page.getByLabel("Email").fill(email);
    await page.getByLabel("Kata sandi").fill(PW);
    await page.getByRole("button", { name: "Masuk" }).click();
    await page.waitForURL((u) => !u.pathname.startsWith("/masuk"));
  }
  return { ctx, page };
}

async function shot(page, name, { full = false } = {}) {
  await page.waitForLoadState("networkidle");
  await page.waitForTimeout(400);
  await page.screenshot({ path: join(OUT, `${name}.png`), fullPage: full });
  done.push(name);
}

async function appId(page, candidate) {
  await page.goto(BASE + "/staf/lamaran");
  await page.getByRole("link", { name: candidate }).first().click();
  await page.waitForURL(/\/staf\/lamaran\//);
  return page.url();
}

// Publik
if (want("publik")) {
  const { ctx, page } = await session(null);
  await page.goto(BASE + "/");
  await shot(page, "01-portal");
  await page.goto(BASE + "/lowongan/operator-produksi-karawang");
  await shot(page, "02-lowongan");
  await page.goto(BASE + "/daftar");
  await page.getByLabel("Nama lengkap sesuai KTP").fill("Putri Anggraini");
  await page.getByLabel("Email").fill("putri.anggraini@contoh.test");
  await page.getByLabel("NIK").fill("3215 0145 0799 0201");
  await page.getByLabel("Kata sandi").fill("Contoh-Sandi-2026");
  await page.getByRole("checkbox").check();
  await shot(page, "03-daftar");
  await ctx.close();
}

// Kandidat
if (want("kandidat")) {
  const { ctx, page } = await session("siti.nurhaliza@contoh.test");
  await page.goto(BASE + "/akun/profil");
  await shot(page, "04-profil");
  await page.getByRole("heading", { name: "Dokumen" }).scrollIntoViewIfNeeded();
  await page.evaluate(() => window.scrollBy(0, -80));
  await shot(page, "05-dokumen");
  await ctx.close();
}
if (want("kandidat")) {
  const { ctx, page } = await session("rina.amelia@contoh.test");
  await page.goto(BASE + "/akun");
  await shot(page, "06-lamaran-saya");
  await page.getByRole("link", { name: "Operator Produksi" }).click();
  await shot(page, "07-timeline");
  await page.goto(BASE + "/akun/notifikasi");
  await shot(page, "08-kotak-masuk");
  await ctx.close();
}
if (want("ujian")) {
  const { ctx, page } = await session("yoga.pratama@contoh.test");
  await page.goto(BASE + "/akun/ujian");
  await shot(page, "09-undangan-ujian");
  await page.getByRole("button", { name: "Mulai ujian" }).click();
  await page.waitForURL(/\/ujian\//);
  await page.locator("main input[type=radio], main input[type=checkbox]").first().check().catch(async () => {
    await page.getByLabel("Jawaban esai").fill("Hentikan lini dan laporkan ke supervisor.");
  });
  await page.locator("header [role=status]").filter({ hasText: "Tersimpan" }).waitFor({ timeout: 10000 });
  await shot(page, "10-ruang-ujian");
  await page.getByRole("button", { name: "Berikutnya" }).click();
  await ctx.setOffline(true);
  const essay = page.getByLabel("Jawaban esai");
  if (await essay.isVisible()) await essay.fill("Jawaban saat koneksi terputus.");
  else await page.locator("main fieldset li input").first().check();
  await page.locator("header [role=status]").filter({ hasText: "Menunggu koneksi" }).waitFor({ timeout: 15000 });
  await shot(page, "11-ujian-offline");
  await ctx.setOffline(false);
  await page.locator("header [role=status]").filter({ hasText: "Tersimpan" }).waitFor({ timeout: 20000 });
  await page.getByRole("button", { name: "Selesai dan kirim" }).click();
  await shot(page, "12-konfirmasi-kirim");
  await page.getByRole("dialog").getByRole("button", { name: "Kirim jawaban" }).click();
  await page.getByRole("heading", { name: "Bukti penerimaan jawaban" }).waitFor();
  await shot(page, "13-bukti-penerimaan");
  await ctx.close();
}
if (want("ujian")) {
  const { ctx, page } = await session("intan.permatasari@contoh.test", { viewport: { width: 390, height: 844 }, mobile: true });
  await page.goto(BASE + "/akun/ujian");
  await page.getByRole("button", { name: "Mulai ujian" }).click();
  await page.waitForURL(/\/ujian\//);
  await page.locator("main fieldset li label").first().click().catch(() => {});
  await page.waitForTimeout(1500);
  await shot(page, "14-ujian-ponsel");
  await page.getByRole("button", { name: "Daftar soal" }).click();
  await shot(page, "15-ujian-ponsel-navigator");
  await ctx.close();
}

// Recruiter OPS
if (want("recruiter")) {
  const { ctx, page } = await session("rekruter.ops@arunika.example");
  await page.goto(BASE + "/staf/lamaran");
  await shot(page, "16-daftar-lamaran");
  await appId(page, "Andi Prasetyo");
  await shot(page, "17-detail-lamaran");
  await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
  await shot(page, "18-interview-scorecard");
  await appId(page, "Lestari Handayani").catch(() => {});
  await page.goto(BASE + "/staf/lamaran");
  await page.getByRole("row").filter({ hasText: "Interview" }).filter({ hasText: "Aktif" }).getByRole("link").first().click();
  await page.getByRole("button", { name: "Jadwalkan interview" }).click();
  const d = new Date(Date.now() + 6 * 86400000);
  await page.getByRole("dialog").getByLabel("Tanggal dan jam").fill(`${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}T13:30`);
  await page.getByRole("dialog").getByLabel("Lokasi atau tautan").fill("https://meet.contoh.test/arunika-hr");
  await page.getByRole("dialog").getByLabel("Bambang Hartono").check();
  await shot(page, "19-jadwal-interview");
  await ctx.close();
}

// HR Admin OPS
if (want("hr")) {
  const { ctx, page } = await session("hr.ops@arunika.example");
  await page.goto(BASE + "/staf/lamaran");
  await shot(page, "00-sampul");
  await page.goto(BASE + "/staf/lowongan");
  await shot(page, "20-lowongan-staf");
  await page.goto(BASE + "/staf/permintaan");
  await shot(page, "21-permintaan");
  await page.goto(BASE + "/staf/ujian");
  await page.getByRole("link", { name: "Gelombang November" }).click();
  await shot(page, "22-monitoring-ujian");
  await page.goto(BASE + "/staf/laporan");
  await shot(page, "23-laporan");
  await appId(page, "Siti Nurhaliza");
  await page.getByRole("button", { name: "Tampilkan NIK" }).click();
  await page.getByRole("dialog").getByLabel("Alasan").fill("Verifikasi dokumen kontrak kerja");
  await shot(page, "24-reveal-nik");
  await page.keyboard.press("Escape");
  await page.goto(BASE + "/staf/audit");
  await shot(page, "29-audit");
  await ctx.close();
}

// Hiring Manager
if (want("hm")) {
  const { ctx, page } = await session("hm.ops@arunika.example");
  await appId(page, "Andi Prasetyo");
  await page.getByRole("button", { name: "Setujui", exact: true }).click();
  await page.getByRole("dialog").getByLabel("Catatan").fill("Sesuai kebutuhan lini produksi 2");
  await shot(page, "25-persetujuan-hm");
  await ctx.close();
}

// Examiner
if (want("examiner")) {
  const { ctx, page } = await session("penguji@arunika.example");
  await page.goto(BASE + "/staf/bank-soal");
  await page.getByRole("button", { name: "Buka" }).first().click();
  await page.getByLabel("Isi CSV").fill("jenis;pertanyaan;opsi;kunci;bobot;rubrik\ntunggal;Satuan tekanan udara kompresor adalah;bar|meter|volt;A;1;\njamak;Pilih APD untuk area pengelasan;Kacamata las|Sarung tangan kulit|Sandal;A,B;2;\ntunggal;Kunci di luar pilihan;Ya|Tidak;C;1;");
  await page.getByRole("button", { name: "Pratinjau" }).click();
  await page.getByText(/baris perlu diperbaiki/).waitFor();
  await page.getByRole("heading", { name: "Impor soal (CSV, pemisah titik koma)" }).scrollIntoViewIfNeeded();
  await shot(page, "26-bank-soal");
  await page.goto(BASE + "/staf/ujian");
  await page.getByRole("link", { name: "Gelombang November" }).click();
  await page.getByRole("link", { name: "Penilaian esai" }).first().click();
  await shot(page, "27-penilaian-esai");
  await ctx.close();
}

// Super Admin
if (want("admin")) {
  const { ctx, page } = await session("admin@arunika.example");
  await page.goto(BASE + "/staf/workflow");
  await page.getByRole("link", { name: "Rekrutmen Standar" }).click();
  await shot(page, "28-workflow");
  await page.getByRole("heading", { name: "Simulasi hasil ujian" }).scrollIntoViewIfNeeded();
  await page.getByRole("textbox", { name: "Nilai (opsional)", exact: true }).fill("68");
  await page.getByRole("button", { name: "Jalankan simulasi" }).click();
  await page.getByText(/Berakhir di/).waitFor();
  await page.getByRole("heading", { name: "Simulasi hasil ujian" }).evaluate((e) => e.scrollIntoView({ block: "start" }));
  await page.evaluate(() => window.scrollBy(0, -24));
  await shot(page, "28b-simulasi");
  await page.goto(BASE + "/staf/pengguna");
  await page.getByText("Matriks peran dan izin").click();
  await shot(page, "30-pengguna");
  await page.getByText("Matriks peran dan izin").scrollIntoViewIfNeeded();
  await shot(page, "30b-matriks");
  await page.goto(BASE + "/staf/pengaturan");
  await shot(page, "31-pengaturan");
  await page.goto(BASE + "/staf/komunikasi");
  await shot(page, "32-komunikasi");
  await page.goto(BASE + "/staf/keamanan");
  await page.getByRole("button", { name: "Mulai aktivasi" }).click();
  await page.getByText("Kunci rahasia").waitFor();
  await shot(page, "33-mfa");
  await ctx.close();
}

await browser.close();
console.log(`${done.length} screenshot:`, done.join(", "));
