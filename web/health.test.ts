import assert from "node:assert/strict";
import { test } from "node:test";
import { signalLine } from "./src/features/library/health.ts";

const counted = { continuityErrors: 0, transportErrors: 0, syncLosses: 0, packets: 10 };

test("a recording line appears only when the signal broke up", () => {
  assert.equal(signalLine(undefined), "");
  assert.equal(signalLine(counted), "");
  assert.equal(signalLine({ ...counted, continuityErrors: 1 }), "Signal broke up once");
  assert.equal(signalLine({ ...counted, continuityErrors: 14 }), "Signal broke up 14 times");
  assert.equal(signalLine({ ...counted, transportErrors: 2, syncLosses: 1 }), "Signal broke up 3 times");
});

test("time the signal dropped comes first", () => {
  assert.equal(signalLine({ ...counted, continuityErrors: 30, gaps: 2, lostSeconds: 12.4 }), "Signal dropped for 12 s");
  assert.equal(signalLine({ ...counted, continuityErrors: 3, gaps: 1, lostSeconds: 0.2 }), "Signal broke up 3 times");
});
