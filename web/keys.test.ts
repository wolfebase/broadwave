import assert from "node:assert/strict";
import { test } from "node:test";
import { escapeAction, ignoreHeldKey, pendingTuneFires } from "./src/features/player/keys.ts";

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

test("a typed channel does not tune after the player docks", () => {
  assert.equal(pendingTuneFires("full"), true);
  assert.equal(pendingTuneFires("mini"), false);
});

test("Escape leaves fullscreen before it leaves the player", () => {
  assert.equal(escapeAction(true, true), "exit-fullscreen");
  assert.equal(escapeAction(true, false), "exit-fullscreen");
  assert.equal(escapeAction(false, true), "close-panel");
  assert.equal(escapeAction(false, false), "minimize");
});
