import assert from "node:assert/strict";
import { test } from "node:test";
import { clearBroadcast, fromStillOn, resolveClear, standInIds, type ClearChannel } from "./src/features/multiview/clear.ts";

const hd = { id: 1, twinId: 2 };
const wide: ClearChannel = { id: 2, twinId: 1 };
const encrypted: ClearChannel = { id: 9, protected: true, playsAs: 3, twinId: 3 };
const clear: ClearChannel = { id: 3 };
const other: ClearChannel = { id: 4 };
const lineup = [hd, wide, encrypted, clear, other];

test("a channel on the guide plays as itself", () => {
  assert.deepEqual(clearBroadcast(1, [hd, other], lineup), { id: 1, note: false });
  assert.deepEqual(resolveClear([1, 4], [hd, other], lineup), { ids: [1, 4], from: [] });
});

test("a hidden half plays as the half on the guide", () => {
  assert.deepEqual(clearBroadcast(1, [wide, other], lineup), { id: 2, note: false });
  assert.deepEqual(clearBroadcast(2, [hd, other], lineup), { id: 1, note: false });
  assert.deepEqual(resolveClear([1, 2, 4], [wide, other], lineup), { ids: [2, 4], from: [] });
});

test("an encrypted 3.0 id plays its clear twin and names the stand-in", () => {
  assert.deepEqual(clearBroadcast(9, [clear, other], lineup), { id: 3, note: true });
  assert.deepEqual(resolveClear([9, 4], [clear, other], lineup), { ids: [3, 4], from: [9] });
  assert.deepEqual(standInIds([9], lineup), [3]);
});

test("an encrypted station with no visible twin is left out", () => {
  assert.equal(clearBroadcast(9, [other], lineup), null);
  assert.deepEqual(resolveClear([9], [other], lineup), { ids: [], from: [] });
});

test("an unknown id is left out", () => {
  assert.equal(clearBroadcast(99, [hd], lineup), null);
});

test("the encrypted note stays while its clear twin is on screen", () => {
  assert.deepEqual(fromStillOn([9], [3, 4], lineup), [9]);
  assert.deepEqual(fromStillOn([9], [4], lineup), []);
});
