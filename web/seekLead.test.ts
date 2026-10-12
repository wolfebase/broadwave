import assert from "node:assert/strict";
import { test } from "node:test";
import { nextSeekLead } from "./src/lib/seekLead.ts";

test("the next seek aims less far ahead when the last one overshot", () => {
  assert.equal(nextSeekLead(1, 500), 0.5);
  assert.equal(nextSeekLead(0.25, 500), 0);
  assert.equal(nextSeekLead(3, -100), 2);
});
