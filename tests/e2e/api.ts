import { expect, request, type APIRequestContext } from "@playwright/test";
import { linkFromMail, PASSWORD } from "./helpers";

const BASE = "http://localhost:15173";

/** Klien API bersesi untuk menyiapkan data uji (cookie + CSRF), terpisah dari browser yang diuji. */
export class Api {
  private constructor(
    readonly ctx: APIRequestContext,
    private csrf: string,
  ) {}

  static async login(email: string, password = PASSWORD) {
    const ctx = await request.newContext({ baseURL: BASE, extraHTTPHeaders: { Origin: BASE } });
    const r = await ctx.post("/api/v1/auth/login", { data: { email, password } });
    expect(r.status(), `login ${email}`).toBe(200);
    return new Api(ctx, (await r.json()).csrfToken);
  }

  async call(method: "GET" | "POST" | "PUT", path: string, data?: unknown, headers: Record<string, string> = {}) {
    const r = await this.ctx.fetch(`/api/v1${path}`, { method, data, headers: { "X-CSRF-Token": this.csrf, ...headers } });
    const body = r.status() === 204 ? null : await r.json();
    return { status: r.status(), body };
  }
}

/** Kandidat baru yang terverifikasi, profil lengkap, dan sudah melamar Operator Produksi. */
export async function candidateWithApplication(email: string, nik: string, name: string) {
  const anon = await request.newContext({ baseURL: BASE, extraHTTPHeaders: { Origin: BASE } });
  const reg = await anon.post("/api/v1/auth/register", { data: { email, password: PASSWORD, fullName: name, nik, acceptPrivacy: true } });
  expect(reg.status()).toBe(202);
  const token = new URL(await linkFromMail(email)).searchParams.get("token");
  expect((await anon.post("/api/v1/auth/verify-email", { data: { token } })).status()).toBe(200);
  const c = await Api.login(email);
  const p = await c.call("GET", "/me/profile");
  const put = await c.call("PUT", "/me/profile", {
    fullName: name, phone: "081234567890", birthDate: "1999-04-12", city: "Karawang", address: "", summary: "",
    education: [{ level: "SMK", institution: "SMK Negeri 1 Karawang", major: "Mesin", startYear: 2014, endYear: 2017, gpa: null }],
    experiences: [], certifications: [], version: p.body.version,
  });
  expect(put.status).toBe(200);
  const ap = await c.call("POST", "/jobs/operator-produksi-karawang/apply", {});
  expect(ap.status).toBe(201);
  return { api: c, applicationId: ap.body.applicationId as string };
}

/** Recruiter memindahkan ke tahap Ujian, HR mengundang ke jadwal seed. */
export async function moveToExamAndInvite(applicationId: string) {
  const rec = await Api.login("rekruter.ops@arunika.example");
  const det = await rec.call("GET", `/applications/${applicationId}`);
  const tr = await rec.call("POST", `/applications/${applicationId}/transitions`, { kind: "manual", to: "ujian", reason: "Lolos berkas", expectedVersion: det.body.version });
  expect(tr.status).toBe(200);
  const hr = await Api.login("hr.ops@arunika.example");
  const sch = await hr.call("GET", "/exam-schedules");
  const scheduleId = sch.body.items[0].id as string;
  const inv = await hr.call("POST", `/exam-schedules/${scheduleId}/invitations`, { applicationIds: [applicationId] });
  expect(inv.body.results[0].outcome).toBe("invited");
  return { hr, scheduleId };
}

/** Kandidat baru terverifikasi dengan profil awal (tanpa pengalaman), untuk uji pengisian profil dari CV. */
export async function verifiedCandidate(email: string, nik: string, name: string) {
  const anon = await request.newContext({ baseURL: BASE, extraHTTPHeaders: { Origin: BASE } });
  const reg = await anon.post("/api/v1/auth/register", { data: { email, password: PASSWORD, fullName: name, nik, acceptPrivacy: true } });
  expect(reg.status()).toBe(202);
  const token = new URL(await linkFromMail(email)).searchParams.get("token");
  expect((await anon.post("/api/v1/auth/verify-email", { data: { token } })).status()).toBe(200);
  return Api.login(email);
}
