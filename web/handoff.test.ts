import assert from "node:assert/strict";
import { test } from "node:test";
import { noteHandoff, takeHandoff } from "./src/features/multiview/handoff.ts";

test("multiview joins the player's rendition once, soon after, for that channel", () => {
  noteHandoff(4, "1080.aac2.broadcast", 1000);
  assert.equal(takeHandoff(5, 2000), "");
  assert.equal(takeHandoff(4, 2000), "1080.aac2.broadcast");
  assert.equal(takeHandoff(4, 2100), "");
  noteHandoff(4, "720.aac2.broadcast", 1000);
  assert.equal(takeHandoff(4, 12_000), "");
  noteHandoff(4, undefined, 1000);
  assert.equal(takeHandoff(4, 1500), "");
});
