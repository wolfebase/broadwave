import assert from "node:assert/strict";
import { test } from "node:test";
import { introSkip, takeUpNext, upNext } from "./src/features/recordings/ends.ts";
import type { Recording } from "./src/types.ts";

const rec = (extra: Partial<Recording> = {}): Recording => ({ id: 1, channelId: 1, guideNumber: "4.1", title: "Mystery Hour", status: "complete", startedAt: "2026-10-01T19:00:00Z", ...extra });

test("skip intro shows inside the intro and jumps to its end", () => {
  const r = rec({ introStart: 90, introEnd: 120 });
  assert.equal(introSkip(r, 89), null);
  assert.equal(introSkip(r, 90), 120);
  assert.equal(introSkip(r, 118.9), 120);
  // The last second plays on: a skip there would land as the button vanishes.
  assert.equal(introSkip(r, 119.5), null);
  assert.equal(introSkip(rec(), 100), null);
  assert.equal(introSkip(rec({ introStart: 90, introEnd: 90 }), 90), null);
});

test("up next counts down from the end titles with autoplay", () => {
  assert.equal(upNext(1700, 1800, 1750, true), null);
  assert.deepEqual(upNext(1750, 1800, 1750, true), { left: 10 });
  assert.deepEqual(upNext(1753.2, 1800, 1750, true), { left: 7 });
  assert.deepEqual(upNext(1761, 1800, 1750, true), { left: 0 });
  // Without autoplay it offers the next one and waits.
  assert.deepEqual(upNext(1761, 1800, 1750, false), { left: null });
});

test("up next plays only a countdown the viewer saw", () => {
  assert.deepEqual(takeUpNext(false, null), { counted: false, play: false });
  assert.deepEqual(takeUpNext(false, undefined), { counted: false, play: false });
  assert.deepEqual(takeUpNext(false, 8), { counted: true, play: false });
  assert.deepEqual(takeUpNext(true, 1), { counted: true, play: false });
  assert.deepEqual(takeUpNext(true, 0), { counted: false, play: true });
  // A new episode that is already at its end has not counted down.
  assert.deepEqual(takeUpNext(false, 0), { counted: false, play: false });
});

test("up next falls back to the last seconds", () => {
  assert.equal(upNext(1789, 1800, undefined, true), null);
  assert.deepEqual(upNext(1790, 1800, undefined, true), { left: 10 });
  // End titles past the end of the file are not where it ends.
  assert.deepEqual(upNext(1790, 1800, 1900, true), { left: 10 });
  // A clip too short to count down over.
  assert.equal(upNext(15, 20, undefined, true), null);
});
