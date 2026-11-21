import { expect, test } from "@playwright/test";
import { resolve } from "node:path";
import { verifiedCandidate } from "./api";
import { login } from "./helpers";

const fixture = (split: string, name: string) => resolve(process.cwd(), "..", "tests", "cv-fixtures", "out", split, name);

// Unggah CV dua kolom -> bukti -> koreksi -> refresh -> simpan ke profil. Membutuhkan fixture (go run ./cmd/cvfixtures)
// dan tool pembaca CV pada mesin uji.
test("isi profil dari CV: bukti, koreksi, refresh, simpan", async ({ page }) => {
  test.setTimeout(150_000);
  const email = `sinta.e2e.${Date.now()}@contoh.test`;
  await verifiedCandidate(email, `32730145079${String(Date.now()).slice(-5)}`, "Sinta E2E");
  await login(page, email);
  await page.goto("/akun/profil");

  await page.getByLabel("Berkas CV").setInputFiles(fixture("dev", "sinta.pdf-2col.pdf"));
  await page.getByRole("button", { name: "Isi profil dari CV" }).click();
  await expect(page).toHaveURL(/\/akun\/profil\/cv\//);
  await expect(page.getByRole("heading", { name: "Periksa data dari CV" })).toBeVisible();
  await expect(page.getByText("Siap diperiksa")).toBeVisible({ timeout: 90_000 });

  // Pengalaman kedua: pasangan jabatan + perusahaan, dan "Lihat sumber" menunjuk potongan CV yang benar.
  const exp2 = page.getByRole("region", { name: /Junior Developer di CV Kreasi Digital Mandiri/ });
  await expect(exp2).toBeVisible();
  await exp2.getByRole("button", { name: "Lihat sumber Periode" }).click();
  const source = page.getByRole("complementary", { name: "Sumber di CV" });
  await expect(source.locator("blockquote").first()).toContainText("Jan 2020");
  await expect(source.getByRole("img", { name: /Potongan halaman 1 CV/ })).toBeVisible();

  // Koreksi satu digit telepon; tolak email dari CV.
  const contact = page.getByRole("region", { name: "Kontak" });
  await contact.getByRole("group", { name: "Keputusan untuk Telepon" }).getByRole("button", { name: "Ubah" }).click();
  await contact.getByLabel("Telepon").fill("0812-3456-7809");
  await contact.getByRole("button", { name: "Simpan perubahan" }).click();
  await expect(contact.getByText("+6281234567809")).toBeVisible();
  await contact.getByRole("group", { name: "Keputusan untuk Email" }).getByRole("button", { name: "Tolak" }).click();
  await page.getByRole("button", { name: "Terima semua yang tidak perlu diperiksa" }).click();
  await expect(page.getByText(/sudah diputuskan/)).toBeVisible();

  // Refresh: pilihan tersimpan di server.
  await page.reload();
  await expect(page.getByText("Siap diperiksa")).toBeVisible();
  await expect(page.getByRole("region", { name: "Kontak" }).getByText("+6281234567809")).toBeVisible();

  await page.getByRole("button", { name: "Simpan pilihan ke profil" }).click();
  await expect(page.getByText("Profil diperbarui")).toBeVisible();

  await page.getByRole("link", { name: "Lihat profil" }).click();
  await expect(page.getByLabel("Nomor telepon")).toHaveValue("+6281234567809");
  await expect(page.getByText(`Email: ${email}`)).toBeVisible(); // email akun tidak berubah
  await expect(page.getByLabel("Email kontak")).toHaveValue("");
  const companies = await page.getByLabel("Perusahaan").evaluateAll((els) => els.map((e) => (e as HTMLInputElement).value));
  expect(companies).toEqual(expect.arrayContaining(["PT Nusantara Data Teknologi", "CV Kreasi Digital Mandiri"]));
  expect(companies).toHaveLength(2);
});
