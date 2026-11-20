import { expect, type Page } from "@playwright/test";
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { e2eEnv } from "../../frontend/playwright.config";

export const PASSWORD = e2eEnv.SEED_PASSWORD!;

/** Menunggu email terbaru untuk alamat tertentu di sink file worker, lalu mengambil tautannya. */
export async function linkFromMail(to: string, timeoutMs = 15_000): Promise<string> {
  const dir = e2eEnv.MAIL_DIR!;
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const files = readdirSync(dir).filter((f) => f.endsWith(".eml")).sort().reverse();
    for (const f of files) {
      const body = readFileSync(join(dir, f), "utf8");
      if (body.includes(`To: ${to}`)) {
        const m = body.match(/http:\/\/localhost:15173\/\S+token=[A-Za-z0-9_-]+/);
        if (m) return m[0];
      }
    }
    await new Promise((r) => setTimeout(r, 300));
  }
  throw new Error(`email untuk ${to} tidak ditemukan`);
}

export async function login(page: Page, email: string, password = PASSWORD) {
  await page.goto("/masuk");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Kata sandi").fill(password);
  await page.getByRole("button", { name: "Masuk" }).click();
  await expect(page).not.toHaveURL(/\/masuk/);
}

export async function logout(page: Page) {
  await page.getByRole("button", { name: "Keluar" }).click();
}
