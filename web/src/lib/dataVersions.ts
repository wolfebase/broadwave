/** The server's data counters, from the socket's hello. */
export type DataVersions = Partial<Record<"guide" | "channels" | "recordings" | "schedule" | "settings", number>>;

export type RefreshKey = "airings" | "channels" | "recordings" | "passes" | "settings";

/** What each data change event makes stale. */
export const changedEvents: Record<string, RefreshKey> = {
  "guide.changed": "airings",
  "channels.changed": "channels",
  "recordings.changed": "recordings",
  "schedule.changed": "passes",
  "settings.changed": "settings",
};

const byKind: Record<keyof DataVersions, RefreshKey> = {
  guide: "airings",
  channels: "channels",
  recordings: "recordings",
  schedule: "passes",
  settings: "settings",
};

/** Read a hello's versions. A server that predates them sends none. */
export function readVersions(hello: unknown): DataVersions | null {
  const v = (hello as { versions?: unknown } | null)?.versions;
  if (!v || typeof v !== "object") return null;
  const out: DataVersions = {};
  for (const kind of Object.keys(byKind) as (keyof DataVersions)[]) {
    const n = (v as Record<string, unknown>)[kind];
    if (typeof n === "number") out[kind] = n;
  }
  return out;
}

/** After a reconnect, only what moved while the socket was away is read again. */
export function movedSince(prev: DataVersions, next: DataVersions): RefreshKey[] {
  const out: RefreshKey[] = [];
  for (const kind of Object.keys(byKind) as (keyof DataVersions)[]) {
    if (prev[kind] !== next[kind]) out.push(byKind[kind]);
  }
  return out;
}
