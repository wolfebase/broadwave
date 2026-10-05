import assert from "node:assert/strict";
import { test } from "node:test";
import { playerTabTarget } from "./src/features/player/focusCycle.ts";

test("Tab stays on the player's controls", () => {
  assert.equal(playerTabTarget(0, 0, false), null);
  assert.equal(playerTabTarget(4, -1, false), 0);
  assert.equal(playerTabTarget(4, -1, true), 3);
  assert.equal(playerTabTarget(4, 1, false), null);
  assert.equal(playerTabTarget(4, 2, true), null);
  assert.equal(playerTabTarget(4, 3, false), 0);
  assert.equal(playerTabTarget(4, 0, true), 3);
  assert.equal(playerTabTarget(1, 0, false), 0);
  assert.equal(playerTabTarget(1, 0, true), 0);
});
