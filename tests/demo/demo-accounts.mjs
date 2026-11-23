// Akun kandidat siap pakai untuk presentasi: didaftarkan lewat API seperti pengguna nyata, lalu diverifikasi dengan
// tautan dari kotak surat dev, sehingga penonton tidak perlu menunggu email. Profil sengaja kosong agar fitur
// "Isi profil dari CV" bisa diperagakan. Juga menyiapkan CV contoh (PDF) untuk diunggah.
// Dijalankan oleh ops/windows/demo.ps1 setelah demo-data.mjs. Semua nama, email, dan NIK fiktif.
import { createRequire } from "node:module";
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "../..");
const API = process.env.API || "http://127.0.0.1:8100/api/v1";
const ORIGIN = process.env.ORIGIN || "http://localhost:4173";
const MAIL = process.env.MAIL_DIR || join(root, ".data/demo/mail");
const PW = process.env.SEED_PASSWORD;
if (!PW) throw new Error("SEED_PASSWORD wajib diisi");

const accounts = [
  { name: "Nadia Pratama", email: "nadia.pratama@example.com", nik: "3275015403029901" },
  { name: "Kandidat Tamu Satu", email: "tamu1@example.com", nik: "3275011001990001" },
  { name: "Kandidat Tamu Dua", email: "tamu2@example.com", nik: "3275011001990002" },
  { name: "Kandidat Tamu Tiga", email: "tamu3@example.com", nik: "3275011001990003" },
];

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function post(path, body) {
  const r = await fetch(API + path, { method: "POST", headers: { Origin: ORIGIN, "Content-Type": "application/json" }, body: JSON.stringify(body) });
  return { status: r.status, text: await r.text() };
}
function tokenFor(email, since) {
  if (!existsSync(MAIL)) return null;
  for (const f of readdirSync(MAIL).filter((f) => f.endsWith(".eml") && statSync(join(MAIL, f)).mtimeMs >= since)) {
    const b = readFileSync(join(MAIL, f), "utf8");
    if (b.includes(`To: ${email}`)) {
      const m = /token=([A-Za-z0-9_-]+)/.exec(b);
      if (m) return m[1];
    }
  }
  return null;
}

for (const a of accounts) {
  const since = Date.now() - 1000;
  const r = await post("/auth/register", { email: a.email, password: PW, fullName: a.name, nik: a.nik, acceptPrivacy: true });
  if (r.status !== 202) throw new Error(`daftar ${a.email}: ${r.status} ${r.text.slice(0, 200)}`);
  let token = null;
  for (let i = 0; i < 50 && !token; i++) {
    await sleep(300);
    token = tokenFor(a.email, since);
  }
  if (!token) throw new Error(`email verifikasi ${a.email} tidak ditemukan di ${MAIL}`);
  const v = await post("/auth/verify-email", { token });
  if (v.status >= 300) throw new Error(`verifikasi ${a.email}: ${v.status}`);
  console.log(`  akun siap: ${a.email} (${a.name})`);
}

// CV contoh dari sumber HTML di repo, dirender ke PDF dengan Playwright milik frontend.
const html = join(root, "video/ats-demo/capture/cv-nadia-pratama.html");
const out = join(root, ".data/demo/CV-Nadia-Pratama.pdf");
try {
  const { chromium } = createRequire(join(root, "frontend/package.json"))("playwright");
  const b = await chromium.launch();
  const p = await b.newPage();
  await p.goto("file:///" + html.replace(/\\/g, "/"));
  await p.pdf({ path: out, format: "A4", printBackground: true });
  await b.close();
  console.log(`  CV contoh: ${out}`);
} catch (e) {
  console.log(`  CV contoh tidak dibuat (${e.message.split("\n")[0]}). Unggah CV fiktif lain secara manual.`);
}
