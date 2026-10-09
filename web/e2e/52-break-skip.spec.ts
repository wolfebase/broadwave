// Auto skips a sure break, an unsure one keeps Skip break, Manual shows nothing,
// and a second Forward 30 inside a break jumps to its end.
import { spawnSync } from "node:child_process";
import { mkdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";

const here = path.dirname(fileURLToPath(import.meta.url));

function harness() {
  return JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as { base: string; db: string; config: string };
}

function sql(statement: string) {
  const result = spawnSync("sqlite3", [harness().db, `PRAGMA busy_timeout=5000; ${statement}`], { encoding: "utf8" });
  if (result.status !== 0) throw new Error(result.stderr || result.stdout || statement);
  return result.stdout.trim().split("\n").pop() ?? "";
}

// A minute of picture and tone, the same shape 51-episode-ends uses.
function minute(name: string) {
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

// Sure break 10–20 (0.95), unsure 30–45 (0.5). breaks_scanned so the queue does not replace them.
function add(file: string) {
  const started = new Date(Date.now() - 86_400_000).toISOString().replace(/\.\d+Z$/, "Z");
  const id = Number(
    sql(
      `INSERT INTO recordings (channel_id, guide_number, title, subtitle, path, status, started_at, duration_sec, breaks_scanned)
       VALUES (1, '4.1', 'Desk Breaks', 'The Quiet Hour', '${file}', 'complete', '${started}', 60, 1); SELECT last_insert_rowid();`,
    ),
  );
  sql(
    `INSERT INTO markers (recording_id, start_sec, end_sec, confidence) VALUES
       (${id}, 10, 20, 0.95),
       (${id}, 30, 45, 0.5);`,
  );
  return id;
}

const at = (page: Page) => page.locator("video").evaluate((v: HTMLVideoElement) => v.currentTime);

async function ready(page: Page, to: number) {
  await expect.poll(
    () => page.locator("video").evaluate((v: HTMLVideoElement) => (v.seekable.length ? v.seekable.end(v.seekable.length - 1) : 0)),
    { timeout: 45_000 },
  ).toBeGreaterThan(to);
}

async function seek(page: Page, to: number) {
  await ready(page, to + 2);
  await page.locator("video").evaluate((v: HTMLVideoElement, t) => {
    v.currentTime = t;
    void v.play().catch(() => undefined);
  }, to);
}

async function skipMode(page: Page, name: "Skip auto" | "Skip button" | "Manual") {
  const choice = page.getByRole("button", { name, exact: true });
  if (!(await choice.isVisible())) await page.getByRole("button", { name: "Options" }).click();
  await choice.click();
  await expect(choice).toHaveAttribute("aria-pressed", "true");
}

test("auto skips a sure break, and only a sure one", async ({ page }) => {
  test.setTimeout(120_000);
  const id = add(minute("break-skip"));
  await page.goto(`/play?recording=${id}`);
  await expect(page.getByRole("heading", { name: "Desk Breaks · The Quiet Hour" })).toBeVisible();
  // Muted, the test browser plays in real time. With sound it barely moves.
  await page.locator("video").evaluate((v: HTMLVideoElement) => {
    v.muted = true;
  });

  const pause = () => page.locator("video").evaluate((v: HTMLVideoElement) => {
    v.pause();
  });

  // Confidence 0.95: the playhead leaves the break on its own. No Skip break.
  await seek(page, 12);
  await expect.poll(() => at(page), { timeout: 8_000 }).toBeGreaterThanOrEqual(19.5);
  await pause();
  await expect(page.getByRole("button", { name: "Skip break" })).toBeHidden();

  // Confidence 0.5 stays put and offers the button. This fails if auto ignores confidence.
  // Pause keeps the dock up: while it is faded the bar is inert and the picture takes the click.
  await seek(page, 32);
  await page.waitForTimeout(500);
  expect(await at(page)).toBeLessThan(44);
  await pause();
  const skip = page.getByRole("button", { name: "Skip break" });
  await expect(skip).toBeVisible();
  // On a first play the playlist still grows; a jump past its end stops there.
  await ready(page, 46);
  await skip.click();
  await expect.poll(() => at(page)).toBeGreaterThanOrEqual(44.5);

  // Skip button: a sure break waits for the button too.
  await skipMode(page, "Skip button");
  await seek(page, 12);
  await page.waitForTimeout(500);
  expect(await at(page)).toBeLessThan(19);
  await pause();
  await expect(page.getByRole("button", { name: "Skip break" })).toBeVisible();

  // Manual: nothing to press, and the playhead stays in the break.
  await skipMode(page, "Manual");
  await seek(page, 32);
  await page.waitForTimeout(500);
  expect(await at(page)).toBeLessThan(44);
  await pause();
  await expect(page.getByRole("button", { name: "Skip break" })).toBeHidden();

  // A second Forward 30 inside the break lands on its end, not 30 s later.
  await seek(page, 32);
  await pause();
  await page.getByRole("button", { name: "Forward 30 seconds" }).evaluate((btn: HTMLButtonElement) => {
    btn.click();
    btn.click();
  });
  await expect.poll(() => at(page)).toBeGreaterThan(44);
  await expect.poll(() => at(page)).toBeLessThan(46);
});
