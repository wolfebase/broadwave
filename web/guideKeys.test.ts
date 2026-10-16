import assert from "node:assert/strict";
import { test } from "node:test";
import { guideEnterAction, guidePageRow, isInfoKey, slotUnderNow } from "./src/lib/guideKeys.ts";

test("page keys move a screenful of rows and stop at the ends", () => {
  // 800 px view, 48 px head, 68 px rows: 11 rows fit, 10 move.
  assert.equal(guidePageRow(0, 1, 800, 68, 48, 40), 10);
  assert.equal(guidePageRow(10, -1, 800, 68, 48, 40), 0);
  assert.equal(guidePageRow(35, 1, 800, 68, 48, 40), 39);
  assert.equal(guidePageRow(3, -1, 800, 68, 48, 40), 0);
  // A view shorter than two rows still moves one.
  assert.equal(guidePageRow(4, 1, 100, 68, 48, 40), 5);
});

test("the half-hour label under the now pill steps aside", () => {
  assert.equal(slotUnderNow(500, 510, 40), true);
  assert.equal(slotUnderNow(560, 510, 40), false);
  // A label that ends where the pill begins is covered too.
  assert.equal(slotUnderNow(400, 510, 40), true);
  assert.equal(slotUnderNow(330, 510, 40), false);
  // On a TV the label is wider.
  assert.equal(slotUnderNow(330, 510, 60, 110), true);
});

test("OK on a TV tunes the show on now; holding it or Info opens the details", () => {
  assert.equal(guideEnterAction(true, true, false), "watch");
  assert.equal(guideEnterAction(true, true, true), "sheet");
  assert.equal(guideEnterAction(true, false, false), "sheet");
  assert.equal(guideEnterAction(false, true, false), "sheet");
  assert.equal(isInfoKey("i"), true);
  assert.equal(isInfoKey("ContextMenu"), true);
  assert.equal(isInfoKey("Enter"), false);
});
