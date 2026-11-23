import { expect, test } from "@playwright/test";
import { candidateWithApplication, moveToExamAndInvite } from "./api";
import { login } from "./helpers";

// Log perilaku ujian: peserta diberi tahu, aktivitas tempel tercatat, HR melihat ringkasan dan detail di monitor.
test("log perilaku peserta ujian", async ({ page, browser }) => {
  test.setTimeout(60_000);
  const stamp = String(Date.now()).slice(-5);
  const name = `Peserta Pengawasan ${stamp}`;
  const email = `pengawasan.${stamp}@contoh.test`;
  const { applicationId } = await candidateWithApplication(email, `32010145077${stamp}`, name);
  const { scheduleId } = await moveToExamAndInvite(applicationId);

  await login(page, email);
  await page.goto("/akun/ujian");
  await page.getByRole("button", { name: "Mulai ujian" }).click();
  await expect(page.getByText(/menyalin dan menempel teks dicatat untuk panitia\. Kamera dan isi layar tidak direkam/)).toBeVisible();
  // Peristiwa tempel dari papan klip (dikirim sebagai event agar tidak bergantung pada izin clipboard browser uji).
  await page.evaluate(() => document.dispatchEvent(new Event("paste", { bubbles: true })));
  await expect
    .poll(() => page.evaluate(() => Object.keys(localStorage).some((k) => k.startsWith("exam-activity:") && JSON.parse(localStorage.getItem(k)!).pending.length === 0)), { timeout: 15_000 })
    .toBe(true);

  // Ruang ujian tidak punya tombol keluar; HR memakai konteks browser terpisah.
  const hrPage = await (await browser.newContext()).newPage();
  await login(hrPage, "hr.ops@arunika.example");
  await hrPage.goto(`/staf/ujian/${scheduleId}`);
  const row = hrPage.getByRole("row").filter({ hasText: name });
  await expect(row).toContainText("Tempel 1x");
  await row.getByRole("button", { name: `Detail catatan perilaku ${name}` }).click();
  await expect(hrPage.getByRole("dialog", { name: `Catatan perilaku: ${name}` }).getByText("Tempel")).toBeVisible();
});
