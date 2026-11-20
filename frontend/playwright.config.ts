import { defineConfig, devices } from "@playwright/test";
import { resolve } from "node:path";

// Lingkungan E2E terisolasi: database ats_e2e, API :18080, worker health :18081, Vite :15173.
// Kunci di bawah khusus E2E dan tidak dipakai di lingkungan lain.
// Dijalankan dari direktori frontend (npm run e2e).
const root = resolve(process.cwd(), "..");
const exe = process.platform === "win32" ? ".exe" : "";
export const e2eEnv: Record<string, string> = {
  APP_ENV: "test",
  HTTP_ADDR: "127.0.0.1:18080",
  PUBLIC_ORIGIN: "http://localhost:15173",
  DATABASE_URL: process.env.E2E_DATABASE_URL ?? "postgres://pos:pos@localhost:5432/ats_e2e?sslmode=disable",
  ORG_SLUG: "arunika",
  SESSION_KEY: "ZTJlLXNlc3Npb24ta2V5LTAwMDAwMDAwMDAwMDAwMDA=",
  NIK_ENC_KEY: "ZTJlLW5pay1lbmMta2V5LTAwMDAwMDAwMDAwMDAwMDA=",
  NIK_INDEX_KEY: "ZTJlLW5pay1pZHgta2V5LTAwMDAwMDAwMDAwMDAwMDA=",
  MAIL_DIR: resolve(root, ".data/e2e-mail"),
  WORKER_HEALTH_ADDR: "127.0.0.1:18081",
  WORKER_POLL_INTERVAL: "300ms",
  SEED_PASSWORD: "Uji-Sandi-2026",
  LOG_LEVEL: "warn",
};

export default defineConfig({
  testDir: "../tests/e2e",
  globalSetup: "../tests/e2e/global-setup.ts",
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [["list"], ["html", { open: "never", outputFolder: "playwright-report" }]],
  use: {
    baseURL: "http://localhost:15173",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    locale: "id-ID",
    timezoneId: "Asia/Jakarta",
  },
  projects: [
    { name: "desktop", use: { ...devices["Desktop Chrome"], viewport: { width: 1440, height: 900 } } },
    { name: "mobile", use: { ...devices["Pixel 7"], viewport: { width: 390, height: 844 } }, testMatch: /mobile\.spec\.ts/ },
  ],
  webServer: [
    { command: `"${resolve(root, "backend/bin/api" + exe)}"`, url: "http://127.0.0.1:18080/readyz", env: e2eEnv, reuseExistingServer: false, timeout: 60_000 },
    { command: `"${resolve(root, "backend/bin/worker" + exe)}"`, url: "http://127.0.0.1:18081/healthz", env: e2eEnv, reuseExistingServer: false, timeout: 60_000 },
    {
      command: "npx vite --port 15173",
      url: "http://localhost:15173",
      env: { VITE_PORT: "15173", VITE_API_TARGET: "http://127.0.0.1:18080" },
      reuseExistingServer: false,
      timeout: 60_000,
    },
  ],
});
