import assert from "node:assert/strict";
import { test } from "node:test";
import { foundHeading, setupSummary } from "./src/features/setup/summary.ts";

test("setup says what it found in words", () => {
  assert.equal(setupSummary([{ tunerCount: 2 }], 3), "3 channels from 2 tuners.");
  assert.equal(setupSummary([{ tunerCount: 2 }, { tunerCount: 4 }], 120), "120 channels from 6 tuners.");
  assert.equal(setupSummary([{ tunerCount: 1 }], 1), "1 channel from 1 tuner.");
  assert.equal(setupSummary([{ tunerCount: 0 }], 40), "40 channels from 1 source.", "a playlist is a source, not a tuner");
  assert.equal(setupSummary([{ tunerCount: 0 }, { tunerCount: 0 }], 40), "40 channels from 2 sources.");
  assert.equal(setupSummary([{ tunerCount: 2 }], 0), "No channels yet from 2 tuners.");
});

test("setup names what it found by kind", () => {
  assert.equal(foundHeading([{ tunerCount: 2 }]), "Your tuner");
  assert.equal(foundHeading([{ tunerCount: 2 }, { tunerCount: 4 }]), "Your tuners");
  assert.equal(foundHeading([{ tunerCount: 0 }]), "Your channel source", "a playlist is not a tuner");
  assert.equal(foundHeading([{ tunerCount: 0 }, { tunerCount: 0 }]), "Your channel sources");
  assert.equal(foundHeading([{ tunerCount: 2 }, { tunerCount: 0 }]), "Your tuners and sources");
});
