import assert from "node:assert/strict";
import { test } from "node:test";
import { newTimedDraft, timedBody, timedDraftOf, timedEnd, timedLink, timedProblem, timedRequest } from "./src/features/schedule/manual.ts";

test("a new timed recording starts at the next five minutes for an hour", () => {
  const draft = newTimedDraft(new Date(2026, 9, 21, 21, 57, 30), 4);
  assert.equal(draft.date, "2026-10-21");
  assert.equal(draft.timeStart, "22:00");
  assert.equal(draft.duration, 60);
  assert.equal(draft.channelId, 4);
  assert.equal(draft.repeat, false);
  assert.equal(draft.padBefore, 0);
  // Late at night the next five minutes is tomorrow.
  const late = newTimedDraft(new Date(2026, 9, 21, 23, 58), 4);
  assert.equal(late.date, "2026-10-22");
  assert.equal(late.timeStart, "00:00");
});

test("from a show in the guide it takes that show's start and length", () => {
  const now = new Date(2026, 9, 21, 20, 0);
  const draft = newTimedDraft(now, 4, new Date(2026, 9, 21, 22, 30), 90);
  assert.equal(draft.timeStart, "22:30");
  assert.equal(draft.duration, 90);
  // A show that already ended is not a start.
  assert.equal(newTimedDraft(now, 4, new Date(2026, 9, 21, 18, 0), 30).timeStart, "20:05");
  assert.equal(newTimedDraft(now, 4, undefined, 9999).duration, 480);
});

test("the body sends a date once, or days to repeat, and renames a saved one", () => {
  const draft = { ...newTimedDraft(new Date(2026, 9, 21, 21, 0), 4), title: "  Late news " };
  const once = timedBody(draft, false);
  assert.equal(once.kind, "manual");
  assert.equal(once.title, "Late news");
  assert.equal(once.date, "2026-10-21");
  assert.deepEqual(once.days, []);
  assert.equal(once.rename, undefined);
  assert.equal(once.keepCount, 0);
  const repeat = timedBody({ ...draft, repeat: true, days: [1, 3], keepMode: "last", keepCount: 4 }, true);
  assert.equal(repeat.date, undefined);
  assert.deepEqual(repeat.days, [1, 3]);
  assert.equal(repeat.rename, "Late news");
  assert.equal(repeat.keepCount, 4);
});

test("a saved pass comes back as the draft it was", () => {
  const now = new Date(2026, 9, 21, 12, 0);
  const draft = timedDraftOf({ title: "", kind: "manual", channelId: 4, timeStart: "23:30", duration: 45, days: [5, 1], keepMode: "last", keepCount: 2, commercials: false }, now);
  assert.equal(draft.repeat, true);
  assert.deepEqual(draft.days, [1, 5]);
  assert.equal(draft.timeStart, "23:30");
  assert.equal(draft.duration, 45);
  assert.equal(draft.keepMode, "last");
  assert.equal(draft.commercials, false);
  const once = timedDraftOf({ title: "x", kind: "manual", channelId: 4, timeStart: "06:00", duration: 30, date: "2026-11-01" }, now);
  assert.equal(once.repeat, false);
  assert.equal(once.date, "2026-11-01");
});

test("a draft says what is missing before it is sent", () => {
  const ok = newTimedDraft(new Date(2026, 9, 21, 21, 0), 4);
  assert.equal(timedProblem(ok), "");
  assert.equal(timedProblem({ ...ok, channelId: 0 }), "channel");
  assert.equal(timedProblem({ ...ok, timeStart: "" }), "time");
  assert.equal(timedProblem({ ...ok, duration: 0 }), "duration");
  assert.equal(timedProblem({ ...ok, duration: 481 }), "duration");
  assert.equal(timedProblem({ ...ok, date: "" }), "date");
  assert.equal(timedProblem({ ...ok, repeat: true, days: [] }), "days");
});

test("the end is on the clock and can fall on the next day", () => {
  assert.deepEqual(timedEnd("22:00", 90), { clock: "23:30", nextDay: false });
  assert.deepEqual(timedEnd("23:30", 60), { clock: "00:30", nextDay: true });
  assert.deepEqual(timedEnd("00:00", 480), { clock: "08:00", nextDay: false });
});

test("the guide's link opens Record a time for its channel and show", () => {
  const link = timedLink(7, { start: "2026-10-22T03:00:00Z", end: "2026-10-22T04:30:00Z" });
  const params = new URLSearchParams(link.split("?")[1]);
  const req = timedRequest(params);
  assert.equal(req?.channelId, 7);
  assert.equal(req?.minutes, 90);
  assert.equal(req?.start?.toISOString(), "2026-10-22T03:00:00.000Z");
  assert.equal(timedLink(7), "/schedule?record=7");
  assert.equal(timedRequest(new URLSearchParams("record=abc")), null);
  assert.equal(timedRequest(new URLSearchParams("")), null);
});
