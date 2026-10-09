import type { Airing, Channel, PlannedAiring, Recording } from "../types";

export type Category = "sports" | "news" | "movies" | "kids" | "series" | "other";

const sportsWords =
  /\b(sports?|football|basketball|baseball|hockey|soccer|golf|tennis|racing|nascar|motorsports?|boxing|mma|ufc|wrestling|olympics?|nfl|nba|mlb|nhl|mls|wnba|ncaa|college (football|basketball)|bowl|playoffs?|pregame|postgame|game day)\b/i;
const vs = /\b(vs\.?|at|@)\b/;

export function categoryOf(airing: Pick<Airing, "category" | "title"> | undefined): Category {
  if (!airing) return "other";
  const c = `${airing.category ?? ""}`;
  if (sportsWords.test(c) || (sportsWords.test(airing.title) && vs.test(airing.title))) return "sports";
  if (/news/i.test(c) || /\bnews\b/i.test(airing.title)) return "news";
  if (/movie|film/i.test(c)) return "movies";
  if (/child|kids|animat|family|educational/i.test(c)) return "kids";
  if (c.trim() !== "") return "series";
  return "other";
}

export const categoryLabel: Record<Category, string> = {
  sports: "Sports",
  news: "News",
  movies: "Movies",
  kids: "Kids",
  series: "Shows",
  other: "Other",
};

export type AiringIndex = Map<number, Airing[]>;

export function indexAirings(airings: Airing[]): AiringIndex {
  const map: AiringIndex = new Map();
  for (const a of airings) {
    let list = map.get(a.channelId);
    if (!list) map.set(a.channelId, (list = []));
    list.push(a);
  }
  for (const list of map.values()) list.sort((a, b) => a.start.localeCompare(b.start));
  return map;
}

export function airingAt(index: AiringIndex, channelId: number, at: number): Airing | undefined {
  const list = index.get(channelId);
  if (!list) return undefined;
  for (const a of list) {
    const s = Date.parse(a.start);
    if (s > at) return undefined;
    if (Date.parse(a.end) > at) return a;
  }
  return undefined;
}

export function nextAfter(index: AiringIndex, channelId: number, at: number): Airing | undefined {
  return index.get(channelId)?.find((a) => Date.parse(a.start) >= at);
}

export function progress(airing: Airing | undefined, now: number): number {
  if (!airing) return 0;
  const s = Date.parse(airing.start);
  const e = Date.parse(airing.end);
  return Math.min(1, Math.max(0, (now - s) / Math.max(1, e - s)));
}

/** Keys of airings that will record or are recording, for guide badges. */
export function recordingKeys(planned: PlannedAiring[], recordings: Recording[]): Set<string> {
  const keys = new Set<string>();
  for (const p of planned) if (!p.skipped && !p.conflict) keys.add(`${p.airing.channelId}@${p.airing.start}`);
  for (const r of recordings) if (r.status === "recording") keys.add(`live:${r.channelId}`);
  return keys;
}

export function isRecording(keys: Set<string>, airing: Airing, now: number): "recording" | "scheduled" | null {
  if (keys.has(`live:${airing.channelId}`) && Date.parse(airing.start) <= now && Date.parse(airing.end) > now) return "recording";
  if (keys.has(`${airing.channelId}@${airing.start}`)) return "scheduled";
  return null;
}

export function sortChannels(channels: Channel[]): Channel[] {
  const num = (v: string) => v.split(".").map((p) => Number(p) || 0);
  return [...channels].sort((a, b) => {
    const x = num(a.displayNumber);
    const y = num(b.displayNumber);
    return (x[0] ?? 0) - (y[0] ?? 0) || (x[1] ?? 0) - (y[1] ?? 0) || a.displayName.localeCompare(b.displayName);
  });
}

/** 8:00 PM on the viewer's clock, rolling to the next day once that hour has passed. */
export function primeTime(now: number): number {
  const at = new Date(now);
  at.setHours(20, 0, 0, 0);
  if (at.getTime() < now) at.setDate(at.getDate() + 1);
  return at.getTime();
}

export function timeLabel(iso: string | number): string {
  return new Date(iso).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
}

export function spanLabel(a: Airing): string {
  return `${timeLabel(a.start)} – ${timeLabel(a.end)}`;
}

export function minutesLeft(a: Airing, now: number): string {
  const m = Math.max(0, Math.round((Date.parse(a.end) - now) / 60000));
  if (m >= 60) return `${Math.floor(m / 60)}h ${m % 60}m left`;
  return `${m}m left`;
}

const sourceLine: Record<string, string> = {
  silicondust: "From the tuner guide.",
  "schedules-direct": "From Schedules Direct.",
  xmltv: "From your guide file.",
  playlist: "From the playlist.",
  broadcast: "From the broadcast.",
};

export function guideSourceLine(source?: string): string {
  return source ? sourceLine[source] ?? "" : "";
}

// emptyGuideLabel is what a row says when the time on screen has no program.
// After the last listing, it names the day the data runs out.
/** Id of the guide cell the keyboard is on. The grid points at it with aria-activedescendant. */
export function guideCellId(channelId: number, airingId?: number): string {
  return airingId ? `guide-cell-${airingId}` : `guide-empty-${channelId}`;
}

export function emptyGuideLabel(airings: Airing[], windowStart: number): string {
  if (airings.length === 0) return "No listings";
  const end = Date.parse(airings[airings.length - 1].end);
  if (windowStart < end - 60_000) return "No listings";
  const day = new Date(end).toLocaleDateString([], { weekday: "long" });
  return `Listings through ${day}`;
}

export function dayLabel(iso: string, now: number): string {
  const d = new Date(iso);
  const today = new Date(now);
  const startOf = (x: Date) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
  const diff = Math.round((startOf(d) - startOf(today)) / 86_400_000);
  if (diff === 0) return "Today";
  if (diff === 1) return "Tomorrow";
  return d.toLocaleDateString([], { weekday: "long", month: "short", day: "numeric" });
}

export type GuideFocus = { row: number; id: number };

// id 0 means the keyboard has not adopted a channel yet. A later render
// clamps the index and takes that row's id. Any other id is followed when
// the row list shrinks; a missing id keeps the clamped index and adopts it.
export function keptRow(rows: readonly { id: number }[], focus: GuideFocus): GuideFocus {
  if (rows.length === 0) return { row: 0, id: focus.id };
  const match = focus.id === 0 ? -1 : rows.findIndex((row) => row.id === focus.id);
  if (match >= 0) return { row: match, id: focus.id };
  const row = Math.max(0, Math.min(rows.length - 1, focus.row));
  return { row, id: rows[row].id };
}

/** Phone portrait is the on-now list. Phone landscape uses the same grid as desktop and tv. */
export function guideShowsGrid(layout: "desktop" | "tv" | "phone", landscape: boolean): boolean {
  return layout !== "phone" || landscape;
}

export type GuideBox = { top: number; left: number; height: number; width: number };

/** A measure after cleanup, or on a node React has already detached, must not write the viewport. */
export function guideViewBox(
  el: { isConnected: boolean; scrollTop: number; scrollLeft: number; clientHeight: number; clientWidth: number } | null,
  alive: boolean,
): GuideBox | null {
  if (!alive || !el?.isConnected) return null;
  return { top: el.scrollTop, left: el.scrollLeft, height: el.clientHeight, width: el.clientWidth };
}

/** The row from the latest list. The object the sheet opened with is only the fallback. */
export function liveChannel<T extends { id: number }>(channels: readonly T[], channel: T): T {
  return channels.find((row) => row.id === channel.id) ?? channel;
}

/**
 * Move one channel within the full order. A drag on a filtered guide must
 * not rebuild the order from the rows on screen.
 */
export function reorderChannels(orderedIds: readonly number[], fromId: number, toId: number): number[] | null {
  if (fromId === toId) return null;
  const ids = orderedIds.slice();
  const from = ids.indexOf(fromId);
  const to = ids.indexOf(toId);
  if (from < 0 || to < 0) return null;
  const [moved] = ids.splice(from, 1);
  ids.splice(to, 0, moved);
  return ids;
}

/** The near window paints first. The rest of the two weeks fills in after. */
export function guideSpan(now = Date.now()) {
  return {
    from: new Date(now - 30 * 60_000).toISOString(),
    to: new Date(now + 4 * 60 * 60_000).toISOString(),
    restTo: new Date(now + 14 * 24 * 60 * 60_000).toISOString(),
  };
}

/** A refresh keeps those two weeks. With no window the server returns two days. */
export function keptGuideWindow(now = Date.now()): { from: string; to: string } {
  const span = guideSpan(now);
  return { from: span.from, to: span.restTo };
}

export type SearchRows<A, R> = { airings: A[]; recordings: R[] };

/** Pending and failed searches show no rows. The previous list is not an argument, so it cannot be kept. */
export function searchRows<A, R>(
  next: { status: "pending" | "error" } | { status: "done"; airings: A[]; recordings: R[] },
): SearchRows<A, R> {
  if (next.status !== "done") return { airings: [], recordings: [] };
  return { airings: next.airings, recordings: next.recordings };
}
