import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";
import { settle } from "./snap";

const here = path.dirname(fileURLToPath(import.meta.url));
const evidence = path.resolve(here, "../../.evidence/lane");

// Ninety seconds, one flash and one beep every five seconds. Half a period is
// the widest offset that still pairs a flash with its own beep.
const seconds = 90;
const halfPeriodMs = 2_500;
const limitMs = 45;

type Reading = {
  tracks: number;
  ac: string;
  flashes: number;
  beeps: number;
  pairs: number;
  median: number | null;
  min: number | null;
  max: number | null;
  pauses: number[];
  paused: boolean;
  drift: [number, string, boolean][];
  offsets: number[];
};

type Channel = { id: number; name: string };

function channelId() {
  const runtime = JSON.parse(readFileSync(path.join(here, ".run/runtime.json"), "utf8")) as { channels: Channel[] };
  const wdaf = runtime.channels.find((item) => item.name === "WDAF");
  if (!wdaf) throw new Error("no WDAF");
  return wdaf.id;
}

async function openPlayer(page: Page) {
  await page.goto(`/watch?channel=${channelId()}`);
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
  await expect
    .poll(
      () =>
        page.locator("video.stage-video").evaluate((video: HTMLVideoElement) => video.videoWidth > 0 && !video.paused && video.currentTime > 0),
      { timeout: 60_000 },
    )
    .toBe(true);
}

test.use({
  viewport: { width: 1280, height: 800 },
  launchOptions: { args: ["--autoplay-policy=no-user-gesture-required"] },
});

test("sound stays with the picture", async ({ page }) => {
  test.skip(process.env.E2E_AVSYNC !== "1", "Set E2E_AVSYNC=1 to measure flash and beep.");
  test.setTimeout(240_000);
  const errors: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") errors.push(message.text().slice(0, 180));
  });
  await openPlayer(page);
  await page.locator("video.stage-video").click({ force: true });
  await page.evaluate(arm);
  await page.waitForTimeout(seconds * 1000);
  const reading = await page.evaluate(summarize, halfPeriodMs);
  const report = { seconds, limitMs, halfPeriodMs, ...reading, console: errors.slice(0, 8) };
  mkdirSync(evidence, { recursive: true });
  writeFileSync(path.join(evidence, "l4-avsync.json"), JSON.stringify(report, null, 2));
  await page.screenshot({ path: path.join(evidence, "l4-avsync.jpg"), animations: "disabled" });
  const median = reading.median == null ? "none" : `${reading.median} ms`;
  const detail = `median ${median} over ${reading.pairs} pairs (${reading.flashes} flashes, ${reading.beeps} beeps); pauses ${reading.pauses.length ? reading.pauses.join(", ") : "none"}`;
  expect(reading.pairs, detail).toBeGreaterThanOrEqual(8);
  expect(reading.pauses, detail).toEqual([]);
  expect(reading.paused, detail).toBe(false);
  expect(Math.abs(reading.median ?? 10_000), detail).toBeLessThanOrEqual(limitMs);
});

/** Watches the picture for a white flash and the sound for a beep. */
async function arm() {
  const video = document.querySelector("video");
  if (!video) throw new Error("no video");
  const el = video as HTMLVideoElement & {
    captureStream?: () => MediaStream;
    requestVideoFrameCallback?: (cb: (now: number, meta: { expectedDisplayTime: number }) => void) => void;
  };
  el.muted = false;
  el.volume = 1;
  if (el.paused) await el.play();
  const meter = (window.__m = {
    flash: [] as number[],
    beep: [] as number[],
    pauses: [] as number[],
    drift: [] as [number, string, boolean][],
    t0: performance.now(),
    tracks: 0,
    ac: "",
  });
  el.addEventListener("pause", () => meter.pauses.push(Math.round(performance.now() - meter.t0)));
  const canvas = document.createElement("canvas");
  canvas.width = 8;
  canvas.height = 8;
  const ctx = canvas.getContext("2d", { willReadFrequently: true });
  if (!ctx || !el.requestVideoFrameCallback || !el.captureStream) throw new Error("this browser cannot time the picture");
  let wasBright = false;
  const onFrame = (_now: number, meta: { expectedDisplayTime: number }) => {
    ctx.drawImage(el, 0, 0, 8, 8);
    const pixels = ctx.getImageData(0, 0, 8, 8).data;
    let sum = 0;
    for (let i = 0; i < pixels.length; i += 4) sum += pixels[i];
    const bright = sum / (pixels.length / 4) > 200;
    if (bright && !wasBright) meter.flash.push(meta.expectedDisplayTime);
    wasBright = bright;
    el.requestVideoFrameCallback?.(onFrame);
  };
  el.requestVideoFrameCallback(onFrame);
  const audio = new AudioContext();
  if (audio.state === "suspended") await audio.resume();
  const captured = el.captureStream();
  meter.tracks = captured.getAudioTracks().length;
  meter.ac = audio.state;
  const source = audio.createMediaStreamSource(new MediaStream(captured.getAudioTracks()));
  const processor = audio.createScriptProcessor(256, 1, 1);
  let wasLoud = false;
  processor.onaudioprocess = (event) => {
    const samples = event.inputBuffer.getChannelData(0);
    let peak = 0;
    for (let i = 0; i < samples.length; i++) peak = Math.max(peak, Math.abs(samples[i]));
    const loud = peak > 0.05;
    if (loud && !wasLoud) {
      const stamp = audio.getOutputTimestamp();
      meter.beep.push(stamp.performanceTime + (event.playbackTime - stamp.contextTime) * 1000);
    }
    wasLoud = loud;
  };
  const quiet = audio.createGain();
  quiet.gain.value = 0;
  source.connect(processor);
  processor.connect(quiet);
  quiet.connect(audio.destination);
  window.setInterval(() => {
    meter.drift.push([Math.round(performance.now() - meter.t0), el.dataset.syncDrift ?? "", el.paused]);
  }, 1000);
}

function summarize(half: number): Reading {
  const meter = window.__m;
  const offsets: number[] = [];
  if (!meter) {
    return { tracks: 0, ac: "", flashes: 0, beeps: 0, pairs: 0, median: null, min: null, max: null, pauses: [], paused: true, drift: [], offsets };
  }
  for (const flash of meter.flash) {
    let nearest: number | null = null;
    for (const beep of meter.beep) {
      if (nearest == null || Math.abs(beep - flash) < Math.abs(nearest - flash)) nearest = beep;
    }
    if (nearest != null && Math.abs(nearest - flash) < half) offsets.push(Math.round(nearest - flash));
  }
  const sorted = [...offsets].sort((a, b) => a - b);
  const video = document.querySelector("video") as HTMLVideoElement | null;
  return {
    tracks: meter.tracks,
    ac: meter.ac,
    flashes: meter.flash.length,
    beeps: meter.beep.length,
    pairs: offsets.length,
    median: sorted.length ? sorted[Math.floor(sorted.length / 2)] : null,
    min: sorted[0] ?? null,
    max: sorted[sorted.length - 1] ?? null,
    pauses: meter.pauses,
    paused: video ? video.paused : true,
    drift: meter.drift.filter((_, index) => index % 5 === 0),
    offsets,
  };
}

declare global {
  interface Window {
    __m?: {
      flash: number[];
      beep: number[];
      pauses: number[];
      drift: [number, string, boolean][];
      t0: number;
      tracks: number;
      ac: string;
    };
  }
}
