// Skip intro jumps past a found intro; Up next counts down from the end titles into the next episode.
import { spawnSync } from "node:child_process";
import { mkdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/e5");

function harness() {
  return JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as { base: string; db: string; config: string };
}

function sql(statement: string) {
  const result = spawnSync("sqlite3", [harness().db, `PRAGMA busy_timeout=5000; ${statement}`], { encoding: "utf8" });
  if (result.status !== 0) throw new Error(result.stderr || result.stdout || statement);
  return result.stdout.trim().split("\n").pop() ?? "";
}

// A minute of picture and tone, as a recording.
function episode(name: string) {
  const file = path.join(harness().config, "work", "recordings", `${name}.ts`);
  mkdirSync(path.dirname(file), { recursive: true });
  const made = spawnSync(
    "ffmpeg",
    ["-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=640x360:rate=30", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
      "-t", "60", "-c:v", "libx264", "-preset", "ultrafast", "-g", "30", "-pix_fmt", "yuv420p", "-c:a", "ac3", "-ac", "2", "-f", "mpegts", file],
    { encoding: "utf8" },
  );
  if (made.status !== 0) throw new Error(made.stderr);
  return file;
}

function add(file: string, subtitle: string, ep: number, ends: string) {
  const started = new Date(Date.now() - (10 - ep) * 86_400_000).toISOString().replace(/\.\d+Z$/, "Z");
  return Number(
    sql(
      `INSERT INTO recordings (channel_id, guide_number, title, subtitle, path, status, started_at, duration_sec, season, episode, intro_start, intro_end, credits_start)
       VALUES (1, '4.1', 'Night Owls', '${subtitle}', '${file}', 'complete', '${started}', 60, 1, ${ep}, ${ends}); SELECT last_insert_rowid();`,
    ),
  );
}

async function seek(page: Page, to: number) {
  await expect.poll(() => page.locator("video").evaluate((v: HTMLVideoElement) => (v.seekable.length ? v.seekable.end(v.seekable.length - 1) : 0)), { timeout: 30_000 }).toBeGreaterThan(to + 2);
  await page.locator("video").evaluate((v: HTMLVideoElement, t) => {
    v.currentTime = t;
    void v.play().catch(() => undefined);
  }, to);
}

// The test browser's clock barely moves with its sound on (0.1 s in 8 s on a
// Mac); muted, it plays in real time.
const mute = (page: Page) => page.locator("video").evaluate((v: HTMLVideoElement) => {
  v.muted = true;
});

const at = (page: Page) => page.locator("video").evaluate((v: HTMLVideoElement) => v.currentTime);

test("skip intro, then up next plays the next episode", async ({ page }) => {
  test.setTimeout(150_000);
  const first = add(episode("night-owls-1"), "The First Night", 1, "1, 6, 20");
  // End titles 3 s in: the count runs while it plays from the start, with no seek.
  const second = add(episode("night-owls-2"), "The Second Night", 2, "0, 0, 3");
  const third = add(episode("night-owls-3"), "The Third Night", 3, "0, 0, 0");
  mkdirSync(evidence, { recursive: true });

  await page.goto(`/play?recording=${first}`);
  await expect(page.getByRole("heading", { name: "Night Owls · The First Night" })).toBeVisible();
  await mute(page);
  const skip = page.getByRole("button", { name: "Skip intro" });
  await expect(skip).toBeVisible({ timeout: 30_000 });
  await page.screenshot({ path: path.join(evidence, "skip-intro.jpg"), type: "jpeg", quality: 70 });
  await expect.poll(() => page.locator("video").evaluate((v: HTMLVideoElement) => (v.seekable.length ? v.seekable.end(v.seekable.length - 1) : 0)), { timeout: 30_000 }).toBeGreaterThan(8);
  await skip.click();
  await expect.poll(() => at(page)).toBeGreaterThanOrEqual(5.9);
  await expect(skip).toBeHidden();

  // Not now holds for the rest of this one.
  await seek(page, 21);
  await expect(page.getByText("Up next: S1 E2 · The Second Night. Plays on its own.")).toBeVisible();
  await page.getByRole("button", { name: "Not now" }).click();
  await expect(page.getByText(/Up next/)).toBeHidden();
  await seek(page, 25);
  await page.waitForTimeout(2000);
  await expect(page.getByText(/Up next/)).toBeHidden();
  await expect(page).toHaveURL(new RegExp(`recording=${first}$`));

  await page.goto(`/play?recording=${second}`);
  await expect(page.getByRole("heading", { name: "Night Owls · The Second Night" })).toBeVisible();
  await mute(page);
  await expect(page.getByText("Up next: S1 E3 · The Third Night. Plays on its own.")).toBeVisible({ timeout: 30_000 });
  await expect(page.getByText(/Playing in \d+ s/)).toBeVisible();
  await page.screenshot({ path: path.join(evidence, "up-next.jpg"), type: "jpeg", quality: 70 });
  await expect(page).toHaveURL(new RegExp(`recording=${third}$`), { timeout: 30_000 });
  await expect(page.getByRole("heading", { name: "Night Owls · The Third Night" })).toBeVisible();

  // The last episode offers nothing after it, and has no intro to skip.
  await seek(page, 52);
  await page.waitForTimeout(1000);
  await expect(page.getByText(/Up next/)).toBeHidden();
  await expect(page.getByRole("button", { name: "Skip intro" })).toBeHidden();
});
