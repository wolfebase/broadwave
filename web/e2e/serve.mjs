// Fake HDHomeRun on 127.0.0.1 plus a staging Broadwave. No LAN discovery.
import { spawn } from "node:child_process";
import { createWriteStream, mkdirSync, rmSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { setTimeout as sleep } from "node:timers/promises";

const here = path.dirname(fileURLToPath(import.meta.url));
const run = path.join(here, ".run");
const config = path.join(run, "config");
const port = 18731;
const base = `http://127.0.0.1:${port}`;
const db = path.join(config, "broadwave.db");
rmSync(config, { recursive: true, force: true });
mkdirSync(config, { recursive: true });
writeFileSync(path.join(run, "server.json"), JSON.stringify({ base, db, config }));

const log = createWriteStream(path.join(run, "server.log"), { flags: "a" });
const children = new Set();

function start(cmd, args, extra = {}, onStdout) {
  const child = spawn(cmd, args, { stdio: ["ignore", "pipe", "pipe"], ...extra });
  children.add(child);
  child.stdout?.on("data", (chunk) => {
    log.write(chunk);
    onStdout?.(chunk);
  });
  child.stderr?.on("data", (chunk) => log.write(chunk));
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

const sample = path.join(run, "sample.ts");
const encoded = spawn("ffmpeg", [
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
  "sine=frequency=500",
  "-t",
  "4",
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
  "-f",
  "mpegts",
  sample,
], { stdio: ["ignore", "pipe", "pipe"] });
encoded.stdout?.pipe(log);
encoded.stderr?.pipe(log);
const encodedCode = await new Promise((resolve) => encoded.on("exit", resolve));
if (encodedCode !== 0) {
  console.error("ffmpeg did not write the sample");
  process.exit(1);
}

let fakeOut = "";
const fake = start(path.join(run, "fakehdhr"), ["-realtime", "-ts", sample], {}, (chunk) => {
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

const server = start(path.join(run, "broadwave"), [
  "-config",
  config,
  "-addr",
  `127.0.0.1:${port}`,
  "-hdhr",
  hdhr,
  "-bonjour=false",
  "-staging",
], {
  env: {
    ...process.env,
    BROADWAVE_E2E: "1",
    HDHR_CONTROL_PORT: control,
  },
});

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
