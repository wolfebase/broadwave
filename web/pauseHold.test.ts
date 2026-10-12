import assert from "node:assert/strict";
import { test } from "node:test";
import { PauseHold } from "./src/lib/pauseHold.ts";

test("a later pause replaces the first, and leaving cancels the resume", () => {
  const queued: { fn: () => void; ms: number }[] = [];
  let cancelled = 0;
  const hold = new PauseHold(
    (fn, ms) => {
      queued.push({ fn, ms });
      return queued.length;
    },
    () => {
      cancelled += 1;
    },
  );
  let ran = 0;
  hold.arm(400, () => {
    ran += 1;
  });
  hold.arm(800, () => {
    ran += 1;
  });
  // The first arm also cancels the empty id, then the second cancels the first timer.
  assert.equal(cancelled, 2);
  queued[0].fn();
  assert.equal(ran, 0);
  queued[1].fn();
  assert.equal(ran, 1);
  hold.stop();
  assert.equal(cancelled, 3);
});
