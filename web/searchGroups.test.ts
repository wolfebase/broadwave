import assert from "node:assert/strict";
import { test } from "node:test";
import { groupByShow, seriesChannelId } from "./src/features/search/groups.ts";

const at = (h: number) => new Date(Date.UTC(2026, 9, 16, h)).toISOString();
const now = Date.parse(at(12)) + 10 * 60_000;

test("one row per show, on now first, then the soonest", () => {
  const groups = groupByShow(
    [
      { id: 1, title: "NBC News Daily", start: at(13), end: at(14), channelId: 4 },
      { id: 2, title: "NBC News Daily ", start: at(12), end: at(13), channelId: 5, imageUrl: "x" },
      { id: 3, title: "Evening News", start: at(17), end: at(18), channelId: 4 },
      { id: 4, title: "Morning News", start: at(8), end: at(9), channelId: 4 },
      { id: 5, title: "Late News", start: at(12), end: at(13), channelId: 9 },
    ],
    now,
    (id) => id !== 9,
  );
  assert.deepEqual(groups.map((g) => g.title), ["NBC News Daily", "Late News", "Evening News"]);
  const nbc = groups[0];
  assert.equal(nbc.onNow?.id, 2);
  assert.equal(nbc.next?.id, 1);
  assert.equal(nbc.channels, 2);
  assert.equal(nbc.art?.id, 2);
  // A show on a channel this screen cannot play is not "on now" to watch.
  assert.equal(groups[1].onNow, undefined);
});

test("a show on several stations records on any channel", () => {
  const groups = groupByShow(
    [
      { id: 1, title: "Jeopardy!", start: at(13), end: at(14), channelId: 4 },
      { id: 2, title: "Jeopardy!", start: at(15), end: at(16), channelId: 7 },
      { id: 3, title: "Local Talk", start: at(13), end: at(14), channelId: 4 },
    ],
    now,
  );
  const show = (title: string) => groups.find((g) => g.title === title)!;
  assert.equal(seriesChannelId(show("Jeopardy!"), 4), 0);
  assert.equal(seriesChannelId(show("Local Talk"), 4), 4);
});
