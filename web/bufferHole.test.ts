import assert from "node:assert/strict";
import { test } from "node:test";
import { holeEnd, maxHoleS } from "./src/lib/bufferHole.ts";

test("a playhead just before a small hole continues at the next range", () => {
  const end = holeEnd(10, [
    [9, 10.1],
    [10.4, 12],
  ]);
  assert.equal(end, 10.4);
  assert.ok(10.4 - 10.1 <= maxHoleS);
});

test("a gap wider than the hole, or a playhead already past it, stays put", () => {
  assert.equal(
    holeEnd(10, [
      [9, 10],
      [11, 12],
    ]),
    null,
  );
  assert.equal(
    holeEnd(12.5, [
      [9, 10.1],
      [10.4, 12],
    ]),
    null,
  );
  assert.equal(holeEnd(10, [[9, 12]]), null);
});
