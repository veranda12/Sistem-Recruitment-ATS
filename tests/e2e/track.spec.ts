import { expect, test } from "@playwright/test";
import { login, logout } from "./helpers";
import { Api } from "./api";

// Track rekrutmen: filter portal lewat URL, permintaan MT dari HM sampai lowongan, dan batas kuota di form.
const besok = new Date(Date.now() + 2 * 24 * 3600 * 1000).toISOString().slice(0, 10);

test("portal: filter track lewat URL dan label track di kartu", async ({ page }) => {
  await page.goto("/?track=OPERATOR");
  await expect(page.getByRole("combobox", { name: /Track/ })).toHaveValue("OPERATOR");
  await expect(page.getByRole("link", { name: "Operator Produksi" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Backend Engineer" })).toHaveCount(0);
  await expect(page.getByText("Track Operator").first()).toBeVisible();
});

test.describe("ponsel 390 px", () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test("portal: filter track tanpa scroll horizontal", async ({ page }) => {
    await page.goto("/?track=MT_S1_D4");
    await expect(page.getByRole("combobox", { name: /Track/ })).toHaveValue("MT_S1_D4");
    await expect(page.getByRole("link", { name: "Management Trainee Teknologi" })).toBeVisible();
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow).toBeLessThanOrEqual(0);
  });
});

test("staf: permintaan MT dari HM, disetujui HR, lowongan dari permintaan itu, dan kuota melebihi kebutuhan ditolak", async ({ page }) => {
  const judul = `Management Trainee E2E ${Date.now()}`;
  const permintaan = `${judul} · 2 orang · Management Trainee S1/D4`;

  await login(page, "hm.ops@arunika.example");
  await page.goto("/staf/permintaan");
  await page.getByRole("button", { name: "Ajukan permintaan" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Posisi").fill(judul);
  await dialog.getByLabel("Unit").selectOption({ label: "Operasional Pabrik" });
  await dialog.getByLabel("Lokasi").selectOption({ label: "Pabrik Karawang" });
  await dialog.getByLabel("Jumlah kebutuhan").fill("2");
  await dialog.getByLabel("Track").selectOption("MT_S1_D4");
  await dialog.getByLabel("Alasan permintaan").selectOption("penambahan");
  await dialog.getByLabel("Target tanggal masuk").fill(besok);
  await dialog.getByRole("button", { name: "Simpan draft" }).click();
  const baris = page.locator("li", { hasText: judul });
  await expect(baris).toContainText("Track Management Trainee S1/D4");
  await expect(baris).toContainText("Penambahan");
  await baris.getByRole("button", { name: "Ajukan untuk persetujuan" }).click();
  await expect(baris).toContainText("Menunggu persetujuan");
  await logout(page);

  await login(page, "hr.ops@arunika.example");
  await page.goto("/staf/permintaan");
  await page.locator("li", { hasText: judul }).getByRole("button", { name: "Setujui", exact: true }).click();
  await expect(page.locator("li", { hasText: judul })).toContainText("Disetujui");

  await page.goto("/staf/lowongan");
  await page.getByRole("button", { name: "Buat lowongan" }).click();
  const form = page.getByRole("dialog");
  const slug = `mt-e2e-${Date.now()}`;
  await form.getByLabel("Judul").fill(`Lowongan ${judul}`);
  await form.getByLabel("Slug URL").fill(slug);
  await form.getByLabel("Unit").selectOption({ label: "Operasional Pabrik" });
  await form.getByLabel("Lokasi").selectOption({ label: "Pabrik Karawang" });
  await expect(form.locator("option", { hasText: judul })).toHaveCount(1);
  await form.getByLabel("Permintaan tenaga kerja").selectOption({ label: permintaan });
  await expect(form.getByLabel("Track")).toBeDisabled();
  await form.getByLabel("Deskripsi").fill("Program rotasi untuk lulusan S1 atau D4.");
  await form.getByLabel("Kualifikasi").fill("Lulusan S1 atau D4 dari perguruan tinggi terakreditasi.");
  await form.getByRole("button", { name: "Simpan draft" }).click();
  await expect(page.locator("li", { hasText: `Lowongan ${judul}` })).toContainText("Track Management Trainee S1/D4");

  // Lowongan kedua dari permintaan yang sama: 1 (lowongan pertama) + 3 melebihi kebutuhan 2.
  await page.getByRole("button", { name: "Buat lowongan" }).click();
  const kedua = page.getByRole("dialog");
  await kedua.getByLabel("Judul").fill(`Lowongan kedua ${judul}`);
  await kedua.getByLabel("Slug URL").fill(`${slug}-2`);
  await kedua.getByLabel("Unit").selectOption({ label: "Operasional Pabrik" });
  await kedua.getByLabel("Lokasi").selectOption({ label: "Pabrik Karawang" });
  await expect(kedua.locator("option", { hasText: judul })).toHaveCount(1);
  await kedua.getByLabel("Permintaan tenaga kerja").selectOption({ label: permintaan });
  await kedua.getByLabel("Kuota").fill("3");
  await kedua.getByLabel("Deskripsi").fill("Uji batas kuota.");
  await kedua.getByLabel("Kualifikasi").fill("Uji batas kuota.");
  await kedua.getByRole("button", { name: "Simpan draft" }).click();
  await expect(kedua.getByText(/melebihi kebutuhan 2/)).toBeVisible();
});

test("form lowongan kosong lagi setelah lowongan dibuat", async ({ page }) => {
  const slug = `reset-form-${Date.now()}`;
  await login(page, "hr.ops@arunika.example");
  await page.goto("/staf/lowongan");
  await page.getByRole("button", { name: "Buat lowongan" }).click();
  const form = page.getByRole("dialog");
  await form.getByLabel("Judul").fill(`Lowongan reset ${slug}`);
  await form.getByLabel("Slug URL").fill(slug);
  await form.getByLabel("Unit").selectOption({ label: "Operasional Pabrik" });
  await form.getByLabel("Lokasi").selectOption({ label: "Pabrik Karawang" });
  await form.getByLabel("Track").selectOption("OPERATOR");
  await form.getByLabel("Deskripsi").fill("Uji reset form.");
  await form.getByLabel("Kualifikasi").fill("Uji reset form.");
  await form.getByRole("button", { name: "Simpan draft" }).click();
  await expect(page.getByRole("dialog")).toBeHidden();
  await page.getByRole("button", { name: "Buat lowongan" }).click();
  const baru = page.getByRole("dialog");
  await expect(baru.getByLabel("Judul")).toHaveValue("");
  await expect(baru.getByLabel("Slug URL")).toHaveValue("");
  await expect(baru.getByLabel("Unit")).toHaveValue("");
  await expect(baru.getByLabel("Lokasi")).toHaveValue("");
  await expect(baru.getByLabel("Deskripsi")).toHaveValue("");
  await expect(baru.getByLabel("Kuota")).toHaveValue("1");
});

test("ganti unit membuang permintaan tenaga kerja yang tidak lagi cocok", async ({ page }) => {
  const judul = `Permintaan ganti unit ${Date.now()}`;
  const hm = await Api.login("hm.ops@arunika.example");
  const dibuat = await hm.call("POST", "/requisitions", {
    unitCode: "OPS", locationCode: "KRW", title: judul, headcount: 2, employmentType: "contract", track: "STAFF", reasonCategory: "penambahan",
  });
  expect(dibuat.status).toBe(201);
  const reqId = dibuat.body.id as string;
  expect((await hm.call("POST", `/requisitions/${reqId}/submit`, { version: dibuat.body.version })).status).toBe(200);
  const hr = await Api.login("hr.ops@arunika.example");
  expect((await hr.call("POST", `/requisitions/${reqId}/approve`, { version: dibuat.body.version + 1, note: "E2E ganti unit" })).status).toBe(200);

  await login(page, "hr.ops@arunika.example");
  await page.goto("/staf/lowongan");
  await page.getByRole("button", { name: "Buat lowongan" }).click();
  const form = page.getByRole("dialog");
  await form.getByLabel("Unit").selectOption({ label: "Operasional Pabrik" });
  await expect(form.locator(`option[value="${reqId}"]`)).toHaveCount(1);
  await form.getByLabel("Permintaan tenaga kerja").selectOption(reqId);
  await expect(form.getByLabel("Track")).toBeDisabled();
  await form.getByLabel("Unit").selectOption({ label: "Teknologi Informasi" });
  await expect(form.getByLabel("Permintaan tenaga kerja")).toHaveValue("");
  await expect(form.getByLabel("Track")).toBeEnabled();
  await expect(form.getByLabel("Track")).toHaveValue("");
});
