import assert from "node:assert/strict";
import { test } from "node:test";
import { primeTime } from "./src/lib/guide.ts";

test("tonight is 8 PM, and the next day after that", () => {
  const afternoon = new Date(2026, 8, 26, 15, 0, 0).getTime();
  const eight = new Date(primeTime(afternoon));
  assert.equal(eight.getFullYear(), 2026);
  assert.equal(eight.getMonth(), 8);
  assert.equal(eight.getDate(), 26);
  assert.equal(eight.getHours(), 20);
  assert.equal(eight.getMinutes(), 0);

  const exactly = new Date(2026, 8, 26, 20, 0, 0).getTime();
  assert.equal(new Date(primeTime(exactly)).getDate(), 26);

  const later = new Date(2026, 8, 26, 23, 30, 0).getTime();
  const next = new Date(primeTime(later));
  assert.equal(next.getDate(), 27);
  assert.equal(next.getHours(), 20);
});
