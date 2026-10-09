import assert from "node:assert/strict";
import { test } from "node:test";
import { goLiveAction } from "./src/features/player/goLive.ts";

test("live does not seek while sync is already on", () => {
  assert.equal(goLiveAction(false, true), "play");
  assert.equal(goLiveAction(true, true), "group");
  assert.equal(goLiveAction(false, false), "arm");
  assert.equal(goLiveAction(true, false), "group");
});
