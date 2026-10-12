import assert from "node:assert/strict";
import { test } from "node:test";
import { noteAfterRecord, noteTone, searchFieldValue } from "./src/features/search/field.ts";
import { setLocaleMode } from "./src/i18n/runtime.ts";

test("back and forward show the query in the address, and typing keeps the box", () => {
  assert.equal(searchFieldValue("news", "sports", true), "news");
  assert.equal(searchFieldValue("news", "sports", false), "sports");
  assert.equal(searchFieldValue("", "a", false), "a");
});

test("recording every airing does not write its note onto the next query", () => {
  assert.equal(noteAfterRecord("news", "news", "Nightly", ""), "Recording every Nightly.");
  assert.equal(noteAfterRecord("news", "sports", "Nightly", "Nothing matches."), "Nothing matches.");
});

test("a saved pass is a status even when the sentence is stretched", () => {
  assert.equal(noteTone(noteAfterRecord("news", "news", "Nightly", "")), "status");
  assert.equal(noteTone(noteAfterRecord("news", "news", "Mr. Smith", "")), "status");
  assert.equal(noteTone("Nothing matches."), "status");
  assert.equal(noteTone("Search failed."), "alert");
  assert.equal(noteTone("Recording every Nightly"), "alert");
  setLocaleMode("pseudo");
  try {
    assert.equal(noteTone(noteAfterRecord("news", "news", "Nightly", "")), "status");
    assert.equal(noteTone("Nothing matches.Nothing matches."), "status");
    assert.equal(noteTone("Search failed.Search failed."), "alert");
  } finally {
    setLocaleMode("en");
  }
});
