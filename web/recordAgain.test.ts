import assert from "node:assert/strict";
import { test } from "node:test";
import { recordAgainOffered } from "./src/features/library/againOffer.ts";
import type { Recording } from "./src/types.ts";

function rec(extra: Partial<Recording> = {}): Recording {
  return { id: 1, channelId: 1, guideNumber: "4.1", title: "Mystery Hour", status: "complete", startedAt: "2026-10-01T19:00:00Z", ...extra };
}

const damaged = { continuityErrors: 30, transportErrors: 0, syncLosses: 0, packets: 10, gaps: 2, lostSeconds: 12.4, damaged: true as const };

test("a damaged recording whose file is gone can be recorded again", () => {
  assert.equal(recordAgainOffered(rec({ missing: true, programId: "EP1", health: damaged })), true);
});

test("a missing recording with no episode name is not offered", () => {
  assert.equal(recordAgainOffered(rec({ missing: true, health: damaged })), false);
});

test("a recording still in progress is not offered", () => {
  assert.equal(recordAgainOffered(rec({ status: "recording", programId: "EP1", health: damaged })), false);
});

test("a clean recording is not offered", () => {
  assert.equal(recordAgainOffered(rec({ programId: "EP1", health: { continuityErrors: 0, transportErrors: 0, syncLosses: 0, packets: 10, damaged: false } })), false);
});
