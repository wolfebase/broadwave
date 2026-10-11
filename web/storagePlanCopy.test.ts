import assert from "node:assert/strict";
import { test } from "node:test";
import { removalWhen, spaceLine } from "./src/features/library/storagePlanCopy.ts";

const gb = 1_000_000_000;

test("a clean-up row is removed on its own", () => {
  assert.equal(removalWhen("now"), "Removed on its own.");
});

test("a keep rule waits until another episode of that show is saved", () => {
  assert.equal(removalWhen("next"), "Removed the next time another episode of this show is saved.");
});

test("a row that is not a removal has no time", () => {
  assert.equal(removalWhen(undefined), "");
  assert.equal(removalWhen(""), "");
  assert.equal(removalWhen("later"), "");
  assert.equal(removalWhen("Now"), "");
});

test("free space above the reserve names both rules when they are off", () => {
  assert.equal(
    spaceLine({ freeBytes: 120.4 * gb, reserveBytes: 10 * gb, makeRoom: false, deleteWatchedDays: 0 }),
    "120.4 GB free, above the 10.0 GB reserve. Making room is off. Deleting watched recordings is off.",
  );
});

test("free space under the reserve names making room and the day count", () => {
  assert.equal(
    spaceLine({ freeBytes: 4.2 * gb, reserveBytes: 10 * gb, makeRoom: true, deleteWatchedDays: 7 }),
    "4.2 GB free, under the 10.0 GB reserve. Making room is on. Watched recordings are deleted after 7 days.",
  );
});

test("free space sitting on the reserve is not under it", () => {
  assert.equal(
    spaceLine({ freeBytes: 10 * gb, reserveBytes: 10 * gb, makeRoom: false, deleteWatchedDays: 1 }),
    "10.0 GB free, at the 10.0 GB reserve. Making room is off. Watched recordings are deleted after 1 day.",
  );
});

test("a reserve of zero is off, and making room does not mean the days rule is on", () => {
  assert.equal(
    spaceLine({ freeBytes: 80 * gb, reserveBytes: 0, makeRoom: true, deleteWatchedDays: 0 }),
    "80.0 GB free. The free-space reserve is off. Making room is on. Deleting watched recordings is off.",
  );
});
