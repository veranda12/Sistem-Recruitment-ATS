import { expect, test } from "@playwright/test";
import { Api, candidateWithApplication, moveToExamAndInvite } from "./api";
import { login, logout } from "./helpers";

// Alur demonstrasi: rilis nilai dengan esai belum dinilai -> review manual -> interview -> persetujuan HM ->
// pemberitahuan oleh recruiter -> HR menelusuri pelaku dan alasan.
test.describe.serial("keputusan rekrutmen", () => {
  const email = `keputusan.${Date.now()}@contoh.test`;
  let appId = "";

  test("persiapan: ujian dengan esai belum dinilai lalu dirilis -> tahap review", async () => {
    const c = await candidateWithApplication(email, "3201014507998804", "Laras Keputusan");
    appId = c.applicationId;
    const { hr, scheduleId } = await moveToExamAndInvite(appId);
    const inv = await c.api.call("GET", "/me/exam-invitations");
    const st = await c.api.call("POST", `/exam-invitations/${inv.body.items[0].id}/start`, {});
    expect(st.status).toBe(201);
    const essay = st.body.items.find((i: { kind: string }) => i.kind === "essay");
    const save = await c.api.call("PUT", `/exam-attempts/${st.body.id}/answers`, {
      opId: "op-keputusan-001",
      answers: [{ itemId: essay.itemId, baseRevision: 0, value: { text: "Hentikan lini dan laporkan." } }],
    });
    expect(save.status).toBe(200);
    const sub = await c.api.call("POST", `/exam-attempts/${st.body.id}/submit`, { expectedRevision: save.body.revision }, { "Idempotency-Key": "submit-keputusan-1" });
    expect(sub.status).toBe(200);
    expect((await hr.call("POST", `/exam-schedules/${scheduleId}/release`, {})).status).toBe(200);
    await expect.poll(async () => (await hr.call("GET", `/applications/${appId}`)).body.stage.key, { timeout: 15_000 }).toBe("review");
  });

  test("recruiter: lanjut interview, jadwalkan dengan HM, ajukan persetujuan", async ({ page }) => {
    await login(page, "rekruter.ops@arunika.example");
    await page.goto(`/staf/lamaran/${appId}`);
    await page.getByRole("button", { name: /Lanjut interview: ke Interview/ }).click();
    await page.getByRole("dialog").getByLabel("Catatan").fill("Esai cukup baik setelah review");
    await page.getByRole("dialog").getByRole("button", { name: "Konfirmasi" }).click();
    await expect(page.getByText("Perubahan tersimpan.")).toBeVisible();

    await page.getByRole("button", { name: "Jadwalkan interview" }).click();
    const dlg = page.getByRole("dialog");
    const d = new Date(Date.now() + 3 * 86_400_000);
    const local = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}T10:00`;
    await dlg.getByLabel("Tanggal dan jam").fill(local);
    await dlg.getByLabel("Lokasi atau tautan").fill("Ruang rapat Karawang");
    await dlg.getByLabel("Bambang Hartono").check();
    await dlg.getByRole("button", { name: "Simpan" }).click();
    await expect(page.getByText(/10\.00 WIB/)).toBeVisible();
    await expect(page.getByText(/Interviewer: Bambang Hartono/)).toBeVisible();

    await page.getByRole("button", { name: /Ajukan persetujuan: ke Persetujuan/ }).click();
    await page.getByRole("dialog").getByRole("button", { name: "Konfirmasi" }).click();
    await expect(page.getByText("Tahap: Persetujuan")).toBeVisible();
    await logout(page);
  });

  test("hiring manager: isi penilaian lalu menyetujui", async ({ page }) => {
    await login(page, "hm.ops@arunika.example");
    await page.goto(`/staf/lamaran/${appId}`);
    await page.getByRole("button", { name: "Isi penilaian" }).click();
    const dlg = page.getByRole("dialog");
    await dlg.getByLabel("Catatan").fill("Paham keselamatan kerja");
    await dlg.getByRole("button", { name: "Simpan" }).click();
    await expect(page.getByText(/Bambang Hartono: 4\/5, Lanjut/)).toBeVisible();
    await page.getByRole("button", { name: "Setujui", exact: true }).click();
    await page.getByRole("dialog").getByLabel("Catatan").fill("Sesuai kebutuhan lini");
    await page.getByRole("dialog").getByRole("button", { name: "Konfirmasi" }).click();
    await expect(page.getByText("Diterima").first()).toBeVisible();
    await logout(page);
  });

  test("recruiter mengirim pemberitahuan; HR menelusuri; kandidat menerima hasil", async ({ page }) => {
    await login(page, "rekruter.ops@arunika.example");
    await page.goto(`/staf/lamaran/${appId}`);
    await page.getByRole("button", { name: "Kirim pemberitahuan keputusan" }).click();
    await expect(page.getByText(/Pemberitahuan keputusan sudah dikirim/)).toBeVisible();
    await logout(page);

    // HR: histori menjelaskan siapa, kapan, dan mengapa.
    await login(page, "hr.ops@arunika.example");
    await page.goto(`/staf/lamaran/${appId}`);
    await expect(page.getByText("Alasan: Esai cukup baik setelah review")).toBeVisible();
    await expect(page.getByText(/Persetujuan: Persetujuan ke Diterima/)).toBeVisible();
    await expect(page.getByText(/Hasil ujian: Ujian ke Review/)).toBeVisible();
    await page.getByRole("navigation", { name: "Menu back office" }).getByRole("link", { name: "Audit" }).click();
    await page.getByLabel("Awalan tindakan").fill("application.");
    await page.getByRole("button", { name: "Saring" }).click();
    await expect(page.getByRole("cell", { name: "application.decision_notice" })).toBeVisible();
    await expect(page.getByRole("cell", { name: "application.approval" })).toBeVisible();

    // Kandidat menerima hasil seleksi.
    const cand = await Api.login(email);
    await expect
      .poll(async () => (await cand.call("GET", "/me/notifications")).body.items.map((n: { subject: string }) => n.subject).join("|"), { timeout: 15_000 })
      .toContain("Hasil seleksi");
  });
});
