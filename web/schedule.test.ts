import assert from "node:assert/strict";
import { test } from "node:test";
import { formatClock } from "./src/time.ts";
import { missedLine } from "./src/features/schedule/missed.ts";

test("a one-shot names every showing it will not record", () => {
  const first = new Date(2026, 5, 15, 19, 0, 0);
  const second = new Date(2026, 5, 16, 19, 0, 0);
  const third = new Date(2026, 5, 17, 19, 0, 0);
  const when = (date: Date) => `${["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"][date.getMonth()]} ${date.getDate()} at ${formatClock(date)}`;
  assert.equal(
    missedLine([{ title: "News", start: first.toISOString() }]),
    `News on ${when(first)} will not record.`,
  );
  assert.notEqual(when(first), when(second));
  assert.equal(
    missedLine([
      { title: "News", start: first.toISOString(), guideNumber: "9.1" },
      { title: "Game", start: second.toISOString(), guideNumber: "4.1" },
    ]),
    `News on ${when(first)} on 9.1 and Game on ${when(second)} on 4.1 will not record.`,
  );
  assert.equal(
    missedLine([
      { title: "News", start: first.toISOString() },
      { title: "Game", start: second.toISOString() },
      { title: "  ", start: third.toISOString() },
    ]),
    `News on ${when(first)}, Game on ${when(second)}, and A show on ${when(third)} will not record.`,
  );
});
