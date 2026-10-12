import assert from "node:assert/strict";
import { test } from "node:test";
import { tileArrow } from "./src/features/multiview/tileArrow.ts";

test("an arrow at the edge of a multiview leaves the grid", () => {
  assert.equal(tileArrow(1, 0, "ArrowUp", 1), null);
  assert.equal(tileArrow(1, 0, "ArrowDown", 1), null);
  assert.equal(tileArrow(1, 0, "ArrowLeft", 1), null);
  assert.equal(tileArrow(1, 0, "ArrowRight", 1), null);
  assert.equal(tileArrow(4, 0, "ArrowUp", 2), null);
  assert.equal(tileArrow(4, 0, "ArrowLeft", 2), null);
  assert.equal(tileArrow(4, 3, "ArrowDown", 2), null);
  assert.equal(tileArrow(4, 3, "ArrowRight", 2), null);
  assert.equal(tileArrow(3, 0, "ArrowUp", 1), null);
  assert.equal(tileArrow(3, 2, "ArrowDown", 1), null);
  assert.equal(tileArrow(0, 0, "ArrowDown", 2), null);
  assert.equal(tileArrow(4, 1, "Enter", 2), null);
  assert.equal(tileArrow(4, 1, "Escape", 2), null);

  assert.equal(tileArrow(4, 0, "ArrowRight", 2), 1);
  assert.equal(tileArrow(4, 1, "ArrowLeft", 2), 0);
  assert.equal(tileArrow(4, 1, "ArrowRight", 2), 2);
  assert.equal(tileArrow(4, 0, "ArrowDown", 2), 2);
  assert.equal(tileArrow(4, 1, "ArrowDown", 2), 3);
  assert.equal(tileArrow(4, 2, "ArrowDown", 2), 3);
  assert.equal(tileArrow(4, 2, "ArrowUp", 2), 0);
  assert.equal(tileArrow(4, 3, "ArrowUp", 2), 1);
  assert.equal(tileArrow(3, 0, "ArrowDown", 1), 1);
  assert.equal(tileArrow(3, 1, "ArrowRight", 1), 2);
});
