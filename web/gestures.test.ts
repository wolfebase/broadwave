import assert from "node:assert/strict";
import { test } from "node:test";
import { classifyTouch, seeksOnTap, swipeStep, tapZone, touchEcho } from "./src/features/player/gestures.ts";

test("a still touch is a tap, a long vertical drag is a swipe", () => {
  assert.equal(classifyTouch(3, 4, 120), "tap");
  assert.equal(classifyTouch(3, 4, 700), "none");
  assert.equal(classifyTouch(10, -90, 250), "swipe-up");
  assert.equal(classifyTouch(-8, 120, 300), "swipe-down");
  // Sideways is not a channel change.
  assert.equal(classifyTouch(90, 70, 250), "none");
  // A slow drag is reading, not swiping.
  assert.equal(classifyTouch(0, 200, 1500), "none");
});

test("double taps seek only on the outer thirds", () => {
  assert.equal(tapZone(50, 390), "left");
  assert.equal(tapZone(195, 390), "center");
  assert.equal(tapZone(350, 390), "right");
  assert.equal(seeksOnTap({ at: 1000, zone: "left" }, 1200, "left"), true);
  assert.equal(seeksOnTap({ at: 1000, zone: "left" }, 1500, "left"), false);
  assert.equal(seeksOnTap({ at: 1000, zone: "left" }, 1200, "right"), false);
  assert.equal(seeksOnTap({ at: 1000, zone: "center" }, 1100, "center"), false);
  assert.equal(seeksOnTap(null, 1100, "left"), false);
});

test("swipe up is channel up; a touch's click is not a second command", () => {
  assert.equal(swipeStep("swipe-up"), 1);
  assert.equal(swipeStep("swipe-down"), -1);
  assert.equal(swipeStep("tap"), 0);
  assert.equal(touchEcho(1000, 1500), true);
  assert.equal(touchEcho(1000, 2000), false);
});
