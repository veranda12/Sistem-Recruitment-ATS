import { mkdirSync } from "node:fs";
import { e2eEnv } from "../../frontend/playwright.config";

// Reset database dilakukan oleh perintah webServer API (seed --reset), karena webServer start sebelum globalSetup.
export default function globalSetup() {
  mkdirSync(e2eEnv.MAIL_DIR!, { recursive: true });
}
