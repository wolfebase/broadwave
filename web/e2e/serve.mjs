// Fake HDHomeRun on 127.0.0.1 plus a staging Broadwave. No LAN discovery,
// except with E2E_BONJOUR=1: then the server listens on every address and
// announces itself, so while it runs a fresh server that needs setup is
// reachable from the local network.
import { spawn, spawnSync } from "node:child_process";
import { closeSync, createWriteStream, mkdirSync, openSync, readFileSync, readSync, renameSync, rmSync, statSync, unlinkSync, writeFileSync } from "node:fs";
import http from "node:http";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { setTimeout as sleep } from "node:timers/promises";

const here = path.dirname(fileURLToPath(import.meta.url));
const run = path.join(here, ".run");
const config = path.join(run, "config");
const port = Number(process.env.E2E_PORT || 18731);
const base = `http://127.0.0.1:${port}`;
const admin = `http://127.0.0.1:${port + 10}`;
const lane = `http://127.0.0.1:${port + 9}`;
const db = path.join(config, "broadwave.db");
rmSync(config, { recursive: true, force: true });
mkdirSync(config, { recursive: true });
writeFileSync(path.join(run, "server.json"), JSON.stringify({ base, db, config, admin, control: lane }));

const log = createWriteStream(path.join(run, "server.log"), { flags: "a" });
const children = new Set();
let recent = "";

function noteLog(chunk, onStdout) {
  log.write(chunk);
  recent = (recent + chunk.toString()).slice(-2000);
  onStdout?.(chunk);
}

function start(cmd, args, extra = {}, onStdout) {
  const child = spawn(cmd, args, { stdio: ["ignore", "pipe", "pipe"], ...extra });
  children.add(child);
  child.stdout?.on("data", (chunk) => noteLog(chunk, onStdout));
  child.stderr?.on("data", (chunk) => noteLog(chunk));
  child.on("exit", () => children.delete(child));
  return child;
}

function stop() {
  for (const child of children) child.kill("SIGINT");
  if (process.env.E2E_IMAGE) spawnSync("docker", ["rm", "-f", `broadwave-e2e-${port}`], { stdio: "ignore" });
}

process.on("SIGTERM", () => {
  stop();
  process.exit(0);
});
process.on("SIGINT", () => {
  stop();
  process.exit(0);
});

const avsync = process.env.E2E_AVSYNC === "1";
const brk = process.env.E2E_BREAK === "1";
const playlist = process.env.E2E_PLAYLIST === "1";
// Two sound tracks (English, Spanish), so a watch that asks for alternates gets a
// master. The realtime pattern has one sound, so the sample streams as the source.
const tracks = process.env.E2E_TRACKS === "2";
// Four channels on two tuners, and the software budget of four pictures.
const quad = process.env.E2E_QUAD === "1";
// E2E_BONJOUR=1 advertises the server under E2E_NAME and listens on every
// address, so a simulator can find it and reach the address Bonjour gives it.
const bonjour = process.env.E2E_BONJOUR === "1";
const bonjourName = process.env.E2E_NAME || `Broadwave E2E ${port}`;
const listen = bonjour ? ["-addr", `:${port}`] : ["-addr", `127.0.0.1:${port}`, "-bonjour=false"];
const sample = path.join(run, brk ? "loop.ts" : avsync ? "sync5.ts" : "sample.ts");

function runFfmpeg(args) {
  return new Promise((resolve) => {
    const child = spawn("ffmpeg", args, { stdio: ["ignore", "pipe", "pipe"] });
    let err = "";
    child.stdout?.on("data", (chunk) => noteLog(chunk));
    child.stderr?.on("data", (chunk) => {
      noteLog(chunk);
      err = (err + chunk.toString()).slice(-2000);
    });
    child.on("exit", (code) => resolve({ code, err }));
  });
}

// How long one pass of a TS plays, from the first PCR stream. Same rule as the
// fake tuner: a file with no PCR would be rushed through in four seconds.
function pcrSpanSeconds(file) {
  const fd = openSync(file, "r");
  try {
    const size = statSync(file).size;
    const buf = Buffer.alloc(188 * 512);
    let pos = 0;
    let pid = -1;
    let first = 0;
    let last = 0;
    let n = 0;
    while (pos + 188 <= size) {
      const got = readSync(fd, buf, 0, buf.length, pos);
      const packets = got - (got % 188);
      if (packets < 188) break;
      for (let i = 0; i < packets; i += 188) {
        if (buf[i] !== 0x47 || (buf[i + 3] & 0x20) === 0 || buf[i + 4] < 7 || (buf[i + 5] & 0x10) === 0) continue;
        const id = ((buf[i + 1] & 0x1f) << 8) | buf[i + 2];
        if (pid >= 0 && id !== pid) continue;
        const base = buf[i + 6] * 2 ** 25 + buf[i + 7] * 2 ** 17 + buf[i + 8] * 2 ** 9 + buf[i + 9] * 2 + (buf[i + 10] >> 7);
        if (pid < 0) {
          pid = id;
          first = base;
        }
        last = base;
        n++;
      }
      pos += packets;
    }
    if (n < 2) return 0;
    let span = (last - first) & (2 ** 33 - 1);
    if (span === 0) return 0;
    span += span / (n - 1);
    return span / 90000;
  } finally {
    closeSync(fd);
  }
}

function videoPackets(file) {
  const probed = spawnSync(
    "ffprobe",
    ["-v", "error", "-select_streams", "v:0", "-show_entries", "packet=pts_time,flags", "-of", "csv=p=0", file],
    { encoding: "utf8", maxBuffer: 16 * 1024 * 1024 },
  );
  const keys = [];
  let last = null;
  let firstFlags = "";
  for (const line of (probed.stdout || "").trim().split("\n")) {
    if (!line) continue;
    const [pts, flags = ""] = line.split(",");
    const time = Number(pts);
    if (!Number.isFinite(time)) continue;
    if (last == null) firstFlags = flags;
    last = time;
    if (flags.includes("K")) keys.push(time);
  }
  return { keys, last, firstIsKey: firstFlags.includes("K") };
}

// A flush audio ending does not stop the playhead. The hole a restart leaves
// shows up when the sound ends a little before the picture, which is what a
// broadcast cut does. Drop the audio packets in that last stretch.
function trimTrailingAudio(file, videoLast) {
  const probed = spawnSync(
    "ffprobe",
    ["-v", "error", "-select_streams", "a:0", "-show_entries", "stream=id", "-of", "csv=p=0", file],
    { encoding: "utf8" },
  );
  const raw = (probed.stdout || "").trim().split("\n")[0] || "";
  const pid = Number.parseInt(raw, raw.startsWith("0x") ? 16 : 10);
  if (!Number.isFinite(pid) || videoLast == null) return;
  const cut = videoLast - 0.07;
  const data = readFileSync(file);
  const out = [];
  let drop = false;
  for (let i = 0; i + 188 <= data.length; i += 188) {
    const id = ((data[i + 1] & 0x1f) << 8) | data[i + 2];
    if (id === pid) {
      const start = (data[i + 1] & 0x40) !== 0;
      if (start) {
        let off = i + 4;
        if ((data[i + 3] & 0x20) !== 0) off += 1 + data[i + 4];
        const pes = off + 14 <= i + 188 && data[off] === 0 && data[off + 1] === 0 && data[off + 2] === 1 && (data[off + 7] & 0x80) !== 0;
        if (pes) {
          const b = off + 9;
          const pts =
            ((data[b] & 0x0e) * 2 ** 29) +
            (data[b + 1] * 2 ** 22) +
            ((data[b + 2] & 0xfe) * 2 ** 14) +
            (data[b + 3] * 2 ** 7) +
            (data[b + 4] >> 1);
          if (pts / 90000 > cut) drop = true;
        }
      }
      if (drop) continue;
    }
    out.push(data.subarray(i, i + 188));
  }
  writeFileSync(file, Buffer.concat(out));
}

// A ~20s loop cut on a keyframe, so each raw pass is a clean timestamp break.
async function buildBreakLoop(dest) {
  const src = `${dest}.src.ts`;
  const encoded = await runFfmpeg([
    "-hide_banner",
    "-loglevel",
    "error",
    "-y",
    "-f",
    "lavfi",
    "-i",
    "testsrc2=size=1280x720:rate=60000/1001",
    "-f",
    "lavfi",
    "-i",
    "sine=frequency=500:sample_rate=48000",
    "-t",
    "25",
    "-c:v",
    "libx264",
    "-preset",
    "ultrafast",
    "-pix_fmt",
    "yuv420p",
    "-g",
    "30",
    "-keyint_min",
    "30",
    "-x264-params",
    "scenecut=0:keyint=30:min-keyint=30",
    "-c:a",
    "ac3",
    "-ac",
    "2",
    "-b:a",
    "192k",
    "-f",
    "mpegts",
    src,
  ]);
  if (encoded.code !== 0) {
    console.error(encoded.err || "ffmpeg did not write the break source");
    process.exit(1);
  }
  const srcVideo = videoPackets(src);
  const seg = `${dest}.seg%d.ts`;
  // segment_time cuts on the next keyframe, so the file starts and ends on a GOP.
  const copied = await runFfmpeg([
    "-hide_banner",
    "-loglevel",
    "error",
    "-y",
    "-i",
    src,
    "-map",
    "0",
    "-c",
    "copy",
    "-f",
    "segment",
    "-segment_time",
    "20",
    "-break_non_keyframes",
    "0",
    "-segment_format",
    "mpegts",
    seg,
  ]);
  if (copied.code !== 0) {
    console.error(copied.err || "ffmpeg did not cut the break loop");
    process.exit(1);
  }
  const piece = `${dest}.seg0.ts`;
  renameSync(piece, dest);
  for (const extra of [`${dest}.seg1.ts`, `${dest}.seg2.ts`, src]) {
    try {
      unlinkSync(extra);
    } catch {
      // The loop is one segment; a short source has no remainder.
    }
  }
  const out = videoPackets(dest);
  const next = srcVideo.keys.find((key) => out.keys.length && key > out.keys.at(-1) + 0.001);
  const gap = next == null || out.last == null ? Infinity : next - out.last;
  const origin = srcVideo.keys[0];
  if (!out.firstIsKey || !out.keys.length || origin == null || Math.abs(out.keys[0] - origin) > 0.02 || !(gap > 0.01 && gap < 0.04)) {
    console.error(`break loop is not GOP aligned (first ${out.keys[0]}, last ${out.last}, next key ${next}, gap ${gap})`);
    process.exit(1);
  }
  const probed = spawnSync("ffprobe", ["-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", dest], { encoding: "utf8" });
  const dur = Number((probed.stdout || "").trim());
  const pcr = pcrSpanSeconds(dest);
  if (!(dur >= 18 && dur <= 22) || !(pcr >= 15 && pcr <= 25)) {
    console.error(`break loop duration ${dur}s pcr ${pcr}s, wanted about 20`);
    process.exit(1);
  }
  trimTrailingAudio(dest, out.last);
  console.log(`break loop ${dur.toFixed(3)}s pcr ${pcr.toFixed(3)}s`);
}

// A 5s flash and beep, with the picture held back 1.3s. A 1s period would fold
// a whole-second lip-sync error back to zero, and a pattern that starts together
// hides a sound lead. The flash is moved earlier by that lead so the two meet.
// The playlist run wants one long pass so the loop seam is not what stops the picture.
const pattern = avsync
  ? [
      "-itsoffset",
      "1.3",
      "-f",
      "lavfi",
      "-i",
      "testsrc2=s=1280x720:r=60000/1001,drawbox=x=0:y=0:w=iw:h=ih:color=white:t=fill:enable='lt(mod(t+1.3\\,5)\\,0.05)'",
      "-f",
      "lavfi",
      "-i",
      "sine=f=1000:sample_rate=48000,volume=0:enable='gte(mod(t\\,5)\\,0.08)'",
      "-t",
      "150",
    ]
  : playlist
    ? [
        "-f",
        "lavfi",
        "-i",
        "testsrc2=size=640x360:rate=30",
        "-f",
        "lavfi",
        "-i",
        "sine=frequency=500",
        "-t",
        "30",
      ]
    : [
        "-f",
        "lavfi",
        "-i",
        "testsrc2=size=1280x720:rate=60000/1001",
        "-f",
        "lavfi",
        "-i",
        "sine=frequency=500",
        ...(tracks ? ["-f", "lavfi", "-i", "sine=frequency=800"] : []),
        "-t",
        tracks ? "30" : "4",
        ...(tracks
          ? ["-map", "0:v", "-map", "1:a", "-map", "2:a", "-metadata:s:a:0", "language=eng", "-metadata:s:a:1", "language=spa"]
          : []),
      ];
if (brk) {
  await buildBreakLoop(sample);
} else {
  const encoded = await runFfmpeg([
    "-hide_banner",
    "-loglevel",
    "error",
    "-y",
    ...pattern,
    "-c:v",
    "libx264",
    "-preset",
    "ultrafast",
    "-g",
    "30",
    "-pix_fmt",
    "yuv420p",
    "-c:a",
    "ac3",
    "-ac",
    "2",
    "-f",
    "mpegts",
    sample,
  ]);
  if (encoded.code !== 0) {
    console.error(encoded.err || "ffmpeg did not write the sample");
    process.exit(1);
  }
  if (avsync) {
    const probed = spawnSync(
      "ffprobe",
      ["-v", "error", "-show_entries", "stream=codec_type,start_time", "-of", "csv=p=0", sample],
      { encoding: "utf8" },
    );
    const began = {};
    for (const line of (probed.stdout || "").split("\n")) {
      const parts = line.trim().split(",").filter(Boolean);
      const kind = parts.find((part) => part === "video" || part === "audio");
      const at = parts.map(Number).find((value) => Number.isFinite(value));
      if (kind && at != null && began[kind] == null) began[kind] = at;
    }
    const lead = began.video - began.audio;
    if (!(lead > 1.15 && lead < 1.5)) {
      console.error(`sync pattern lead is ${lead}s, wanted about 1.3\n${probed.stdout}`);
      process.exit(1);
    }
    console.log(`sync pattern lead ${lead.toFixed(3)}s`);
  }
}

// One M3U channel whose stream this process can stop and start. No HDHomeRun.
function startOrigin(file, listenPort) {
  const raw = readFileSync(file);
  const data = raw.subarray(0, raw.length - (raw.length % 188));
  if (data.length < 188) {
    console.error("playlist sample is empty");
    process.exit(1);
  }
  const span = pcrSpanSeconds(file) || 30;
  const bytesPerMs = data.length / (span * 1000);
  let serving = true;
  let generation = 0;
  const sockets = new Set();
  const origin = http.createServer((req, res) => {
    const route = (req.url || "/").split("?")[0];
    if (req.method === "GET" && route === "/pl.m3u") {
      const body = `#EXTM3U\n#EXTINF:-1 tvg-id="local" tvg-chno="801",Local News\nhttp://127.0.0.1:${listenPort}/live.ts\n`;
      res.writeHead(200, { "content-type": "audio/x-mpegurl", "cache-control": "no-store" });
      res.end(body);
      return;
    }
    if (req.method === "POST" && (route === "/stop" || route === "/start")) {
      if (route === "/stop") {
        serving = false;
        generation += 1;
        for (const socket of sockets) socket.destroy();
        sockets.clear();
      } else {
        serving = true;
      }
      res.writeHead(204);
      res.end();
      return;
    }
    if (req.method === "GET" && route === "/live.ts") {
      if (!serving) {
        res.writeHead(503);
        res.end();
        return;
      }
      const gen = generation;
      res.writeHead(200, { "content-type": "video/mp2t", "cache-control": "no-store" });
      const socket = res.socket;
      if (socket) sockets.add(socket);
      const finish = () => {
        if (socket) sockets.delete(socket);
      };
      res.on("close", finish);
      const pump = async () => {
        let offset = 0;
        try {
          while (serving && gen === generation && !res.destroyed && !res.writableEnded) {
            const n = Math.min(188 * 49, data.length - offset);
            if (n < 188) {
              offset = 0;
              continue;
            }
            if (!res.write(data.subarray(offset, offset + n))) {
              await new Promise((resolve) => res.once("drain", resolve));
            }
            offset += n;
            if (offset >= data.length) offset = 0;
            await sleep(n / bytesPerMs);
          }
        } catch {
          // The viewer, or stop, closed this response.
        }
        finish();
        if (!res.writableEnded) {
          try {
            res.end();
          } catch {
            // Already destroyed.
          }
        }
      };
      void pump();
      return;
    }
    res.writeHead(404);
    res.end();
  });
  origin.listen(listenPort, "127.0.0.1");
  return origin;
}

let fakeBase = "";
let serverArgs;
let serverEnv;
const originPort = port + 8;
if (playlist) {
  startOrigin(sample, originPort);
  const noted = JSON.parse(readFileSync(path.join(run, "server.json"), "utf8"));
  noted.origin = `http://127.0.0.1:${originPort}`;
  writeFileSync(path.join(run, "server.json"), JSON.stringify(noted));
  serverArgs = ["-config", config, ...listen, "-staging"];
  serverEnv = { ...process.env, BROADWAVE_E2E: "1" };
  // An address in the environment would still be passed as -hdhr's default and
  // would tune a real device. This run has only the playlist.
  delete serverEnv.HDHR_HOST;
  delete serverEnv.HDHR_CONTROL_PORT;
} else {
  let fakeOut = "";
  // E2E_SOURCE streams a broadcast recording on every channel, for a real encode load.
  const source = process.env.E2E_SOURCE;
  const fakeArgs = brk ? ["-raw", "-ts", sample] : avsync ? ["-ts", sample, "-source", sample] : source ? ["-ts", sample, "-source", path.resolve(source)] : tracks ? ["-ts", sample, "-source", sample] : ["-realtime", "-ts", sample];
  if (quad) fakeArgs.push("-quad");
  // FLEX-4K adds a clear 3.0 row and an encrypted 3.0 twin of 5.1 (115.1 WTST).
  if (process.env.E2E_ATSC3 === "1") fakeArgs.push("-profile", "FLEX-4K");
  start(path.join(run, "fakehdhr"), fakeArgs, { env: { ...process.env, FAKEHDHR_ADMIN: `127.0.0.1:${port + 10}` } }, (chunk) => {
    fakeOut += chunk.toString();
  });
  for (let i = 0; i < 50 && !fakeOut.includes("CONTROL_PORT="); i++) await sleep(100);
  fakeBase = fakeOut.match(/^BASE=(.+)$/m)?.[1]?.trim() || "";
  const control = fakeOut.match(/^CONTROL_PORT=(.+)$/m)?.[1]?.trim();
  if (!fakeBase || !control) {
    console.error("fake tuner did not start");
    stop();
    process.exit(1);
  }
  const hdhr = fakeBase.replace(/^https?:\/\//, "");
  serverArgs = ["-config", config, ...listen, "-hdhr", hdhr, "-staging"];
  serverEnv = { ...process.env, BROADWAVE_E2E: "1", HDHR_CONTROL_PORT: control };
  if (quad) serverEnv.BROADWAVE_ENCODER ||= "software";
  // E2E_SPEED=1.8 gives the server the picture budget of a machine that encodes at 1.8x.
  if (process.env.E2E_SPEED) serverEnv.BROADWAVE_SPEED = process.env.E2E_SPEED;
}
// E2E_BROADWAVE runs this same harness against another binary. E2E_IMAGE runs
// the server from a Docker image instead, on the host network so it reaches the
// fake tuner on 127.0.0.1, with the config folder at the same path.
const serverBin = process.env.E2E_BROADWAVE || path.join(run, "broadwave");
const image = process.env.E2E_IMAGE || "";
const container = `broadwave-e2e-${port}`;
const imageZone = "America/Chicago";

function removeContainer() {
  spawnSync("docker", ["rm", "-f", container], { stdio: "ignore" });
  // `docker run --rm` removes it in the background after a kill.
  for (let i = 0; i < 100; i++) {
    if (spawnSync("docker", ["inspect", container], { stdio: "ignore" }).status !== 0) return;
    spawnSync("sleep", ["0.1"]);
  }
}

function launchServer() {
  if (!image) {
    server = start(serverBin, serverArgs, { env: serverEnv });
    return server;
  }
  removeContainer();
  const env = ["BROADWAVE_E2E", "HDHR_CONTROL_PORT", "BROADWAVE_ENCODER", "BROADWAVE_SPEED"]
    .filter((key) => serverEnv[key])
    .flatMap((key) => ["-e", `${key}=${serverEnv[key]}`]);
  // Started as root with PUID and PGID, as the README installs it, so the
  // switch to that user and the hand-over of the config folder are tested.
  const identity = ["-e", `PUID=${process.getuid()}`, "-e", `PGID=${process.getgid()}`, "-e", `TZ=${imageZone}`];
  const args = ["run", "--rm", "--name", container, "--network", "host", "-e", "HOME=/tmp", ...identity, ...env, "-v", `${config}:${config}`, image, ...serverArgs];
  server = start("docker", args);
  return server;
}

let server;
launchServer();

function gone(child) {
  if (!child || child.exitCode != null || child.signalCode != null) return true;
  try {
    process.kill(child.pid, 0);
    return false;
  } catch {
    return true;
  }
}

async function stopServer() {
  const child = server;
  if (image) removeContainer();
  if (gone(child)) return;
  // Closing the pipes lets the exit event arrive after a kill. A grandchild
  // can otherwise keep them open and the wait never ends.
  child.stdout?.destroy();
  child.stderr?.destroy();
  try {
    child.kill("SIGKILL");
  } catch {
    return;
  }
  for (let i = 0; i < 30 && !gone(child); i++) await sleep(100);
}

async function probe(url) {
  const res = await fetch(url, { signal: AbortSignal.timeout(1500), headers: { connection: "close" } });
  return res;
}

async function waitHealth() {
  // A hosted runner can take well over 8 s to load the lineup after a restart.
  const deadline = Date.now() + 60_000;
  let last = "no answer";
  while (Date.now() < deadline) {
    try {
      const res = await probe(`${base}/api/v1/health`);
      last = `health ${res.status}`;
      if (res.ok) {
        const lineup = await probe(`${base}/api/v1/channels`);
        last = `channels ${lineup.status}`;
        if (lineup.ok) {
          const body = await lineup.json();
          const count = (body.channels ?? []).length;
          last = `${count} channels`;
          if (count >= (playlist ? 1 : 3)) return;
        }
      }
    } catch (err) {
      // Still binding, or the process was stopped on purpose.
      last = String(err?.cause?.code ?? err);
    }
    if (gone(server)) throw new Error(`server exited before health\n${recent}`);
    await sleep(200);
  }
  throw new Error(`server did not answer in 60 s (last: ${last})\n${recent}`);
}

let gate = Promise.resolve();
function queued(fn) {
  const run = gate.then(fn, fn);
  gate = run.then(
    () => {},
    () => {},
  );
  return run;
}

let ready = !playlist && !bonjour;
const controls = http.createServer((req, res) => {
  if (req.method === "GET" && req.url === "/ready") {
    res.writeHead(ready ? 200 : 503);
    res.end(ready ? "ok" : "");
    return;
  }
  if (req.method !== "POST") {
    res.writeHead(404);
    res.end();
    return;
  }
  const job =
    req.url === "/stop"
      ? () => stopServer()
      : req.url === "/start"
        ? async () => {
            await stopServer();
            launchServer();
            await waitHealth();
          }
        : null;
  if (!job) {
    res.writeHead(404);
    res.end();
    return;
  }
  queued(job).then(
    () => {
      res.writeHead(204);
      res.end();
    },
    (err) => {
      res.writeHead(500);
      res.end(String(err));
    },
  );
});
controls.listen(port + 9, "127.0.0.1");

const quietSettings = async () => {
  const { spawnSync } = await import("node:child_process");
  const sql = `
PRAGMA busy_timeout=5000;
INSERT INTO settings(key, value) VALUES('checkUpdates', '0')
  ON CONFLICT(key) DO UPDATE SET value='0';
INSERT INTO settings(key, value) VALUES('nextGuidePull', '2099-01-01T00:00:00Z')
  ON CONFLICT(key) DO UPDATE SET value='2099-01-01T00:00:00Z';
`;
  for (let i = 0; i < 200; i++) {
    const result = spawnSync("sqlite3", [db, sql], { encoding: "utf8" });
    if (result.status === 0) return;
    await sleep(50);
  }
};
void quietSettings();

for (let i = 0; i < 100; i++) {
  try {
    const res = await fetch(`${base}/api/v1/health`);
    if (res.ok) break;
  } catch {
    // The process is still binding.
  }
  if (server.exitCode != null) {
    console.error("server exited before health");
    stop();
    process.exit(1);
  }
  await sleep(200);
}

if (playlist) {
  await quietSettings();
  const origin = `http://127.0.0.1:${originPort}`;
  let added = false;
  let last = "";
  for (let i = 0; i < 40 && !added; i++) {
    try {
      const res = await fetch(`${base}/api/v1/sources`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ kind: "m3u", name: "Home", url: `${origin}/pl.m3u` }),
      });
      last = await res.text();
      if (res.ok) added = true;
    } catch (err) {
      last = String(err);
    }
    if (!added) await sleep(250);
  }
  if (!added) {
    console.error(`playlist was not added\n${last}\n${recent}`);
    stop();
    process.exit(1);
  }
  const marked = spawnSync(
    "sqlite3",
    [db, "PRAGMA busy_timeout=5000; INSERT INTO settings(key, value) VALUES('setupComplete', '1') ON CONFLICT(key) DO UPDATE SET value='1';"],
    { encoding: "utf8" },
  );
  if (marked.status !== 0) {
    console.error(marked.stderr || "could not finish setup");
    stop();
    process.exit(1);
  }
  ready = true;
  console.log(`e2e server ${base} playlist ${origin}`);
} else {
  console.log(`e2e server ${base} fake ${fakeBase}`);
}
if (bonjour) {
  // Bonjour announces the name the server started with, and every server on
  // this computer starts with the same one. Rename, then restart to announce it.
  const res = await fetch(`${base}/api/v1/server`, {
    method: "PATCH",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ name: bonjourName }),
  });
  if (!res.ok) {
    console.error(`rename: ${res.status} ${await res.text()}`);
    stop();
    process.exit(1);
  }
  await queued(async () => {
    await stopServer();
    launchServer();
    await waitHealth();
  });
  const noted = JSON.parse(readFileSync(path.join(run, "server.json"), "utf8"));
  noted.name = bonjourName;
  writeFileSync(path.join(run, "server.json"), JSON.stringify(noted));
  ready = true;
  console.log(`bonjour ${bonjourName}`);
}
await new Promise(() => {});
