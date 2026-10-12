// A 1920×1080 TV browser. Left and Right on a recording's buttons move
// between them. They do not skip through the picture.
import { spawnSync } from "node:child_process";
import path from "node:path";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";

const here = path.dirname(fileURLToPath(import.meta.url));

function harness() {
  return JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as { db: string; config: string };
}

function sql(statement: string) {
  const result = spawnSync("sqlite3", [harness().db, `PRAGMA busy_timeout=5000; ${statement}`], { encoding: "utf8" });
  if (result.status !== 0) throw new Error(result.stderr || result.stdout || statement);
  return result.stdout.trim().split("\n").pop() ?? "";
}

function plant() {
  const started = new Date(Date.now() - 3_600_000).toISOString().replace(/\.\d+Z$/, "Z");
  const file = path.join(harness().config, "work", "recordings", "quiet-hour.ts");
  return Number(
    sql(
      `INSERT INTO recordings (channel_id, guide_number, title, subtitle, category, path, status, started_at, duration_sec)
       VALUES (1, '4.1', 'Quiet Hour', 'The Late Desk', 'Series', '${file}', 'complete', '${started}', 1800);
       SELECT last_insert_rowid();`,
    ),
  );
}

async function tvRemote(page: Page) {
  await page.addInitScript(() => {
    const orig = window.matchMedia.bind(window);
    window.matchMedia = (query: string) =>
      String(query).includes("pointer") && String(query).includes("coarse")
        ? ({ matches: true, media: query, onchange: null, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {}, dispatchEvent: () => false } as MediaQueryList)
        : orig(query);
  });
}

test("a TV remote walks a recording's buttons", async ({ page }) => {
  await tvRemote(page);
  const setup = await page.request.put("/api/v1/settings", { data: { setupComplete: "1" } });
  expect(setup.ok()).toBeTruthy();
  const id = plant();
  await page.setViewportSize({ width: 1920, height: 1080 });
  await page.goto(`/play?recording=${id}`);
  await expect(page.locator("html")).toHaveAttribute("data-layout", "tv");
  await expect(page.getByRole("heading", { name: "Quiet Hour · The Late Desk" })).toBeVisible();

  const back = page.getByRole("button", { name: "Back 15 seconds" });
  const forward = page.getByRole("button", { name: "Forward 30 seconds" });
  await back.focus();
  await expect(back).toBeFocused();
  const at = await page.locator("video").evaluate((video: HTMLVideoElement) => video.currentTime);
  await page.keyboard.press("ArrowRight");
  await expect(forward).toBeFocused();
  const after = await page.locator("video").evaluate((video: HTMLVideoElement) => video.currentTime);
  expect(Math.abs(after - at)).toBeLessThan(2);
});
