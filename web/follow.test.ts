import assert from "node:assert/strict";
import { test } from "node:test";
import { takeTeams } from "./src/features/sports/follow.ts";

test("an older team list does not wipe a follow that just landed", () => {
  assert.equal(takeTeams(1, 2, [{ name: "before" }]), null);
  assert.deepEqual(takeTeams(2, 2, [{ name: "after" }]), [{ name: "after" }]);
  assert.equal(takeTeams(2, 2, null), null);
});
