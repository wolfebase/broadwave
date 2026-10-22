import assert from "node:assert/strict";
import { test } from "node:test";
import { channelMatches } from "./src/features/search/channelMatch.ts";

const kbwv = { id: 1, displayNumber: "4.1", displayName: "KBWV-DT", network: "FOX", twinId: 3 };
const rewind = { id: 2, displayNumber: "4.2", displayName: "Rewind" };
const kbwv3 = { id: 3, displayNumber: "104.1", displayName: "KBWV", network: "FOX", twinId: 1 };
const krvt = { id: 4, displayNumber: "41.1", displayName: "KRVT-TV" };
const all = [kbwv, rewind, kbwv3, krvt];
const found = (q: string) => all.filter((c) => channelMatches(c, q, all)).map((c) => c.displayNumber);

test("a number finds the channel, its start, and its twin, never 41.1 for 4.1", () => {
  assert.deepEqual(found("4.1"), ["4.1", "104.1"]);
  assert.deepEqual(found("4-1"), ["4.1", "104.1"]);
  assert.deepEqual(found("104.1"), ["4.1", "104.1"]);
  assert.deepEqual(found("4"), ["4.1", "4.2", "41.1"]);
});

test("a name, call sign, or network finds the channel", () => {
  assert.deepEqual(found("fox"), ["4.1", "104.1"]);
  assert.deepEqual(found("krvt"), ["41.1"]);
  assert.deepEqual(found("rew"), ["4.2"]);
  assert.deepEqual(found(""), ["4.1", "4.2", "104.1", "41.1"]);
});
