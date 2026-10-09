import assert from "node:assert/strict";
import { test } from "node:test";
import { menuStep } from "./src/lib/menuStep.ts";

test("menu arrows wrap, and a closed menu opens at the end the key points to", () => {
  assert.equal(menuStep(4, -1, "ArrowDown"), 0);
  assert.equal(menuStep(4, -1, "Home"), 0);
  assert.equal(menuStep(4, -1, "ArrowUp"), 3);
  assert.equal(menuStep(4, -1, "End"), 3);
  assert.equal(menuStep(4, 1, "ArrowDown"), 2);
  assert.equal(menuStep(4, 1, "ArrowRight"), 2);
  assert.equal(menuStep(4, 1, "ArrowUp"), 0);
  assert.equal(menuStep(4, 0, "ArrowUp"), 3);
  assert.equal(menuStep(4, 3, "ArrowDown"), 0);
  assert.equal(menuStep(4, 2, "Home"), 0);
  assert.equal(menuStep(4, 2, "End"), 3);
  assert.equal(menuStep(0, 0, "ArrowDown"), null);
  assert.equal(menuStep(4, 1, "Enter"), null);
});
