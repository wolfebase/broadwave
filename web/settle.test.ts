import assert from "node:assert/strict";
import { test } from "node:test";
import { newSettle, SETTLE_MS, settleDue } from "./src/lib/settle.ts";

test("a drift inside the band never corrects, and a long stay clears misses", () => {
  const s = newSettle();
  s.misses = 2;
  assert.equal(settleDue(s, SETTLE_MS, 0), false);
  assert.equal(settleDue(s, 0, 60_000), false);
  assert.equal(s.misses, 0);
  assert.equal(settleDue(s, Number.NaN, 1), false);
});

test("a screen off the band waits, then backs off after each miss", () => {
  const s = newSettle();
  assert.equal(settleDue(s, 80, 0), false);
  assert.equal(settleDue(s, 80, 9_999), false);
  assert.equal(settleDue(s, 80, 10_000), true);
  assert.equal(s.misses, 1);
  // The off clock starts over, and the next gap is 30 seconds from the last correction.
  assert.equal(settleDue(s, 80, 10_000), false);
  assert.equal(settleDue(s, 80, 20_000), false);
  assert.equal(settleDue(s, 80, 40_000), true);
  assert.equal(s.misses, 2);
});
