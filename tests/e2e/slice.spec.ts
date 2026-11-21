import { expect, test } from "@playwright/test";
import { linkFromMail, login, logout, PASSWORD } from "./helpers";

// Alur pertama: kandidat mendaftar sampai melamar dua lowongan, recruiter membuka dan memindahkan lamaran.
test.describe.serial("vertical slice karier -> recruiter", () => {
  const email = `rina.e2e.${Date.now()}@contoh.test`;

  test("portal karier: daftar, filter di URL, detail, lowongan tutup", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByRole("heading", { level: 1 })).toContainText("PT Arunika Daya Persada");
    await expect(page.getByRole("link", { name: "Operator Produksi" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Data Analyst" })).toHaveCount(0);

    await page.getByLabel("Unit").selectOption({ label: "Teknologi Informasi" });
    await expect(page).toHaveURL(/unit=TI/);
    await expect(page.getByRole("link", { name: "Backend Engineer" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Operator Produksi" })).toHaveCount(0);
    await page.reload();
    await expect(page.getByLabel("Unit")).toHaveValue("TI");

    await page.getByLabel("Kata kunci").fill("tidak-ada-yang-cocok");
    await page.getByRole("button", { name: "Cari" }).click();
    await expect(page.getByText("Tidak ada lowongan yang cocok dengan filter ini.")).toBeVisible();

    await page.goto("/lowongan/admin-gudang-surabaya");
    await expect(page.getByText("Lowongan ini sudah ditutup")).toBeVisible();
    await expect(page.getByRole("button", { name: "Lamar posisi ini" })).toHaveCount(0);
  });

  test("kandidat: daftar, verifikasi, lengkapi profil, melamar dua lowongan", async ({ page }) => {
    // CTA Lamar tanpa login mengarah ke login dengan tujuan kembali.
    await page.goto("/lowongan/operator-produksi-karawang");
    await page.getByRole("button", { name: "Lamar posisi ini" }).click();
    await expect(page).toHaveURL(/\/masuk\?next=/);

    await page.goto("/daftar");
    await page.getByRole("button", { name: "Daftar" }).click();
    await expect(page.getByText("Periksa kembali isian yang ditandai.")).toBeVisible();
    await page.getByLabel("Nama lengkap sesuai KTP").fill("Rina Amelia");
    await page.getByLabel("Email").fill(email);
    await page.getByLabel("NIK").fill("3201 0145 0799 0777");
    await page.getByLabel("Kata sandi").fill(PASSWORD);
    await page.getByRole("checkbox").check();
    await page.getByRole("button", { name: "Daftar" }).click();
    await expect(page.getByText("Pendaftaran diproses")).toBeVisible();

    await page.goto(await linkFromMail(email));
    await expect(page.getByText("Email terverifikasi")).toBeVisible();

    await login(page, email);
    await page.goto("/akun/profil");
    await expect(page.getByText("Profil belum lengkap")).toBeVisible();
    await expect(page.getByText("************0777")).toBeVisible();
    await page.getByLabel("Nomor telepon").fill("081234567890");
    await page.getByLabel("Tanggal lahir").fill("1999-04-12");
    await page.getByLabel("Kota domisili").fill("Karawang");
    await page.getByRole("button", { name: "Tambah pendidikan" }).click();
    await page.getByLabel("Institusi").fill("SMK Negeri 1 Karawang");
    await page.getByRole("button", { name: "Simpan profil" }).click();
    await expect(page.getByText("Profil lengkap")).toBeVisible();
    await page.reload();
    await expect(page.getByLabel("Kota domisili")).toHaveValue("Karawang");

    for (const [slug, title] of [
      ["operator-produksi-karawang", "Operator Produksi"],
      ["backend-engineer", "Backend Engineer"],
    ]) {
      await page.goto(`/lowongan/${slug}`);
      await page.getByRole("button", { name: "Lamar posisi ini" }).click();
      const dialog = page.getByRole("dialog");
      await expect(dialog).toBeVisible();
      await dialog.getByRole("button", { name: "Kirim lamaran" }).click();
      await expect(page.getByText("Lamaran terkirim")).toBeVisible();
      await expect(page.getByRole("heading", { level: 1 })).toHaveText(title);
    }
    await page.goto("/akun");
    await expect(page.getByRole("listitem").filter({ hasText: "Operator Produksi" })).toBeVisible();
    await expect(page.getByRole("listitem").filter({ hasText: "Backend Engineer" })).toBeVisible();

    // Melamar ulang ditolak dengan tautan ke lamaran yang ada.
    await page.goto("/lowongan/operator-produksi-karawang");
    await page.getByRole("button", { name: "Lamar posisi ini" }).click();
    await page.getByRole("dialog").getByRole("button", { name: "Kirim lamaran" }).click();
    await expect(page.getByText("Anda sudah melamar lowongan ini.")).toBeVisible();
    await page.keyboard.press("Escape");
    await logout(page);
  });

  test("recruiter OPS: melihat lamaran, memindahkan tahap dengan alasan, tidak melihat unit lain", async ({ page }) => {
    await login(page, "rekruter.ops@arunika.example");
    await expect(page).toHaveURL(/\/staf\/lamaran/);
    const row = page.getByRole("row").filter({ hasText: "Rina Amelia" });
    await expect(row).toContainText("Operator Produksi");
    await expect(page.getByRole("row").filter({ hasText: "Backend Engineer" })).toHaveCount(0);

    await row.getByRole("link", { name: "Rina Amelia" }).click();
    await expect(page.getByText("************0777")).toBeVisible();
    await expect(page.getByRole("button", { name: "Tampilkan NIK" })).toHaveCount(0);
    await page.getByRole("button", { name: /Lolos screening: ke Ujian/ }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByLabel("Catatan").fill("Berkas sesuai kualifikasi");
    await dialog.getByRole("button", { name: "Konfirmasi" }).click();
    await expect(page.getByText("Perubahan tersimpan.")).toBeVisible();
    await page.reload();
    await expect(page.getByText("Dipindahkan: Screening ke Ujian")).toBeVisible();
    await expect(page.getByText("Alasan: Berkas sesuai kualifikasi")).toBeVisible();
    await expect(page.getByText(/Maya Sari/).first()).toBeVisible();
    const url = page.url();
    await logout(page);

    // Recruiter TI tidak dapat membuka lamaran unit OPS lewat URL langsung.
    await login(page, "rekruter.ti@arunika.example");
    await page.goto(url.replace("http://localhost:15173", ""));
    await expect(page.getByText("Tidak ditemukan", { exact: true })).toBeVisible();
  });

  test("HR Admin: reveal NIK butuh alasan; kandidat melihat tahap terbaru", async ({ page }) => {
    await login(page, "hr.ops@arunika.example");
    await page.getByRole("link", { name: "Rina Amelia" }).first().click();
    await page.getByRole("button", { name: "Tampilkan NIK" }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByLabel("Alasan").fill("Verifikasi dokumen kontrak");
    await dialog.getByRole("button", { name: "Tampilkan" }).click();
    await expect(page.getByText("3201014507990777")).toBeVisible();
    await logout(page);

    await login(page, email);
    await page.goto("/akun");
    await expect(page.getByRole("listitem").filter({ hasText: "Operator Produksi" })).toContainText("Tahap: Ujian");
    await page.goto("/akun/notifikasi");
    await expect(page.getByText(/Pembaruan lamaran: Operator Produksi/)).toBeVisible();
  });

  test("keyboard: login hanya dengan keyboard dan skip link", async ({ page }) => {
    await page.goto("/masuk");
    await expect(page.getByLabel("Email")).toBeVisible();
    await page.keyboard.press("Tab");
    await expect(page.getByRole("link", { name: "Lewati ke konten" })).toBeFocused();
    await page.getByLabel("Email").focus();
    await page.keyboard.type("hm.ops@arunika.example");
    await page.keyboard.press("Tab");
    await page.keyboard.type(PASSWORD);
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(/\/staf/);
    // HM hanya melihat lamaran dan permintaan tenaga kerja; tidak ada lowongan/workflow/admin.
    await expect(page.getByRole("navigation", { name: "Menu back office" }).getByRole("link")).toHaveText(["Lamaran", "Permintaan tenaga kerja", "Keamanan akun"]);
  });
});

test("kandidat: unggah CV, menunggu pindai, lalu dapat diunduh", async ({ page }) => {
  await login(page, "sari.kandidat@contoh.test");
  await page.goto("/akun/profil");
  await page.getByLabel("Berkas (wajib)", { exact: true }).setInputFiles({ name: "cv-sari.pdf", mimeType: "application/pdf", buffer: Buffer.from("%PDF-1.7\nCV Sari Wulandari\n") });
  await page.getByRole("button", { name: "Unggah" }).click();
  await expect(page.getByText("Dokumen diunggah dan menunggu pemindaian keamanan.")).toBeVisible();
  await expect
    .poll(async () => {
      await page.reload();
      return page.getByRole("listitem").filter({ hasText: "cv-sari.pdf" }).textContent();
    }, { timeout: 20_000 })
    .toContain("Siap");
  const [download] = await Promise.all([
    page.waitForEvent("download"),
    page.getByRole("listitem").filter({ hasText: "cv-sari.pdf" }).getByRole("button", { name: "Unduh" }).click(),
  ]);
  expect(download.suggestedFilename()).toBe("cv-sari.pdf");
  // Berkas dengan isi bukan PDF ditolak walau berekstensi .pdf.
  await page.getByLabel("Berkas (wajib)", { exact: true }).setInputFiles({ name: "palsu.pdf", mimeType: "application/pdf", buffer: Buffer.from("MZ bukan pdf") });
  await page.getByRole("button", { name: "Unggah" }).click();
  await expect(page.getByText(/Format tidak didukung/)).toBeVisible();
});
