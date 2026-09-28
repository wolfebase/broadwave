import assert from "node:assert/strict";
import { test } from "node:test";
import { holeEnd } from "./src/lib/bufferHole.ts";
import { roomStart, roomTarget } from "./src/lib/roomStart.ts";
import { nextSeekLead } from "./src/lib/seekLead.ts";
import { newSettle, settleDue } from "./src/lib/settle.ts";

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

test("a screen that stopped trimming corrects a steady offset once, then waits longer", () => {
  // WebKit held -117 ms for minutes: under the 120 ms seek threshold, no trims.
  const s = newSettle();
  const at = (sec: number) => sec * 1000;
  assert.equal(settleDue(s, -117, at(0)), false);
  assert.equal(settleDue(s, -117, at(9)), false);
  assert.equal(settleDue(s, -117, at(10)), true);
  // It missed: the next one waits 30 s, then 60 s, then 120 s.
  const fires: number[] = [];
  for (let t = 11; t <= 400; t++) if (settleDue(s, -90, at(t))) fires.push(t);
  assert.deepEqual(fires, [40, 100, 220]);
  // Never more often than the cap, however long it misses.
  let fired = 0;
  for (let t = 101; t < 101 + 3600; t++) if (settleDue(s, -90, at(t))) fired++;
  assert.ok(fired <= 3600 / 300 + 1, `fired ${fired}`);
});

test("a screen inside the band is left alone, and a minute there starts the waits over", () => {
  const s = newSettle();
  const at = (sec: number) => sec * 1000;
  for (let t = 0; t < 120; t++) assert.equal(settleDue(s, 30, at(t)), false);
  assert.equal(settleDue(s, -60, at(120)), false);
  assert.equal(settleDue(s, -60, at(130)), true);
  // Landed inside: after a minute there, the next drift waits only its 10 s.
  for (let t = 131; t < 200; t++) assert.equal(settleDue(s, 5, at(t)), false);
  assert.equal(settleDue(s, 70, at(200)), false);
  assert.equal(settleDue(s, 70, at(210)), true);
  // A drift that comes and goes inside 10 s never corrects.
  const t2 = newSettle();
  for (let t = 0; t < 100; t++) assert.equal(settleDue(t2, t % 8 < 5 ? 80 : 10, at(t)), false);
  assert.equal(settleDue(t2, Number.NaN, at(200)), false);
});

test("a screen joining a room starts on the room's frame, a moment ahead", () => {
  // Six one-second fragments from 12:00:00; the playlist edge is 12:00:06.
  const t0 = Date.UTC(2026, 8, 28, 12, 0, 0);
  const frags = Array.from({ length: 6 }, (_, i) => ({ start: 100 + i, duration: 1, programDateTime: t0 + i * 1000 }));
  const room = { room: "channel:2", channelId: 2, mode: "follow" as const, anchorServer: t0 + 10_000, anchorMedia: t0 + 1_000, rate: 1, latency: "balanced" as const, version: 1, members: 1 };
  // At 12:00:11.5 the room plays 12:00:02.5; the first picture shows a moment later.
  assert.equal(roomTarget(room, t0 + 11_500), t0 + 2_500);
  assert.ok(Math.abs((roomStart(frags, room, t0 + 11_500) ?? 0) - 103) < 1e-9);
  // A room easing back at 0.975x leads by less.
  assert.ok(Math.abs((roomStart(frags, { ...room, rate: 0.975 }, t0 + 10_000) ?? 0) - (101 + 0.4875)) < 1e-9);
  // A paused group room starts on its frame exactly.
  assert.ok(Math.abs((roomStart(frags, { ...room, rate: 0 }, t0 + 60_000) ?? 0) - 101) < 1e-9);
});

test("a room whose frame the playlist does not hold leaves the start alone", () => {
  const t0 = Date.UTC(2026, 8, 28, 12, 0, 0);
  const frags = Array.from({ length: 6 }, (_, i) => ({ start: i, duration: 1, programDateTime: t0 + i * 1000 }));
  // A fresh tune: the room waits on its first frame until 1.5 s from now.
  const fresh = { room: "channel:2", channelId: 2, mode: "follow" as const, anchorServer: t0 + 7_500, anchorMedia: t0, rate: 1, latency: "balanced" as const, version: 1, members: 1 };
  assert.equal(roomStart(frags, fresh, t0 + 6_000), null);
  // Past the edge, and a playlist with no program date-times.
  assert.equal(roomStart(frags, { ...fresh, anchorServer: t0, anchorMedia: t0 + 5_500 }, t0), null);
  assert.equal(roomStart(frags.map((f) => ({ ...f, programDateTime: null })), { ...fresh, anchorServer: t0 }, t0 + 2_000), null);
});
