import assert from "node:assert/strict";
import { test } from "node:test";
import { neighbourIds } from "./src/features/player/warmPlan.ts";

test("channel up comes first, then the last channel, then channel down", () => {
  assert.deepEqual(neighbourIds([1, 2, 3, 4], 2, 4), [3, 4, 1]);
  assert.deepEqual(neighbourIds([1, 2, 3, 4], 4), [1, 3]);
  assert.deepEqual(neighbourIds([1, 2, 3], 2, 3), [3, 1]);
  assert.deepEqual(neighbourIds([1, 2], 1, 2), [2]);
  assert.deepEqual(neighbourIds([1], 1), []);
  assert.deepEqual(neighbourIds([1, 2, 3], 9), []);
});
