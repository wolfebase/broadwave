import assert from "node:assert/strict";
import { test } from "node:test";
import { noteAfterRecord, searchFieldValue } from "./src/features/search/field.ts";

test("back and forward show the query in the address, and typing keeps the box", () => {
  assert.equal(searchFieldValue("news", "sports", true), "news");
  assert.equal(searchFieldValue("news", "sports", false), "sports");
  assert.equal(searchFieldValue("", "a", false), "a");
});

test("recording every airing does not write its note onto the next query", () => {
  assert.equal(noteAfterRecord("news", "news", "Nightly", ""), "Recording every Nightly.");
  assert.equal(noteAfterRecord("news", "sports", "Nightly", "Nothing matches."), "Nothing matches.");
});
