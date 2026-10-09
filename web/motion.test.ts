import assert from "node:assert/strict";
import fs from "node:fs";
import { test } from "node:test";
import { scrollBehavior } from "./src/lib/motion.ts";

test("reduced motion turns a smooth scroll into a jump", () => {
  assert.equal(scrollBehavior(false, "smooth"), "smooth");
  assert.equal(scrollBehavior(false), "smooth");
  assert.equal(scrollBehavior(true, "smooth"), "auto");
  assert.equal(scrollBehavior(true, "auto"), "auto");
});

test("reduced motion stops animation, transitions, and smooth scrolling", () => {
  const css = fs.readFileSync(new URL("./src/theme/base.css", import.meta.url), "utf8");
  const block = css.match(/@media \(prefers-reduced-motion: reduce\) \{([\s\S]*?)\n\}/);
  assert.ok(block, "missing reduced-motion rule");
  assert.match(block[1], /animation:\s*none\s*!important/);
  assert.match(block[1], /transition:\s*none\s*!important/);
  assert.match(block[1], /scroll-behavior:\s*auto\s*!important/);
});
