import assert from "node:assert/strict";
import { test } from "node:test";
import { mediaTime, parseCaptionPlaylist, parseVtt } from "./src/features/player/liveCaptions.ts";

test("a server segment gives its 90 kHz start and cues in local seconds", () => {
  const body = "WEBVTT\nX-TIMESTAMP-MAP=LOCAL:00:00:00.000,MPEGTS:900000\n\n00:00:00.500 --> 00:00:01.500\nHello &amp; bye\n\n00:01.600 --> 00:02.000 line:90%\nTwo\nrows\n";
  const got = parseVtt(body);
  assert.equal(got.mpegts, 900000);
  assert.deepEqual(got.cues, [
    { start: 0.5, end: 1.5, text: "Hello &amp; bye", settings: "" },
    { start: 1.6, end: 2, text: "Two\nrows", settings: "line:90%" },
  ]);
});

test("a silent segment has no cues, and LOCAL moves them", () => {
  assert.deepEqual(parseVtt("WEBVTT\nX-TIMESTAMP-MAP=LOCAL:00:00:00.000,MPEGTS:0\n").cues, []);
  const shifted = parseVtt("WEBVTT\nX-TIMESTAMP-MAP=MPEGTS:90000,LOCAL:00:00:10.000\n\n00:00:10.250 --> 00:00:11.000\nHi\n");
  assert.equal(shifted.mpegts, 90000);
  assert.equal(shifted.cues[0].start, 0.25);
});

test("media time follows the first picture across the 33-bit wrap", () => {
  assert.equal(mediaTime(900000 + 90000 * 3, 900000), 3);
  const nearEnd = 2 ** 33 - 90000;
  assert.equal(mediaTime(45000, nearEnd), 1.5);
  assert.equal(mediaTime(nearEnd, 45000), -1.5);
  // A timeline 14 h long: the 33-bit time wrapped, and near picks the right lap.
  const zero = 90000;
  const late = (zero + 14 * 3600 * 90000) % 2 ** 33;
  assert.equal(mediaTime(late, zero, 14 * 3600), 14 * 3600);
});

test("the captions playlist numbers segments and counts breaks", () => {
  const body = [
    "#EXTM3U",
    "#EXT-X-TARGETDURATION:2",
    "#EXT-X-MEDIA-SEQUENCE:40",
    "#EXT-X-DISCONTINUITY-SEQUENCE:3",
    "#EXT-X-PROGRAM-DATE-TIME:2026-09-28T20:00:00.000Z",
    "#EXTINF:2.000,",
    "40.vtt",
    "#EXT-X-DISCONTINUITY",
    "#EXT-X-PROGRAM-DATE-TIME:2026-09-28T20:00:02.000Z",
    "#EXTINF:2.000,",
    "41.vtt",
  ].join("\n");
  const got = parseCaptionPlaylist(body, "http://server/media/live/1/720/captions.m3u8");
  assert.equal(got.target, 2);
  assert.deepEqual(got.segments, [
    { sn: 40, cc: 3, url: "http://server/media/live/1/720/40.vtt", date: Date.parse("2026-09-28T20:00:00Z") },
    { sn: 41, cc: 4, url: "http://server/media/live/1/720/41.vtt", date: Date.parse("2026-09-28T20:00:02Z") },
  ]);
});
