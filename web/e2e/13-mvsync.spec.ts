import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";
import { settle } from "./snap";

// Opt-in: E2E_MV=1 npm run e2e. E2E_MV_LAYOUT picks 2up (default) or 1+2;
// E2E_SOURCE plays a broadcast recording on every channel instead of the test
// pattern. Cut the recording to start on its first picture: a file whose sound
// leads leaves a hole in the picture at every loop.
const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/mvsync");
const playMs = Number(process.env.E2E_MV_MS || 120_000);
// 2up moves the sound half way; 1+2 keeps it on the big tile, whose small tiles are the half-rate "tile" picture.
const layout = process.env.E2E_MV_LAYOUT || "2up";
const equal = layout === "2up" || layout === "quad";
// A tile that runs dry makes the room step back, and every tile pauses for it.
const stallLimitMs = 500;
const apartMs = 50;

type Stall = { ms: number; t: number; buf: number };
type Pause = { t: number; drift: string };
type Tile = {
  channel: string;
  firstPictureMs: number;
  stalls: Stall[];
  pauses: Pause[];
  bufMin: number;
  bufLast: number;
  drift: string;
  offset: string;
  paused: boolean;
  width: number;
  dropped: number;
  total: number;
};

function channelIds() {
  const runtime = JSON.parse(readFileSync(path.join(here, ".run/runtime.json"), "utf8")) as { channels: { id: number; name: string }[] };
  const id = (name: string) => {
    const found = runtime.channels.find((item) => item.name === name);
    if (!found) throw new Error(`no ${name}`);
    return found.id;
  };
  return { wdaf: id("WDAF"), wdaf2: id("WDAF2"), kctv: id("KCTV") };
}

async function openChannel(page: Page, id: number) {
  await page.goto(`/watch?channel=${id}`);
  await settle(page);
  const setup = page.getByRole("heading", { name: "Let's set up your TV" });
  const player = page.getByRole("region", { name: "Player" });
  await expect(setup.or(player)).toBeVisible();
  if (await setup.isVisible()) {
    const cont = page.getByRole("button", { name: "Continue" });
    if (await cont.isVisible()) await cont.click();
    await page.getByRole("button", { name: "Watch", exact: true }).click();
    await expect(player).toBeVisible();
  }
  if (!page.url().includes(`channel=${id}`)) {
    await page.goto(`/watch?channel=${id}`);
    await settle(page);
  }
  await expect(player).toBeVisible();
}

/** Per tile: time to its first moving picture, every stall, and every pause the viewer did not ask for. */
function installProbe() {
  const began = performance.now();
  const state = {
    viewerUntil: 0,
    tiles: {} as Record<string, { first: number; stalls: { ms: number; t: number; buf: number }[]; pauses: { t: number; drift: string }[]; bufMin: number; open: { at: number; t: number; buf: number } | null }>,
    reset() {
      for (const tile of Object.values(this.tiles)) {
        tile.stalls = [];
        tile.pauses = [];
        tile.bufMin = Infinity;
      }
    },
  };
  (window as unknown as { __bwMv: typeof state }).__bwMv = state;
  document.addEventListener(
    "click",
    (event) => {
      if (event.target instanceof Element && event.target.closest(".mv-tile, [aria-label='Pause'], [aria-label='Play']")) state.viewerUntil = performance.now() + 500;
    },
    true,
  );
  const ahead = (video: HTMLVideoElement) => (video.buffered.length ? video.buffered.end(video.buffered.length - 1) - video.currentTime : 0);
  window.setInterval(() => {
    for (const video of document.querySelectorAll<HTMLVideoElement>("video.mv-video")) {
      const channel = video.dataset.channel || "?";
      let tile = state.tiles[channel];
      if (!tile) tile = state.tiles[channel] = { first: 0, stalls: [], pauses: [], bufMin: Infinity, open: null };
      if (!tile.first && video.videoWidth > 0 && !video.paused && video.currentTime > 0.2) tile.first = performance.now() - began;
      if (tile.first && !video.paused) tile.bufMin = Math.min(tile.bufMin, ahead(video));
      if (video.dataset.mvHook === "1") continue;
      video.dataset.mvHook = "1";
      video.addEventListener("waiting", () => {
        const now = state.tiles[channel];
        if (!now.first || now.open) return;
        now.open = { at: performance.now(), t: video.currentTime, buf: ahead(video) };
      });
      video.addEventListener("playing", () => {
        const now = state.tiles[channel];
        if (!now.open) return;
        now.stalls.push({ ms: Math.round(performance.now() - now.open.at), t: now.open.t, buf: now.open.buf });
        now.open = null;
      });
      video.addEventListener("pause", () => {
        const now = state.tiles[channel];
        if (!now.first || performance.now() < state.viewerUntil) return;
        now.pauses.push({ t: video.currentTime, drift: video.dataset.syncDrift || "" });
      });
    }
  }, 100);
}

async function readTiles(page: Page): Promise<Tile[]> {
  return page.evaluate(() => {
    const state = (window as unknown as {
      __bwMv: { tiles: Record<string, { first: number; stalls: Stall[]; pauses: Pause[]; bufMin: number; open: { at: number; t: number; buf: number } | null }> };
    }).__bwMv;
    return [...document.querySelectorAll<HTMLVideoElement>("video.mv-video")].map((video) => {
      const channel = video.dataset.channel || "?";
      const tile = state.tiles[channel] ?? { first: 0, stalls: [], pauses: [], bufMin: Infinity, open: null };
      const stalls = tile.stalls.slice();
      if (tile.open) stalls.push({ ms: Math.round(performance.now() - tile.open.at), t: tile.open.t, buf: tile.open.buf });
      const q = video.getVideoPlaybackQuality();
      return {
        channel,
        firstPictureMs: Math.round(tile.first),
        stalls,
        pauses: tile.pauses.slice(),
        bufMin: Number.isFinite(tile.bufMin) ? +tile.bufMin.toFixed(2) : -1,
        bufLast: video.buffered.length ? +(video.buffered.end(video.buffered.length - 1) - video.currentTime).toFixed(2) : 0,
        drift: video.dataset.syncDrift || "",
        offset: video.dataset.syncOffset || "",
        paused: video.paused,
        width: video.videoWidth,
        dropped: q.droppedVideoFrames,
        total: q.totalVideoFrames,
      };
    });
  });
}

test.use({
  launchOptions: {
    args: [
      "--autoplay-policy=no-user-gesture-required",
      "--disable-background-timer-throttling",
      "--disable-renderer-backgrounding",
      "--disable-backgrounding-occluded-windows",
    ],
  },
});

test(`${layout} with a fresh tune plays without a stall or a pause`, async ({ page, context }) => {
  test.skip(process.env.E2E_MV !== "1", "Set E2E_MV=1 to measure two tiles for two minutes.");
  test.setTimeout(playMs + 180_000);
  await context.addInitScript(installProbe);
  const { wdaf, wdaf2, kctv } = channelIds();
  const channels = layout === "2up" ? [wdaf, kctv] : [wdaf, kctv, wdaf2];
  await page.setViewportSize({ width: 1440, height: 900 });
  // The viewer watches one channel first, then adds a second on another frequency.
  await openChannel(page, wdaf);
  await expect.poll(() => page.locator("video.stage-video").evaluate((v: HTMLVideoElement) => v.videoWidth > 0 && !v.paused), { timeout: 45_000 }).toBe(true);
  await page.waitForTimeout(15_000);
  const opened = Date.now();
  await page.goto(`/multiview?ch=${channels.join(",")}&layout=${layout}&focus=${wdaf}`);
  const moving = async () => (await readTiles(page)).filter((tile) => tile.firstPictureMs > 0).length;
  await expect.poll(moving, { timeout: 60_000 }).toBe(channels.length);
  const bothMs = Date.now() - opened;
  const start = await readTiles(page);

  // Half way, move the sound: equal tiles must not restart or pause for it.
  await page.waitForTimeout(playMs / 2);
  if (equal) {
    await page.getByRole("group", { name: /^5\.1 KCTV$/ }).click();
    await expect(page.getByRole("group", { name: "5.1 KCTV, sound on" })).toBeVisible();
  }
  await page.waitForTimeout(playMs / 2);

  const tiles = await readTiles(page);
  let gap: number | null = null;
  for (let i = 0; i < 10 && gap == null; i++) {
    const now = await readTiles(page);
    if (now.every((tile) => tile.offset !== "" && !tile.paused && tile.width > 0)) {
      const offsets = now.map((tile) => Number(tile.offset));
      gap = Math.max(...offsets) - Math.min(...offsets);
    }
    else await page.waitForTimeout(200);
  }
  mkdirSync(evidence, { recursive: true });
  writeFileSync(path.join(evidence, "summary.json"), JSON.stringify({ layout, playMs, bothMs, start, tiles, gap }, null, 2));
  await page.screenshot({ path: path.join(evidence, "end.jpg"), animations: "disabled" });

  expect(tiles).toHaveLength(channels.length);
  for (const tile of tiles) {
    const total = tile.stalls.reduce((sum, item) => sum + item.ms, 0);
    expect(tile.width, `tile ${tile.channel} has no picture`).toBeGreaterThan(0);
    expect(tile.paused, `tile ${tile.channel} was paused at the end`).toBe(false);
    expect(tile.pauses, `tile ${tile.channel} paused for sync ${JSON.stringify(tile.pauses)}`).toEqual([]);
    expect(total, `tile ${tile.channel} stalled ${total} ms ${JSON.stringify(tile.stalls)}`).toBeLessThanOrEqual(stallLimitMs);
  }
  expect(gap, "tiles were not both playing at the end").not.toBeNull();
  expect(gap ?? 1_000, `tiles were ${gap} ms apart`).toBeLessThanOrEqual(apartMs);
});
