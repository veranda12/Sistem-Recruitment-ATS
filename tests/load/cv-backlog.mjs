// Benchmark backlog pembacaan CV di lingkungan dev (start.ps1 berjalan, worker CV aktif).
// Mengukur: latensi API saat sepi vs saat backlog, waktu antre & proses (dari DB), memori/CPU worker CV + tool.
//   node tests/load/cv-backlog.mjs [jumlah=20] [out=tests/load/results/cv-<waktu>]
import { execFileSync, spawn } from "node:child_process";
import { mkdirSync, readFileSync, readdirSync, statSync, writeFileSync } from "node:fs";
import { join, resolve } from "node:path";

const N = Number(process.argv[2] ?? 20);
const ROOT = resolve(import.meta.dirname, "..", "..");
const OUT = process.argv[3] ?? join(ROOT, "tests", "load", "results", `cv-${new Date().toISOString().replace(/[:.]/g, "-")}`);
const API = "http://127.0.0.1:8080/api/v1";
const ORIGIN = "http://localhost:5173";
const MAIL = join(ROOT, ".data", "mail");
const PW = process.env.SEED_PASSWORD || "Arunika-Dev-2026";
const FX = join(ROOT, "tests", "cv-fixtures", "out");
mkdirSync(OUT, { recursive: true });

const files = [
  "dev/sinta.pdf-2col.pdf", "dev/budi.pdf-1col.pdf", "dev/dewi.pdf-duplayer.pdf", "dev/joko.pdf-2col.pdf", "dev/kevin.pdf-1col.pdf",
  "dev/sinta.scan.pdf", "dev/joko.scan.pdf", "dev/rahmat.scan.pdf", "dev/sinta.pdf-mixed.pdf",
  "dev/sinta.photo-skew.jpg", "dev/budi.photo-rot90.jpg", "dev/dewi.photo-rot180.jpg", "holdout/fajar.photo-skew.jpg",
  "dev/sari.docx.docx", "dev/nur.docx.docx", "dev/budi.doc.doc", "dev/ayu.doc.doc",
  "holdout/andi.pdf-2col.pdf", "holdout/lina.scan.pdf", "holdout/maya.photo-skew.webp",
];

class Client {
  cookie = "";
  csrf = "";
  async call(method, path, body, raw = false) {
    const h = { Origin: ORIGIN };
    if (this.cookie) h.Cookie = this.cookie;
    if (this.csrf && method !== "GET") h["X-CSRF-Token"] = this.csrf;
    let payload = body;
    if (body !== undefined && !(body instanceof FormData)) {
      h["Content-Type"] = "application/json";
      payload = JSON.stringify(body);
    }
    const t0 = performance.now();
    const r = await fetch(API + path, { method, headers: h, body: payload });
    const ms = performance.now() - t0;
    for (const c of r.headers.getSetCookie?.() ?? []) if (c.startsWith("ats_session=")) this.cookie = c.split(";")[0];
    const text = await r.text();
    if (!r.ok && !raw) throw new Error(`${method} ${path} ${r.status} ${text.slice(0, 200)}`);
    return { status: r.status, body: text ? JSON.parse(text) : null, ms };
  }
}

async function candidate(i) {
  const email = `bench.cv.${Date.now()}.${i}@contoh.test`;
  const since = Date.now() - 1000;
  const c = new Client();
  await c.call("POST", "/auth/register", { email, password: PW, fullName: `Bench CV ${i}`, nik: `3273019${String(Date.now()).slice(-7)}${String(i).padStart(2, "0")}`, acceptPrivacy: true });
  let tok = null;
  for (let k = 0; k < 60 && !tok; k++) {
    await new Promise((r) => setTimeout(r, 250));
    for (const f of readdirSync(MAIL).filter((f) => f.endsWith(".eml") && statSync(join(MAIL, f)).mtimeMs >= since)) {
      const b = readFileSync(join(MAIL, f), "utf8");
      if (b.includes(`To: ${email}`)) tok = /token=([A-Za-z0-9_-]+)/.exec(b)?.[1];
    }
  }
  await c.call("POST", "/auth/verify-email", { token: tok });
  const l = await c.call("POST", "/auth/login", { email, password: PW });
  c.csrf = l.body.csrfToken;
  return c;
}

const pct = (xs, q) => {
  if (!xs.length) return 0;
  const s = [...xs].sort((a, b) => a - b);
  return s[Math.min(s.length - 1, Math.round(q * (s.length - 1)))];
};

// Latensi API: endpoint publik + profil kandidat (jalur yang dipakai kandidat lain dan alur ujian).
async function probe(c, ms) {
  const lat = [];
  const end = Date.now() + ms;
  while (Date.now() < end) {
    lat.push((await c.call("GET", "/jobs?limit=10")).ms);
    lat.push((await c.call("GET", "/me/profile")).ms);
    await new Promise((r) => setTimeout(r, 50));
  }
  return { n: lat.length, p50: pct(lat, 0.5), p95: pct(lat, 0.95), max: Math.max(...lat) };
}

function startSampler() {
  // Memori (working set) dan CPU kumulatif proses worker CV + tool turunannya, setiap 500 ms.
  const ps = `
$names = 'cvworker','tesseract','pdftoppm','pdftotext','pdfinfo','pdfimages','soffice.bin','soffice'
while ($true) {
  $p = Get-Process -Name $names -ErrorAction SilentlyContinue
  $ws = ($p | Measure-Object WorkingSet64 -Sum).Sum
  $cpu = ($p | Measure-Object CPU -Sum).Sum
  $w = ($p | Where-Object { $_.ProcessName -eq 'cvworker' } | Measure-Object WorkingSet64 -Sum).Sum
  "{0},{1},{2},{3},{4}" -f [DateTimeOffset]::Now.ToUnixTimeMilliseconds(), [int64]$ws, [int64]$w, [double]$cpu, $p.Count
  Start-Sleep -Milliseconds 500
}`;
  const child = spawn("powershell", ["-NoProfile", "-Command", ps], { stdio: ["ignore", "pipe", "ignore"] });
  const rows = [];
  child.stdout.on("data", (d) => {
    for (const line of d.toString().split(/\r?\n/)) {
      const f = line.split(",");
      if (f.length === 5) rows.push({ t: +f[0], ws: +f[1], worker: +f[2], cpu: +f[3], n: +f[4] });
    }
  });
  return { stop: () => child.kill(), rows };
}

const main = async () => {
  console.log(`benchmark ${N} unggahan CV -> ${OUT}`);
  const cands = [];
  for (let i = 0; i < Math.ceil(N / 8); i++) cands.push(await candidate(i));
  const prober = await candidate(99);
  const idle = await probe(prober, 15_000);
  console.log("API sepi:", idle);

  const docs = [];
  for (let i = 0; i < N; i++) {
    const rel = files[i % files.length];
    const fd = new FormData();
    fd.append("kind", "cv");
    fd.append("file", new Blob([readFileSync(join(FX, rel))]), rel.split("/")[1]);
    const c = cands[Math.floor(i / 8)];
    const up = await c.call("POST", "/me/documents", fd);
    docs.push({ c, id: up.body.id, rel });
  }
  const sampler = startSampler();
  const t0 = Date.now();
  const runs = [];
  for (const d of docs) runs.push({ ...d, run: (await d.c.call("POST", `/me/documents/${d.id}/parse`)).body.id });
  const busyProbe = probe(prober, 1_000_000_000); // dihentikan setelah semua selesai
  let stopProbe;
  const busy = await Promise.race([
    (async () => {
      const lat = [];
      while (true) {
        lat.push((await prober.call("GET", "/jobs?limit=10")).ms);
        lat.push((await prober.call("GET", "/me/profile")).ms);
        const pending = (await Promise.all(runs.map((r) => r.c.call("GET", `/me/cv-parses/${r.run}`).then((x) => x.body.status)))).filter((s) =>
          ["waiting_scan", "queued", "processing"].includes(s),
        );
        if (pending.length === 0) return { n: lat.length, p50: pct(lat, 0.5), p95: pct(lat, 0.95), max: Math.max(...lat) };
        if (Date.now() - t0 > 30 * 60_000) throw new Error("backlog tidak selesai dalam 30 menit");
        await new Promise((r) => setTimeout(r, 300));
      }
    })(),
    new Promise((_, rej) => (stopProbe = rej)),
  ]);
  void busyProbe;
  void stopProbe;
  const wall = Date.now() - t0;
  await new Promise((r) => setTimeout(r, 1500));
  sampler.stop();
  console.log("API saat backlog:", busy);

  const ids = runs.map((r) => `'${r.run}'`).join(",");
  const sql = `SELECT r.id, r.status, coalesce(r.queue_ms,0), coalesce(r.process_ms,0), coalesce(r.pages_total,0), coalesce(r.manifest->>'format','')
               FROM cv_parse_runs r WHERE r.id IN (${ids})`;
  const out = execFileSync("psql", ["-h", "localhost", "-U", "pos", "-d", "ats_dev", "-tA", "-F", ",", "-c", sql], { env: { ...process.env, PGPASSWORD: "pos" }, encoding: "utf8" });
  const rows = out.trim().split(/\r?\n/).map((l) => {
    const [id, status, q, p, pages, format] = l.split(",");
    return { id, status, queueMs: +q, processMs: +p, pages: +pages, format, file: runs.find((r) => r.run === id)?.rel };
  });
  const byFmt = {};
  for (const r of rows) (byFmt[r.format] ??= []).push(r);
  const summary = {
    n: N,
    wallMs: wall,
    statuses: rows.reduce((m, r) => ((m[r.status] = (m[r.status] ?? 0) + 1), m), {}),
    queueMs: { p50: pct(rows.map((r) => r.queueMs), 0.5), p95: pct(rows.map((r) => r.queueMs), 0.95) },
    processMs: { p50: pct(rows.map((r) => r.processMs), 0.5), p95: pct(rows.map((r) => r.processMs), 0.95) },
    processByFormat: Object.fromEntries(Object.entries(byFmt).map(([f, xs]) => [f, { n: xs.length, p50: pct(xs.map((x) => x.processMs), 0.5), p95: pct(xs.map((x) => x.processMs), 0.95) }])),
    pagesPerSecond: rows.reduce((s, r) => s + r.pages, 0) / (wall / 1000),
    peakMemMB: Math.round(Math.max(0, ...sampler.rows.map((r) => r.ws)) / 2 ** 20),
    peakWorkerMB: Math.round(Math.max(0, ...sampler.rows.map((r) => r.worker)) / 2 ** 20),
    cpuSeconds: sampler.rows.length ? +(Math.max(...sampler.rows.map((r) => r.cpu)) - Math.min(...sampler.rows.map((r) => r.cpu))).toFixed(1) : 0,
    apiIdle: idle,
    apiBacklog: busy,
    concurrency: Number(process.env.CV_WORKER_CONCURRENCY ?? 2),
  };
  writeFileSync(join(OUT, "summary.json"), JSON.stringify(summary, null, 2));
  writeFileSync(join(OUT, "runs.json"), JSON.stringify(rows, null, 2));
  writeFileSync(join(OUT, "resources.csv"), "t,ws_total,ws_cvworker,cpu_total_s,procs\n" + sampler.rows.map((r) => `${r.t},${r.ws},${r.worker},${r.cpu},${r.n}`).join("\n"));
  console.log(JSON.stringify(summary, null, 2));
  process.exit(0);
};
main().catch((e) => {
  console.error(e);
  process.exit(1);
});
