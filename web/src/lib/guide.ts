import { guideMessages } from "../i18n/messages/guide.ts";
import { bind, formatDateTime, formatTime } from "../i18n/runtime.ts";
import type { Airing, Channel, PlannedAiring, Recording } from "../types";
import { primeHour } from "./primeHour.ts";

const t = bind(guideMessages);

export type Category = "sports" | "news" | "movies" | "kids" | "series" | "other";

const sportsWords =
  /\b(sports?|football|basketball|baseball|hockey|soccer|golf|tennis|racing|nascar|motorsports?|boxing|mma|ufc|wrestling|olympics?|nfl|nba|mlb|nhl|mls|wnba|ncaa|college (football|basketball)|bowl|playoffs?|pregame|postgame|game day)\b/i;
const vs = /\b(vs\.?|at|@)\b/;
// A sitcom or a movie about a sport carries the sport in its category. It is not sports.
const fiction = /\b(comedy|drama|sitcom|soap|animated|animation|cartoon|movie|film|thriller|romance)\b/i;

/** A story about a sport, not a sport: a sitcom, a drama, a movie. A scoreboard match always counts as sports. */
export function isFiction(a: { category?: string; gameId?: string }): boolean {
  return !a.gameId && fiction.test(a.category ?? "");
}

export function categoryOf(airing: Pick<Airing, "category" | "title" | "gameId"> | undefined): Category {
  if (!airing) return "other";
  const c = `${airing.category ?? ""}`;
  if (!isFiction(airing) && (sportsWords.test(c) || (sportsWords.test(airing.title) && vs.test(airing.title)))) return "sports";
  if (/news/i.test(c) || /\bnews\b/i.test(airing.title)) return "news";
  if (/movie|film/i.test(c)) return "movies";
  if (/child|kids|animat|family|educational/i.test(c)) return "kids";
  if (c.trim() !== "") return "series";
  return "other";
}

export const categoryLabel: Record<Category, string> = {
  get sports() {
    return t("guide.category.sports");
  },
  get news() {
    return t("guide.category.news");
  },
  get movies() {
    return t("guide.category.movies");
  },
  get kids() {
    return t("guide.category.kids");
  },
  get series() {
    return t("guide.category.series");
  },
  get other() {
    return t("guide.category.other");
  },
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

/** Where a channel sits in the guide. A shown 3.0 channel uses its 1.0 number,
 * so 104.1 stays next to the 4s. A number the viewer set is kept. A 3.0
 * channel with no pair keeps its own number. `lineup` supplies a hidden twin. */
export function channelOrderNumber(channel: Channel, lineup: readonly Channel[] = []): string {
  const paired = channel.standard === "atsc3" && !channel.hidden && channel.twinId != null && channel.displayNumber === channel.guideNumber;
  if (!paired) return channel.displayNumber;
  const twin = lineup.find((row) => row.id === channel.twinId && row.standard !== "atsc3");
  if (twin?.displayNumber) return twin.displayNumber;
  const dot = channel.displayNumber.indexOf(".");
  const majorText = dot === -1 ? channel.displayNumber : channel.displayNumber.slice(0, dot);
  const major = Number(majorText);
  if (!Number.isFinite(major) || major < 100) return channel.displayNumber;
  const minor = dot === -1 ? "" : channel.displayNumber.slice(dot);
  return `${major - 100}${minor}`;
}

export function sortChannels(channels: Channel[], lineup: readonly Channel[] = channels): Channel[] {
  const num = (channel: Channel) => channelOrderNumber(channel, lineup).split(".").map((p) => Number(p) || 0);
  return [...channels].sort((a, b) => {
    const x = num(a);
    const y = num(b);
    return (
      (x[0] ?? 0) - (y[0] ?? 0) ||
      (x[1] ?? 0) - (y[1] ?? 0) ||
      a.displayNumber.localeCompare(b.displayNumber, undefined, { numeric: true }) ||
      a.displayName.localeCompare(b.displayName)
    );
  });
}

/**
 * When prime time starts on the viewer's clock (8 PM, or 7 PM in the Central
 * and Mountain zones), rolling to the next day once that hour has passed.
 */
export function primeTime(now: number, hour: number = primeHour(now)): number {
  const at = new Date(now);
  at.setHours(hour, 0, 0, 0);
  if (at.getTime() < now) at.setDate(at.getDate() + 1);
  return at.getTime();
}

export function timeLabel(iso: string | number): string {
  return formatTime(new Date(iso));
}

export function spanLabel(a: Airing): string {
  return `${timeLabel(a.start)} – ${timeLabel(a.end)}`;
}

export function minutesLeft(a: Airing, now: number): string {
  const m = Math.max(0, Math.round((Date.parse(a.end) - now) / 60000));
  if (m >= 60) return t("guide.leftHours", { hours: Math.floor(m / 60), minutes: m % 60 });
  return t("guide.leftMinutes", { minutes: m });
}

const sourceLine: Record<string, "guide.source.silicondust" | "guide.source.schedulesDirect" | "guide.source.xmltv" | "guide.source.playlist" | "guide.source.broadcast"> = {
  silicondust: "guide.source.silicondust",
  "schedules-direct": "guide.source.schedulesDirect",
  xmltv: "guide.source.xmltv",
  playlist: "guide.source.playlist",
  broadcast: "guide.source.broadcast",
};

export function guideSourceLine(source?: string): string {
  const id = source ? sourceLine[source] : undefined;
  return id ? t(id) : "";
}

// emptyGuideLabel is what a row says when the time on screen has no program.
// After the last listing, it names the day the data runs out.
/** Id of the guide cell the keyboard is on. The grid points at it with aria-activedescendant. */
export function guideCellId(channelId: number, airingId?: number): string {
  return airingId ? `guide-cell-${airingId}` : `guide-empty-${channelId}`;
}

/** What a guide cell says. The label replaces the cell's contents, including the recording dot and the score. */
export function guideCellName(
  title: string,
  span: string,
  channel: string,
  opts?: { recording?: "recording" | "scheduled" | null; fresh?: boolean; score?: string },
): string {
  const parts = [title];
  if (opts?.recording === "recording") parts.push(t("guide.recording"));
  else if (opts?.recording === "scheduled") parts.push(t("guide.willRecord"));
  if (opts?.fresh) parts.push(t("guide.new"));
  if (opts?.score) parts.push(opts.score);
  parts.push(span, channel);
  return parts.join(", ");
}

/** What the channel column says. The star is hidden from the name, so a favorite has to be in the label. */
export function channelWatchName(number: string, name: string, opts?: { atsc3?: boolean; favorite?: boolean }): string {
  let label = t("guide.watchChannel", { number, name });
  if (opts?.atsc3) label += t("guide.atsc3Suffix");
  if (opts?.favorite) label += t("guide.favoriteSuffix");
  return label;
}

/** What the Recordings tab adds when something is taping. One stays singular. */
export function recordingCountLabel(count: number): string {
  return t("guide.recordings", count);
}

export function emptyGuideLabel(airings: Airing[], windowStart: number): string {
  if (airings.length === 0) return t("guide.noListings");
  const end = Date.parse(airings[airings.length - 1].end);
  if (windowStart < end - 60_000) return t("guide.noListings");
  const day = formatDateTime(new Date(end), { weekday: "long" });
  return t("guide.listingsThrough", { day });
}

export function dayLabel(iso: string, now: number): string {
  const d = new Date(iso);
  const today = new Date(now);
  const startOf = (x: Date) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
  const diff = Math.round((startOf(d) - startOf(today)) / 86_400_000);
  if (diff === 0) return t("guide.today");
  if (diff === 1) return t("guide.tomorrow");
  return formatDateTime(d, { weekday: "long", month: "short", day: "numeric" });
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

const HALF_MS = 30 * 60_000;

/** Local half-hour at or before t. offsetMinutes matches Date#getTimezoneOffset. */
export function floorHalfHour(t: number, offsetMinutes = new Date(t).getTimezoneOffset()): number {
  const local = t - offsetMinutes * 60_000;
  return Math.floor(local / HALF_MS) * HALF_MS + offsetMinutes * 60_000;
}

/** Left edge of the guide: one local half-hour before the floor. A UTC bucket is not this. */
export function guideOrigin(t: number, offsetMinutes = new Date(t).getTimezoneOffset()): number {
  return floorHalfHour(t, offsetMinutes) - HALF_MS;
}

/**
 * scrollTop that puts the row fully under the sticky header.
 * The row does not have to be mounted. Returns scrollTop when it already is.
 */
export function guideRowScroll(scrollTop: number, row: number, rowH: number, clientHeight: number, headH: number): number {
  const y = Math.max(0, row) * rowH;
  if (y < scrollTop) return y;
  if (y + rowH > scrollTop + clientHeight - headH) return y + rowH - clientHeight + headH;
  return scrollTop;
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
