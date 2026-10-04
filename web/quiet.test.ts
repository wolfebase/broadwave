import assert from "node:assert/strict";
import { test } from "node:test";
import { gateHoldMs, pageFitsTiles, StartGate } from "./src/features/player/quietStart.ts";

test("the picture budget or a full set of answers covers the tiles", () => {
  assert.equal(pageFitsTiles(4, 3, 1), true);
  assert.equal(pageFitsTiles(2, 3, 3), true);
  assert.equal(pageFitsTiles(2, 3, 2), false);
  assert.equal(pageFitsTiles(null, 3, 3), true);
  assert.equal(pageFitsTiles(null, 3, 2), false);
  assert.equal(pageFitsTiles(2, 2, 0), true);
  assert.equal(pageFitsTiles(null, 1, 0), true);
});

function gate() {
  const clock = { t: 0 };
  return { clock, g: new StartGate(() => clock.t) };
}
const settle = () => new Promise((resolve) => setTimeout(resolve, 0));

test("tiles start in rank order: the sound tile, then a tile that showed a picture, then the rest", async () => {
  const { g } = gate();
  const started: number[] = [];
  g.rank(1, 2);
  g.rank(2, 1);
  g.rank(3, 0);
  for (const id of [1, 2, 3]) g.wait(id, () => started.push(id));
  await settle();
  assert.deepEqual(started, [3]);
  g.answered(3);
  assert.deepEqual(started, [3, 2]);
  g.answered(2);
  assert.deepEqual(started, [3, 2, 1]);
});

test("a refusal counts as an answer, and a tile that never answers holds the others 8 s", async () => {
  const { g, clock } = gate();
  const started: number[] = [];
  g.rank(1, 0);
  g.rank(2, 2);
  g.wait(2, () => started.push(2));
  await settle();
  assert.deepEqual(started, []);
  clock.t = gateHoldMs - 1;
  assert.equal(g.blocked(2), true);
  clock.t = gateHoldMs;
  assert.equal(g.blocked(2), false);
  g.stop();
});

test("after a restart every tile asks again in order, and a tile reaching the server first waits", async () => {
  const { g, clock } = gate();
  g.rank(1, 0);
  g.rank(2, 1);
  g.rank(3, 2);
  for (const id of [1, 2, 3]) g.answered(id);
  assert.equal(g.blocked(3), false);
  g.dropped();
  // The server is down longer than a hold: the outage tick of a quiet tile still waits.
  clock.t = 20_000;
  assert.equal(g.blocked(3), true);
  assert.equal(g.blocked(1), false);
  g.restarted();
  g.back();
  clock.t = 20_000 + gateHoldMs - 1;
  assert.equal(g.blocked(2), true);
  g.answered(1);
  assert.equal(g.blocked(2), false);
  assert.equal(g.blocked(3), true);
  g.answered(2);
  assert.equal(g.blocked(3), false);
});

test("a dropped socket with no restart lets the tiles that were playing go on hello", () => {
  const { g } = gate();
  g.rank(1, 0);
  g.rank(2, 2);
  g.answered(1);
  g.answered(2);
  g.dropped();
  assert.equal(g.blocked(2), true);
  g.back();
  assert.equal(g.blocked(2), false);
});

test("moving the sound frees a tile waiting on the old order, and a tile that leaves holds nobody", async () => {
  const { g } = gate();
  const started: number[] = [];
  g.rank(1, 0);
  g.rank(2, 2);
  g.wait(2, () => started.push(2));
  await settle();
  assert.deepEqual(started, []);
  g.rank(2, 0);
  g.rank(1, 2);
  assert.deepEqual(started, [2]);
  g.rank(3, 2);
  g.wait(3, () => started.push(3));
  await settle();
  g.leave(2);
  assert.deepEqual(started, [2, 3]);
  g.stop();
});
