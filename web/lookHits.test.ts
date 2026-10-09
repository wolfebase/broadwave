import assert from "node:assert/strict";
import { test } from "node:test";
import { guideAfterFree, hitsAfterLook } from "./src/features/setup/lookHits.ts";

const shown = [
  { kind: "tuner", name: "one", addr: "a" },
  { kind: "tuner", name: "two", addr: "b" },
];
const next = [{ kind: "tuner", name: "three", addr: "c" }];

test("a failed look keeps the hits already shown", () => {
  assert.equal(hitsAfterLook(1, 1, shown, false, undefined), shown);
  assert.equal(hitsAfterLook(1, 1, shown, false, []), shown);
  assert.equal(hitsAfterLook(1, 1, shown, false, next), shown);
});

test("a stale success or stale failure writes nothing", () => {
  assert.equal(hitsAfterLook(1, 2, shown, true, next), null);
  assert.equal(hitsAfterLook(1, 2, shown, true, []), null);
  assert.equal(hitsAfterLook(1, 2, shown, false, undefined), null);
  assert.equal(hitsAfterLook(1, 2, shown, false, next), null);
});

test("a current success with found:[] returns []", () => {
  assert.deepEqual(hitsAfterLook(2, 2, shown, true, []), []);
  assert.deepEqual(hitsAfterLook(2, 2, shown, true, null), []);
  assert.deepEqual(hitsAfterLook(2, 2, shown, true, undefined), []);
});

test("a current success replaces the list", () => {
  assert.deepEqual(hitsAfterLook(2, 2, shown, true, next), next);
});

test("a failed free-channel look keeps the guide text", () => {
  assert.equal(guideAfterFree(false, undefined, "one"), null);
  assert.equal(guideAfterFree(false, [], "one"), null);
  assert.equal(guideAfterFree(true, next, "one"), "");
  assert.equal(guideAfterFree(true, [], "one"), "one");
  assert.equal(guideAfterFree(true, null, undefined), "");
});
