import assert from "node:assert/strict";
import { test } from "node:test";
import { cappedCss } from "./src/lib/art.ts";

test("a search thumb stays within 1.25× the picture", () => {
  assert.equal(cappedCss(40, 84, 2), 25);
  assert.equal(cappedCss(480, 84, 2), 84);
  assert.equal(cappedCss(40, 84, 1), 50);
  assert.equal(cappedCss(0, 84, 2), 0);
});
