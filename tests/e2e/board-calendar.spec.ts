import { expect, test } from "@playwright/test";
import { Api, candidateWithApplication } from "./api";
import { login, logout } from "./helpers";

// Papan: seret kartu dari Screening ke Ujian (sesuai workflow). Kalender: interview tampil untuk interviewer.
test("papan kanban dan kalender interview", async ({ page }) => {
  const stamp = String(Date.now()).slice(-5);
  const name = `Kandidat Papan ${stamp}`;
  const { applicationId } = await candidateWithApplication(`papan.${stamp}@contoh.test`, `32010145078${stamp}`, name);
  const rec = await Api.login("rekruter.ops@arunika.example");
  const jobs = await rec.call("GET", "/staff/jobs");
  const jobId = jobs.body.items.find((j: { title: string }) => j.title === "Operator Produksi").id as string;

  await login(page, "rekruter.ops@arunika.example");
  await page.goto(`/staf/lamaran?tampilan=papan&lowongan=${jobId}`);
  const col = (n: string) => page.getByRole("listitem", { name: new RegExp(`^${n}, \\d+ lamaran$`) });
  const card = col("Screening").getByRole("listitem").filter({ hasText: name });
  await expect(card).toBeVisible();

  // Ke tahap yang tidak diizinkan workflow: ditolak dengan penjelasan, tanpa dialog.
  await card.dragTo(col("Interview"));
  await expect(page.getByText(/tidak dapat dipindahkan dari Screening ke Interview/)).toBeVisible();

  await card.dragTo(col("Ujian"));
  const dlg = page.getByRole("dialog", { name: `Pindahkan ${name} ke Ujian?` });
  await dlg.getByLabel("Catatan").fill("Berkas lengkap");
  await dlg.getByRole("button", { name: "Konfirmasi" }).click();
  await expect(col("Ujian").getByRole("listitem").filter({ hasText: name })).toBeVisible();

  // Interview dua jam lagi dengan hiring manager sebagai interviewer.
  const hmId = (await (await Api.login("hm.ops@arunika.example")).call("GET", "/auth/session")).body.user.id as string;
  const startsAt = new Date(Math.ceil((Date.now() + 2 * 3_600_000) / 900_000) * 900_000).toISOString();
  const iv = await rec.call("POST", `/applications/${applicationId}/interviews`, {
    startsAt, durationMinutes: 60, timezone: "Asia/Jakarta", mode: "video", location: "https://meet.contoh.test/papan", interviewerIds: [hmId],
  });
  expect(iv.status).toBe(201);
  await logout(page);

  await login(page, "hm.ops@arunika.example");
  await page.goto("/staf/kalender");
  await page.getByLabel("Hanya interview saya").check();
  const entry = page.getByRole("link", { name });
  await page.waitForLoadState("networkidle");
  if (!(await entry.isVisible())) await page.getByRole("button", { name: "Minggu berikutnya" }).click(); // jadwal melewati Minggu malam
  await expect(entry).toBeVisible();
  await expect(page.getByText("Interviewer: Bambang Hartono").first()).toBeVisible();
});
