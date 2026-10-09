import assert from "node:assert/strict";
import { test } from "node:test";
import { noteAfterFetch } from "./src/features/setup/fetchNote.ts";

const silent = "This network did not answer.";

test("a scan that answers clears only the failure note", () => {
  assert.equal(noteAfterFetch(silent, true, silent), "");
  assert.equal(noteAfterFetch("Tuner added.", true, silent), "Tuner added.");
  assert.equal(noteAfterFetch("", true, silent), "");
  assert.equal(noteAfterFetch("", false, silent), silent);
  assert.equal(noteAfterFetch("Tuner added.", false, silent), silent);
});
