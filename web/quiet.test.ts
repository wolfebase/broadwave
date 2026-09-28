import assert from "node:assert/strict";
import { test } from "node:test";
import { holdQuietStart } from "./src/features/player/quietStart.ts";

test("a quiet tile waits for the sound tile, and a kept page does not", () => {
  assert.equal(holdQuietStart(true, false), true);
  assert.equal(holdQuietStart(false, false), false);
  assert.equal(holdQuietStart(false, true), false);
  assert.equal(holdQuietStart(true, true), false);
});
