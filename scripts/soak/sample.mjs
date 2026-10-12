import { execFile } from "node:child_process";
import { promisify } from "node:util";

const exec = promisify(execFile);

export async function sampleRuntime(base, gc) {
  const url = new URL("/api/v1/runtime", base);
  if (gc) url.searchParams.set("gc", "1");
  const res = await fetch(url, { signal: AbortSignal.timeout(8_000), cache: "no-store" });
  if (!res.ok) throw new Error(`runtime ${res.status}`);
  return res.json();
}

export async function sampleMetrics(base) {
  const res = await fetch(new URL("/api/v1/metrics", base), { signal: AbortSignal.timeout(4_000), cache: "no-store" });
  if (!res.ok) throw new Error(`metrics ${res.status}`);
  const text = await res.text();
  return {
    ffmpeg: metric(text, "broadwave_ffmpeg_processes"),
    viewers: metric(text, "broadwave_viewers"),
    feeds: metric(text, "broadwave_feeds"),
  };
}

function metric(text, name) {
  const found = text.match(new RegExp(`^${name} (\\d+)$`, "m"));
  return found ? Number(found[1]) : 0;
}

// sampleOS reads the server process and its ffmpeg children. rss is bytes.
export async function sampleOS(pid) {
  if (!pid) return { rss: null, ffmpeg: null, zombies: null };
  const { stdout } = await exec("ps", ["-ax", "-o", "pid=,ppid=,rss=,stat=,comm="], { timeout: 8_000, maxBuffer: 4 * 1024 * 1024 });
  return parseProcessTable(stdout, pid);
}

// parseProcessTable counts ffmpeg children and unreaped children.
// A macOS zombie is named <defunct>, so the name alone would hide it.
export function parseProcessTable(text, pid) {
  let rss = null;
  let ffmpeg = 0;
  let zombies = 0;
  for (const line of String(text).split("\n")) {
    const match = line.trim().match(/^(\d+)\s+(\d+)\s+(\d+)\s+(\S+)\s+(.*)$/);
    if (!match) continue;
    const id = Number(match[1]);
    const parent = Number(match[2]);
    const zombie = match[4].startsWith("Z");
    if (id === pid) rss = Number(match[3]) * 1024;
    if (parent !== pid) continue;
    if (zombie) zombies += 1;
    if (/ffmpeg/i.test(match[5])) ffmpeg += 1;
  }
  return { rss, ffmpeg, zombies };
}
