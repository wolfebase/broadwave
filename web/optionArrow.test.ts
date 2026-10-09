import assert from "node:assert/strict";
import { test } from "node:test";
import { optionArrow } from "./src/lib/optionArrow.ts";

test("arrows move inside the open player options", () => {
  const rows = [3, 2, 1];
  assert.deepEqual(optionArrow(rows, 0, 0, "ArrowRight"), { row: 0, col: 1 });
  assert.deepEqual(optionArrow(rows, 0, 2, "ArrowRight"), { row: 0, col: 0 });
  assert.deepEqual(optionArrow(rows, 0, 0, "ArrowLeft"), { row: 0, col: 2 });
  assert.deepEqual(optionArrow(rows, 0, 1, "ArrowDown"), { row: 1, col: 1 });
  assert.deepEqual(optionArrow(rows, 0, 2, "ArrowDown"), { row: 1, col: 1 });
  assert.deepEqual(optionArrow(rows, 1, 0, "ArrowDown"), { row: 2, col: 0 });
  assert.deepEqual(optionArrow(rows, 2, 0, "ArrowDown"), { row: 2, col: 0 });
  assert.deepEqual(optionArrow(rows, 1, 1, "ArrowUp"), { row: 0, col: 1 });
  assert.deepEqual(optionArrow(rows, 0, 1, "ArrowUp"), { row: 0, col: 1 });
  assert.equal(optionArrow(rows, 0, 0, "Enter"), null);
  assert.equal(optionArrow(rows, -1, 0, "ArrowDown"), null);
  assert.equal(optionArrow([], 0, 0, "ArrowRight"), null);
});
