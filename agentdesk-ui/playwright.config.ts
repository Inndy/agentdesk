import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  workers: 1,
  timeout: 60000,
  reporter: "list",
  outputDir: "../artifacts/playwright",
  // Secrets are used only in memory. Do not record traces or login URLs.
  use: { trace: "off", screenshot: "off", video: "off" },
});
