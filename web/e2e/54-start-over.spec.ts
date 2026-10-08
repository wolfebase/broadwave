// Start over on live TV plays the show on now from its start while the live window still holds it.
import { spawnSync } from "node:child_process";
import { mkdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "./fixture";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/e7");

function harness() {
  return JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as { base: string; db: string };
}

function sql(statement: string) {
  const result = spawnSync("sqlite3", [harness().db, `PRAGMA busy_timeout=5000; ${statement}`], { encoding: "utf8" });
  if (result.status !== 0) throw new Error(result.stderr || result.stdout || statement);
  return result.stdout.trim();
}

const iso = (ms: number) => new Date(ms).toISOString().replace(/\.\d+Z$/, "Z");

test("start over jumps to the show's start in the live window", async ({ page }) => {
  test.setTimeout(120_000);
  const now = Date.now();
  // A show that starts 20 s after the channel opens, so the window holds its start.
  const showStart = now + 20_000;
  sql(`DROP TABLE IF EXISTS e2e_kept_airings; CREATE TABLE e2e_kept_airings AS SELECT * FROM airings WHERE channel_id = 1;
       DELETE FROM airings WHERE channel_id = 1;
       INSERT INTO airings (channel_id, title, starts_at, ends_at) VALUES (1, 'Morning Desk', '${iso(now - 3_600_000)}', '${iso(showStart)}');
       INSERT INTO airings (channel_id, title, starts_at, ends_at) VALUES (1, 'Night Owls Live', '${iso(showStart)}', '${iso(showStart + 3_600_000)}');`);
  try {
    await page.goto("/watch?channel=1");
    const video = page.locator("video").first();
    await video.evaluate((v: HTMLVideoElement) => {
      v.muted = true;
    });
    await expect.poll(() => video.evaluate((v: HTMLVideoElement) => v.currentTime > 1 && !v.paused), { timeout: 45_000 }).toBe(true);
    // The page's clock moves every 15 s; the new show is on by then.
    await expect(page.getByRole("heading", { name: "Night Owls Live" })).toBeVisible({ timeout: 45_000 });
    const button = page.getByRole("button", { name: "Start over" });
    await expect(button).toBeVisible();
    // Some way into the show, so the jump is plain to see.
    await expect.poll(() => Date.now() - showStart, { timeout: 30_000 }).toBeGreaterThan(12_000);
    // The controls fade while the show plays; a mouse move brings them back.
    await page.mouse.move(300, 300);
    await page.mouse.move(320, 320);
    // data-sync-offset is the playhead's broadcast time minus the wall clock.
    const offset = Number(await video.getAttribute("data-sync-offset"));
    const before = await video.evaluate((v: HTMLVideoElement) => v.currentTime);
    const into = (Date.now() + offset - showStart) / 1000;
    await button.click();
    const after = await video.evaluate((v: HTMLVideoElement) => v.currentTime);
    // The playhead lands on the show's start, within a few seconds.
    expect(Math.abs(before - after - into)).toBeLessThan(3);
    await expect.poll(() => video.evaluate((v: HTMLVideoElement) => !v.paused && v.readyState >= 3), { timeout: 15_000 }).toBe(true);
    // Already at the start: nothing left to start over.
    await expect(button).toBeHidden();
    mkdirSync(evidence, { recursive: true });
    await page.screenshot({ path: path.join(evidence, "start-over.jpg"), type: "jpeg", quality: 70 });
  } finally {
    sql(`DELETE FROM airings WHERE channel_id = 1; INSERT INTO airings SELECT * FROM e2e_kept_airings; DROP TABLE e2e_kept_airings;`);
  }
});
