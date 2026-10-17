import assert from "node:assert/strict";
import { test } from "node:test";
import { toneOf } from "./src/lib/logoTone.ts";

function pixels(...list: [number, number, number, number][]) {
  return list.flat();
}

test("a white logo on a clear background is light", () => {
  assert.equal(toneOf(pixels([255, 255, 255, 255], [250, 250, 250, 255], [0, 0, 0, 0], [0, 0, 0, 0])), "light");
});

test("a black logo is dark, a colored one is mixed", () => {
  assert.equal(toneOf(pixels([0, 0, 0, 255], [20, 20, 20, 255], [255, 255, 255, 0])), "dark");
  assert.equal(toneOf(pixels([120, 40, 200, 255], [255, 255, 255, 255], [10, 10, 10, 255])), "mixed");
});

test("a clear image has no tone", () => {
  assert.equal(toneOf(pixels([255, 255, 255, 0], [0, 0, 0, 10])), null);
});
