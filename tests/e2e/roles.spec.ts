import { expect, test } from "@playwright/test";
import { login } from "./helpers";

// Super Admin membuat peran kustom dari katalog izin, mengubahnya, lalu mengarsipkannya.
test("editor peran kustom", async ({ page }) => {
  const name = `Peninjau Laporan ${String(Date.now()).slice(-4)}`.replace(/\d/g, (d) => "abcdefghij"[Number(d)]!);
  await login(page, "admin@arunika.example");
  await page.goto("/staf/pengguna");
  const section = page.getByRole("region", { name: "Peran" });
  await expect(section.getByText("Recruiter").first()).toBeVisible();
  await section.getByRole("button", { name: "Buat peran" }).click();

  const dlg = page.getByRole("dialog", { name: "Buat peran" });
  await expect(dlg.getByLabel("Mengelola peran")).toHaveCount(0); // izin khusus Super Admin tidak ditawarkan
  await dlg.getByLabel("Nama peran").fill(name);
  await dlg.getByLabel("Melihat laporan").check();
  await dlg.getByRole("button", { name: "Simpan" }).click();
  const row = section.getByRole("listitem").filter({ hasText: name });
  await expect(row).toContainText("Kustom · 1 izin · 0 penugasan aktif");

  await row.getByRole("button", { name: "Ubah" }).click();
  const edit = page.getByRole("dialog", { name: `Ubah peran ${name}` });
  await edit.getByLabel("Mengekspor laporan").check();
  await edit.getByRole("button", { name: "Simpan" }).click();
  await expect(row).toContainText("2 izin");

  await row.getByRole("button", { name: "Arsipkan" }).click();
  await expect(section.getByRole("listitem").filter({ hasText: name })).toHaveCount(0);
});
