import assert from "node:assert/strict";
import { test } from "node:test";
import { createBreakScans } from "./src/features/recordings/breaks.ts";
import { copy } from "./src/strings.ts";

test("a commercial scan says how many breaks it found", () => {
  assert.equal(copy.library.foundBreaks(0), "No breaks found.");
  assert.equal(copy.library.foundBreaks(1), "Found 1 break.");
  assert.equal(copy.library.foundBreaks(3), "Found 3 breaks.");
});

test("a scan already running is not started again", () => {
  const scans = createBreakScans();
  let notes = 0;
  scans.subscribe(() => {
    notes += 1;
  });
  assert.equal(scans.begin(4), true);
  assert.equal(scans.begin(4), false);
  assert.equal(scans.get(4).running, true);
  assert.equal(scans.get(4).note, "");
  assert.equal(scans.get(4), scans.get(4));
  const markers = [
    { id: 1, start: 12, end: 40 },
    { id: 2, start: 90, end: 120 },
    { id: 3, start: 200, end: 230 },
  ];
  scans.finish(4, markers, copy.library.foundBreaks(markers.length));
  assert.equal(scans.get(4).running, false);
  assert.equal(scans.get(4).note, "Found 3 breaks.");
  assert.deepEqual(scans.get(4).markers, markers);
  assert.equal(scans.begin(4), true);
  scans.finish(4, [], copy.library.foundBreaks(0));
  assert.equal(scans.get(4).note, "No breaks found.");
  scans.fail(4, copy.library.findFailed);
  assert.equal(scans.get(4).running, false);
  assert.equal(scans.get(4).note, "Could not look for commercials. Try again.");
  assert.equal(notes, 5);
});
