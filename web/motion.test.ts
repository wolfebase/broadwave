import assert from "node:assert/strict";
import { test } from "node:test";
import { scrollBehavior } from "./src/lib/motion.ts";

test("reduced motion turns a smooth scroll into a jump", () => {
  assert.equal(scrollBehavior(false, "smooth"), "smooth");
  assert.equal(scrollBehavior(false), "smooth");
  assert.equal(scrollBehavior(true, "smooth"), "auto");
  assert.equal(scrollBehavior(true, "auto"), "auto");
});
