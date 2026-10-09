import assert from "node:assert/strict";
import { test } from "node:test";
import { channelMissing, channelToDock } from "./src/app/lineup.ts";

test("a watch link waits for this visit's lineup before leaving for the guide", () => {
  assert.equal(channelMissing(false, 5, [1, 2]), false);
  assert.equal(channelMissing(true, 5, [1, 2]), true);
  assert.equal(channelMissing(true, 5, [5]), false);
  assert.equal(channelMissing(true, 0, []), false);
  assert.equal(channelMissing(true, 5, []), false);
});

test("leaving a player opened from a link keeps that channel", () => {
  const held = { id: 4 };
  const fromUrl = { id: 7 };
  assert.equal(channelToDock(null, fromUrl), fromUrl);
  assert.equal(channelToDock(held, fromUrl), held);
  assert.equal(channelToDock(null, null), null);
});
