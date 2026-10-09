import assert from "node:assert/strict";
import { test } from "node:test";
import { actionName } from "./src/lib/actionName.ts";

test("a repeated action names the show it changes", () => {
  assert.equal(actionName("Play", "S1 E4, Desk, The Quiet Hour"), "Play S1 E4, Desk, The Quiet Hour");
  assert.equal(actionName("Record every airing", "Night Game"), "Record every airing Night Game");
  assert.equal(actionName("  Delete this file  ", "  Mystery Hour  "), "Delete this file Mystery Hour");
  assert.equal(actionName("Delete", "   "), "Delete");
  assert.equal(actionName("Watch", "Bears at Bills"), "Watch Bears at Bills");
});
