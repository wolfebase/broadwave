import { defineConfig } from "@playwright/test";

const port = Number(process.env.E2E_PORT || 18731);
const baseURL = `http://127.0.0.1:${port}`;
const avsync = process.env.E2E_AVSYNC === "1";
const tab = process.env.E2E_TAB === "1";
const brk = process.env.E2E_BREAK === "1";
const mv = process.env.E2E_MV === "1";
const playlist = process.env.E2E_PLAYLIST === "1";
const rec = process.env.E2E_REC === "1";
const readme = process.env.E2E_README === "1";
// Opt-in runs replace the suite: each one is minutes against its own server.
const only = avsync
  ? "08-avsync\\.spec\\.ts"
  : tab
    ? "10-tab\\.spec\\.ts"
    : brk
      ? "11-break\\.spec\\.ts"
      : mv
        ? "13-mvsync\\.spec\\.ts"
        : playlist
          ? "15-playlist\\.spec\\.ts"
          : rec
            ? "19-record\\.spec\\.ts"
            : readme
              ? "25-readme\\.spec\\.ts"
              : "";

export default defineConfig({
  testDir: ".",
  testMatch: /.*\.spec\.ts/,
  testIgnore: only
    ? new RegExp(`^(?!.*${only}).*$`)
    : [/08-avsync\.spec\.ts/, /10-tab\.spec\.ts/, /11-break\.spec\.ts/, /13-mvsync\.spec\.ts/, /15-playlist\.spec\.ts/, /19-record\.spec\.ts/, /25-readme\.spec\.ts/],
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: avsync ? 240_000 : tab ? 1_200_000 : brk ? 420_000 : mv ? 360_000 : playlist ? 240_000 : rec ? 420_000 : readme ? 300_000 : 180_000,
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
    url: playlist ? `http://127.0.0.1:${port + 9}/ready` : `${baseURL}/api/v1/health`,
    reuseExistingServer: false,
    timeout: avsync || brk || playlist ? 180_000 : 120_000,
  },
});
