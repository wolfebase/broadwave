import assert from "node:assert/strict";
import { test } from "node:test";
import { airingAt, categoryLabel, categoryOf, dayLabel, guideSourceLine, indexAirings, isRecording, minutesLeft, nextAfter, progress, recordingKeys, sortChannels } from "./src/lib/guide.ts";
import type { Airing, Channel } from "./src/types.ts";

function air(partial: Partial<Airing> & Pick<Airing, "start" | "end">): Airing {
  return { id: 1, channelId: 4, title: "Night Show", ...partial };
}

test("a listing is sports, news, a movie, or a show from its category, then its title", () => {
  assert.equal(categoryOf(undefined), "other");
  assert.equal(categoryOf(air({ start: "a", end: "b", category: "   " })), "other");
  assert.equal(categoryOf(air({ start: "a", end: "b", category: "Sports" })), "sports");
  assert.equal(categoryOf(air({ start: "a", end: "b", category: "Sports News" })), "sports");
  assert.equal(categoryOf(air({ start: "a", end: "b", title: "Hawks at Lions" })), "other");
  assert.equal(categoryOf(air({ start: "a", end: "b", title: "NFL: Hawks at Lions" })), "sports");
  assert.equal(categoryOf(air({ start: "a", end: "b", title: "NFL Sunday" })), "other");
  assert.equal(categoryOf(air({ start: "a", end: "b", title: "News at 6" })), "news");
  assert.equal(categoryOf(air({ start: "a", end: "b", category: "Film" })), "movies");
  assert.equal(categoryOf(air({ start: "a", end: "b", category: "Family Movie" })), "movies");
  assert.equal(categoryOf(air({ start: "a", end: "b", category: "Animated" })), "kids");
  assert.equal(categoryOf(air({ start: "a", end: "b", category: "Drama" })), "series");
  assert.equal(categoryOf(air({ start: "a", end: "b", title: "Movie Night" })), "other");
  assert.equal(categoryLabel.series, "Shows");
});

test("the show on now ends at the next listing's start, and a gap is empty", () => {
  const first = air({ id: 1, start: "2026-10-11T23:00:00Z", end: "2026-10-12T00:00:00Z", title: "Early" });
  const next = air({ id: 2, start: "2026-10-12T00:00:00Z", end: "2026-10-12T01:00:00Z", title: "Later" });
  const index = indexAirings([next, first, air({ id: 3, channelId: 7, start: "2026-10-11T23:30:00Z", end: "2026-10-12T00:30:00Z" })]);
  const at = Date.parse("2026-10-11T23:30:00Z");
  const boundary = Date.parse("2026-10-12T00:00:00Z");
  assert.equal(airingAt(index, 4, at)?.id, 1);
  assert.equal(airingAt(index, 4, boundary)?.id, 2);
  assert.equal(airingAt(index, 4, Date.parse("2026-10-11T22:00:00Z")), undefined);
  assert.equal(airingAt(index, 9, at), undefined);
  assert.equal(nextAfter(index, 4, boundary)?.id, 2);
  assert.equal(nextAfter(index, 4, Date.parse("2026-10-12T00:30:00Z"))?.id, undefined);
  assert.deepEqual(
    index.get(4)?.map((item) => item.id),
    [1, 2],
  );
});

test("progress stays between the start and the end, including a listing with no length", () => {
  const show = air({ start: "2026-10-11T23:00:00Z", end: "2026-10-12T00:00:00Z" });
  const start = Date.parse(show.start);
  assert.equal(progress(undefined, start), 0);
  assert.equal(progress(show, start - 1000), 0);
  assert.equal(progress(show, start + 30 * 60_000), 0.5);
  assert.equal(progress(show, start + 2 * 60 * 60_000), 1);
  const dot = air({ start: "2026-10-11T23:00:00Z", end: "2026-10-11T23:00:00Z" });
  assert.equal(progress(dot, Date.parse(dot.start)), 0);
  assert.equal(progress(dot, Date.parse(dot.start) + 5000), 1);
});

test("a live recording wins over a scheduled pass, and a skipped pass leaves no badge", () => {
  const on = air({ id: 1, channelId: 4, start: "2026-10-11T23:00:00Z", end: "2026-10-12T00:00:00Z" });
  const later = air({ id: 2, channelId: 4, start: "2026-10-12T01:00:00Z", end: "2026-10-12T02:00:00Z" });
  const now = Date.parse("2026-10-11T23:30:00Z");
  const keys = recordingKeys(
    [
      { skipped: false, conflict: false, airing: on },
      { skipped: true, conflict: false, airing: later },
      { skipped: false, conflict: true, airing: later },
    ] as never,
    [{ status: "recording", channelId: 4 }, { status: "complete", channelId: 7 }] as never,
  );
  assert.equal(isRecording(keys, on, now), "recording");
  assert.equal(isRecording(keys, later, now), null);
  const scheduled = recordingKeys([{ skipped: false, conflict: false, airing: later }] as never, []);
  assert.equal(isRecording(scheduled, later, now), "scheduled");
  assert.equal(isRecording(scheduled, on, now), null);
});

test("channels sort by number, then by name", () => {
  const channel = (displayNumber: string, displayName: string): Channel => ({ displayNumber, displayName }) as Channel;
  const sorted = sortChannels([
    channel("10.2", "Zebra"),
    channel("2.1", "News 4"),
    channel("10.2", "Alpha"),
    channel("10.10", "Den TV"),
    channel("10", "City"),
  ]);
  assert.deepEqual(
    sorted.map((item) => `${item.displayNumber} ${item.displayName}`),
    ["2.1 News 4", "10 City", "10.2 Alpha", "10.2 Zebra", "10.10 Den TV"],
  );
});

test("minutes left, the guide source line, and today or tomorrow", () => {
  const end = Date.parse("2026-10-12T01:10:00Z");
  const show = air({ start: "2026-10-12T00:00:00Z", end: "2026-10-12T01:10:00Z" });
  assert.equal(minutesLeft(show, end), "0m left");
  assert.equal(minutesLeft(show, end - 90 * 60_000), "1h 30m left");
  assert.equal(minutesLeft(show, end - 5 * 60_000), "5m left");
  assert.equal(guideSourceLine(undefined), "");
  assert.equal(guideSourceLine("xmltv"), "From your guide file.");
  assert.equal(guideSourceLine("nope"), "");

  const morning = new Date(2026, 9, 11, 8, 0, 0);
  assert.equal(dayLabel(new Date(2026, 9, 11, 23, 0, 0).toISOString(), morning.getTime()), "Today");
  assert.equal(dayLabel(new Date(2026, 9, 12, 1, 0, 0).toISOString(), morning.getTime()), "Tomorrow");
  const later = dayLabel(new Date(2026, 9, 14, 12, 0, 0).toISOString(), morning.getTime());
  assert.equal(later === "Today" || later === "Tomorrow", false);
});
