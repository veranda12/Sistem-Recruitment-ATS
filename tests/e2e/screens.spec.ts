import { test } from "@playwright/test";
import { login } from "./helpers";

// Screenshot untuk tinjauan visual (data, kosong, error). Disimpan di frontend/test-results/screens.
const shot = (name: string) => `test-results/screens/${test.info().project.name}-${name}.png`;

test("screenshot tinjauan", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("link", { name: "Operator Produksi" }).waitFor();
  await page.screenshot({ path: shot("karier"), fullPage: true });
  await page.goto("/?q=tidakada");
  await page.getByText("Tidak ada lowongan yang cocok").waitFor();
  await page.screenshot({ path: shot("karier-kosong"), fullPage: true });
  await page.goto("/lowongan/admin-gudang-surabaya");
  await page.getByText("Lowongan ini sudah ditutup").waitFor();
  await page.screenshot({ path: shot("lowongan-tutup"), fullPage: true });
  await page.goto("/daftar");
  await page.getByRole("button", { name: "Daftar" }).click();
  await page.getByText("Periksa kembali isian").waitFor();
  await page.screenshot({ path: shot("daftar-error"), fullPage: true });
  await login(page, "hr.ops@arunika.example");
  await page.waitForURL(/staf\/lamaran/);
  await page.waitForTimeout(500);
  await page.screenshot({ path: shot("staf-lamaran"), fullPage: true });
  const first = page.getByRole("link").filter({ hasText: /Amelia|Wulandari|Santoso/ }).first();
  if (await first.count()) {
    await first.click();
    await page.getByText("Riwayat tahap").waitFor();
    await page.screenshot({ path: shot("staf-detail"), fullPage: true });
  }
  await page.goto("/staf/workflow");
  await page.getByRole("link", { name: "Rekrutmen Standar" }).click();
  await page.getByText("Simulasi hasil ujian").waitFor();
  await page.screenshot({ path: shot("workflow"), fullPage: true });
});
