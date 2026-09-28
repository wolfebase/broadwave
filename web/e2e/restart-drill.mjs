// Restart drill: a browser and Apple simulators play one channel, the server
// restarts under them, and each screen's recovery is timed.
//
//   node e2e/prepare.mjs && node e2e/restart-drill.mjs
//
// By default the fake-tuner harness (serve.mjs) plays the server and its
// control port restarts the process. DRILL_CONTAINER=1 runs this tree's Linux
// build in the published image (DRILL_IMAGE) with a fake tuner inside it, and
// `docker restart` is the restart. Or point DRILL_URL at any server and give
// DRILL_RESTART the command that restarts it.
// DRILL_SIMS lists simulator names or UDIDs with the app installed, comma separated.
// DRILL_MV=1 opens the first two channels side by side on them instead of one.
// Writes DRILL_OUT (default .evidence/p6/restart-drill.json at the repo root).
import { execSync, spawn } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { setTimeout as sleep } from "node:timers/promises";
import { chromium } from "@playwright/test";

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, "../..");
const port = Number(process.env.E2E_PORT || 18671);
const container = process.env.DRILL_CONTAINER === "1" ? "broadwave-drill" : "";
const external = container ? `http://127.0.0.1:${port + 20}` : process.env.DRILL_URL || "";
const base = external || `http://127.0.0.1:${port}`;
const sims = (process.env.DRILL_SIMS || "").split(",").map((s) => s.trim()).filter(Boolean);
const playSeconds = Number(process.env.DRILL_PLAY || 30);
const afterSeconds = Number(process.env.DRILL_AFTER || 40);
const out = process.env.DRILL_OUT || path.join(root, ".evidence/p6/restart-drill.json");
const bundle = "com.wolfeup.broadwave";

const children = [];
let browser = null;
const t0 = Date.now();
const at = () => Date.now() - t0;
const say = (line) => console.log(`[${(at() / 1000).toFixed(1)}s] ${line}`);

async function health() {
  try {
    return (await fetch(`${base}/api/v1/health`)).ok;
  } catch {
    return false;
  }
}

async function until(what, seconds, fn) {
  const end = Date.now() + seconds * 1000;
  while (Date.now() < end) {
    if (await fn()) return;
    await sleep(200);
  }
  throw new Error(`${what}: not within ${seconds} s`);
}

async function startHarness() {
  const child = spawn("node", [path.join(here, "serve.mjs")], {
    env: { ...process.env, E2E_PORT: String(port), BROADWAVE_E2E: "1" },
    stdio: ["ignore", "ignore", "inherit"],
  });
  children.push(child);
  const server = () => JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8"));
  await until("harness", 90, async () => {
    try {
      return (await fetch(`${server().control}/ready`)).ok && (await health());
    } catch {
      return false;
    }
  });
  return server();
}

// A fresh container: this tree's server and fake tuner, the image's ffmpeg.
function startContainer() {
  const run = path.join(here, ".run");
  const bin = path.join(run, "linux");
  mkdirSync(bin, { recursive: true });
  const env = { ...process.env, GOOS: "linux", GOARCH: process.arch === "arm64" ? "arm64" : "amd64", CGO_ENABLED: "0" };
  execSync(`go build -o ${bin}/broadwave ./server/cmd/broadwave && go build -o ${bin}/fakehdhr ./server/cmd/fakehdhr`, { cwd: root, env, stdio: "inherit" });
  execSync(`cp ${path.join(run, "sample.ts")} ${bin}/sample.ts`);
  writeFileSync(
    `${bin}/entry.sh`,
    [
      "/drill/fakehdhr -realtime -ts /drill/sample.ts > /tmp/fake.out &",
      "for i in $(seq 50); do grep -q CONTROL_PORT= /tmp/fake.out && break; sleep 0.1; done",
      "BASE=$(sed -n 's#^BASE=http://##p' /tmp/fake.out)",
      "export BROADWAVE_E2E=1 HDHR_CONTROL_PORT=$(sed -n 's#^CONTROL_PORT=##p' /tmp/fake.out)",
      'exec /drill/broadwave -config /config -addr :8477 -hdhr "$BASE" -bonjour=false -staging',
      "",
    ].join("\n"),
  );
  execSync(`docker rm -f ${container}`, { stdio: "ignore" });
  const image = process.env.DRILL_IMAGE || "ghcr.io/wolfebase/broadwave:latest";
  execSync(`docker run -d --name ${container} -p 127.0.0.1:${port + 20}:8477 -v ${bin}:/drill:ro --entrypoint /bin/sh ${image} /drill/entry.sh`, { stdio: "inherit" });
}

async function restart(harness) {
  if (container) {
    execSync(`docker restart -t 5 ${container}`, { stdio: "inherit" });
    return;
  }
  if (external) {
    execSync(process.env.DRILL_RESTART || "false", { stdio: "inherit" });
    return;
  }
  const res = await fetch(`${harness.control}/start`, { method: "POST" });
  if (!res.ok) throw new Error(`restart: ${res.status}`);
}

const multiview = process.env.DRILL_MV === "1";

function launchSim(sim, channelId, pair) {
  const lines = [];
  const child = spawn(
    "xcrun",
    [
      "simctl", "launch", "--console-pty", "--terminate-running-process", sim, bundle,
      "-BroadwaveServerURL", base,
      ...(pair ? ["-BroadwaveMultiview", pair.join(","), "-BroadwaveMultiviewLayout", "2up"] : ["-BroadwaveWatch", String(channelId)]),
      "-BroadwaveSyncLog", "1",
      "-ApplePersistenceIgnoreState", "YES",
    ],
    { stdio: ["ignore", "pipe", "pipe"] },
  );
  children.push(child);
  let rest = "";
  child.stdout.on("data", (chunk) => {
    rest += chunk.toString();
    const parts = rest.split("\n");
    rest = parts.pop() ?? "";
    for (const text of parts) if (text.startsWith("broadwave ")) lines.push({ t: at(), text: text.trim() });
  });
  return { sim, lines };
}

// The first moment after `from` a line matches, in ms from the drill start.
function first(lines, from, re) {
  return lines.find((l) => l.t >= from && re.test(l.text))?.t ?? null;
}

async function main() {
  if (container) {
    startContainer();
    await until("container", 60, health);
  }
  const harness = external ? null : await startHarness();
  const channels = await (await fetch(`${base}/api/v1/channels`)).json();
  const channel = (channels.channels ?? [])[0];
  if (!channel) throw new Error("no channel");
  say(`server ${base}, channel ${channel.number ?? channel.id} (${channel.id})`);

  // Playwright's own Chromium has no H.264.
  browser = await chromium.launch({ channel: process.env.DRILL_BROWSER || "chrome" });
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  const watches = [];
  page.on("request", (req) => {
    if (req.method() === "POST" && new URL(req.url()).pathname === "/api/v1/watch") watches.push(at());
  });
  await page.goto(`${base}/watch?channel=${channel.id}`);
  // A fresh catalog opens on setup's last step.
  const setup = page.getByRole("heading", { name: "Let's set up your TV" });
  const player = page.getByRole("region", { name: "Player" });
  await setup.or(player).first().waitFor({ timeout: 30_000 });
  if (await setup.isVisible()) {
    const cont = page.getByRole("button", { name: "Continue" });
    if (await cont.isVisible()) await cont.click();
    await page.getByRole("button", { name: "Watch", exact: true }).click();
    await page.goto(`${base}/watch?channel=${channel.id}`);
  }
  const pair = multiview ? (channels.channels ?? []).slice(0, 2).map((c) => c.id) : null;
  const apple = sims.map((sim) => launchSim(sim, channel.id, pair));

  // One sample every 100 ms: is the picture moving?
  const samples = [];
  const sampler = (async () => {
    for (;;) {
      if (page.isClosed() || !browser) return;
      const s = await page
        .locator("video.stage-video")
        .evaluate((v) => ({ time: v.currentTime, paused: v.paused, width: v.videoWidth, drift: Number(v.dataset.syncDrift ?? NaN) }))
        .catch(() => null);
      if (s) samples.push({ t: at(), ...s });
      await sleep(100);
    }
  })();

  await until("browser picture", 60, () => samples.some((s) => s.width > 0 && !s.paused && s.time > 1)).catch(async (err) => {
    mkdirSync(path.dirname(out), { recursive: true });
    await page.screenshot({ path: out.replace(/\.json$/, "-nopicture.png") }).catch(() => undefined);
    throw new Error(`${err.message}; last sample ${JSON.stringify(samples.at(-1) ?? null)}`);
  });
  const playing = (a) =>
    pair ? pair.every((id) => a.lines.some((l) => l.text === `broadwave tile ${id} moving`)) : a.lines.some((l) => /^broadwave moving /.test(l.text));
  for (const a of apple) await until(`${a.sim} picture`, 90, () => playing(a));
  say(`every screen is playing; ${playSeconds} s before the restart`);
  await sleep(playSeconds * 1000);

  const restartAt = at();
  say("restarting the server");
  await restart(harness);
  await until("server back", 120, health);
  const upAt = at();
  say(`server answers ${upAt - restartAt} ms after the restart began`);
  await sleep(afterSeconds * 1000);

  // The longest time the browser's picture stood still from the restart on.
  let frozenFrom = null;
  let longest = 0;
  let movingAgain = null;
  for (let i = 1; i < samples.length; i++) {
    const s = samples[i];
    if (s.t < restartAt) continue;
    const advanced = s.time > samples[i - 1].time + 0.01 && !s.paused;
    if (!advanced && frozenFrom == null) frozenFrom = s.t;
    if (advanced && frozenFrom != null) {
      longest = Math.max(longest, s.t - frozenFrom);
      if (s.t >= upAt && movingAgain == null) movingAgain = s.t;
      frozenFrom = null;
    }
  }
  const settled = samples.filter((s) => s.t > at() - 10_000 && Number.isFinite(s.drift)).map((s) => s.drift);
  const web = {
    newWatchAfterUpMs: (watches.find((t) => t >= restartAt) ?? NaN) - upAt,
    movingAfterUpMs: movingAgain == null ? null : movingAgain - upAt,
    longestFreezeMs: longest,
    stillMovingAtEnd: samples.slice(-10).some((s, i, all) => i > 0 && s.time > all[i - 1].time),
    driftLast10s: settled.length ? { min: Math.min(...settled), max: Math.max(...settled) } : null,
  };
  const screens = apple.map((a) => {
    const told = first(a.lines, restartAt, /^broadwave server restarted/);
    const ttff = first(a.lines, restartAt, /^broadwave ttff /);
    const moving = first(a.lines, restartAt, /^broadwave moving /);
    const stalled = first(a.lines, restartAt, /^broadwave stall \d/);
    const ready = first(a.lines, restartAt, /^broadwave handoff /);
    const tiles = (pair ?? []).map((id) => {
      const handoff = first(a.lines, restartAt, new RegExp(`^broadwave tile ${id} handoff `));
      const back = first(a.lines, restartAt, new RegExp(`^broadwave tile ${id} moving`));
      return { channel: id, handoffAfterUpMs: handoff == null ? null : handoff - upAt, movingAfterUpMs: back == null ? null : back - upAt };
    });
    return {
      sim: a.sim,
      tiles,
      restartNamedAfterUpMs: told == null ? null : told - upAt,
      firstFrameAfterUpMs: ttff == null ? null : ttff - upAt,
      movingAfterUpMs: moving == null ? null : moving - upAt,
      // The old picture plays from its buffer until it stalls or the new one replaces it.
      oldPictureStalledAfterUpMs: stalled == null ? null : stalled - upAt,
      handoffReadyAfterUpMs: ready == null ? null : ready - upAt,
      log: a.lines.filter((l) => l.t >= restartAt - 1000 && l.t <= restartAt + 20_000 && !/^broadwave (beat|sync) /.test(l.text)).map((l) => `${l.t - upAt} ${l.text}`),
      outages: a.lines.filter((l) => l.t >= restartAt && /^broadwave outage /.test(l.text)).map((l) => l.text),
    };
  });
  let rooms = [];
  if (!external || container) {
    const log = (container ? execSync(`docker logs ${container} 2>&1`, { encoding: "utf8", maxBuffer: 64 << 20 }) : readFileSync(path.join(here, ".run/server.log"), "utf8")).split("\n");
    rooms = [...new Set(log.filter((l) => l.includes("sync report")).map((l) => l.match(/room=(\S+)/)?.[1]).filter(Boolean))];
  }
  // One viewer per screen: a stale stop for a watch the old process had must not take one away.
  const tuners = await (await fetch(`${base}/api/v1/tuners`)).json().catch(() => ({}));
  const viewers = (tuners.tuners ?? []).reduce((n, t) => n + (t.viewers ?? 0), 0);
  const watchers = 1 + sims.length * (pair ? pair.length : 1);
  const result = { at: new Date().toISOString(), mode: container ? "container" : external ? "external" : "harness", server: base, channel: channel.id, restartToUpMs: upAt - restartAt, viewers: { counted: viewers, watches: watchers }, web, apple: screens, roomsReported: rooms };
  mkdirSync(path.dirname(out), { recursive: true });
  writeFileSync(out, JSON.stringify(result, null, 2));
  console.log(JSON.stringify(result, null, 2));
  await browser.close();
  browser = null;
  await sampler;
}

main()
  .catch((err) => {
    console.error(err);
    process.exitCode = 1;
  })
  .finally(async () => {
    await browser?.close().catch(() => undefined);
    browser = null;
    if (container && process.env.DRILL_KEEP !== "1") execSync(`docker rm -f ${container}`, { stdio: "ignore" });
    for (const child of children) child.kill("SIGINT");
    for (const sim of sims) {
      try {
        execSync(`xcrun simctl terminate "${sim}" ${bundle}`, { stdio: "ignore" });
      } catch {
        // Not running.
      }
    }
  });
