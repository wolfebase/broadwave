/** Device finds ("New iPhone found") and routine guide updates are the house's news, not the recorder's. */
export function isHouseNews(event: { kind: string }): boolean {
  return event.kind === "home" || event.kind === "guide";
}

/** The recorder's own lines, or everything. hidden counts what a viewer can bring back. */
export function activityRows<T extends { kind: string }>(events: T[], everything: boolean): { rows: T[]; hidden: number } {
  if (everything) return { rows: events, hidden: 0 };
  const rows = events.filter((event) => !isHouseNews(event));
  return { rows, hidden: events.length - rows.length };
}

export type Day = "today" | "tomorrow" | "yesterday" | "other";

/** Which calendar day a time falls on, next to now, on this device's clock. */
export function dayOf(date: Date, now: Date): Day {
  const start = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
  const days = Math.round((start(date) - start(now)) / 86_400_000);
  if (days === 0) return "today";
  if (days === 1) return "tomorrow";
  if (days === -1) return "yesterday";
  return "other";
}

/** A planned item whose recording is already running shows once, as the recording. */
export function notRecording<T extends { passId: number; airing: { channelId: number; start: string; end: string } }>(
  items: T[],
  active: { passId?: number; channelId: number; startedAt: string; endsAt?: string }[],
): T[] {
  const covers = (rec: (typeof active)[number], item: T) =>
    rec.passId === item.passId &&
    rec.channelId === item.airing.channelId &&
    Date.parse(rec.startedAt) < Date.parse(item.airing.end) &&
    (!rec.endsAt || Date.parse(rec.endsAt) > Date.parse(item.airing.start));
  return items.filter((item) => !active.some((rec) => covers(rec, item)));
}
