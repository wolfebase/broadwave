import { spawnSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));

type Channel = { id: number; number: string; name: string };

function stamp(ms: number) {
  return new Date(ms).toISOString().replace(/\.\d{3}Z$/, "Z");
}

function sqlQuote(value: string) {
  return `'${value.replaceAll("'", "''")}'`;
}

async function channels(base: string): Promise<Channel[]> {
  const res = await fetch(`${base}/api/v1/channels`);
  if (!res.ok) throw new Error(`channels ${res.status}`);
  const body = (await res.json()) as { channels?: { id: number; displayNumber: string; displayName: string }[] };
  return (body.channels ?? []).map((channel) => ({
    id: channel.id,
    number: channel.displayNumber,
    name: channel.displayName,
  }));
}

function quiet(db: string) {
  const sql = `
PRAGMA busy_timeout=5000;
INSERT INTO settings(key, value) VALUES('checkUpdates', '0')
  ON CONFLICT(key) DO UPDATE SET value='0';
INSERT INTO settings(key, value) VALUES('nextGuidePull', '2099-01-01T00:00:00Z')
  ON CONFLICT(key) DO UPDATE SET value='2099-01-01T00:00:00Z';
`;
  const result = spawnSync("sqlite3", [db, sql], { encoding: "utf8" });
  if (result.status !== 0) throw new Error(result.stderr || result.stdout || "could not quiet the catalog");
}

export default async function globalSetup() {
  const server = JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as { base: string; db: string };
  quiet(server.db);
  const playlist = process.env.E2E_PLAYLIST === "1";
  const need = playlist ? 1 : 3;
  let list: Channel[] = [];
  for (let i = 0; i < 50; i++) {
    try {
      list = await channels(server.base);
      if (list.length >= need) break;
    } catch {
      // Discovery is still writing the lineup.
    }
    await new Promise((resolve) => setTimeout(resolve, 300));
  }
  if (list.length < need) throw new Error(`${playlist ? "playlist" : "fake"} lineup has ${list.length} channels`);

  if (playlist) {
    const half = 30 * 60_000;
    const wall = Date.now();
    const now = Math.floor(wall / half) * half + 15 * 60_000;
    writeFileSync(path.join(here, ".run/runtime.json"), JSON.stringify({ base: server.base, now, channels: list }, null, 2));
    const ch = list[0];
    const sql = `
PRAGMA busy_timeout=5000;
DELETE FROM airings;
INSERT INTO airings (channel_id, title, subtitle, description, category, starts_at, ends_at, program_id, is_live, guide_source)
VALUES (${ch.id}, 'Evening News', 'Local headlines', 'The evening newscast.', 'News', ${sqlQuote(stamp(now - 15 * 60_000))}, ${sqlQuote(stamp(now + 45 * 60_000))}, 'e2e-playlist', 0, 'e2e');
`;
    const inserted = spawnSync("sqlite3", [server.db, sql], { encoding: "utf8" });
    if (inserted.status !== 0) throw new Error(inserted.stderr || inserted.stdout || "could not seed listings");
    return;
  }

  const byName = new Map(list.map((channel) => [channel.name, channel]));
  const byNumber = new Map(list.map((channel) => [channel.number, channel]));
  const kbwv = byNumber.get("4.1") ?? byName.get("KBWV");
  const kbwv2 = byName.get("KBWV2");
  const wtst = byNumber.get("5.1") ?? byName.get("WTST");
  if (!kbwv || !kbwv2 || !wtst) throw new Error(`unexpected lineup: ${list.map((c) => `${c.number} ${c.name}`).join(", ")}`);
  const encrypted = process.env.E2E_ATSC3 === "1" ? byNumber.get("115.1") : undefined;
  if (process.env.E2E_ATSC3 === "1" && !encrypted) throw new Error("115.1 WTST is not in the lineup");

  // Fifteen minutes into the current half hour. Program bars then sit on the
  // same pixels in every run; the clock labels are hidden in the snapshots.
  const half = 30 * 60_000;
  const wall = Date.now();
  const now = Math.floor(wall / half) * half + 15 * 60_000;
  writeFileSync(path.join(here, ".run/runtime.json"), JSON.stringify({ base: server.base, now, channels: list }, null, 2));
  const rows: [number, string, string, string, string, number, number][] = [
    [kbwv.id, "NFL: Bears at Bills", "Chicago at Buffalo", "Sunday football.", "Sports", now - 20 * 60_000, now + 100 * 60_000],
    [kbwv.id, "Late Local News", "The late newscast.", "The late newscast.", "News", now + 100 * 60_000, now + 160 * 60_000],
    // Ended an hour ago: search leaves it out.
    [kbwv2.id, "Morning News", "Early headlines", "The morning newscast.", "News", now - 2 * 60 * 60_000, now - 60 * 60_000],
    [kbwv2.id, "Evening News", "Local headlines", "The evening newscast.", "News", now - 15 * 60_000, now + 45 * 60_000],
    [wtst.id, "The Night Show", "A guest and a band", "Talk.", "Series", now - 5 * 60_000, now + 2 * 60 * 60_000],
    [wtst.id, "NBA: Lakers at Celtics", "Los Angeles at Boston", "Basketball.", "Sports", now + 3 * 60 * 60_000, now + 6 * 60 * 60_000],
  ];
  if (encrypted) {
    rows.push([encrypted.id, "Sealed Signal", "A locked hour", "The encrypted broadcast.", "Series", now - 10 * 60_000, now + 50 * 60_000]);
  }
  const values = rows
    .map(([id, title, subtitle, description, category, start, end], index) => {
      const live = start <= now ? 1 : 0;
      return `(${id}, ${sqlQuote(title)}, ${sqlQuote(subtitle)}, ${sqlQuote(description)}, ${sqlQuote(category)}, ${sqlQuote(stamp(start))}, ${sqlQuote(stamp(end))}, ${sqlQuote(`e2e-${index}`)}, ${live}, 'e2e')`;
    })
    .join(",\n");
  const sql = `
PRAGMA busy_timeout=5000;
DELETE FROM airings;
INSERT INTO airings (channel_id, title, subtitle, description, category, starts_at, ends_at, program_id, is_live, guide_source)
VALUES ${values};
`;
  const inserted = spawnSync("sqlite3", [server.db, sql], { encoding: "utf8" });
  if (inserted.status !== 0) {
    throw new Error(inserted.stderr || inserted.stdout || "could not seed listings");
  }
  if (process.env.E2E_TRACKS === "2") await storeTracks(server.base, kbwv.id);
}

// A first tune whose scan misses a track plays without it until the next tune;
// a slow runner does. A silent tile tunes the channel long enough for the server
// to store its tracks, so the tests' tune carries both sounds from its start.
async function storeTracks(base: string, id: number) {
  const caps = { platform: "web", video: ["h264"], audio: ["aac"] };
  const res = await fetch(`${base}/api/v1/watch`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ channelId: id, caps, prefs: { quality: "tile" } }),
  });
  if (!res.ok) throw new Error(`warm-up watch ${res.status} ${await res.text()}`);
  const session = (await res.json()) as { rendition: string; boot?: string };
  await new Promise((resolve) => setTimeout(resolve, 10_000));
  await fetch(`${base}/api/v1/watch/${id}/stop`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ rendition: session.rendition, boot: session.boot ?? "" }),
  });
  for (let i = 0; i < 60; i++) {
    const tuners = (await (await fetch(`${base}/api/v1/tuners`)).json()) as { tuners: { ours?: boolean }[] };
    if (!tuners.tuners.some((t) => t.ours)) return;
    await new Promise((resolve) => setTimeout(resolve, 1_000));
  }
  throw new Error("the warm-up tune did not end");
}
