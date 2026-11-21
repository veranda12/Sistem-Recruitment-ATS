// Skenario k6 ujian. Setiap VU = satu peserta sintetis unik (fixture dari backend/cmd/loadseed), cookie jar sendiri.
// Profil: PROFILE=smoke|baseline|ramp|burst-login|burst-submit. Contoh:
//   k6 run -e BASE=http://127.0.0.1:8080 -e ORIGIN=http://localhost:5173 -e FIXTURE=fixture.json -e PASSWORD=... -e PROFILE=smoke tests/load/exam.js
import http from "k6/http";
import { check, sleep } from "k6";
import { Counter, Trend } from "k6/metrics";
import exec from "k6/execution";
import { SharedArray } from "k6/data";

const BASE = __ENV.BASE || "http://127.0.0.1:8080";
const ORIGIN = __ENV.ORIGIN || "http://localhost:5173";
const PASSWORD = __ENV.PASSWORD;
const PROFILE = __ENV.PROFILE || "smoke";
const EXAM_SECONDS = Number(__ENV.EXAM_SECONDS || 120); // lama mengerjakan per VU
const SAVE_EVERY = Number(__ENV.SAVE_EVERY || 10); // rata-rata satu batch save per 10 detik
const SEED = Number(__ENV.SEED || 20261121);
const fixture = new SharedArray("peserta", () => JSON.parse(open(__ENV.FIXTURE)).participants);

const scenarios = {
  smoke: { executor: "per-vu-iterations", vus: 5, iterations: 1, maxDuration: "15m" },
  baseline: { executor: "per-vu-iterations", vus: Number(__ENV.VUS || 100), iterations: 1, maxDuration: "30m" },
  // Naik bertahap ke 500 attempt aktif dan bertahan (plateau) selama EXAM_SECONDS; peserta yang selesai tidak diganti.
  ramp: { executor: "ramping-vus", startVUs: 0, stages: [{ duration: "2m", target: 100 }, { duration: "3m", target: 500 }, { duration: `${EXAM_SECONDS + 120}s`, target: 500 }], gracefulRampDown: "2m" },
  // Burst tersinkron: 500 login/start dalam 30 detik.
  "burst-login": { executor: "shared-iterations", vus: 500, iterations: 500, maxDuration: "10m" },
  // Burst submit: VU mengerjakan lalu submit serentak pada SUBMIT_AT (epoch ms) dalam jendela 10 detik.
  "burst-submit": { executor: "per-vu-iterations", vus: Number(__ENV.VUS || 500), iterations: 1, maxDuration: "30m" },
};

export const options = {
  scenarios: { exam: scenarios[PROFILE] },
  // Ambang dari dokumen target; dinilai per operasi dengan tag berkardinalitas rendah.
  thresholds: {
    "op_duration{op:save}": ["p(95)<400", "p(99)<1000"],
    "op_duration{op:resume}": ["p(95)<800"],
    "op_duration{op:submit}": ["p(95)<1000", "p(99)<2000"],
    "op_duration{op:login}": ["p(95)<1500"],
    "op_duration{op:start}": ["p(95)<1500"],
    "op_duration{op:heartbeat}": ["p(95)<1000"],
    "unexpected_errors": ["count<1"],
    "submits_ok": [`count>=${Math.floor((scenarios[PROFILE]?.vus || 500) * 0.995)}`],
  },
  summaryTrendStats: ["avg", "med", "p(95)", "p(99)", "max", "count"],
};

const opDuration = new Trend("op_duration", true);
const unexpected = new Counter("unexpected_errors");
const rejected = new Counter("rejected_responses"); // 429/503 dihitung sebagai kegagalan pada beban nominal
const retries = new Counter("retries");
const ackedOps = new Counter("acked_ops"); // opId unik yang mendapat ACK commit
const conflicts = new Counter("save_conflicts");
const submitsOk = new Counter("submits_ok");

// PRNG deterministik per VU (mulberry32) agar pilihan jawaban dan think time dapat diulang.
function rng(seed) {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function req(op, method, path, body, csrf, extraHeaders = {}) {
  const headers = { "Content-Type": "application/json", Origin: ORIGIN, ...extraHeaders };
  if (csrf) headers["X-CSRF-Token"] = csrf;
  const res = http.request(method, `${BASE}/api/v1${path}`, body === undefined ? null : JSON.stringify(body), { headers, tags: { op, name: op }, timeout: "30s" });
  opDuration.add(res.timings.duration, { op });
  if (res.status === 429 || res.status === 503) rejected.add(1, { op });
  return res;
}

// Mengirim ulang permintaan yang sama persis (opId/kunci idempoten sama) saat jaringan/5xx/429. Kegagalan awal tetap tercatat.
function withRetry(op, fn) {
  for (let i = 0; i < 5; i++) {
    const r = fn();
    if (r.status !== 0 && r.status < 500 && r.status !== 429) return r;
    retries.add(1, { op });
    const ra = Number(r.headers["Retry-After"] || 0);
    sleep(ra > 0 ? ra : 0.5 * 2 ** i * (0.7 + 0.6 * Math.random()));
  }
  return fn();
}

export default function () {
  const idx = exec.scenario.iterationInTest;
  const p = fixture[idx];
  if (!p) {
    sleep(5); // peserta habis: VU menganggur, tidak mengganti peserta yang sudah selesai dengan login berulang
    return;
  }
  const rand = rng(SEED + idx);
  const think = (min, max) => sleep(min + rand() * (max - min));

  sleep(rand() * (PROFILE === "burst-login" ? 30 : 5)); // sebaran awal (burst: 500 dalam 30 detik)
  const lg = withRetry("login", () => req("login", "POST", "/auth/login", { email: p.email, password: PASSWORD }));
  if (!check(lg, { "login 200": (r) => r.status === 200 })) return unexpected.add(1, { op: "login" });
  const csrf = lg.json("csrfToken");

  const st = withRetry("start", () => req("start", "POST", `/exam-invitations/${p.invitationId}/start`, {}, csrf));
  if (!check(st, { "start 200/201": (r) => r.status === 200 || r.status === 201 })) return unexpected.add(1, { op: "start" });
  if (PROFILE === "burst-login") return;
  const attempt = st.json();
  const items = attempt.items;
  const rev = {};
  for (const a of attempt.answers) rev[a.itemId] = a.revision;
  let attemptRev = attempt.revision;

  const ld = req("resume", "GET", `/exam-attempts/${attempt.id}`);
  if (ld.status !== 200) unexpected.add(1, { op: "resume" });

  let opSeq = 0;
  const dirty = {};
  let lastSave = Date.now();
  let lastBeat = Date.now();
  const end = Date.now() + EXAM_SECONDS * 1000;
  let cur = 0;
  while (Date.now() < end) {
    // Navigasi + menjawab/mengubah jawaban.
    const it = items[cur];
    if (it.kind === "essay") dirty[it.itemId] = { text: `Jawaban peserta ${idx} versi ${opSeq} ` + "x".repeat(Math.floor(rand() * 200)) };
    else if (it.kind === "multiple") dirty[it.itemId] = { selected: it.options.filter(() => rand() < 0.5).map((o) => o.id).slice(0, 3) };
    else dirty[it.itemId] = { selected: [it.options[Math.floor(rand() * it.options.length)].id] };
    if (rand() < 0.7) cur = (cur + 1) % items.length;
    think(2, 6);

    if ((Date.now() - lastSave) / 1000 >= SAVE_EVERY * (0.7 + 0.6 * rand()) && Object.keys(dirty).length) {
      const opId = `k6-${idx}-${++opSeq}-${SEED}`;
      const answers = Object.keys(dirty).map((id) => ({ itemId: id, value: dirty[id], baseRevision: rev[id] || 0 }));
      const sv = withRetry("save", () => req("save", "PUT", `/exam-attempts/${attempt.id}/answers`, { opId, answers }, csrf));
      if (sv.status === 200) {
        ackedOps.add(1);
        for (const x of sv.json("items")) rev[x.itemId] = x.revision;
        attemptRev = sv.json("revision");
        for (const k of Object.keys(dirty)) delete dirty[k];
      } else if (sv.status === 409 && sv.json("error.code") === "answer_conflict") {
        conflicts.add(1);
      } else unexpected.add(1, { op: "save" });
      lastSave = Date.now();
    }
    if (Date.now() - lastBeat > 30000 * (0.8 + 0.4 * rand())) {
      req("heartbeat", "POST", `/exam-attempts/${attempt.id}/heartbeat`, {}, csrf);
      lastBeat = Date.now();
    }
    if (rand() < 0.02) {
      const r = req("resume", "GET", `/exam-attempts/${attempt.id}`);
      if (r.status !== 200) unexpected.add(1, { op: "resume" });
      else attemptRev = r.json("revision");
    }
  }

  // Burst submit: semua VU menunggu titik waktu yang sama lalu submit tersebar dalam 10 detik.
  if (__ENV.SUBMIT_AT) {
    const wait = Number(__ENV.SUBMIT_AT) - Date.now();
    if (wait > 0) sleep(wait / 1000);
    sleep(rand() * 10);
  }
  // Submit membawa batch terakhir; Idempotency-Key stabil per peserta sehingga retry mengembalikan receipt yang sama.
  const answers = Object.keys(dirty).map((id) => ({ itemId: id, value: dirty[id], baseRevision: rev[id] || 0 }));
  const body = { expectedRevision: attemptRev, opId: `k6-${idx}-final-${SEED}`, answers };
  if (!answers.length) delete body.answers;
  const sb = withRetry("submit", () => req("submit", "POST", `/exam-attempts/${attempt.id}/submit`, body, csrf, { "Idempotency-Key": `k6-submit-${idx}-${SEED}` }));
  if (check(sb, { "submit 200": (r) => r.status === 200 && !!r.json("code") })) {
    submitsOk.add(1);
    if (answers.length) ackedOps.add(1);
  } else unexpected.add(1, { op: "submit" });
}

export function handleSummary(data) {
  const out = __ENV.SUMMARY || "tests/load/results/summary.json";
  return { [out]: JSON.stringify(data, null, 2), stdout: textSummary(data) };
}

function textSummary(d) {
  const m = d.metrics;
  const line = (op) => {
    const v = m[`op_duration{op:${op}}`]?.values;
    return v ? `${op.padEnd(9)} n=${v.count} p95=${v["p(95)"].toFixed(0)}ms p99=${v["p(99)"].toFixed(0)}ms max=${v.max.toFixed(0)}ms` : `${op}: -`;
  };
  return [
    `profil=${PROFILE}`,
    ...["login", "start", "resume", "save", "heartbeat", "submit"].map(line),
    `acked_ops=${m.acked_ops?.values.count ?? 0} submits_ok=${m.submits_ok?.values.count ?? 0} conflicts=${m.save_conflicts?.values.count ?? 0}`,
    `unexpected=${m.unexpected_errors?.values.count ?? 0} rejected=${m.rejected_responses?.values.count ?? 0} retries=${m.retries?.values.count ?? 0}`,
    `http_req_failed=${(m.http_req_failed.values.rate * 100).toFixed(2)}% iterasi selesai=${m.iterations?.values.count ?? 0} dropped=${m.dropped_iterations?.values.count ?? 0}`,
    "",
  ].join("\n");
}
