import assert from "node:assert/strict";
import { test } from "node:test";
import { clearPicturePick, nextPicturePick, shownPicture, storedSync, visitSync } from "./src/features/player/livePref.ts";

test("a detach turns sync off for this visit and leaves the saved checkbox", () => {
  assert.equal(visitSync(true, true), false);
  assert.equal(storedSync(true, true), true);
  assert.equal(storedSync(true, false), true);
  assert.equal(storedSync(false, true), false);
  assert.equal(visitSync(true, false), true);
  assert.equal(visitSync(false, false), false);
});

test("picture follows settings that arrive after the default", () => {
  assert.equal(shownPicture(null, "broadcast"), "broadcast");
  assert.equal(shownPicture(null, "film"), "film");
  assert.equal(shownPicture(null, undefined), "broadcast");
  assert.equal(shownPicture("smooth", "film"), "smooth");
  assert.equal(clearPicturePick("smooth", "film"), "smooth");
  assert.equal(clearPicturePick("smooth", "smooth"), null);
  assert.equal(nextPicturePick("broadcast", "broadcast", false), "broadcast");
  assert.equal(nextPicturePick("broadcast", "film", true), "broadcast");
  assert.equal(nextPicturePick("broadcast", "broadcast", true), null);
  assert.equal(shownPicture(nextPicturePick(null, "film", true), "film"), "film");
});
