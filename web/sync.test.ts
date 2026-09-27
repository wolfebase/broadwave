import assert from "node:assert/strict";
import { test } from "node:test";
import { holeEnd } from "./src/lib/bufferHole.ts";
import { nextSeekLead } from "./src/lib/seekLead.ts";

test("a seek that lands behind makes the next one lead by the loss", () => {
  // Safari lost 850 ms decoding up to the target; the next seek aims that far ahead.
  const lead = nextSeekLead(0, -850);
  assert.equal(lead, 0.85);
  // Leading by the loss lands on target, so the lead holds.
  assert.equal(nextSeekLead(lead, 0), 0.85);
  assert.ok(Math.abs(nextSeekLead(lead, -40) - 0.89) < 1e-9);
});

test("a seek that lands ahead shortens the lead, never below zero or past two seconds", () => {
  assert.ok(Math.abs(nextSeekLead(0.85, 300) - 0.55) < 1e-9);
  assert.equal(nextSeekLead(0.2, 900), 0);
  assert.equal(nextSeekLead(1.5, -4000), 2);
  // Chrome lands on target; it never gains a lead.
  assert.equal(nextSeekLead(0, 15), 0);
});

test("a playhead at a small hole continues at the next range", () => {
  // The buffer around a timestamp break: old encode to 19.97, new from 20.04.
  assert.equal(holeEnd(19.888, [[0, 19.97], [20.04, 22.02]]), 20.04);
  assert.equal(holeEnd(19.99, [[0, 19.97], [20.04, 22.02]]), 20.04);
  // Well before the hole, playback just continues.
  assert.equal(holeEnd(18, [[0, 19.97], [20.04, 22.02]]), null);
  // A gap wider than a hole is the edge of what has loaded.
  assert.equal(holeEnd(19.9, [[0, 19.97], [21, 22]]), null);
  // One range, no hole.
  assert.equal(holeEnd(19.9, [[0, 19.97]]), null);
  // Already past the hole.
  assert.equal(holeEnd(20.5, [[0, 19.97], [20.04, 22.02]]), null);
});
