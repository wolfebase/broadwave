import assert from "node:assert/strict";
import { test } from "node:test";
import { planRecordingSeek } from "./src/features/recordings/seek.ts";

test("a seek inside the encoded part stays on this playlist", () => {
  assert.deepEqual(planRecordingSeek(305, 300, 310), { action: "within", to: 305 });
  assert.deepEqual(planRecordingSeek(11, 0, 10), { action: "within", to: 10 });
  assert.deepEqual(planRecordingSeek(0, 0, 0), { action: "within", to: 0 });
});

test("a seek past the encode, or back before it started, asks the server to begin there", () => {
  assert.deepEqual(planRecordingSeek(40 * 60, 0, 8), { action: "reload", to: 40 * 60 });
  assert.deepEqual(planRecordingSeek(50, 300, 310), { action: "reload", to: 50 });
  assert.deepEqual(planRecordingSeek(0, 300, 310), { action: "reload", to: 0 });
  assert.deepEqual(planRecordingSeek(30, 0, 0), { action: "reload", to: 30 });
});

test("a seek that is not a number starts over", () => {
  assert.deepEqual(planRecordingSeek(Number.NaN, 20, 40), { action: "reload", to: 0 });
});
