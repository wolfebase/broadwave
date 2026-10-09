import assert from "node:assert/strict";
import { test } from "node:test";
import { cardLabel } from "./src/features/sports/matchup.ts";

test("a record button names the matchup, not the shared program title", () => {
  assert.equal(cardLabel({ title: "NFL Football", subtitle: "Chicago at Buffalo" }), "Chicago at Buffalo");
  assert.equal(cardLabel({ title: "NFL: Bears at Bills", subtitle: "Sunday night" }), "Bears at Bills");
  assert.equal(cardLabel({ title: "Golf", subtitle: "Final round" }), "Final round");
  assert.equal(cardLabel({ title: "Tennis" }), "Tennis");
});
