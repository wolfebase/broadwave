import { defineConfig } from "@playwright/test";

const baseURL = `http://127.0.0.1:${process.env.E2E_PORT || 18731}`;
const avsync = process.env.E2E_AVSYNC === "1";
const tab = process.env.E2E_TAB === "1";
const brk = process.env.E2E_BREAK === "1";
const mv = process.env.E2E_MV === "1";
// Opt-in runs replace the suite: each one is minutes against its own server.
const only = avsync ? "08-avsync\\.spec\\.ts" : tab ? "10-tab\\.spec\\.ts" : brk ? "11-break\\.spec\\.ts" : mv ? "13-mvsync\\.spec\\.ts" : "";

export default defineConfig({
  testDir: ".",
  testMatch: /.*\.spec\.ts/,
  testIgnore: only ? new RegExp(`^(?!.*${only}).*$`) : [/08-avsync\.spec\.ts/, /10-tab\.spec\.ts/, /11-break\.spec\.ts/, /13-mvsync\.spec\.ts/],
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: avsync ? 240_000 : tab ? 1_200_000 : brk ? 420_000 : mv ? 360_000 : 180_000,
  expect: { timeout: 20_000 },
  outputDir: ".run/test-results",
  globalSetup: "./global-setup.ts",
  use: {
    baseURL,
    channel: "chrome",
    headless: true,
    viewport: { width: 1440, height: 900 },
    deviceScaleFactor: 1,
    reducedMotion: "reduce",
    colorScheme: "dark",
    locale: "en-US",
    timezoneId: "UTC",
    screenshot: "only-on-failure",
    video: "off",
    trace: "off",
  },
  webServer: {
    command: "node serve.mjs",
    url: `${baseURL}/api/v1/health`,
    reuseExistingServer: false,
    timeout: avsync || brk ? 180_000 : 120_000,
  },
});
