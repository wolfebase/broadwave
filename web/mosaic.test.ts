import assert from "node:assert/strict";
import { test } from "node:test";
import { addMosaic, mosaicKey, mosaicShareMax, sharedMosaics } from "./src/lib/mosaic.ts";

test("a shared mosaic list splits on commas and an empty setting is one open slot", () => {
  assert.deepEqual(sharedMosaics(undefined), [""]);
  assert.deepEqual(sharedMosaics(""), [""]);
  assert.deepEqual(sharedMosaics("4-7,9-2"), ["4-7", "9-2"]);
});

test("the mosaic key puts the sound first and does not repeat it", () => {
  assert.equal(mosaicKey(7, [4, 7, 9]), "7-4-9");
  assert.equal(mosaicKey(4, [4]), "4");
});

test("a mosaic fills the first empty slot and keeps the later numbers", () => {
  const added = addMosaic(["4-7", "", "9-2"], "3-8");
  assert.equal(added.slot, 1);
  assert.deepEqual(added.slots, ["4-7", "3-8", "9-2"]);
});

test("a mosaic appends when every slot so far is taken", () => {
  const added = addMosaic(["4-7"], "3-8");
  assert.equal(added.slot, 1);
  assert.deepEqual(added.slots, ["4-7", "3-8"]);
});

test("adding a mosaic that is already shared does not take a second slot", () => {
  const added = addMosaic(["4-7", "3-8"], "4-7");
  assert.equal(added.slot, 0);
  assert.deepEqual(added.slots, ["4-7", "3-8"]);
});

test("other apps stop at eight mosaics", () => {
  const full = Array.from({ length: mosaicShareMax }, (_, i) => `1-${i + 2}`);
  const added = addMosaic(full, "9-9");
  assert.equal(added.slot, -1);
  assert.equal(added.slots.length, mosaicShareMax);
  assert.equal(added.slots.includes("9-9"), false);
});

test("a ninth stored slot is ignored and an empty one inside the eight is reused", () => {
  const slots = ["", ...Array.from({ length: mosaicShareMax }, () => "1-2")];
  const added = addMosaic(slots, "9-9");
  assert.equal(added.slot, 0);
  assert.equal(added.slots[0], "9-9");
  assert.equal(added.slots.length, mosaicShareMax);
});
