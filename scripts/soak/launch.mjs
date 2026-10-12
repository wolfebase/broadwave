// Local soak stack: a localhost fake tuner and a staging server.
// BROADWAVE_E2E keeps discovery off the LAN. HDHR_CONTROL_PORT keeps tuner
// control on the fake, which listens on localhost only.

import { spawn, spawnSync } from "node:child_process";
import { createWriteStream, mkdirSync, rmSync } from "node:fs";
import path from "node:path";
import { setTimeout as sleep } from "node:timers/promises";

export function soakPorts(port) {
  const server = port;
  const fakeHttp = port + 1;
  const fakeControl = port + 2;
  for (const value of [server, fakeHttp, fakeControl]) {
    if (value < 19180 || value > 19189) {
      throw new Error(`port ${value} is outside the soak ports (19180-19189)`);
    }
  }
  return { server, fakeHttp, fakeControl };
}

export async function buildBinaries(root, runDir) {
  mkdirSync(runDir, { recursive: true });
  const web = path.join(root, "web");
  step("npm", ["run", "build"], web);
  step("go", ["build", "-o", path.join(runDir, "broadwave"), "./server/cmd/broadwave"], root);
  step("go", ["build", "-o", path.join(runDir, "fakehdhr"), "./server/cmd/fakehdhr"], root);
}

function step(cmd, args, cwd) {
  const result = spawnSync(cmd, args, { cwd, stdio: "inherit" });
  if (result.status !== 0) throw new Error(`${cmd} ${args.join(" ")} failed`);
}

export async function writeSample(runDir) {
  const sample = path.join(runDir, "sample.ts");
  const result = spawnSync(
    "ffmpeg",
    [
      "-hide_banner",
      "-loglevel",
      "error",
      "-y",
      "-f",
      "lavfi",
      "-i",
      "testsrc2=size=640x360:rate=30",
      "-f",
      "lavfi",
      "-i",
      "sine=frequency=440:sample_rate=48000",
      "-t",
      "30",
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
    ],
    { stdio: "inherit" },
  );
  if (result.status !== 0) throw new Error("ffmpeg did not write the sample");
  return sample;
}

export async function startStack({ root, runDir, ports, sample }) {
  const config = path.join(runDir, "config");
  rmSync(config, { recursive: true, force: true });
  mkdirSync(config, { recursive: true });
  const fakeLog = createWriteStream(path.join(runDir, "fake.log"));
  const serverLog = createWriteStream(path.join(runDir, "server.log"));
  const children = [];

  const fake = spawn(path.join(runDir, "fakehdhr"), ["-ts", sample, "-source", sample], {
    env: {
      ...cleanEnv(),
      FAKEHDHR_HTTP: `127.0.0.1:${ports.fakeHttp}`,
      FAKEHDHR_CONTROL: `127.0.0.1:${ports.fakeControl}`,
    },
    stdio: ["ignore", "pipe", "pipe"],
  });
  children.push(fake);
  const fakeOut = follow(fake, fakeLog);
  const started = await fakeOut.until(/CONTROL_PORT=(\d+)/, 10_000);
  const control = started.match(/CONTROL_PORT=(\d+)/)?.[1];
  if (control !== String(ports.fakeControl)) {
    stopAll(children);
    throw new Error(`fake tuner control ${control}, wanted ${ports.fakeControl}`);
  }

  const server = spawn(path.join(runDir, "broadwave"), [
    "-config",
    config,
    "-addr",
    `127.0.0.1:${ports.server}`,
    "-hdhr",
    `127.0.0.1:${ports.fakeHttp}`,
    "-bonjour=false",
    "-staging",
  ], {
    env: {
      ...cleanEnv(),
      BROADWAVE_E2E: "1",
      BROADWAVE_ENCODER: "software",
      BROADWAVE_BENCH: "0",
      HDHR_CONTROL_PORT: String(ports.fakeControl),
    },
    stdio: ["ignore", "pipe", "pipe"],
  });
  children.push(server);
  follow(server, serverLog);
  const base = `http://127.0.0.1:${ports.server}`;
  await waitHealth(base, server);
  return {
    base,
    pid: server.pid,
    stop: () => stopAll(children),
    serverGone: () => server.exitCode != null || server.signalCode != null,
  };
}

function cleanEnv() {
  const env = { ...process.env };
  delete env.HDHR_HOST;
  delete env.HDHR_CONTROL_PORT;
  return env;
}

// follow copies a child's output to the log and keeps the text, so a line
// printed before the caller starts waiting is still visible.
function follow(child, log) {
  let text = "";
  const waiters = [];
  const onData = (chunk) => {
    log.write(chunk);
    text += chunk.toString();
    for (const waiter of waiters) waiter(text);
  };
  child.stdout?.on("data", onData);
  child.stderr?.on("data", onData);
  return {
    until(pattern, timeout) {
      const hit = (body) => {
        pattern.lastIndex = 0;
        return pattern.test(body);
      };
      if (hit(text)) return Promise.resolve(text);
      return new Promise((resolve, reject) => {
        const timer = setTimeout(() => reject(new Error(`fake tuner did not start\n${text}`)), timeout);
        let settled = false;
        const check = (body) => {
          if (settled || !hit(body)) return;
          settled = true;
          clearTimeout(timer);
          resolve(body);
        };
        waiters.push(check);
        check(text);
        child.on("exit", () => {
          if (settled) return;
          settled = true;
          clearTimeout(timer);
          reject(new Error(`fake tuner exited\n${text}`));
        });
      });
    },
  };
}

function stopAll(children) {
  for (const child of children) {
    if (child.exitCode == null && child.signalCode == null) child.kill("SIGTERM");
  }
  setTimeout(() => {
    for (const child of children) {
      if (child.exitCode == null && child.signalCode == null) child.kill("SIGKILL");
    }
  }, 3_000).unref();
}

async function waitHealth(base, server) {
  const deadline = Date.now() + 60_000;
  let last = "no answer";
  while (Date.now() < deadline) {
    if (server.exitCode != null) throw new Error(`server exited (${last})`);
    try {
      const health = await fetch(new URL("/api/v1/health", base), { signal: AbortSignal.timeout(1_500) });
      if (health.ok) {
        const lineup = await fetch(new URL("/api/v1/channels", base), { signal: AbortSignal.timeout(1_500) });
        if (lineup.ok) {
          const body = await lineup.json();
          const channels = body.channels || [];
          if (channels.length >= 3) return channels;
          last = `${channels.length} channels`;
        }
      }
    } catch (err) {
      last = String(err?.cause?.code || err?.message || err);
    }
    await sleep(300);
  }
  throw new Error(`server did not list channels (${last})`);
}

// prepareServer marks setup done and skips the update check, in the
// throwaway catalog this run started.
export async function prepareServer(base) {
  const res = await fetch(new URL("/api/v1/settings", base), {
    method: "PUT",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ setupComplete: "1", checkUpdates: "0" }),
    signal: AbortSignal.timeout(8_000),
  });
  if (!res.ok) throw new Error(`settings ${res.status}`);
  const channels = await fetch(new URL("/api/v1/channels", base), { signal: AbortSignal.timeout(8_000) });
  const body = await channels.json();
  return (body.channels || []).filter((channel) => channel.enabled !== false && !channel.hidden);
}
