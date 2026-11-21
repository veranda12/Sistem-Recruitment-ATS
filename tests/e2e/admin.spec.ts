import { expect, test } from "@playwright/test";
import { readFileSync } from "node:fs";
import { candidateWithApplication } from "./api";
import { login } from "./helpers";

test("HR: dashboard laporan dan unduh ekspor funnel", async ({ page }) => {
  await candidateWithApplication(`laporan.${Date.now()}@contoh.test`, "3201014507998803", "Kandidat Laporan");
  await login(page, "hr.ops@arunika.example");
  await page.getByRole("navigation", { name: "Menu back office" }).getByRole("link", { name: "Laporan" }).click();
  await expect(page.getByRole("heading", { name: "Laporan rekrutmen" })).toBeVisible();
  await expect(page.getByText("Kandidat unik")).toBeVisible();
  await page.getByText("Cara menghitung").click();
  await expect(page.getByText(/Kandidat dengan dua lamaran dihitung dua kali/)).toBeVisible();

  await page.getByRole("button", { name: "Ekspor funnel (CSV)" }).click();
  await expect(page.getByText(/Ekspor masuk antrean/)).toBeVisible();
  await expect
    .poll(async () => {
      await page.getByRole("button", { name: "Muat ulang daftar" }).click();
      return page.getByRole("button", { name: "Unduh" }).count();
    }, { timeout: 15_000 })
    .toBeGreaterThan(0);
  const [download] = await Promise.all([page.waitForEvent("download"), page.getByRole("button", { name: "Unduh" }).first().click()]);
  const csv = readFileSync((await download.path())!, "utf8");
  expect(csv).toContain("lowongan;lamaran;kandidat_unik");

  // Ekspor data pribadi ditolak saat limiter global tidak tersedia (mode degraded tanpa Redis).
  await page.getByRole("button", { name: "Ekspor data pelamar (CSV)" }).click();
  await expect(page.getByText(/sementara tidak tersedia/)).toBeVisible();
});

test("Super admin: matriks peran, admin terakhir tidak bisa dicabut, audit", async ({ page }) => {
  await login(page, "admin@arunika.example");
  await page.getByRole("navigation", { name: "Menu back office" }).getByRole("link", { name: "Pengguna" }).click();
  await page.getByText("Matriks peran dan izin").click();
  await expect(page.getByRole("columnheader", { name: "Hiring Manager" })).toBeVisible();
  await page.getByRole("button", { name: "Cabut Super Admin dari Rangga Admin" }).click();
  await expect(page.getByText("Tidak dapat menghapus Super Admin aktif terakhir.")).toBeVisible();
  await page.getByRole("navigation", { name: "Menu back office" }).getByRole("link", { name: "Audit" }).click();
  await expect(page.getByRole("cell", { name: "seed.run" })).toBeVisible();
});
