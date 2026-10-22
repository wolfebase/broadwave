import assert from "node:assert/strict";
import { test } from "node:test";
import { tuneStepId } from "./src/features/player/tuneSteps.ts";

test("a slow start says lining up only while lining up is on", () => {
  assert.equal(tuneStepId(0, false), "player.tuneAntenna");
  assert.equal(tuneStepId(3500, true), "player.tunePicture");
  assert.equal(tuneStepId(8000, false), "player.tuneAlmost");
  assert.equal(tuneStepId(8000, true), "player.tuneLive");
  assert.equal(tuneStepId(20000, true), "player.tuneWeak");
});
