/** Device finds ("New iPhone found") are the house's news, not the recorder's. */
export function isHouseNews(event: { kind: string }): boolean {
  return event.kind === "home";
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
