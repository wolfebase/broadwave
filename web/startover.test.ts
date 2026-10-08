import assert from "node:assert/strict";
import { test } from "node:test";
import { recordingHoldingStart, startOverChoice } from "./src/features/player/startOver.ts";
import type { Airing, Recording } from "./src/types.ts";

const show = Date.parse("2026-10-08T19:00:00Z");

test("start over plays from the live window when it holds the start", () => {
  assert.equal(startOverChoice(show, show - 60_000, undefined), "live");
  assert.equal(startOverChoice(show, show - 60_000, show - 120_000), "live");
  assert.equal(startOverChoice(show, show + 60_000, show - 120_000), "recording");
  assert.equal(startOverChoice(show, show + 60_000, show + 30_000), null);
  assert.equal(startOverChoice(show, null, undefined), null);
  assert.equal(startOverChoice(undefined, show - 60_000, undefined), null);
});

test("a recording holds the start when it is this showing and began before it", () => {
  const airing = { id: 1, channelId: 4, title: "Night Owls", programId: "EP7", start: "2026-10-08T19:00:00Z", end: "2026-10-08T20:00:00Z" } as Airing;
  const rec = (id: number, extra: Partial<Recording>): Recording =>
    ({ id, channelId: 4, guideNumber: "4.1", title: "Night Owls", status: "recording", startedAt: "2026-10-08T18:59:00Z", ...extra }) as Recording;
  const list = [
    rec(1, { startedAt: "2026-10-08T19:05:00Z" }),
    rec(2, { channelId: 5 }),
    rec(3, { programId: "EP6" }),
    rec(4, { startedAt: "2026-10-07T19:00:00Z", endedAt: "2026-10-07T20:00:00Z" }),
    rec(5, { missing: true }),
    rec(6, { title: " night owls " }),
    rec(7, { startedAt: "2026-10-08T18:58:00Z", programId: "EP7" }),
  ];
  assert.equal(recordingHoldingStart(airing, list)?.id, 7);
  assert.equal(recordingHoldingStart(airing, list.slice(0, 5)), undefined);
});
