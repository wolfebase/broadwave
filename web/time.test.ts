import assert from "node:assert/strict";
import { test } from "node:test";
import { addMinutes, halfHourFloor, lastSeenPhrase } from "./src/time.ts";

test("half hours floor to the slot and minutes add on the clock", () => {
  const at = new Date(2026, 9, 11, 15, 44, 20);
  const floor = halfHourFloor(at);
  assert.equal(floor.getMinutes(), 30);
  assert.equal(floor.getSeconds(), 0);
  const early = halfHourFloor(new Date(2026, 9, 11, 15, 10, 0));
  assert.equal(early.getMinutes(), 0);
  const later = addMinutes(at, 90);
  assert.equal(later.getHours(), 17);
  assert.equal(later.getMinutes(), 14);
});

test("last seen is empty for a bad stamp and names the span", () => {
  const now = Date.parse("2026-10-11T15:00:00Z");
  assert.equal(lastSeenPhrase(undefined, now), "");
  assert.equal(lastSeenPhrase("not-a-time", now), "");
  assert.equal(lastSeenPhrase("2026-10-11T14:59:40Z", now), "just now");
  assert.equal(lastSeenPhrase("2026-10-11T14:59:00Z", now), "1 minute ago");
  assert.equal(lastSeenPhrase("2026-10-11T14:00:00Z", now), "1 hour ago");
  assert.equal(lastSeenPhrase("2026-10-09T15:00:00Z", now), "2 days ago");
});
