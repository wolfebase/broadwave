import assert from "node:assert/strict";
import { test } from "node:test";
import { activityRows, dayOf, isHouseNews, notRecording } from "./src/features/schedule/view.ts";

test("activity leaves out device finds until asked", () => {
  const events = [
    { id: 1, kind: "home", message: "New iPhone found: Kitchen." },
    { id: 2, kind: "recording", message: "Saved Cupertino on 5.1" },
    { id: 3, kind: "guide", message: "Guide updated, 4151 airings" },
    { id: 4, kind: "home", message: "New AirPlay device found: Den." },
  ];
  assert.equal(isHouseNews(events[0]), true);
  assert.equal(isHouseNews(events[1]), false);
  assert.deepEqual(activityRows(events, false).rows.map((e) => e.id), [2, 3]);
  assert.equal(activityRows(events, false).hidden, 2);
  assert.deepEqual(activityRows(events, true), { rows: events, hidden: 0 });
});

test("days are named next to today on the local clock", () => {
  const now = new Date(2026, 9, 15, 23, 30);
  assert.equal(dayOf(new Date(2026, 9, 15, 0, 5), now), "today");
  assert.equal(dayOf(new Date(2026, 9, 16, 0, 5), now), "tomorrow");
  assert.equal(dayOf(new Date(2026, 9, 14, 23, 59), now), "yesterday");
  assert.equal(dayOf(new Date(2026, 9, 18, 12, 0), now), "other");
  assert.equal(dayOf(new Date(2026, 10, 1, 12, 0), new Date(2026, 9, 31, 22, 0)), "tomorrow");
});

test("a planned item already recording is listed once, as the recording", () => {
  const item = (passId: number, channelId: number, start: string, end: string) => ({ passId, airing: { channelId, start, end } });
  const items = [
    item(1, 3, "2026-10-21T19:20:00Z", "2026-10-21T19:22:00Z"),
    item(1, 3, "2026-10-22T19:20:00Z", "2026-10-22T19:22:00Z"),
    item(2, 3, "2026-10-21T19:20:00Z", "2026-10-21T19:22:00Z"),
  ];
  const active = [{ passId: 1, channelId: 3, startedAt: "2026-10-21T19:18:31Z", endsAt: "2026-10-21T19:22:31Z" }];
  assert.deepEqual(notRecording(items, active), [items[1], items[2]]);
  assert.deepEqual(notRecording(items, []), items);
});
