import assert from "node:assert/strict";
import { test } from "node:test";
import { escapeAction, guideListAction, guideRowOnFocus, ignoreHeldKey, liveArrowActs, pendingTuneFires, recordingSeekArrow, tvPictureEnter } from "./src/features/player/keys.ts";

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

test("Left and Right on a recording button move focus on a TV", () => {
  assert.equal(recordingSeekArrow(true, "ArrowLeft", false), false);
  assert.equal(recordingSeekArrow(true, "ArrowRight", false), false);
  assert.equal(recordingSeekArrow(true, "ArrowRight", true), true);
  assert.equal(recordingSeekArrow(false, "ArrowLeft", false), true);
  assert.equal(recordingSeekArrow(false, "ArrowRight", true), true);
  assert.equal(recordingSeekArrow(true, "ArrowDown", false), false);
});

test("an arrow on the center Play button does not change channel on a TV", () => {
  for (const key of ["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown"]) {
    assert.equal(liveArrowActs(true, key, false), false);
    assert.equal(liveArrowActs(false, key, false), true);
    assert.equal(liveArrowActs(true, key, true), true);
  }
  assert.equal(liveArrowActs(true, " ", false), false);
});

test("OK on the TV picture moves into the controls, and the arrows still zap", () => {
  assert.equal(tvPictureEnter(true, "Enter", true, false), true);
  assert.equal(tvPictureEnter(true, "ArrowDown", true, false), false);
  assert.equal(tvPictureEnter(true, "Enter", false, false), false);
  assert.equal(tvPictureEnter(true, "Enter", true, true), false);
  assert.equal(tvPictureEnter(false, "Enter", true, false), false);
});

test("Escape leaves fullscreen before it leaves the player", () => {
  assert.equal(escapeAction(true, true), "exit-fullscreen");
  assert.equal(escapeAction(true, false), "exit-fullscreen");
  assert.equal(escapeAction(false, true), "close-panel");
  assert.equal(escapeAction(false, false), "minimize");
});

test("mini-guide focus selects that row, and a repeated g does not close it", () => {
  assert.equal(guideRowOnFocus(2, 5, 0), 2);
  assert.equal(guideRowOnFocus(-1, 5, 0), 0);
  assert.equal(guideRowOnFocus(1.5, 5, 0), 0);
  assert.equal(guideListAction("g", false), "close");
  assert.equal(guideListAction("g", true), "ignore");
  assert.equal(guideListAction("ArrowDown", false), "down");
  assert.equal(guideListAction("GoBack", false), "close");
  assert.equal(guideListAction("XF86Back", false), "close");
  assert.equal(guideListAction("Back", false), "close");
});
