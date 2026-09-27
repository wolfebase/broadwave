import { defineConfig } from "@playwright/test";

const baseURL = `http://127.0.0.1:${process.env.E2E_PORT || 18731}`;
const avsync = process.env.E2E_AVSYNC === "1";
const tab = process.env.E2E_TAB === "1";
// Opt-in runs replace the suite: each one is minutes against its own server.
const only = avsync ? "08-avsync\\.spec\\.ts" : tab ? "10-tab\\.spec\\.ts" : "";

export default defineConfig({
  testDir: ".",
  testMatch: /.*\.spec\.ts/,
  testIgnore: only ? new RegExp(`^(?!.*${only}).*$`) : [/08-avsync\.spec\.ts/, /10-tab\.spec\.ts/],
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: avsync ? 240_000 : tab ? 1_200_000 : 180_000,
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
    timeout: avsync ? 180_000 : 120_000,
  },
});
