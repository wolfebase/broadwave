import assert from "node:assert/strict";
import { test } from "node:test";
import { recordAgainOffered, recordAgainTitle, scheduleRecordAgain } from "./src/features/library/againOffer.ts";
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

test("a blank show title names the pass from the next airing", () => {
  assert.equal(
    recordAgainTitle(rec({ title: "  ", programId: "EP1", subtitle: "Pilot" }), { title: "Evening News", subtitle: "Pilot" }),
    "Evening News",
  );
});

test("a blank show title on the recording and the airing uses the episode name", () => {
  assert.equal(recordAgainTitle(rec({ title: "", subtitle: "Finals" }), { title: "", subtitle: "Finals" }), "Finals");
});

test("a show title that is already set stays the pass name", () => {
  assert.equal(
    recordAgainTitle(rec({ title: "Harbor Watch", subtitle: "The Lighthouse" }), { title: "Harbor Watch Late" }),
    "Harbor Watch",
  );
});

test("scheduling a blank show title posts the airing name and reloads passes", async () => {
  let posted = "";
  let refreshed = 0;
  await scheduleRecordAgain(
    rec({ title: "  ", programId: "EP1" }),
    { title: "Evening News", subtitle: "Pilot", channelId: 4, start: "2026-10-12T01:00:00Z" },
    async (title) => {
      posted = title;
    },
    async () => {
      refreshed += 1;
    },
  );
  assert.equal(posted, "Evening News");
  assert.equal(refreshed, 1);
});

test("scheduling with no name at all does not post a pass", async () => {
  let posted = 0;
  let refreshed = 0;
  await assert.rejects(
    scheduleRecordAgain(
      rec({ title: " " }),
      { title: " ", channelId: 4, start: "2026-10-12T01:00:00Z" },
      async () => {
        posted += 1;
      },
      async () => {
        refreshed += 1;
      },
    ),
  );
  assert.equal(posted, 0);
  assert.equal(refreshed, 0);
});
