// Soak run. It starts the fake tuner and a staging server on localhost and
// never talks to a real tuner.
//
//   node scripts/soak/run.mjs --minutes 60

import { existsSync, mkdirSync, writeFileSync } from "node:fs";
import { createRequire } from "node:module";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { passed, recordingIdsToStop, renderReport, runningProcesses, settled, stopRequest, syncSpread, verdicts } from "./analyze.mjs";
import { browse } from "./browser.mjs";
import { createViewer, listRecordings, retune, startRecording, stopRecording, stopViewer, tick } from "./hls.mjs";
import { buildBinaries, soakPorts, prepareServer, startStack, writeSample } from "./launch.mjs";
import { sampleMetrics, sampleOS, sampleRuntime } from "./sample.mjs";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const args = parseArgs(process.argv.slice(2));
const runDir = path.join(root, "scripts/soak/.run");

const report = {
  summary: "",
  serverDied: false,
  samples: [],
  events: [],
  sync: [],
  limits: {
    stalls: args.stalls,
    drops: args.drops,
    syncMs: 3000,
    goroutineAllow: 40,
    goroutineRatio: 1.5,
    fdAllow: 40,
    fdRatio: 1.5,
    heapAllow: 32 * 1024 * 1024,
    heapRatio: 1.75,
  },
};

let stack = null;
let browser = null;
const started = Date.now();
const halt = { now: false };
let finished = false;
let stopHits = 0;
// The first signal ends the viewers and still reads the quiet window.
// A second signal kills the children now, in case the process is exiting
// before that read. A hangup is the same stop: a closed terminal must
// not take the server down in the middle of a sample.
const stopSoon = () => {
  stopHits += 1;
  const request = stopRequest(stopHits);
  halt.now = request.halt;
  if (!request.killChildren) return;
  try {
    stack?.stop?.();
  } catch {
    // The child is already gone.
  }
};
process.on("SIGINT", stopSoon);
process.on("SIGTERM", stopSoon);
process.on("SIGHUP", stopSoon);

try {
  const base = await bringUp();
  const channels = await prepareServer(base);
  if (channels.length < 1) throw new Error("the server has no channels");
  log(`server ${base} channels ${channels.map((channel) => channel.guideNumber).join(", ")}`);
  await sampleFor(base, "base", 3, true);
  const until = Date.now() + args.minutes * 60 * 1000;
  await runViewers(base, channels, until);
  log("viewers stopped, waiting for renditions to leave");
  await waitSettled(base);
  await sampleFor(base, "quiet", 3, true);
} catch (err) {
  report.serverDied = report.serverDied || Boolean(stack?.serverGone?.());
  addEvent({ who: "soak", kind: "error", detail: String(err?.message || err) });
  log(String(err?.stack || err));
  if (!report.samples.some((sample) => sample.phase === "quiet")) {
    try {
      if (stack?.base) await sampleFor(stack.base, "quiet", 1, true);
    } catch {
      // The server is already gone. The report still says so.
    }
  }
} finally {
  if (browser) await browser.close().catch(() => {});
  stack?.stop?.();
  finish();
}

async function bringUp() {
  const ports = soakPorts(args.port);
  mkdirSync(runDir, { recursive: true });
  if (args.build || !existsSync(path.join(runDir, "broadwave")) || !existsSync(path.join(runDir, "fakehdhr"))) {
    await buildBinaries(root, runDir);
  }
  const sample = await writeSample(runDir);
  stack = await startStack({ root, runDir, ports, sample });
  return stack.base;
}

async function runViewers(base, channels, until) {
  const viewers = Array.from({ length: args.hls }, (_, index) => createViewer(`hls ${index + 1}`));
  for (let index = 0; index < viewers.length; index++) {
    await retune(viewers[index], base, channels[index % channels.length].id, args.quality);
    addEvent({ who: viewers[index].id, kind: "watch", detail: channels[index % channels.length].guideNumber });
  }
  const points = new Map();
  const jobs = [];
  if (args.browsers > 0) {
    const { chromium } = loadPlaywright();
    browser = await chromium.launch({
      channel: "chrome",
      headless: true,
      args: ["--mute-audio", "--disable-dev-shm-usage"],
    });
    for (let index = 0; index < args.browsers; index++) {
      jobs.push(browse({
        browser,
        base,
        channels,
        until,
        halt,
        quality: args.quality,
        index,
        onEvent: addEvent,
        onPosition: (point) => points.set(`browser ${index + 1}`, { ...point, at: Date.now(), aligned: false }),
      }));
    }
  }
  let recording = null;
  let recordingsBefore = null;
  try {
    recordingsBefore = await listRecordings(base);
  } catch (err) {
    addEvent({ who: "record", kind: "error", detail: String(err.message || err) });
  }
  let nextSample = Date.now();
  let cursor = 0;
  let round = 0;
  while (Date.now() < until && !halt.now) {
    if (stack?.serverGone?.()) {
      report.serverDied = true;
      throw new Error("server exited during the soak");
    }
    const now = Date.now();
    for (const viewer of viewers) {
      const events = await tick(viewer, base, now);
      for (const event of events) addEvent({ who: viewer.id, ...event });
      points.set(viewer.id, {
        channelId: `hls:${viewer.channelId}`,
        edgePdt: viewer.edgePdt,
        aligned: viewer.aligned && !viewer.paused && now >= viewer.quietUntil,
        at: now,
      });
    }
    rememberSync(points);
    if (now >= nextSample) {
      try {
        await takeSample(base, "run", false);
      } catch (err) {
        if (stack?.serverGone?.()) throw err;
        addEvent({ who: "soak", kind: "error", detail: `sample ${err?.message || err}` });
      }
      nextSample = now + 15_000;
      log(progress(now, until));
    }
    cursor += 1;
    if (cursor % 40 === 0) {
      await nudge(viewers, channels, base, round);
      round += 1;
    }
    if (cursor % 90 === 0) recording = await rollRecording(base, channels, recording);
    await sleep(1_000);
  }
  await Promise.all(jobs.splice(0).map((job) => job.catch((err) => addEvent({ who: "browser", kind: "error", detail: String(err.message || err) }))));
  for (const viewer of viewers) await stopViewer(viewer, base);
  await releaseRecordings(base, recordingsBefore);
}

async function nudge(viewers, channels, base, round) {
  if (viewers.length === 0) return;
  const viewer = viewers[round % viewers.length];
  try {
    // Spread pauses and seeks across viewers. A cursor divisible by 40 was
    // always the first viewer, so the others never left their first channel.
    if (round % 6 === 0) {
      viewer.paused = true;
      addEvent({ who: viewer.id, kind: "pause", detail: "holding the watch" });
      await sleep(8_000);
      viewer.paused = false;
      viewer.quietUntil = Date.now() + 8_000;
      addEvent({ who: viewer.id, kind: "play", detail: "resumed" });
      return;
    }
    if (round % 6 === 3) {
      viewer.aligned = false;
      addEvent({ who: viewer.id, kind: "seek", detail: "back from the live edge" });
      await sleep(10_000);
      viewer.aligned = true;
      return;
    }
    const at = channels.findIndex((channel) => channel.id === viewer.channelId);
    const channel = channels[(Math.max(0, at) + 1) % channels.length];
    if (!channel || channel.id === viewer.channelId) return;
    await retune(viewer, base, channel.id, args.quality);
    addEvent({ who: viewer.id, kind: "channel", detail: channel.guideNumber });
  } catch (err) {
    addEvent({ who: viewer.id, kind: "error", detail: String(err.message || err) });
  }
}

async function releaseRecordings(base, before) {
  let after = null;
  try {
    after = await listRecordings(base);
  } catch (err) {
    addEvent({ who: "record", kind: "error", detail: String(err.message || err) });
  }
  const ids = recordingIdsToStop({ before, after });
  for (const id of ids) {
    const stopped = await stopRecording(base, id);
    addEvent({ who: "record", kind: stopped ? "record" : "error", detail: stopped ? `stopped ${id}` : `stop ${id} failed` });
  }
}

async function rollRecording(base, channels, current) {
  if (current) {
    const stopped = await stopRecording(base, current.id);
    addEvent({ who: "record", kind: stopped ? "record" : "error", detail: stopped ? "stopped" : `stop ${current.id} failed` });
    return null;
  }
  const channel = channels[0];
  try {
    const created = await startRecording(base, channel.id, 2, "Soak hour");
    addEvent({ who: "record", kind: "record", detail: channel.guideNumber });
    return created;
  } catch (err) {
    addEvent({ who: "record", kind: "error", detail: String(err.message || err) });
    return null;
  }
}

function rememberSync(points) {
  const fresh = [...points.values()].filter((point) => point.aligned && Date.now() - point.at < 6_000);
  for (const row of syncSpread(fresh)) report.sync.push(row);
}

// A closed viewer is dropped on the next sweep, about a minute after the
// last fetch. Waiting a fixed 45 seconds raced that sweep and scored a
// healthy server as a leak.
async function waitSettled(base) {
  const deadline = Date.now() + 90_000;
  while (Date.now() < deadline && !stopRequest(stopHits).killChildren) {
    const metrics = await sampleMetrics(base).catch(() => null);
    const os = await sampleOS(stack?.pid).catch(() => ({ ffmpeg: null, zombies: null }));
    const ffmpeg = runningProcesses({ ffmpegHub: metrics?.ffmpeg ?? null, ffmpegOS: os?.ffmpeg ?? null });
    if (settled({ viewers: metrics?.viewers ?? null, ffmpeg, zombies: os?.zombies ?? null })) return;
    await sleep(2_000);
  }
}

async function sampleFor(base, phase, count, gc) {
  for (let i = 0; i < count; i++) {
    await takeSample(base, phase, gc);
    if (i + 1 < count) await sleep(2_000);
  }
}

async function takeSample(base, phase, gc) {
  const runtime = await sampleRuntime(base, gc);
  const metrics = await sampleMetrics(base).catch(() => null);
  const os = await sampleOS(stack?.pid).catch(() => ({ rss: null, ffmpeg: null, zombies: null }));
  // No pid means the operating system was not asked. A failed ps leaves nulls,
  // and the quiet check treats a null as "could not tell".
  const askedOS = Boolean(stack?.pid);
  report.samples.push({
    t: Date.now(),
    phase,
    goroutines: runtime.goroutines,
    heapAlloc: runtime.heapAlloc,
    fds: runtime.openFDs,
    rss: os.rss,
    ffmpegHub: metrics ? metrics.ffmpeg : null,
    ffmpegOS: askedOS ? os.ffmpeg : metrics ? metrics.ffmpeg : null,
    zombies: askedOS ? os.zombies : 0,
    viewers: metrics ? metrics.viewers : null,
    feeds: metrics ? metrics.feeds : null,
  });
}

function addEvent(event) {
  report.events.push({ ...event, at: new Date().toISOString() });
}

function progress(now, until) {
  const left = Math.max(0, Math.round((until - now) / 1000));
  const last = report.samples.at(-1);
  const stalls = report.events.filter((event) => event.kind === "stall").length;
  const drops = report.events.filter((event) => event.kind === "drop").length;
  return `left ${left}s goroutines ${last?.goroutines ?? "?"} ffmpeg ${last?.ffmpegOS ?? "?"} viewers ${last?.viewers ?? "?"} stalls ${stalls} drops ${drops}`;
}

function finish() {
  if (finished) return;
  finished = true;
  const minutes = Math.round((Date.now() - started) / 60000);
  report.summary = `${minutes} min, ${args.browsers} browser viewers, ${args.hls} scripted viewers, quality ${args.quality}.`;
  report.verdicts = verdicts(report);
  const html = renderReport(report);
  mkdirSync(runDir, { recursive: true });
  const htmlPath = args.out || path.join(runDir, "report.html");
  const jsonPath = htmlPath.replace(/\.html$/, "") + ".json";
  writeFileSync(htmlPath, html);
  writeFileSync(jsonPath, JSON.stringify(report, null, 2));
  log(report.summary);
  for (const row of report.verdicts) log(`${row.ok ? "ok" : "FAIL"} ${row.name}: ${row.detail}`);
  log(`report ${htmlPath}`);
  process.exitCode = passed(report.verdicts) ? 0 : 1;
}

function loadPlaywright() {
  const require = createRequire(path.join(root, "web/package.json"));
  try {
    return require("playwright");
  } catch {
    return require("playwright-core");
  }
}

function parseArgs(argv) {
  const out = {
    minutes: 24 * 60,
    port: 19180,
    browsers: 2,
    hls: 4,
    quality: "360",
    build: false,
    out: "",
    stalls: 40,
    drops: 0,
  };
  for (let i = 0; i < argv.length; i++) {
    const flag = argv[i];
    const next = () => argv[++i];
    if (flag === "--minutes") out.minutes = Number(next());
    else if (flag === "--port") out.port = Number(next());
    else if (flag === "--browsers") out.browsers = Number(next());
    else if (flag === "--hls") out.hls = Number(next());
    else if (flag === "--quality") out.quality = next();
    else if (flag === "--out") out.out = next();
    else if (flag === "--stalls") out.stalls = Number(next());
    else if (flag === "--drops") out.drops = Number(next());
    else if (flag === "--build") out.build = true;
    else throw new Error(`unknown argument ${flag}`);
  }
  if (!Number.isFinite(out.minutes) || out.minutes <= 0) throw new Error("--minutes must be a positive number");
  return out;
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function log(line) {
  console.log(`${new Date().toISOString()} ${line}`);
}
