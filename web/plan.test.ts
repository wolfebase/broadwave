import assert from "node:assert/strict";
import { test } from "node:test";
import { heldOrder, releaseHeld, reloadAfterOrder, takeScheduleRead, takeScheduleWrite } from "./src/features/schedule/plan.ts";

test("an older schedule does not replace the fix the viewer just made", () => {
  assert.equal(takeScheduleRead(1, 2, 0, 1, [{ id: "old" }], 2), null);
  assert.equal(takeScheduleRead(2, 2, 0, 1, [{ id: "old" }], 2), null);
  assert.deepEqual(takeScheduleRead(3, 3, 1, 1, [{ id: "new" }], 2), { items: [{ id: "new" }], tunerCount: 2 });
  assert.equal(takeScheduleRead(3, 3, 1, 1, null, 2), null);
});

test("a second fix is the one that lands, and the first does not clear the busy button", () => {
  assert.equal(takeScheduleWrite(1, 2, [{ id: "first" }], 2), null);
  assert.deepEqual(takeScheduleWrite(2, 2, [{ id: "second" }], 1), { items: [{ id: "second" }], tunerCount: 1 });
  assert.equal(takeScheduleWrite(2, 2, null, 1), null);
});

test("only the last pass reorder reloads the list", () => {
  assert.equal(reloadAfterOrder(1, 2), false);
  assert.equal(reloadAfterOrder(2, 2), true);
});

test("a reorder being saved keeps its rows, and passes added or removed meanwhile still show", () => {
  assert.deepEqual(heldOrder([2, 1], [1, 2]), [1, 2]);
  assert.deepEqual(heldOrder([2, 1, 3], [1, 2]), [1, 2, 3]);
  assert.deepEqual(heldOrder([2], [1, 2]), [2]);
});

test("the held order waits for a list read after the save, not one from during it", () => {
  const during = [{ id: 2 }, { id: 1 }];
  const after = [{ id: 1 }, { id: 2 }];
  const held = { ids: [1, 2], saved: true, failed: false, list: during };
  assert.equal(releaseHeld(null, after, [1, 2]), false);
  assert.equal(releaseHeld({ ...held, saved: false }, after, [1, 2]), false);
  assert.equal(releaseHeld(held, during, [2, 1]), false);
  // The old order landing just after the save still waits.
  assert.equal(releaseHeld(held, [...during], [2, 1]), false);
  assert.equal(releaseHeld(held, after, [1, 2]), true);
  assert.equal(releaseHeld(held, after, [1, 3, 2]), true);
  // A save that failed shows the server's order on the next list.
  assert.equal(releaseHeld({ ...held, failed: true }, [...during], [2, 1]), true);
});
