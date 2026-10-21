import assert from "node:assert/strict";
import { test } from "node:test";
import { tunedLabel, tunerHolder } from "./src/features/setup/diagLines.ts";

test("a tuner line leaves out what the tuner didn't report", () => {
  assert.equal(tunerHolder("Another device", "", ""), "Another device");
  assert.equal(tunerHolder("Another device", undefined, undefined), "Another device");
  assert.equal(tunerHolder("This server", "4.1", "KBWV"), "This server · 4.1 KBWV");
  assert.equal(tunerHolder("This server", "4.1", "4.1"), "This server · 4.1");
  assert.equal(tunedLabel("", "KBWV"), "KBWV");
});
