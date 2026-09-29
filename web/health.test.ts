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
