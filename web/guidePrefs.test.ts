import assert from "node:assert/strict";
import { test } from "node:test";
import { dayJumpTarget, parseGuideFilter } from "./src/features/guide/prefs.ts";

const HOUR = 3_600_000;

function at(day: number, hours: number, minutes = 0): number {
  return new Date(2026, 9, day, hours, minutes).getTime();
}

test("a saved guide filter comes back, anything unknown is All", () => {
  assert.equal(parseGuideFilter("favorites"), "favorites");
  assert.equal(parseGuideFilter("news"), "news");
  assert.equal(parseGuideFilter("recording"), "recording");
  assert.equal(parseGuideFilter(null), "all");
  assert.equal(parseGuideFilter("weather"), "all");
});

test("a day with listings into the night lands half an hour before 8 PM", () => {
  const midnight = at(21, 0);
  const ends = [at(23, 6), at(23, 6), at(22, 23)];
  assert.equal(dayJumpTarget(midnight, ends, 3 * HOUR), at(21, 19, 30));
});

test("a short guide stops where most channels still have listings", () => {
  const midnight = at(21, 0);
  // Most channels end at 6:30 PM; one runs late.
  const ends = [at(21, 18, 30), at(21, 18, 30), at(21, 18, 45), at(22, 2)];
  assert.equal(dayJumpTarget(midnight, ends, 3 * HOUR), at(21, 15, 30));
});

test("the jump never leaves the day", () => {
  const midnight = at(21, 0);
  assert.equal(dayJumpTarget(midnight, [at(21, 1), at(21, 1)], 3 * HOUR), midnight);
  // Nothing listed that day at all: the empty evening is the truthful answer.
  assert.equal(dayJumpTarget(midnight, [at(20, 22)], 3 * HOUR), at(21, 19, 30));
  assert.equal(dayJumpTarget(midnight, [], 3 * HOUR), at(21, 19, 30));
});
