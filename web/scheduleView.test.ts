import assert from "node:assert/strict";
import { test } from "node:test";
import { activityRows, dayOf, isHouseNews } from "./src/features/schedule/view.ts";

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
