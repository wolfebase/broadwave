import assert from "node:assert/strict";
import { test } from "node:test";
import { holdQuietStart, pageFitsTiles } from "./src/features/player/quietStart.ts";

test("a quiet tile waits for the sound tile until that watch has answered", () => {
  assert.equal(holdQuietStart(true, false, false, true), true);
  assert.equal(holdQuietStart(false, false, true, true), false);
  assert.equal(holdQuietStart(true, false, true, false), true);
  assert.equal(holdQuietStart(false, false, false, false), false);
});

test("a kept page starts quiet tiles together only when every tile fits", () => {
  assert.equal(holdQuietStart(true, true, true, true), false);
  assert.equal(holdQuietStart(false, true, true, true), false);
  assert.equal(holdQuietStart(true, true, false, true), true);
  assert.equal(holdQuietStart(false, true, false, true), true);
  assert.equal(holdQuietStart(true, true, false, false), false);
});

test("the picture budget or a full set of answers covers the tiles", () => {
  assert.equal(pageFitsTiles(4, 3, 1), true);
  assert.equal(pageFitsTiles(2, 3, 3), true);
  assert.equal(pageFitsTiles(2, 3, 2), false);
  assert.equal(pageFitsTiles(null, 3, 3), true);
  assert.equal(pageFitsTiles(null, 3, 2), false);
  assert.equal(pageFitsTiles(2, 2, 0), true);
  assert.equal(pageFitsTiles(null, 1, 0), true);
});
