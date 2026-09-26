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
  let list: Channel[] = [];
  for (let i = 0; i < 50; i++) {
    try {
      list = await channels(server.base);
      if (list.length >= 3) break;
    } catch {
      // Discovery is still writing the lineup.
    }
    await new Promise((resolve) => setTimeout(resolve, 300));
  }
  if (list.length < 3) throw new Error(`fake lineup has ${list.length} channels`);

  const byName = new Map(list.map((channel) => [channel.name, channel]));
  const wdaf = byName.get("WDAF");
  const wdaf2 = byName.get("WDAF2");
  const kctv = byName.get("KCTV");
  if (!wdaf || !wdaf2 || !kctv) throw new Error(`unexpected lineup: ${list.map((c) => c.name).join(", ")}`);

  // Fifteen minutes into the current half hour. Program bars then sit on the
  // same pixels in every run; the clock labels are hidden in the snapshots.
  const half = 30 * 60_000;
  const wall = Date.now();
  const now = Math.floor(wall / half) * half + 15 * 60_000;
  writeFileSync(path.join(here, ".run/runtime.json"), JSON.stringify({ base: server.base, now, channels: list }, null, 2));
  const rows: [number, string, string, string, string, number, number][] = [
    [wdaf.id, "NFL: Chiefs at Bills", "Kansas City at Buffalo", "Sunday football.", "Sports", now - 20 * 60_000, now + 100 * 60_000],
    [wdaf.id, "Late Local News", "The late newscast.", "The late newscast.", "News", now + 100 * 60_000, now + 160 * 60_000],
    [wdaf2.id, "Evening News", "Local headlines", "The evening newscast.", "News", now - 15 * 60_000, now + 45 * 60_000],
    [kctv.id, "The Night Show", "A guest and a band", "Talk.", "Series", now - 5 * 60_000, now + 2 * 60 * 60_000],
    [kctv.id, "NBA: Lakers at Celtics", "Los Angeles at Boston", "Basketball.", "Sports", now + 3 * 60 * 60_000, now + 6 * 60 * 60_000],
  ];
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

}
