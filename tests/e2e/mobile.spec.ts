import { expect, test } from "@playwright/test";
import { candidateWithApplication, moveToExamAndInvite } from "./api";
import { login } from "./helpers";

// Viewport 390 px: tidak ada scroll horizontal, daftar staf berubah ke susunan label/nilai.
test("mobile: portal karier tanpa scroll horizontal", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("link", { name: "Operator Produksi" })).toBeVisible();
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(overflow).toBeLessThanOrEqual(0);
  await page.goto("/lowongan/operator-produksi-karawang");
  await expect(page.getByRole("button", { name: "Lamar posisi ini" })).toBeInViewport();
});

test("mobile: recruiter membuka lamaran dari daftar kartu", async ({ page }) => {
  await login(page, "rekruter.ops@arunika.example");
  await expect(page.getByRole("table")).toBeHidden();
  await page.getByRole("button", { name: "Menu" }).click();
  await expect(page.getByRole("navigation", { name: "Menu back office" })).toBeVisible();
  await page.getByRole("button", { name: "Menu" }).click();
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(overflow).toBeLessThanOrEqual(0);
});

test("mobile: ruang ujian satu kolom, navigator di drawer, action bar tidak menutup isi", async ({ page }) => {
  const email = `peserta.hp.${Date.now()}@contoh.test`;
  const c = await candidateWithApplication(email, "3201014507998802", "Sinta Ponsel");
  await moveToExamAndInvite(c.applicationId);
  await login(page, email);
  await page.goto("/akun/ujian");
  await page.getByRole("button", { name: "Mulai ujian" }).click();
  await expect(page.getByRole("heading", { name: /Soal 1 dari 6/ })).toBeVisible();
  await expect(page.getByRole("button", { name: "Selesai dan kirim" })).toBeHidden();
  await page.getByRole("button", { name: "Daftar soal" }).click();
  await page.getByRole("dialog").getByRole("button", { name: /^Soal 2,/ }).click();
  await expect(page.getByRole("heading", { name: /Soal 2 dari 6/ })).toBeVisible();
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(overflow).toBeLessThanOrEqual(0);
  // Opsi terakhir (atau kotak esai) dapat digulir ke atas action bar dan dipakai. Urutan soal diacak per attempt.
  const essay = page.getByLabel("Jawaban esai");
  if (await essay.isVisible()) {
    await essay.fill("Jawaban dari ponsel.");
  } else {
    const lastOpt = page.locator("main fieldset li label").last();
    await lastOpt.scrollIntoViewIfNeeded();
    await lastOpt.click();
  }
  await expect(page.locator("header [role=status]")).toHaveText(/Tersimpan pukul/);
});
