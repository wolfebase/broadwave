import assert from "node:assert/strict";
import { test } from "node:test";
import { alignedAhead, groupAxis, stepInGroup, type Bounds } from "./src/lib/focusStep.ts";

function box(left: number, top: number, width = 80, height = 36): Bounds {
  return { left, top, right: left + width, bottom: top + height };
}

test("a row steps sideways and stays at the end instead of jumping out", () => {
  const row = [box(0, 0), box(100, 0), box(200, 0)];
  assert.equal(groupAxis(row), "row");
  assert.equal(stepInGroup(0, row.length, "right", "row"), 1);
  assert.equal(stepInGroup(2, row.length, "right", "row"), "stay");
  assert.equal(stepInGroup(0, row.length, "left", "row"), "stay");
  assert.equal(stepInGroup(1, row.length, "down", "row"), null);
  assert.equal(stepInGroup(1, row.length, "up", "row"), null);
});

test("a column steps down and leaves on left and right", () => {
  const column = [box(0, 0), box(0, 50), box(0, 100)];
  assert.equal(groupAxis(column), "column");
  assert.equal(stepInGroup(0, column.length, "down", "column"), 1);
  assert.equal(stepInGroup(2, column.length, "down", "column"), "stay");
  assert.equal(stepInGroup(0, column.length, "up", "column"), "stay");
  assert.equal(stepInGroup(1, column.length, "right", "column"), null);
});

test("a wrapped grid is still a row, and Right stays at the end of the line", () => {
  const grid = [box(0, 0, 100, 40), box(110, 0, 100, 40), box(0, 50, 100, 40), box(110, 50, 100, 40)];
  assert.equal(groupAxis(grid), "row");
  assert.equal(stepInGroup(0, grid.length, "right", "row", grid), 1);
  assert.equal(stepInGroup(1, grid.length, "right", "row", grid), "stay");
  assert.equal(stepInGroup(2, grid.length, "right", "row", grid), 3);
  assert.equal(stepInGroup(0, grid.length, "down", "row", grid), null);
  assert.equal(stepInGroup(0, 1, "right", "row"), null);
  assert.equal(stepInGroup(-1, 3, "right", "row"), null);
});

test("Right at the end of a row reaches a control beside it, not the next line", () => {
  const chip = box(800, 0, 90, 36);
  const search = box(900, 2, 200, 36);
  const below = box(0, 80, 120, 36);
  assert.equal(alignedAhead(chip, [search, below], "right"), 0);
  assert.equal(alignedAhead(chip, [below], "right"), null);
  assert.equal(alignedAhead(chip, [box(700, 0)], "right"), null);
});

test("Down at the end of a column reaches a control under it", () => {
  const last = box(0, 200, 80, 36);
  const under = box(10, 250, 80, 36);
  const beside = box(400, 250, 80, 36);
  assert.equal(alignedAhead(last, [beside, under], "down"), 1);
  assert.equal(alignedAhead(last, [beside], "down"), null);
});
