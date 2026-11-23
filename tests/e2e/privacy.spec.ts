import { expect, test } from "@playwright/test";
import { Api, candidateWithApplication } from "./api";
import { login } from "./helpers";

// Permintaan subjek data: cari kandidat, legal hold, lalu anonimisasi dengan konfirmasi email.
test("privasi data: legal hold dan anonimisasi", async ({ page }) => {
  const stamp = String(Date.now()).slice(-5);
  const email = `privasi.${stamp}@contoh.test`;
  const { applicationId } = await candidateWithApplication(email, `32010145076${stamp}`, `Kandidat Privasi ${stamp}`);
  const rec = await Api.login("rekruter.ops@arunika.example");
  const d = await rec.call("GET", `/applications/${applicationId}`);
  expect((await rec.call("POST", `/applications/${applicationId}/transitions`, { kind: "reject", reason: "Tidak memenuhi syarat", expectedVersion: d.body.version })).status).toBe(200);

  await login(page, "admin@arunika.example");
  await page.getByRole("navigation", { name: "Menu back office" }).getByRole("link", { name: "Privasi data" }).click();
  await expect(page.getByRole("region", { name: "Retensi data pelamar" })).toContainText("Anonimisasi otomatis mati");
  await page.getByLabel("Email kandidat").fill(email);
  await page.getByRole("button", { name: "Cari" }).click();
  await expect(page.getByText(`Kandidat Privasi ${stamp}`)).toBeVisible();
  await expect(page.getByRole("link", { name: "Unduh data (JSON)" })).toBeVisible();

  await page.getByRole("button", { name: "Pasang legal hold" }).click();
  await page.getByRole("dialog").getByLabel("Alasan").fill("Pemeriksaan internal");
  await page.getByRole("dialog").getByRole("button", { name: "Simpan" }).click();
  await expect(page.getByText(/Legal hold sejak .*: Pemeriksaan internal/)).toBeVisible();
  await page.getByRole("button", { name: "Lepas legal hold" }).click();
  await page.getByRole("dialog").getByLabel("Alasan").fill("Pemeriksaan selesai");
  await page.getByRole("dialog").getByRole("button", { name: "Simpan" }).click();
  await expect(page.getByRole("button", { name: "Pasang legal hold" })).toBeVisible();

  await page.getByRole("button", { name: "Anonimkan data" }).click();
  const dlg = page.getByRole("dialog", { name: `Anonimkan data Kandidat Privasi ${stamp}?` });
  await dlg.getByLabel("Dasar penghapusan").fill("Permintaan subjek data nomor DSR-E2E");
  await dlg.getByLabel(`Ketik ulang email ${email}`).fill("bukan@contoh.test");
  await dlg.getByRole("button", { name: "Anonimkan permanen" }).click();
  await expect(dlg.getByText(/Ketik ulang email kandidat/)).toBeVisible();
  await dlg.getByLabel(`Ketik ulang email ${email}`).fill(email);
  await dlg.getByRole("button", { name: "Anonimkan permanen" }).click();
  // Email lama tidak lagi ditemukan karena akun sudah dianonimkan.
  await expect(page.getByText("Tidak ada kandidat dengan email tersebut.")).toBeVisible();
});
