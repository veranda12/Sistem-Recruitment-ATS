import { expect, test } from "@playwright/test";
import { candidateWithApplication } from "./api";
import { login, logout } from "./helpers";

// HR menandai shortlist dan menugaskan recruiter; recruiter menemukan lamaran lewat filter "Ditugaskan ke saya".
test("shortlist dan penugasan recruiter", async ({ page }) => {
  const stamp = String(Date.now()).slice(-5);
  const name = `Kandidat Penanganan ${stamp}`;
  const { applicationId } = await candidateWithApplication(`penanganan.${stamp}@contoh.test`, `32010145079${stamp}`, name);

  await login(page, "hr.ops@arunika.example");
  await page.goto(`/staf/lamaran/${applicationId}`);
  const handling = page.getByRole("region", { name: "Penanganan" });
  await expect(handling).toContainText("Recruiter: Belum ditugaskan");
  await handling.getByRole("button", { name: "Tambahkan ke shortlist" }).click();
  await expect(handling).toContainText("Masuk shortlist");

  await handling.getByRole("button", { name: "Ubah recruiter" }).click();
  const dlg = page.getByRole("dialog", { name: "Ubah recruiter" });
  await dlg.getByLabel("Recruiter").selectOption({ label: "Maya Sari" });
  await dlg.getByRole("button", { name: "Simpan" }).click();
  await expect(handling).toContainText("Recruiter: Maya Sari");
  await logout(page);

  await login(page, "rekruter.ops@arunika.example");
  await page.goto("/staf/lamaran");
  await page.getByLabel("Penanganan").selectOption({ label: "Ditugaskan ke saya" });
  await expect(page).toHaveURL(/recruiter=me/);
  const row = page.getByRole("row").filter({ hasText: name });
  await expect(row).toContainText("Shortlist");
  await expect(row).toContainText("Maya Sari");
  // Recruiter tidak berwenang menugaskan, tetapi boleh mengubah shortlist.
  await row.getByRole("link", { name }).click();
  await expect(page.getByRole("button", { name: "Ubah recruiter" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Lepas dari shortlist" })).toBeVisible();
});
