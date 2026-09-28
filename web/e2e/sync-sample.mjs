// Chrome and an Apple simulator on one channel: is the browser's frame the
// simulator's frame? scripts/e2e-apple.sh runs this beside EndToEndTests.
//
// Waits for SAMPLE_DIR/watching (the channel id the simulator plays), opens the
// same channel in headless Chrome, and samples the stage video's drift every
// second into SAMPLE_OUT (JSONL). The simulator's drift comes from the
// "sync report" lines it sends the server about every 10 s, read from
// SAMPLE_LOG after byte SAMPLE_LOG_FROM. Both drifts are against the same room
// clock. Writes SAMPLE_DIR/sampled when done and exits 1 unless at least 3
// pairs (within 1 s) have a median |web - apple| under 100 ms and a max under 150.
import { existsSync, openSync, readFileSync, readSync, statSync, closeSync, writeFileSync, appendFileSync } from "node:fs";
import path from "node:path";
import { setTimeout as sleep } from "node:timers/promises";
import { chromium } from "@playwright/test";

const base = process.env.SAMPLE_BASE || "http://127.0.0.1:18731";
const dir = process.env.SAMPLE_DIR || ".";
const logPath = process.env.SAMPLE_LOG || "";
const logFrom = Number(process.env.SAMPLE_LOG_FROM || 0);
const seconds = Number(process.env.SAMPLE_SECONDS || 60);
const settle = Number(process.env.SAMPLE_SETTLE || 10);
const waitFor = Number(process.env.SAMPLE_WAIT || 600);
const out = process.env.SAMPLE_OUT || path.join(dir, "web-drift.jsonl");
const label = process.env.SAMPLE_LABEL || "apple";
const want = { pairs: 3, median: 100, max: 150 };

const watching = path.join(dir, "watching");
const sampled = path.join(dir, "sampled");

function say(line) {
  console.log(`sync-sample ${label}: ${line}`);
}

function logSince() {
  if (!logPath || !existsSync(logPath)) return "";
  const size = statSync(logPath).size;
  if (size <= logFrom) return "";
  const buf = Buffer.alloc(size - logFrom);
  const fd = openSync(logPath, "r");
  try {
    readSync(fd, buf, 0, buf.length, logFrom);
  } finally {
    closeSync(fd);
  }
  return buf.toString("utf8");
}

// time=2026-09-28T00:07:21.964-05:00 level=INFO msg="sync report" screen="…" kind=iphone room=channel:3 drift=12 state=locked …
function reports(channel) {
  const found = [];
  for (const line of logSince().split("\n")) {
    if (!line.includes('msg="sync report"')) continue;
    const field = (key) => line.match(new RegExp(`(?:^| )${key}=("(?:[^"\\\\]|\\\\.)*"|\\S+)`))?.[1]?.replace(/^"|"$/g, "");
    if (field("room") !== `channel:${channel}`) continue;
    const t = Date.parse(field("time") ?? "");
    const drift = Number(field("drift"));
    if (!Number.isFinite(t) || !Number.isFinite(drift)) continue;
    found.push({ t, drift, state: field("state"), kind: field("kind"), screen: field("screen"), build: field("build") });
  }
  return found;
}

function median(values) {
  const sorted = [...values].sort((a, b) => a - b);
  return sorted.length ? sorted[Math.floor(sorted.length / 2)] : NaN;
}

function pair(web, apple, from) {
  const pairs = [];
  for (const a of apple) {
    if (a.t < from || a.state !== "locked") continue;
    let best = null;
    for (const w of web) {
      if (w.t < from || !Number.isFinite(w.drift)) continue;
      if (!best || Math.abs(w.t - a.t) < Math.abs(best.t - a.t)) best = w;
    }
    if (best && Math.abs(best.t - a.t) <= 1000) {
      pairs.push({ t: a.t, apple: a.drift, web: best.drift, apart: best.t - a.t, diff: Math.abs(best.drift - a.drift) });
    }
  }
  return pairs;
}

async function main() {
  const end = Date.now() + waitFor * 1000;
  while (!existsSync(watching)) {
    if (Date.now() > end) throw new Error(`no ${watching} within ${waitFor} s`);
    await sleep(500);
  }
  const channel = Number(readFileSync(watching, "utf8").trim());
  if (!(channel > 0)) throw new Error(`bad channel in ${watching}`);
  say(`channel ${channel}`);

  // Playwright's own Chromium has no H.264.
  const browser = await chromium.launch({ channel: process.env.SAMPLE_BROWSER || "chrome", args: ["--autoplay-policy=no-user-gesture-required"] });
  const web = [];
  try {
    const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });
    await page.goto(`${base}/watch?channel=${channel}`);
    const video = page.locator("video.stage-video");
    await video.waitFor({ timeout: 30_000 });
    writeFileSync(out, "");
    const started = Date.now();
    let playingAt = null;
    for (;;) {
      const s = await video
        .evaluate((v) => ({ time: v.currentTime, paused: v.paused, width: v.videoWidth, drift: Number(v.dataset.syncDrift ?? NaN) }))
        .catch(() => null);
      if (s) {
        const row = { t: Date.now(), ...s, drift: Number.isFinite(s.drift) ? s.drift : null };
        web.push({ ...row, drift: s.drift });
        appendFileSync(out, `${JSON.stringify(row)}\n`);
        if (playingAt == null && s.width > 0 && !s.paused && s.time > 1) playingAt = Date.now();
      }
      if (playingAt == null && Date.now() - started > 60_000) throw new Error("the browser picture did not start");
      if (playingAt != null) {
        const from = playingAt + settle * 1000;
        const enough = Date.now() - from >= seconds * 1000 && pair(web, reports(channel), from).length >= want.pairs;
        if (enough || Date.now() - from >= seconds * 3000) break;
      }
      await sleep(1000);
    }
    await page.screenshot({ path: path.join(dir, "7-web.png") }).catch(() => undefined);
    const from = playingAt + settle * 1000;
    const apple = reports(channel);
    const pairs = pair(web, apple, from);
    const diffs = pairs.map((p) => p.diff);
    const result = {
      label,
      channel,
      webSamples: web.filter((w) => w.t >= from && Number.isFinite(w.drift)).length,
      appleReports: apple.filter((a) => a.t >= from).length,
      pairs,
      median: median(diffs),
      max: diffs.length ? Math.max(...diffs) : NaN,
      want,
    };
    result.pass = pairs.length >= want.pairs && result.median < want.median && result.max < want.max;
    writeFileSync(out.replace(/\.jsonl$/, "") + "-result.json", JSON.stringify({ ...result, apple }, null, 2));
    for (const p of pairs) say(`pair web ${p.web} ms apple ${p.apple} ms (${p.apart} ms apart) diff ${p.diff} ms`);
    say(`${result.pass ? "PASS" : "FAIL"} pairs=${pairs.length} median=${result.median} ms max=${result.max} ms (want >=${want.pairs}, <${want.median}, <${want.max})`);
    return result.pass;
  } finally {
    writeFileSync(sampled, "done\n");
    await browser.close();
  }
}

main().then(
  (ok) => process.exit(ok ? 0 : 1),
  (err) => {
    writeFileSync(sampled, `error ${err.message}\n`);
    say(`FAIL ${err.message}`);
    process.exit(1);
  },
);
