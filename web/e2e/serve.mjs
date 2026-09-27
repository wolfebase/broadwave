// Fake HDHomeRun on 127.0.0.1 plus a staging Broadwave. No LAN discovery.
import { spawn, spawnSync } from "node:child_process";
import { createWriteStream, mkdirSync, rmSync, writeFileSync } from "node:fs";
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
const sample = path.join(run, avsync ? "sync5.ts" : "sample.ts");

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

// A 5s flash and beep, with the picture held back 1.3s. A 1s period would fold
// a whole-second lip-sync error back to zero, and a pattern that starts together
// hides a sound lead. The flash is moved earlier by that lead so the two meet.
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
  : [
      "-f",
      "lavfi",
      "-i",
      "testsrc2=size=1280x720:rate=60000/1001",
      "-f",
      "lavfi",
      "-i",
      "sine=frequency=500",
      "-t",
      "4",
    ];
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

let fakeOut = "";
const fakeArgs = avsync ? ["-ts", sample, "-source", sample] : ["-realtime", "-ts", sample];
const fake = start(path.join(run, "fakehdhr"), fakeArgs, { env: { ...process.env, FAKEHDHR_ADMIN: `127.0.0.1:${port + 10}` } }, (chunk) => {
  fakeOut += chunk.toString();
});
for (let i = 0; i < 50 && !fakeOut.includes("CONTROL_PORT="); i++) await sleep(100);
const fakeBase = fakeOut.match(/^BASE=(.+)$/m)?.[1]?.trim();
const control = fakeOut.match(/^CONTROL_PORT=(.+)$/m)?.[1]?.trim();
if (!fakeBase || !control) {
  console.error("fake tuner did not start");
  stop();
  process.exit(1);
}
const hdhr = fakeBase.replace(/^https?:\/\//, "");

const serverArgs = [
  "-config",
  config,
  "-addr",
  `127.0.0.1:${port}`,
  "-hdhr",
  hdhr,
  "-bonjour=false",
  "-staging",
];
const serverEnv = {
  ...process.env,
  BROADWAVE_E2E: "1",
  HDHR_CONTROL_PORT: control,
};
// E2E_BROADWAVE runs this same harness against another binary.
const serverBin = process.env.E2E_BROADWAVE || path.join(run, "broadwave");
let server = start(serverBin, serverArgs, { env: serverEnv });

function launchServer() {
  server = start(serverBin, serverArgs, { env: serverEnv });
  return server;
}

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
  for (let i = 0; i < 40; i++) {
    try {
      const res = await probe(`${base}/api/v1/health`);
      if (res.ok) {
        const lineup = await probe(`${base}/api/v1/channels`);
        if (lineup.ok) {
          const body = await lineup.json();
          if ((body.channels ?? []).length >= 3) return;
        }
      }
    } catch {
      // Still binding, or the process was stopped on purpose.
    }
    if (gone(server)) throw new Error(`server exited before health\n${recent}`);
    await sleep(200);
  }
  throw new Error(`server did not answer\n${recent}`);
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

const controls = http.createServer((req, res) => {
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

console.log(`e2e server ${base} fake ${fakeBase}`);
await new Promise(() => {});
