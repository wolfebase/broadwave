import assert from "node:assert/strict";
import { test } from "node:test";
import { setupSummary } from "./src/features/setup/summary.ts";

test("setup says what it found in words", () => {
  assert.equal(setupSummary([{ tunerCount: 2 }], 3), "3 channels from 2 tuners.");
  assert.equal(setupSummary([{ tunerCount: 2 }, { tunerCount: 4 }], 120), "120 channels from 6 tuners.");
  assert.equal(setupSummary([{ tunerCount: 1 }], 1), "1 channel from 1 tuner.");
  assert.equal(setupSummary([{ tunerCount: 0 }], 40), "40 channels from 1 source.", "a playlist is a source, not a tuner");
  assert.equal(setupSummary([{ tunerCount: 0 }, { tunerCount: 0 }], 40), "40 channels from 2 sources.");
  assert.equal(setupSummary([{ tunerCount: 2 }], 0), "No channels yet from 2 tuners.");
});
