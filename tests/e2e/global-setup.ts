import { execFileSync } from "node:child_process";
import { mkdirSync, rmSync } from "node:fs";
import { resolve } from "node:path";
import { e2eEnv } from "../../frontend/playwright.config";

// Reset ats_e2e dari migrasi + seed. seed --reset menolak database yang bukan *_e2e/_test/_dev.
export default function globalSetup() {
  const root = resolve(process.cwd(), "..");
  rmSync(e2eEnv.MAIL_DIR!, { recursive: true, force: true });
  mkdirSync(e2eEnv.MAIL_DIR!, { recursive: true });
  const exe = process.platform === "win32" ? ".exe" : "";
  execFileSync(resolve(root, "backend/bin/seed" + exe), ["--reset"], { env: { ...process.env, ...e2eEnv }, stdio: "inherit" });
}
