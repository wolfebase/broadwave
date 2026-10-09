import assert from "node:assert/strict";
import { test } from "node:test";
import { channelMissing } from "./src/app/lineup.ts";

test("a watch link waits for this visit's lineup before leaving for the guide", () => {
  assert.equal(channelMissing(false, 5, [1, 2]), false);
  assert.equal(channelMissing(true, 5, [1, 2]), true);
  assert.equal(channelMissing(true, 5, [5]), false);
  assert.equal(channelMissing(true, 0, []), false);
  assert.equal(channelMissing(true, 5, []), false);
});
