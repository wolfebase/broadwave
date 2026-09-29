// Channel switch timing: Chrome changes channel from the player's Channels
// list, as a viewer does, and each switch's time to first frame is recorded.
//
//   SWITCH_URL=http://server:8477 node e2e/switch-time.mjs
//
// SWITCH_PLAN is a comma-separated list of guide numbers, each optionally
// followed by :label, with "wait<seconds>" for a pause (default: the same
// multiplex, a flip back, a new tune, twice). SWITCH_ROUNDS repeats it.
// SWITCH_DWELL is how long each channel plays (s). SWITCH_HOVER is how long
// the pointer rests on the row before the click (ms), as a viewer reading it
// does; the time is counted from the click. Writes SWITCH_OUT.
import { mkdirSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { setTimeout as sleep } from "node:timers/promises";
import { chromium } from "@playwright/test";

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, "../..");
const base = process.env.SWITCH_URL;
if (!base) throw new Error("set SWITCH_URL");
const plan = (process.env.SWITCH_PLAN || "41.1:start,38.1:same mux,41.1:flip back,5.1:new tune,41.1:flip back,wait25,38.1:same mux,9.1:new tune")
  .split(",")
  .map((s) => s.trim());
const rounds = Number(process.env.SWITCH_ROUNDS || 3);
const dwell = Number(process.env.SWITCH_DWELL || 8);
const hover = Number(process.env.SWITCH_HOVER || 0);
const out = process.env.SWITCH_OUT || path.join(root, ".evidence/f4/switch-time.json");

const channels = await (await fetch(`${base}/api/v1/channels`)).json();
const list = Array.isArray(channels) ? channels : channels.channels;
const idOf = (number) => {
  const c = list.find((x) => x.guideNumber === number);
  if (!c) throw new Error(`no channel ${number}`);
  return c.id;
};

const browser = await chromium.launch({ channel: "chrome" });
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
let watchMs = 0;
let warmed = "";
page.on("requestfinished", async (req) => {
  if (req.method() === "POST" && req.url().endsWith("/api/v1/watch")) {
    const t = req.timing();
    watchMs = Math.round(t.responseEnd - t.requestStart);
  }
  if (req.method() === "POST" && req.url().endsWith("/warm")) {
    const res = await req.response();
    warmed = String((await res?.json().catch(() => null))?.warm ?? "");
  }
});

async function firstFrame(since) {
  const end = Date.now() + 30_000;
  while (Date.now() < end) {
    const ttff = await page.evaluate(() => document.querySelector("video.stage-video")?.dataset.ttff ?? "");
    if (ttff) return { ttff: Number(ttff), wall: Date.now() - since };
    await sleep(25);
  }
  return { ttff: -1, wall: Date.now() - since };
}

async function pick(number) {
  await page.mouse.move(600, 400);
  await page.mouse.move(640, 420);
  const guide = page.locator(".mini-guide");
  if (!(await guide.isVisible())) await page.getByRole("button", { name: "Channels", exact: true }).click();
  const row = guide.getByRole("option").filter({ has: page.locator(".mg-num", { hasText: new RegExp(`^${number.replace(".", "\\.")}$`) }) });
  await row.scrollIntoViewIfNeeded();
  watchMs = 0;
  warmed = "";
  if (hover) {
    await row.hover();
    await sleep(hover);
  }
  const since = Date.now();
  await row.click();
  return since;
}

const results = [];
for (let round = 1; round <= rounds; round++) {
  for (const step of plan) {
    if (step.startsWith("wait")) {
      await sleep(Number(step.slice(4)) * 1000);
      continue;
    }
    const [number, label = ""] = step.split(":");
    let since;
    if (label === "start") {
      since = Date.now();
      await page.goto(`${base}/watch?channel=${idOf(number)}`);
    } else {
      since = await pick(number);
    }
    const frame = await firstFrame(since);
    await sleep(500);
    const row = { round, number, label, ...frame, watchMs, warmed };
    results.push(row);
    console.log(JSON.stringify(row));
    await sleep(dwell * 1000);
  }
  await page.goto("about:blank");
  await sleep(25_000);
}
await browser.close();

const summary = {};
for (const r of results) {
  if (r.label === "start") continue;
  (summary[r.label] ??= []).push(r.ttff);
}
for (const [label, values] of Object.entries(summary)) {
  const sorted = [...values].sort((a, b) => a - b);
  summary[label] = { n: sorted.length, median: sorted[Math.floor(sorted.length / 2)], max: sorted.at(-1), all: values };
}
mkdirSync(path.dirname(out), { recursive: true });
writeFileSync(out, JSON.stringify({ base, plan, rounds, hover, summary, results }, null, 1));
console.log(JSON.stringify(summary));
