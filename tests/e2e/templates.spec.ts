import { expect, test } from "@playwright/test";
import { login } from "./helpers";

// Admin mengubah template email, melihat pratinjau dan riwayat, lalu mengembalikannya ke bawaan.
test("editor template pesan", async ({ page }) => {
  await login(page, "admin@arunika.example");
  await page.getByRole("navigation", { name: "Menu back office" }).getByRole("link", { name: "Template pesan" }).click();
  await expect(page.getByRole("heading", { name: "Template pesan" })).toBeVisible();

  const list = page.getByRole("navigation", { name: "Daftar template" });
  await list.getByRole("button", { name: /Keputusan: diterima\s*Email/ }).click();
  const editor = page.getByRole("region", { name: "Ubah template Keputusan: diterima" });
  await editor.getByLabel("Subjek").fill("Selamat bergabung, {{nama}}");
  await expect(editor.getByText("Selamat bergabung, Nadia Pratama")).toBeVisible(); // pratinjau data contoh

  // Placeholder asing ditolak di pratinjau dan saat menyimpan.
  await editor.getByLabel("Isi pesan").fill("Halo {{nama}}, gaji Anda {{gaji}}.");
  await expect(editor.getByText(/\{\{gaji\}\} tidak dikenal/).first()).toBeVisible();
  await editor.getByLabel("Isi pesan").fill("Halo {{nama}}, selamat bergabung sebagai {{lowongan}}.");
  await editor.getByLabel("Catatan perubahan").fill("Nada lebih hangat");
  await editor.getByRole("button", { name: "Simpan versi baru" }).click();
  await expect(list.getByRole("button", { name: /Keputusan: diterima\s*Email · Diubah, versi 1/ })).toBeVisible();
  await expect(page.getByText("Nada lebih hangat")).toBeVisible();

  await page.getByRole("region", { name: "Ubah template Keputusan: diterima" }).getByRole("button", { name: "Kembalikan ke bawaan" }).click();
  await expect(list.getByRole("button", { name: /Keputusan: diterima\s*Email · Bawaan/ })).toBeVisible();
  await expect(page.getByText(/Versi 2 \(bawaan\)/)).toBeVisible();
});
