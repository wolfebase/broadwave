/** Search results, one row per show. */
export type ShowGroup<A> = {
  key: string;
  title: string;
  airings: A[];
  /** The airing on the air now, if any. */
  onNow?: A;
  /** The first airing still to start. */
  next?: A;
  art?: A;
  channels: number;
};

type Listing = { title: string; start: string; end: string; channelId: number; imageUrl?: string };

/** Group listings by show title. Shows on now come first, then the soonest. */
export function groupByShow<A extends Listing>(airings: A[], now: number, playable: (channelId: number) => boolean = () => true): ShowGroup<A>[] {
  const groups = new Map<string, ShowGroup<A>>();
  const sorted = [...airings].sort((a, b) => Date.parse(a.start) - Date.parse(b.start));
  for (const a of sorted) {
    if (Date.parse(a.end) <= now) continue;
    const key = a.title.trim().toLowerCase();
    let g = groups.get(key);
    if (!g) {
      g = { key, title: a.title.trim(), airings: [], channels: 0 };
      groups.set(key, g);
    }
    g.airings.push(a);
    const on = Date.parse(a.start) <= now;
    if (on && !g.onNow && playable(a.channelId)) g.onNow = a;
    if (!on && !g.next) g.next = a;
    if (!g.art && a.imageUrl) g.art = a;
  }
  for (const g of groups.values()) g.channels = new Set(g.airings.map((a) => a.channelId)).size;
  const first = (g: ShowGroup<A>) => Date.parse((g.onNow ?? g.airings[0]).start);
  return [...groups.values()].sort((a, b) => Number(Boolean(b.onNow)) - Number(Boolean(a.onNow)) || first(a) - first(b));
}
