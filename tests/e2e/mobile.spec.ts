import { expect, test } from "@playwright/test";
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
