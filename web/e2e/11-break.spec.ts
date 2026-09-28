import { mkdirSync, readFileSync, statSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/lane/l17");
// One screen first. A shared room does not step back, so two tabs never show
// the pause a timestamp break used to cause. Then both tabs, long enough for
// several passes of the ~20s raw loop.
const soloMs = Number(process.env.E2E_BREAK_SOLO_MS || 50_000);
const playMs = Number(process.env.E2E_BREAK_MS || 130_000);
const stallLimitMs = 100;
const apartMs = 50;
const minBreaks = 3;

type Break = { cost: number; hole: number; t: number; at: number; open?: boolean };
type Pause = { t: number; at: number; drift: string };
type Probe = { breaks: Break[]; stalls: Break[]; pauses: Pause[]; ticks: number; waits: number };
type Snap = {
  width: number;
  paused: boolean;
  time: number;
  alert: string;
  stallMs: number;
  drift: string;
  offset: string;
};

function channelId() {
  const runtime = JSON.parse(readFileSync(path.join(here, ".run/runtime.json"), "utf8")) as { channels: { id: number; name: string }[] };
  const kbwv = runtime.channels.find((item) => item.name === "KBWV");
  if (!kbwv) throw new Error("no KBWV");
  return kbwv.id;
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

async function snap(page: Page): Promise<Snap> {
  return page.locator("video.stage-video").evaluate((video: HTMLVideoElement) => ({
    width: video.videoWidth,
    paused: video.paused,
    time: video.currentTime,
    alert: document.querySelector("[role='alert']")?.textContent || "",
    stallMs: Number(video.dataset.stallMs || 0),
    drift: video.dataset.syncDrift || "",
    offset: video.dataset.syncOffset || "",
  }));
}

/** A pause the viewer asked for (the control, the picture, or the space bar) is not a sync pause. */
function installProbe() {
  const maxHoleS = 0.5;
  const holeEnd = (t: number, video: HTMLVideoElement) => {
    const ranges: [number, number][] = [];
    for (let i = 0; i < video.buffered.length; i++) ranges.push([video.buffered.start(i), video.buffered.end(i)]);
    for (let i = 0; i + 1 < ranges.length; i++) {
      const [start, end] = ranges[i];
      const next = ranges[i + 1][0];
      if (t < start - 0.05 || t > next) continue;
      if (end - t > maxHoleS || next - end > maxHoleS || next <= t) continue;
      return next;
    }
    return null;
  };
  const state = {
    started: false,
    viewerUntil: 0,
    gen: 0,
    ticks: 0,
    waits: 0,
    breaks: [] as { cost: number; hole: number; t: number; at: number; open?: boolean }[],
    stalls: [] as { cost: number; hole: number; t: number; at: number; open?: boolean }[],
    pauses: [] as { t: number; at: number; drift: string }[],
    open: null as { stallMs: number; hole: number | null; t: number; at: number } | null,
    reset() {
      this.gen++;
      this.ticks = 0;
      this.waits = 0;
      this.breaks = [];
      this.stalls = [];
      this.pauses = [];
      this.open = null;
    },
  };
  (window as unknown as { __bwBreak: typeof state }).__bwBreak = state;

  const markViewer = (event: Event) => {
    const target = event.target;
    if (!(target instanceof Element)) return;
    if (target.closest("video") || target.closest("[aria-label='Pause']") || target.closest("[aria-label='Play']")) {
      state.viewerUntil = performance.now() + 500;
    }
  };
  document.addEventListener("click", markViewer, true);
  document.addEventListener(
    "keydown",
    (event) => {
      if (event.key === " " || event.code === "Space") state.viewerUntil = performance.now() + 500;
    },
    true,
  );

  // Listen on the element. Media events do not travel through document, and the
  // hook has to be in place before the first timestamp break, about 20s in.
  window.setInterval(() => {
    const video = document.querySelector("video.stage-video");
    if (!(video instanceof HTMLVideoElement) || video.dataset.breakHook) return;
    video.dataset.breakHook = "1";
    video.addEventListener("timeupdate", () => {
      state.ticks++;
      if (!state.started && video.currentTime > 0.2 && video.videoWidth > 0 && !video.paused) state.started = true;
    });
    video.addEventListener("waiting", () => {
      state.waits++;
      if (!state.started) return;
      const hole = holeEnd(video.currentTime, video);
      if (state.open) {
        if (state.open.hole == null && hole != null) state.open.hole = hole;
        return;
      }
      state.open = { stallMs: Number(video.dataset.stallMs || 0), hole, t: video.currentTime, at: performance.now() };
    });
    video.addEventListener("playing", () => {
      if (!state.open) return;
      const open = state.open;
      state.open = null;
      const before = open.stallMs;
      const seen = state.gen;
      window.setTimeout(() => {
        if (seen !== state.gen) return;
        const cost = Math.max(0, Number(video.dataset.stallMs || 0) - before);
        const item = { cost, hole: open.hole ?? -1, t: open.t, at: open.at };
        state.stalls.push(item);
        if (open.hole != null) state.breaks.push(item);
      }, 0);
    });
    video.addEventListener("pause", () => {
      if (!state.started) return;
      if (performance.now() < state.viewerUntil) return;
      state.pauses.push({ t: video.currentTime, at: performance.now(), drift: video.dataset.syncDrift || "" });
    });
  }, 50);
}

async function readProbe(page: Page): Promise<Probe> {
  return page.evaluate(() => {
    const state = (window as unknown as {
      __bwBreak: {
        breaks: Break[];
        stalls: Break[];
        pauses: Pause[];
        ticks: number;
        waits: number;
        open: { hole: number | null; t: number; at: number } | null;
      };
    }).__bwBreak;
    const breaks = state.breaks.slice();
    const stalls = state.stalls.slice();
    if (state.open) {
      const item = { cost: performance.now() - state.open.at, hole: state.open.hole ?? -1, t: state.open.t, at: state.open.at, open: true };
      stalls.push(item);
      if (state.open.hole != null) breaks.push(item);
    }
    return { breaks, stalls, pauses: state.pauses.slice(), ticks: state.ticks, waits: state.waits };
  });
}

async function resetProbe(page: Page) {
  await page.evaluate(() => (window as unknown as { __bwBreak: { reset: () => void } }).__bwBreak.reset());
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

test("a timestamp break does not stall, pause, or split the tabs", async ({ page, context }) => {
  test.skip(process.env.E2E_BREAK !== "1", "Set E2E_BREAK=1 to play a raw loop with timestamp breaks.");
  test.setTimeout(soloMs + playMs + 180_000);
  await context.addInitScript(installProbe);
  const id = channelId();
  await page.setViewportSize({ width: 1440, height: 900 });
  await openChannel(page, id);

  const locked = (s: Snap) => s.width > 0 && !s.paused && s.offset !== "" && s.drift !== "" && Math.abs(Number(s.drift)) < 400;
  await expect.poll(() => snap(page).then(locked), { timeout: 45_000 }).toBe(true);
  await resetProbe(page);
  const logPath = path.join(here, ".run/server.log");
  const logFrom = statSync(logPath).size;
  await page.waitForTimeout(soloMs);
  const soloProbe = await readProbe(page);
  const soloEnd = await snap(page);
  const soloBreaks = (readFileSync(logPath).subarray(logFrom).toString("utf8").match(/started again after a timestamp break/g) || []).length;
  const soloWorst = soloProbe.breaks.reduce((max, item) => Math.max(max, item.cost), 0);
  mkdirSync(evidence, { recursive: true });
  writeFileSync(path.join(evidence, "summary.json"), JSON.stringify({ soloMs, soloBreaks, solo: soloProbe, soloEnd }, null, 2));
  expect(soloBreaks, "the raw loop did not break while one screen was watching").toBeGreaterThanOrEqual(1);
  expect(soloEnd.alert, "solo").toBe("");
  expect(soloEnd.paused, "the one screen was paused at the end of its breaks").toBe(false);
  expect(soloProbe.ticks, "the one screen probe saw no pictures").toBeGreaterThan(10);
  expect(soloWorst, `one screen break stalled ${soloWorst} ms`).toBeLessThanOrEqual(stallLimitMs);
  expect(soloEnd.stallMs, `one screen stalled ${soloEnd.stallMs} ms`).toBeLessThanOrEqual(stallLimitMs);
  expect(soloProbe.pauses, `one screen paused for sync ${JSON.stringify(soloProbe.pauses)}`).toEqual([]);

  const other = await context.newPage();
  await other.setViewportSize({ width: 1440, height: 900 });
  await openChannel(other, id);
  await expect.poll(() => snap(page).then(locked), { timeout: 45_000 }).toBe(true);
  await expect.poll(() => snap(other).then(locked), { timeout: 45_000 }).toBe(true);
  await resetProbe(page);
  await resetProbe(other);
  await page.waitForTimeout(playMs);

  await page.waitForTimeout(50);
  const leftProbe = await readProbe(page);
  const rightProbe = await readProbe(other);
  let gap: { gap: number; left: Snap; right: Snap } | null = null;
  const gapStarted = Date.now();
  while (Date.now() - gapStarted < 2000) {
    const left = await snap(page);
    const right = await snap(other);
    if (left.offset !== "" && right.offset !== "" && !left.paused && !right.paused && left.width > 0 && right.width > 0) {
      gap = { gap: Math.abs(Number(left.offset) - Number(right.offset)), left, right };
    }
    await page.waitForTimeout(200);
  }
  const logText = readFileSync(logPath).subarray(logFrom).toString("utf8");
  const serverBreaks = (logText.match(/started again after a timestamp break/g) || []).length;
  const leftEnd = await snap(page);
  const rightEnd = await snap(other);

  mkdirSync(evidence, { recursive: true });
  const report = {
    playMs,
    soloMs,
    stallLimitMs,
    apartMs,
    soloBreaks,
    serverBreaks,
    solo: soloProbe,
    soloEnd,
    left: leftProbe,
    right: rightProbe,
    gap,
    leftEnd,
    rightEnd,
  };
  writeFileSync(path.join(evidence, "summary.json"), JSON.stringify(report, null, 2));
  await page.screenshot({ path: path.join(evidence, "end.jpg"), animations: "disabled" });

  const worst = (probe: Probe) => probe.breaks.reduce((max, item) => Math.max(max, item.cost), 0);
  expect(serverBreaks, "the raw loop did not break the timestamp").toBeGreaterThanOrEqual(minBreaks);
  for (const [name, probe, end] of [
    ["left", leftProbe, leftEnd],
    ["right", rightProbe, rightEnd],
  ] as const) {
    expect(end.alert, name).toBe("");
    expect(end.width, name).toBeGreaterThan(0);
    expect(end.paused, `${name} was paused at the end`).toBe(false);
    expect(probe.ticks, `${name} probe saw no pictures`).toBeGreaterThan(10);
    // No waiting at all means the playhead never stopped on a break. A stall
    // that did land on a hole has to be under the limit, and so does the total:
    // the sum of every stall is at least the worst break.
    expect(worst(probe), `${name} break stalled ${worst(probe)} ms (${JSON.stringify(probe.breaks)})`).toBeLessThanOrEqual(stallLimitMs);
    expect(end.stallMs, `${name} stalled ${end.stallMs} ms across the breaks`).toBeLessThanOrEqual(stallLimitMs);
    expect(probe.pauses, `${name} paused for sync ${JSON.stringify(probe.pauses)}`).toEqual([]);
  }
  expect(gap, "tabs were not both playing at the end").not.toBeNull();
  expect(gap?.gap ?? 1_000, `tabs were ${gap?.gap} ms apart`).toBeLessThanOrEqual(apartMs);
});
