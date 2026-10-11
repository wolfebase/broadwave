import assert from "node:assert/strict";
import { test } from "node:test";
import { groupAxis, stepInGroup, type Bounds } from "./src/lib/focusStep.ts";

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
