import { expect, test } from "@playwright/test";
import { Api, candidateWithApplication, moveToExamAndInvite } from "./api";
import { login } from "./helpers";

// Ujian lewat browser: koneksi putus membedakan jawaban di perangkat vs diterima server, refresh memulihkan draft,
// submit memberi bukti penerimaan, rilis nilai memicu workflow.
test.describe.serial("ujian kandidat", () => {
  const email = `peserta.e2e.${Date.now()}@contoh.test`;
  let applicationId = "";
  let scheduleId = "";
  let hr: Api;

  test("persiapan: lamaran di tahap Ujian dan diundang", async () => {
    const c = await candidateWithApplication(email, "3201014507998801", "Dimas Peserta");
    applicationId = c.applicationId;
    ({ hr, scheduleId } = await moveToExamAndInvite(applicationId));
  });

  test("mengerjakan: simpan, offline, reconnect, refresh, submit", async ({ page, context }) => {
    await login(page, email);
    await page.goto("/akun/ujian");
    await page.getByRole("button", { name: "Mulai ujian" }).click();
    await expect(page).toHaveURL(/\/ujian\//);
    await expect(page.getByRole("heading", { name: /Soal 1 dari 6/ })).toBeVisible();
    const status = page.locator("header [role=status]");

    // Jawab soal pertama yang berupa pilihan. Label tersimpan hanya setelah ACK server.
    const answerCurrent = async () => {
      const essay = page.getByLabel("Jawaban esai");
      if (await essay.isVisible()) await essay.fill("Hentikan lini, pisahkan produk cacat, lapor supervisor.");
      else await page.locator("main input[type=radio], main input[type=checkbox]").first().check();
    };
    await answerCurrent();
    await expect(status).toHaveText(/Tersimpan pukul/);
    await expect(page.getByText("Jawaban tersimpan di server")).toBeVisible();

    // Koneksi putus: jawaban baru hanya di perangkat.
    await page.getByRole("button", { name: "Berikutnya" }).click();
    await context.setOffline(true);
    await answerCurrent();
    await expect(status).toHaveText(/Menunggu koneksi/, { timeout: 10_000 });
    await expect(page.getByText("Jawaban hanya di perangkat, belum diterima server")).toBeVisible();
    await expect(page.getByText("Koneksi terputus")).toBeVisible();

    // Tersambung kembali: batch yang sama dikirim ulang, lalu tercatat di server.
    await context.setOffline(false);
    await expect(status).toHaveText(/Tersimpan pukul/, { timeout: 15_000 });
    await expect(page.getByText("Jawaban tersimpan di server")).toBeVisible();

    // Refresh sebelum debounce selesai: draft IndexedDB memulihkan dan mengirim jawaban.
    await page.getByRole("button", { name: "Berikutnya" }).click();
    await answerCurrent();
    await page.reload();
    await expect(page.getByRole("heading", { name: /Soal 1 dari 6/ })).toBeVisible();
    await expect(status).toHaveText(/Tersimpan pukul|Belum ada perubahan/, { timeout: 15_000 });
    const answered = page.getByRole("navigation", { name: "Navigasi soal" }).first().getByRole("button", { name: /, dijawab/ });
    await expect(answered).toHaveCount(3, { timeout: 15_000 });

    // Navigasi keyboard di navigator, tandai ragu.
    await page.getByRole("button", { name: /^Soal 4,/ }).first().click();
    await page.getByLabel("Tandai ragu-ragu").check();
    await expect(page.getByRole("button", { name: /^Soal 4, belum dijawab, ditandai ragu/ }).first()).toBeVisible();

    // Kirim: dialog konfirmasi menyebut jumlah belum dijawab dan ragu.
    await page.getByRole("button", { name: "Selesai dan kirim" }).click();
    const dlg = page.getByRole("dialog");
    await expect(dlg).toContainText("Dijawab: 3 dari 6");
    await expect(dlg).toContainText("Ditandai ragu: 1 soal");
    await dlg.getByRole("button", { name: "Kirim jawaban" }).click();
    await expect(page.getByRole("heading", { name: "Bukti penerimaan jawaban" })).toBeVisible();
    await expect(page.getByText(/UJ-[A-Z2-7]+/)).toBeVisible();
    await expect(page.getByText("3 dari 6")).toBeVisible();

    // Refresh setelah final tetap menampilkan bukti yang sama; tidak bisa mengubah jawaban.
    const code = await page.getByText(/UJ-[A-Z2-7]+/).textContent();
    await page.reload();
    await expect(page.getByText(code!)).toBeVisible();
  });

  test("monitoring staf, rilis nilai, cabang workflow, kandidat melihat tahap", async ({ page }) => {
    const mon = await hr.call("GET", `/exam-schedules/${scheduleId}/monitor`);
    const me = mon.body.participants.find((p: { candidateName: string }) => p.candidateName === "Dimas Peserta");
    expect(me.participantStatus).toBe("submitted");
    expect(me.answered).toBe(3);

    await login(page, "hr.ops@arunika.example");
    await page.goto(`/staf/ujian/${scheduleId}`);
    await expect(page.getByRole("row").filter({ hasText: "Dimas Peserta" })).toContainText("Sudah kirim");
    await page.getByRole("button", { name: "Rilis nilai" }).click();
    await page.getByRole("dialog").getByRole("button", { name: "Rilis nilai" }).click();
    await expect(page.getByText(/hasil dirilis/)).toBeVisible();

    // Worker memproses event rilis; lamaran pindah dari Ujian sesuai nilai/esai.
    await expect
      .poll(async () => (await hr.call("GET", `/applications/${applicationId}`)).body.stage.key, { timeout: 15_000 })
      .not.toBe("ujian");
    const stage = (await hr.call("GET", `/applications/${applicationId}`)).body;
    const ev = stage.history.find((h: { trigger: string }) => h.trigger === "event");
    expect(ev, "histori memuat transisi dari event hasil ujian").toBeTruthy();
    expect(["interview", "review", "ditolak"]).toContain(stage.stage.key);
  });
});

test("examiner: penilaian esai tanpa identitas kandidat", async ({ page }) => {
  await login(page, "penguji@arunika.example");
  await page.goto("/staf/ujian");
  await page.getByRole("link", { name: "Gelombang November" }).click();
  await expect(page.getByRole("heading", { name: "Gelombang November" })).toBeVisible();
  const grade = page.getByRole("link", { name: "Penilaian esai" }).first();
  if (await grade.count()) {
    await grade.click();
    await expect(page.getByRole("heading", { name: /Penilaian esai P-/ })).toBeVisible();
    await expect(page.getByText("Dimas Peserta")).toHaveCount(0);
  }
});
