import assert from "node:assert/strict";
import { test } from "node:test";
import { emptyGuideLabel, floorHalfHour, spanLabel, timeLabel } from "./src/lib/guide.ts";
import type { Airing } from "./src/types.ts";

const HALF_MS = 30 * 60_000;

function listing(start: string, end: string, title = "Night Desk", id = 1): Airing {
  return { id, channelId: 4, title, start, end } as Airing;
}

function weekday(end: number): string {
  return new Date(end).toLocaleDateString([], { weekday: "long" });
}

test("no listings, until the window is within the last minute of the last listing", () => {
  assert.equal(emptyGuideLabel([], 0), "No listings");
  assert.equal(emptyGuideLabel([], Date.parse("2026-10-14T02:00:00Z")), "No listings");

  const endIso = "2026-10-14T02:00:00Z";
  const end = Date.parse(endIso);
  const show = listing("2026-10-14T00:30:00Z", endIso);
  const through = `Listings through ${weekday(end)}`;

  assert.equal(emptyGuideLabel([show], end - 60_000 - 1), "No listings");
  assert.equal(emptyGuideLabel([show], end - 60_000), through);
  assert.equal(emptyGuideLabel([show], end), through);
  assert.equal(emptyGuideLabel([show], end + 3_600_000), through);
});

test("the empty label follows the last airing, not an earlier end", () => {
  const lastEndIso = "2026-10-14T02:00:00Z";
  const lastEnd = Date.parse(lastEndIso);
  const throughLast = `Listings through ${weekday(lastEnd)}`;
  const later = listing("2026-10-12T00:00:00Z", "2026-10-16T05:00:00Z", "Morning Show", 1);
  const last = listing("2026-10-13T22:00:00Z", lastEndIso, "City News", 2);

  assert.equal(emptyGuideLabel([later, last], lastEnd - 60_000), throughLast);
  assert.notEqual(throughLast, `Listings through ${weekday(Date.parse(later.end))}`);
  assert.equal(emptyGuideLabel([later, last], lastEnd - 60_000 - 1), "No listings");

  const early = listing("2026-10-13T00:00:00Z", "2026-10-13T01:00:00Z", "Morning Show", 1);
  const night = listing("2026-10-13T01:00:00Z", lastEndIso, "Night Desk", 2);
  const stillOpen = lastEnd - 60_000 - 1;
  assert.equal(stillOpen < Date.parse(early.end) - 60_000, false);
  assert.equal(emptyGuideLabel([early, night], stillOpen), "No listings");
});

test("half hours floor on UTC, five hours behind, and twenty minutes ahead", () => {
  const hour = Date.parse("2026-10-14T02:00:00Z");
  const half = Date.parse("2026-10-14T02:30:00Z");
  assert.equal(floorHalfHour(hour, 0), hour);
  assert.equal(floorHalfHour(hour - 1, 0), hour - HALF_MS);
  assert.equal(floorHalfHour(half, 0), half);
  assert.equal(floorHalfHour(half - 1, 0), hour);
  assert.equal(floorHalfHour(Date.parse("2026-10-14T02:17:05.250Z"), 0), hour);
  assert.equal(floorHalfHour(Date.parse("2026-10-14T02:45:00.000Z"), 0), half);

  // Positive minutes are behind UTC. Local 02:00 is 07:00Z.
  const behind = 300;
  const localHour = Date.parse("2026-10-14T02:00:00Z");
  const atLocal = localHour + behind * 60_000;
  assert.equal(atLocal, Date.parse("2026-10-14T07:00:00Z"));
  assert.equal(floorHalfHour(atLocal, behind), atLocal);
  assert.equal(floorHalfHour(atLocal - 1, behind), Date.parse("2026-10-14T06:30:00Z"));
  assert.equal(floorHalfHour(Date.parse("2026-10-14T02:17:00Z") + behind * 60_000, behind), atLocal);

  // Negative minutes are ahead of UTC. 01:40Z is 02:00 on that clock.
  const ahead = -20;
  const aheadOn = localHour + ahead * 60_000;
  assert.equal(aheadOn, Date.parse("2026-10-14T01:40:00Z"));
  assert.equal(floorHalfHour(aheadOn, ahead), aheadOn);
  assert.equal(floorHalfHour(aheadOn - 1, ahead), Date.parse("2026-10-14T01:10:00Z"));
  assert.equal(floorHalfHour(Date.parse("2026-10-14T01:50:00Z"), ahead), aheadOn);
  assert.equal(floorHalfHour(Date.parse("2026-10-14T01:50:00Z"), 0), Date.parse("2026-10-14T01:30:00Z"));
});

test("floorHalfHour defaults to the timestamp's timezone offset", () => {
  const t = Date.parse("2026-10-14T02:17:00Z");
  assert.equal(floorHalfHour(t), floorHalfHour(t, new Date(t).getTimezoneOffset()));
});

test("time labels match the locale clock, and a span uses an en dash", () => {
  const iso = "2026-10-14T02:07:00Z";
  const ms = Date.parse(iso);
  const clock = (value: string | number) => new Date(value).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
  assert.equal(timeLabel(iso), clock(iso));
  assert.equal(timeLabel(ms), clock(ms));

  const start = "2026-10-14T02:00:00Z";
  const end = "2026-10-14T03:30:00Z";
  const label = spanLabel(listing(start, end));
  assert.equal(label, `${timeLabel(start)} \u2013 ${timeLabel(end)}`);
  assert.notEqual(label, `${timeLabel(start)} - ${timeLabel(end)}`);
});
