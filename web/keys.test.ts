import assert from "node:assert/strict";
import { test } from "node:test";
import { ignoreHeldKey } from "./src/features/player/keys.ts";

const up = { repeat: false, metaKey: false, ctrlKey: false, altKey: false };
const held = { ...up, repeat: true };

test("a held Space is ignored", () => {
  assert.equal(ignoreHeldKey(" ", held), true);
});

test("a held r is ignored", () => {
  assert.equal(ignoreHeldKey("r", held), true);
});

test("a held c is ignored", () => {
  assert.equal(ignoreHeldKey("c", held), true);
});

test("Ctrl+C is Copy even on the first press", () => {
  assert.equal(ignoreHeldKey("c", { ...up, ctrlKey: true }), true);
});

test("a held arrow still repeats", () => {
  assert.equal(ignoreHeldKey("ArrowLeft", held), false);
});

test("Space on the first press is not ignored", () => {
  assert.equal(ignoreHeldKey(" ", up), false);
});

test("a held m is ignored", () => {
  assert.equal(ignoreHeldKey("m", held), true);
});
