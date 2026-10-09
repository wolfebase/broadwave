import assert from "node:assert/strict";
import { test } from "node:test";
import { reloadAfterOrder, takeScheduleRead, takeScheduleWrite } from "./src/features/schedule/plan.ts";

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
