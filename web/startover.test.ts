import assert from "node:assert/strict";
import { test } from "node:test";
import { livePlayhead, liveWindowFrom, playheadMedia, recordingHoldingStart, scrubHold, shownScrub, startOverAt, startOverChoice, swallowWakeClick, windowFromFragments } from "./src/features/player/startOver.ts";
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

test("a local seek does not forget a live window that still holds the show", () => {
  const recording = show - 120_000;
  // The room clock is gone, so today's gate would open the recording.
  assert.equal(startOverChoice(show, null, recording), "recording");
  const from = windowFromFragments(100, [{ start: 130, duration: 2, programDateTime: show - 10_000 }]);
  assert.equal(from, show - 40_000);
  assert.equal(startOverChoice(show, from, recording), "live");
  assert.equal(startOverAt(show, from, 100, 400), 140);
  assert.equal(startOverAt(show, null, 100, 400), null);
  // The playhead's own time still wins when the room clock is there.
  assert.equal(liveWindowFrom({ seekable: true, seekStart: 100, currentTime: 130, media: show - 10_000, frags: [] }), show - 40_000);
  assert.equal(liveWindowFrom({ seekable: true, seekStart: 100, currentTime: 130, media: null, frags: [{ start: 130, duration: 2, programDateTime: show - 10_000 }] }), show - 40_000);
  assert.equal(liveWindowFrom({ seekable: false, seekStart: 100, currentTime: 130, media: show, frags: [] }), null);
});

test("safari dates the playhead from the window start when the room clock is off", () => {
  const start = show - 120_000;
  assert.equal(playheadMedia(show - 10_000, start, 130), show - 10_000);
  assert.equal(playheadMedia(null, start, 130), start + 130_000);
  assert.equal(playheadMedia(null, null, 130), null);
  assert.equal(playheadMedia(null, Number.NaN, 130), null);
  assert.equal(playheadMedia(Number.NaN, start, 130), start + 130_000);
  // Dated fragments keep owning a local seek. A start date must not replace them.
  assert.equal(livePlayhead(null, 3, start, 130), null);
  assert.equal(livePlayhead(show - 10_000, 3, start, 130), show - 10_000);
  const media = livePlayhead(null, 0, start, 130);
  assert.equal(media, start + 130_000);
  assert.equal(liveWindowFrom({ seekable: true, seekStart: 100, currentTime: 130, media, frags: [] }), start + 100_000);
  assert.equal(livePlayhead(null, 0, null, 130), null);
});

test("the scrub thumb stays on the drag until the playhead arrives", () => {
  assert.equal(shownScrub(40, 12), 40);
  assert.equal(shownScrub(0, 12), 0);
  assert.equal(shownScrub(null, 12), 12);
  assert.equal(scrubHold(40, 12), 40);
  assert.equal(scrubHold(40, 39.6), null);
  assert.equal(scrubHold(null, 12), null);
  assert.equal(scrubHold(40, Number.NaN), 40);
});

test("a tap that wakes the bar does not press the control under it", () => {
  assert.equal(swallowWakeClick(true, "touchstart"), true);
  assert.equal(swallowWakeClick(false, "touchstart"), false);
  assert.equal(swallowWakeClick(true, "click"), false);
  assert.equal(swallowWakeClick(true, "mousemove"), false);
});
