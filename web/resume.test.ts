import assert from "node:assert/strict";
import { test } from "node:test";
import { awayBeforeSeekMs, comeBackAction, edgeCaughtUp, resumePlan, resumeSeek } from "./src/features/player/resume.ts";

test("a tab that was only gone a moment keeps its playhead", () => {
  assert.equal(resumeSeek({ current: 10, liveSync: 40, awayMs: 800, held: false }), null);
  assert.deepEqual(resumePlan({ current: 10, liveSync: 40, awayMs: 800, held: false, waitedMs: 0, edgeAtLeave: 12, driftMs: null }), {
    action: "play",
  });
});

test("a viewer who paused stays paused", () => {
  assert.equal(resumeSeek({ current: 10, liveSync: 400, awayMs: 300_000, held: true }), null);
  assert.deepEqual(resumePlan({ current: 10, liveSync: 400, awayMs: 300_000, held: true, waitedMs: 0, edgeAtLeave: 12, driftMs: 0 }), {
    action: "ignore",
  });
});

test("a frozen tab waits until the edge moves, then seeks to it", () => {
  // The playlist still shows the edge from when the tab froze.
  assert.equal(edgeCaughtUp(100, 101, 300_000), false);
  assert.deepEqual(resumePlan({ current: 100, liveSync: 101, awayMs: 300_000, held: false, waitedMs: 0, edgeAtLeave: 100, driftMs: -17 }), {
    action: "wait",
  });
  assert.deepEqual(resumePlan({ current: 100, liveSync: 101, awayMs: 300_000, held: false, waitedMs: 2200, edgeAtLeave: 100, driftMs: -17 }), {
    action: "play",
  });

  // The edge moved by about as long as the tab was gone.
  assert.equal(edgeCaughtUp(100, 395, 300_000), true);
  assert.equal(resumeSeek({ current: 100, liveSync: 395, awayMs: 300_000, held: false }), 395);
  assert.deepEqual(resumePlan({ current: 102, liveSync: 395, awayMs: 300_000, held: false, waitedMs: 0, edgeAtLeave: 100, driftMs: -300000 }), {
    action: "seek",
    to: 395,
  });
});

test("a tab that got ahead of the room steps back instead of pausing it out", () => {
  const plan = resumePlan({ current: 392, liveSync: 400, awayMs: 300_000, held: false, waitedMs: 0, edgeAtLeave: 100, driftMs: 4320 });
  assert.equal(plan.action, "seek");
  if (plan.action === "seek") assert.ok(Math.abs(plan.to - 387.68) < 0.001);
});

test("a playhead that kept up with the edge is left there", () => {
  assert.equal(edgeCaughtUp(100, 395, 300_000), true);
  assert.equal(resumeSeek({ current: 394, liveSync: 395, awayMs: 300_000, held: false }), null);
  assert.deepEqual(resumePlan({ current: 394, liveSync: 395, awayMs: 300_000, held: false, waitedMs: 0, edgeAtLeave: 100, driftMs: -20 }), {
    action: "play",
  });
});

test("coming back leaves a named outage that already destroyed the player", () => {
  const named = { tornDown: true, awayMs: 10_000, syncing: true, paused: false };
  assert.equal(comeBackAction(named), "ignore");
  assert.equal(comeBackAction({ ...named, tornDown: false }), "resume");
  assert.equal(comeBackAction({ tornDown: false, awayMs: 1000, syncing: true, paused: true }), "play");
  assert.equal(comeBackAction({ tornDown: false, awayMs: 1000, syncing: true, paused: false }), "ignore");
  assert.equal(comeBackAction({ tornDown: false, awayMs: 1000, syncing: false, paused: true }), "ignore");
  assert.equal(comeBackAction({ tornDown: true, awayMs: 500, syncing: true, paused: true }), "ignore");
  assert.equal(comeBackAction({ tornDown: false, awayMs: awayBeforeSeekMs, syncing: false, paused: true }), "ignore");
  assert.equal(comeBackAction({ tornDown: false, awayMs: awayBeforeSeekMs, syncing: false, paused: false }), "resume");
});
