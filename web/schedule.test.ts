import assert from "node:assert/strict";
import { test } from "node:test";
import { formatClock } from "./src/time.ts";
import { missedLine } from "./src/features/schedule/missed.ts";

test("a one-shot names every showing it will not record", () => {
  const first = new Date(2026, 5, 15, 19, 0, 0);
  const second = new Date(2026, 5, 16, 19, 0, 0);
  const third = new Date(2026, 5, 17, 19, 0, 0);
  assert.equal(
    missedLine([{ title: "News", start: first.toISOString() }]),
    `News at ${formatClock(first)} will not record.`,
  );
  assert.equal(
    missedLine([
      { title: "News", start: first.toISOString(), guideNumber: "9.1" },
      { title: "Game", start: second.toISOString(), guideNumber: "4.1" },
    ]),
    `News at ${formatClock(first)} on 9.1 and Game at ${formatClock(second)} on 4.1 will not record.`,
  );
  assert.equal(
    missedLine([
      { title: "News", start: first.toISOString() },
      { title: "Game", start: second.toISOString() },
      { title: "  ", start: third.toISOString() },
    ]),
    `News at ${formatClock(first)}, Game at ${formatClock(second)}, and A show at ${formatClock(third)} will not record.`,
  );
});
