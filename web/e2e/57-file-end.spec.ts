// A finished recording plays its last segment and fires ended.
// A 60s file used to sit near 57s with Pause still showing and ended never firing.
import { spawnSync } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
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

function clip(name: string) {
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

function probe(file: string) {
  const result = spawnSync("ffprobe", ["-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", file], { encoding: "utf8" });
  if (result.status !== 0) throw new Error(result.stderr || "ffprobe");
  return Number(result.stdout.trim());
}

function playlistStats(body: string) {
  let sum = 0;
  let segments = 0;
  let last = 0;
  for (const line of body.split("\n")) {
    const match = line.match(/^#EXTINF:([0-9.]+)/);
    if (!match) continue;
    last = Number(match[1]);
    sum += last;
    segments += 1;
  }
  return {
    endList: body.includes("#EXT-X-ENDLIST"),
    type: body.match(/#EXT-X-PLAYLIST-TYPE:(\S+)/)?.[1] ?? "",
    target: Number(body.match(/#EXT-X-TARGETDURATION:(\d+)/)?.[1] ?? 0),
    segments,
    sum,
    last,
  };
}

async function videoState(page: Page) {
  return page.locator("video").evaluate((video: HTMLVideoElement) => ({
    currentTime: video.currentTime,
    duration: video.duration,
    paused: video.paused,
    ended: video.ended,
    readyState: video.readyState,
    bufferedEnd: video.buffered.length ? video.buffered.end(video.buffered.length - 1) : 0,
    error: video.error ? `${video.error.code} ${video.error.message}` : "",
  }));
}

test("a finished recording plays to its end", async ({ page }) => {
  test.setTimeout(90_000);
  const file = clip("file-end");
  const sourceDuration = probe(file);
  const started = new Date(Date.now() - 86_400_000).toISOString().replace(/\.\d+Z$/, "Z");
  const id = Number(
    sql(
      `INSERT INTO recordings (channel_id, guide_number, title, path, status, started_at, duration_sec)
       VALUES (1, '4.1', 'File End', '${file}', 'complete', '${started}', 60); SELECT last_insert_rowid();`,
    ),
  );

  // Open while the transcoder is still writing segments, the way a viewer does.
  await page.goto(`/play?recording=${id}`);
  await expect(page.getByRole("heading", { name: "File End" })).toBeVisible();
  await page.locator("video").evaluate((video: HTMLVideoElement) => {
    video.muted = true;
  });
  // The player puts a rate back to 1x, so the spec goes to the last ten
  // seconds once the whole file is listed, and plays them through.
  await expect.poll(async () => (await videoState(page)).duration, { timeout: 45_000 }).toBeGreaterThan(sourceDuration - 1);
  await page.locator("video").evaluate((video: HTMLVideoElement, to) => {
    video.currentTime = to;
    void video.play().catch(() => undefined);
  }, sourceDuration - 10);
  await expect.poll(async () => (await videoState(page)).ended, { timeout: 30_000 }).toBe(true);
  const last = await videoState(page);
  const body = await (await fetch(`${harness().base}/media/file/${id}/index.m3u8`)).text();
  const stats = playlistStats(body);
  const evidence = path.resolve(here, "../../.evidence/lane/l105");
  mkdirSync(evidence, { recursive: true });
  writeFileSync(path.join(evidence, "measure.json"), JSON.stringify({ sourceDuration, playlist: stats, last }, null, 2));

  expect(stats.endList).toBe(true);
  expect(stats.sum).toBeGreaterThan(sourceDuration - 1);
  expect(last.currentTime).toBeGreaterThan(sourceDuration - 1);
  expect(last.bufferedEnd).toBeGreaterThan(sourceDuration - 1);
  expect(last.error).toBe("");
  await expect(page.getByRole("button", { name: "Pause" })).toHaveCount(0);
});
