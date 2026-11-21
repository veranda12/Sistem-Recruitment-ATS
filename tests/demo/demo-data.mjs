// Mengisi data demo realistis lewat API (seperti pengguna nyata) untuk screenshot proposal dan sesi demo.
// Jalankan setelah `seed --reset` dan start.ps1:  node tests/demo/demo-data.mjs
// Semua nama, email, dan NIK fiktif.
import { execFileSync } from "node:child_process";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";

const API = process.env.API || "http://127.0.0.1:8080/api/v1";
const ORIGIN = process.env.ORIGIN || "http://localhost:5173";
const MAIL = process.env.MAIL_DIR || "D:/Portfolio/ats/.data/mail";
const PW = process.env.SEED_PASSWORD || "Arunika-Dev-2026";

class Client {
  constructor() {
    this.cookie = "";
    this.csrf = "";
  }
  async call(method, path, body, headers = {}) {
    const h = { Origin: ORIGIN, ...headers };
    if (this.cookie) h.Cookie = this.cookie;
    if (this.csrf && method !== "GET") h["X-CSRF-Token"] = this.csrf;
    let payload;
    if (body instanceof FormData) payload = body;
    else if (body !== undefined) {
      h["Content-Type"] = "application/json";
      payload = JSON.stringify(body);
    }
    const r = await fetch(API + path, { method, headers: h, body: payload });
    const sc = r.headers.getSetCookie?.() || [];
    for (const c of sc) if (c.startsWith("ats_session=")) this.cookie = c.split(";")[0];
    const text = await r.text();
    let json = null;
    try {
      json = text ? JSON.parse(text) : null;
    } catch {}
    if (!r.ok && !headers["x-allow-error"]) throw new Error(`${method} ${path} -> ${r.status} ${text.slice(0, 200)}`);
    return { status: r.status, body: json };
  }
  async login(email) {
    const r = await this.call("POST", "/auth/login", { email, password: PW });
    this.csrf = r.body.csrfToken;
    return this;
  }
}
const staff = async (email) => new Client().login(email);
const titleOf = { "operator-produksi-karawang": "Operator Produksi", "supervisor-quality-control": "Supervisor Quality Control", "backend-engineer": "Backend Engineer" };

// Hanya email yang ditulis setelah `since` (ms), agar token dari run sebelumnya tidak terpakai.
function tokenFor(email, since) {
  const files = readdirSync(MAIL).filter((f) => f.endsWith(".eml") && statSync(join(MAIL, f)).mtimeMs >= since).sort().reverse();
  for (const f of files) {
    const b = readFileSync(join(MAIL, f), "utf8");
    if (b.includes(`To: ${email}`)) {
      const m = b.match(/token=([A-Za-z0-9_-]+)/);
      if (m) return m[1];
    }
  }
  return null;
}
// Khusus skrip demo: kunci jawaban dibaca langsung dari DB agar peserta "baik" lulus. Aplikasi tidak pernah mengirim kunci ke peserta.
function correctOptions(attemptId) {
  const out = execFileSync("psql", ["-h", "localhost", "-U", process.env.PGUSER || "pos", "-d", process.env.PGDATABASE || "ats_dev", "-tAc",
    `SELECT ai.item_id, string_agg(m.key, ',') FROM attempt_items ai JOIN question_versions qv ON qv.id = ai.question_version_id,
     jsonb_each_text(ai.option_map) m WHERE ai.attempt_id = '${attemptId}' AND qv.answer_key ? m.value GROUP BY ai.item_id`],
    { encoding: "utf8", env: { ...process.env, PGPASSWORD: process.env.PGPASSWORD || "pos" } });
  return Object.fromEntries(out.trim().split(String.fromCharCode(10)).filter(Boolean).map((l) => { const [id, ks] = l.trim().split("|"); return [id, ks.split(",")]; }));
}
// Skenario per kandidat dan lowongan: good = lulus, poor = di bawah ambang, pending = esai belum dinilai, skip = belum mulai, progress = sedang mengerjakan.
const plan = {
  "0:operator-produksi-karawang": "good", "1:operator-produksi-karawang": "good", "2:operator-produksi-karawang": "good",
  "3:operator-produksi-karawang": "poor", "7:operator-produksi-karawang": "pending", "8:operator-produksi-karawang": "good",
  "9:operator-produksi-karawang": "skip", "2:supervisor-quality-control": "poor", "4:supervisor-quality-control": "good",
  "8:supervisor-quality-control": "progress", "12:supervisor-quality-control": "skip", "0:backend-engineer": "poor",
  "5:backend-engineer": "good", "6:backend-engineer": "progress",
};

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

const people = [
  ["Rina Amelia", "Karawang", "SMK", "SMK Negeri 1 Karawang", "Mekatronika", ["operator-produksi-karawang", "backend-engineer"]],
  ["Andi Prasetyo", "Bekasi", "SMK", "SMK Negeri 2 Bekasi", "Teknik Mesin", ["operator-produksi-karawang"]],
  ["Siti Nurhaliza", "Purwakarta", "D3", "Politeknik Manufaktur Bandung", "Teknik Produksi", ["operator-produksi-karawang", "supervisor-quality-control"]],
  ["Bagus Santoso", "Karawang", "SMA", "SMA Negeri 3 Karawang", "IPA", ["operator-produksi-karawang"]],
  ["Dewi Kartika", "Cikarang", "S1", "Universitas Singaperbangsa", "Teknik Industri", ["supervisor-quality-control"]],
  ["Fajar Nugroho", "Jakarta Timur", "S1", "Universitas Gunadarma", "Informatika", ["backend-engineer"]],
  ["Maya Putri", "Depok", "S1", "Universitas Indonesia", "Ilmu Komputer", ["backend-engineer"]],
  ["Hendra Wijaya", "Karawang", "SMK", "SMK Pratama Karawang", "Teknik Elektro", ["operator-produksi-karawang"]],
  ["Lestari Handayani", "Bekasi", "D3", "Politeknik Negeri Jakarta", "Teknik Mesin", ["supervisor-quality-control", "operator-produksi-karawang"]],
  ["Yoga Pratama", "Karawang", "SMK", "SMK Negeri 1 Karawang", "Teknik Kendaraan", ["operator-produksi-karawang"]],
  ["Nadia Rahma", "Tangerang", "S1", "Universitas Bina Nusantara", "Sistem Informasi", ["backend-engineer"]],
  ["Rizky Firmansyah", "Karawang", "SMA", "SMA Negeri 1 Cikampek", "IPA", ["operator-produksi-karawang"]],
  ["Intan Permatasari", "Bandung", "S1", "Institut Teknologi Nasional", "Teknik Industri", ["supervisor-quality-control"]],
  ["Dimas Saputra", "Karawang", "SMK", "SMK Bina Karya", "Teknik Mesin", ["operator-produksi-karawang"]],
];

async function candidate(i, [name, city, level, inst, major, jobs]) {
  const email = name.toLowerCase().replace(/[^a-z]+/g, ".") + "@contoh.test";
  const c = new Client();
  const since = Date.now() - 1000;
  await c.call("POST", "/auth/register", { email, password: PW, fullName: name, nik: `3215014507${String(990000 + i)}`, acceptPrivacy: true });
  let tok = null;
  for (let t = 0; t < 40 && !tok; t++) {
    await sleep(300);
    tok = tokenFor(email, since);
  }
  if (!tok) throw new Error("token verifikasi tidak ditemukan untuk " + email);
  await c.call("POST", "/auth/verify-email", { token: tok });
  await c.login(email);
  const p = await c.call("GET", "/me/profile");
  const exp = i % 3 === 0 ? [] : [{ company: ["PT Sinar Mesin", "CV Karya Teknik", "PT Mitra Komponen"][i % 3], title: ["Operator", "Staf QC", "Programmer"][i % 3], startDate: "2021-03-01", endDate: "2024-06-30", description: "Tanggung jawab harian sesuai jabatan." }];
  await c.call("PUT", "/me/profile", {
    fullName: name, phone: `0812${String(30000000 + i * 7919).slice(0, 8)}`, birthDate: `199${i % 9}-0${(i % 8) + 1}-1${i % 9}`, city, address: `Jl. Contoh No. ${i + 1}, ${city}`,
    summary: "Siap bekerja shift dan belajar hal baru.", version: p.body.version, certifications: [], experiences: exp,
    education: [{ level, institution: inst, major, startYear: 2012 + (i % 6), endYear: 2015 + (i % 6), gpa: level.startsWith("S") || level === "D3" ? 3.1 + (i % 8) / 10 : null }],
  });
  if (i % 2 === 0) {
    const fd = new FormData();
    fd.append("kind", "cv");
    fd.append("file", new Blob([`%PDF-1.4\nCV ${name}\n`], { type: "application/pdf" }), `CV ${name}.pdf`);
    await c.call("POST", "/me/documents", fd);
  }
  const apps = {};
  for (const slug of jobs) apps[slug] = (await c.call("POST", `/jobs/${slug}/apply`, {})).body.applicationId;
  return { name, email, c, apps };
}

async function main() {
  console.log("Kandidat dan lamaran...");
  const cands = [];
  for (let i = 0; i < people.length; i++) cands.push(await candidate(i, people[i]));

  const recOps = await staff("rekruter.ops@arunika.example");
  const recTi = await staff("rekruter.ti@arunika.example");
  const hrOps = await staff("hr.ops@arunika.example");
  const hrTi = await staff("hr.ti@arunika.example");
  const hm = await staff("hm.ops@arunika.example");
  const ex = await staff("penguji@arunika.example");
  const move = async (who, appId, to, reason) => {
    const d = await who.call("GET", `/applications/${appId}`);
    return who.call("POST", `/applications/${appId}/transitions`, { kind: "manual", to, reason, expectedVersion: d.body.version });
  };
  const reject = async (who, appId, reason) => {
    const d = await who.call("GET", `/applications/${appId}`);
    return who.call("POST", `/applications/${appId}/transitions`, { kind: "reject", reason, expectedVersion: d.body.version });
  };

  console.log("Screening -> ujian...");
  const toExamOps = [], toExamTi = [];
  cands.forEach((k, i) => {
    for (const [slug, id] of Object.entries(k.apps)) {
      if (slug === "backend-engineer") { if (i !== 10) toExamTi.push([k, id]); }
      else if (i !== 11 && i !== 13) toExamOps.push([k, id]);
    }
  });
  for (const [, id] of toExamOps) await move(recOps, id, "ujian", "Berkas memenuhi kualifikasi minimum");
  for (const [, id] of toExamTi) await move(recTi, id, "ujian", "Pengalaman relevan dengan kebutuhan tim");
  await reject(recOps, cands[13].apps["operator-produksi-karawang"], "Domisili jauh dan tidak bersedia shift malam");

  const sched = (await hrOps.call("GET", "/exam-schedules")).body.items[0].id;
  await hrOps.call("POST", `/exam-schedules/${sched}/invitations`, { applicationIds: toExamOps.map(([, id]) => id) });
  await hrTi.call("POST", `/exam-schedules/${sched}/invitations`, { applicationIds: toExamTi.map(([, id]) => id), extraMinutes: 15, reason: "Akomodasi waktu atas permintaan peserta" });

  console.log("Mengerjakan ujian...");
  const graded = [];
  let n = 0;
  for (const [ci, k] of cands.entries()) {
    const inv = (await k.c.call("GET", "/me/exam-invitations")).body.items;
    for (const v of inv) {
      n++;
      const slug = Object.entries(k.apps).find(([, id]) => id && v.jobTitle && true)?.[0];
      const key = Object.keys(k.apps).map((sl) => `${ci}:${sl}`).find((kk) => plan[kk] && v.jobTitle && titleOf[kk.split(":")[1]] === v.jobTitle);
      const mode = plan[key] || "poor";
      if (mode === "skip") continue;
      const st = (await k.c.call("POST", `/exam-invitations/${v.id}/start`, {})).body;
      const good = mode === "good" || mode === "pending" ? correctOptions(st.id) : {};
      const answers = st.items.map((it, j) => {
        if (it.kind === "essay") return { itemId: it.itemId, baseRevision: 0, value: { text: "Hentikan mesin, pisahkan produk cacat ke area karantina, lapor ke supervisor, dan catat di formulir ketidaksesuaian." } };
        const pick = good[it.itemId] || [it.options[(n + j) % it.options.length].id];
        return { itemId: it.itemId, baseRevision: 0, value: { selected: pick } };
      });
      const sv = (await k.c.call("PUT", `/exam-attempts/${st.id}/answers`, { opId: `demo-${n}-1`, answers })).body;
      if (mode === "progress") continue;
      await k.c.call("POST", `/exam-attempts/${st.id}/submit`, { expectedRevision: sv.revision }, { "Idempotency-Key": `demo-submit-${n}` });
      if (mode !== "pending") graded.push(st.id);
      void slug;
    }
  }
  console.log("Penilaian esai...");
  for (const a of graded) {
    const rv = (await ex.call("GET", `/exam-attempts/${a}/review`)).body;
    for (const it of rv.items) if (it.answer) await ex.call("POST", `/exam-attempts/${a}/essay-grades`, { itemId: it.itemId, points: 2, reason: "Langkah lengkap dan runtut" });
  }
  console.log("Rilis nilai...");
  await hrOps.call("POST", `/exam-schedules/${sched}/release`);
  await hrTi.call("POST", `/exam-schedules/${sched}/release`);
  await sleep(4000); // worker memproses event rilis

  console.log("Interview dan persetujuan...");
  const hmId = (await hm.call("GET", "/auth/session")).body.user.id;
  const tiId = (await recTi.call("GET", "/auth/session")).body.user.id;
  const opsId = (await recOps.call("GET", "/auth/session")).body.user.id;
  let day = 2;
  const schedule = async (who, a, ids, idx) => {
    const at = new Date(Date.now() + day++ * 86400000);
    at.setUTCHours(2 + (idx % 4), 0, 0, 0);
    return who.call("POST", `/applications/${a.id}/interviews`, { startsAt: at.toISOString(), durationMinutes: 60, timezone: "Asia/Jakarta", mode: idx % 2 ? "video" : "onsite",
      location: idx % 2 ? "https://meet.contoh.test/arunika-hr" : "Ruang rapat Pabrik Karawang", interviewerIds: ids });
  };
  const opsAt = (await recOps.call("GET", "/applications?stage=interview&limit=100")).body.items;
  for (const [idx, a] of opsAt.entries()) {
    await schedule(recOps, a, [hmId, opsId], idx);
    const det = (await hm.call("GET", `/applications/${a.id}`)).body;
    if (a.candidateName === "Rina Amelia" || a.candidateName === "Andi Prasetyo") {
      await hm.call("POST", `/interviews/${det.interviews[0].id}/evaluations`, { rating: a.candidateName === "Rina Amelia" ? 5 : 4, recommendation: "lanjut", notes: "Memahami K3, komunikatif, siap bekerja shift." });
      await move(recOps, a.id, "persetujuan", "Rekomendasi interviewer: lanjut");
      if (a.candidateName === "Rina Amelia") {
        const d2 = (await hm.call("GET", `/applications/${a.id}`)).body;
        await hm.call("POST", `/applications/${a.id}/approvals`, { decision: "approve", comment: "Sesuai kebutuhan lini produksi 2", expectedVersion: d2.version });
        await recOps.call("POST", `/applications/${a.id}/decision-notice`);
      }
    }
  }
  for (const [idx, a] of (await recTi.call("GET", "/applications?stage=interview&limit=100")).body.items.entries()) await schedule(recTi, a, [tiId], idx + 5);
  for (const a of (await recOps.call("GET", "/applications?status=rejected&limit=100")).body.items.slice(0, 2)) await recOps.call("POST", `/applications/${a.id}/decision-notice`);
  console.log("Penarikan lamaran dan permintaan tenaga kerja...");
  const w = cands[8];
  const wid = w.apps["operator-produksi-karawang"];
  const tl = (await w.c.call("GET", `/me/applications/${wid}/timeline`)).body;
  if (tl.status === "active") await w.c.call("POST", `/me/applications/${wid}/withdraw`, { reason: "Sudah menerima tawaran di perusahaan lain", version: tl.version });
  const r1 = (await hm.call("POST", "/requisitions", { unitCode: "OPS", locationCode: "KRW", title: "Teknisi Perawatan Mesin", headcount: 2, employmentType: "contract", justification: "Penambahan lini produksi 3 pada kuartal depan." })).body;
  await hm.call("POST", `/requisitions/${r1.id}/submit`, { version: r1.version });
  const r2 = (await hm.call("POST", "/requisitions", { unitCode: "OPS", locationCode: "KRW", title: "Operator Forklift", headcount: 1, employmentType: "contract", justification: "Pengganti karyawan yang mutasi." })).body;
  await hm.call("POST", `/requisitions/${r2.id}/submit`, { version: r2.version });
  await hrOps.call("POST", `/requisitions/${r2.id}/approve`, { version: r2.version + 1, note: "Sesuai anggaran tenaga kerja 2027" });
  console.log("Selesai.");
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
