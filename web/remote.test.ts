import assert from "node:assert/strict";
import { test } from "node:test";
import { nearest, type Box } from "./src/lib/remote.ts";

function box(id: number, left: number, top: number, width = 40, height = 20): Box {
  return { id, left, top, right: left + width, bottom: top + height };
}

test("arrows walk to the nearest control in that direction", () => {
  const tabs = [box(1, 100, 10, 80, 30), box(2, 200, 10, 80, 30), box(3, 300, 10, 80, 30)];
  const watch = box(4, 120, 200, 100, 40);
  const card = box(5, 110, 320, 120, 80);
  const items = [...tabs, watch, card];
  assert.equal(nearest(tabs[1], items, "right"), 3);
  assert.equal(nearest(tabs[1], items, "left"), 1);
  assert.equal(nearest(tabs[1], items, "down"), 4);
  assert.equal(nearest(watch, items, "down"), 5);
  assert.equal(nearest(card, items, "up"), 4);
  assert.equal(nearest(tabs[0], items, "left"), null);
});

test("a wide row below the tabs wins over a nearer control off to the side", () => {
  const tab = box(1, 400, 10, 80, 30);
  const side = box(2, 40, 80, 80, 30);
  const grid = box(3, 0, 160, 800, 400);
  assert.equal(nearest(tab, [tab, side, grid], "down"), 3);
});
