import assert from "node:assert/strict";
import { test } from "node:test";
import { recordingOn } from "./src/lib/recordingOn.ts";
import type { Airing, Recording } from "./src/types.ts";

const rec = (id: number, title: string, startedAt: string, more: Partial<Recording> = {}) =>
  ({ id, channelId: 4, title, status: "recording", path: "", startedAt, ...more }) as Recording;
const air = (title: string, programId?: string) =>
  ({ id: 1, channelId: 4, title, programId, start: "2026-11-13T20:00:00Z", end: "2026-11-13T21:00:00Z" }) as Airing;

// The first recording on the channel was the one shown and stopped, though
// the next show had its own recording beside the last one's padding.
test("a channel recording two shows picks the program shown", () => {
  const last = rec(1, "Evening News", "2026-11-13T18:59:00Z", { programId: "EP1" });
  const next = rec(2, "Quiz Night", "2026-11-13T19:59:00Z", { programId: "EP2" });
  const done = rec(3, "Quiz Night", "2026-11-13T17:00:00Z", { status: "complete" });
  const all = [last, next, done];
  assert.equal(recordingOn(all, 4, air("Quiz Night", "EP2"))?.id, 2);
  assert.equal(recordingOn(all, 4, air("quiz night "))?.id, 2);
  assert.equal(recordingOn([next, last], 4, air("Evening News", "EP1"))?.id, 1);
  assert.equal(recordingOn(all, 4, air("Something Else"))?.id, 2, "else the newest");
  assert.equal(recordingOn(all, 4)?.id, 2);
  assert.equal(recordingOn([last], 4, air("Quiz Night"))?.id, 1, "one recording is the channel's");
  assert.equal(recordingOn(all, 5, air("Quiz Night")), undefined);
});
