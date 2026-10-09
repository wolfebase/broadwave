import assert from "node:assert/strict";
import { test } from "node:test";
import { guideCellId, guideShowsGrid, guideSpan, guideViewBox, keptGuideWindow, keptRow, liveChannel, primeTime, reorderChannels, searchRows } from "./src/lib/guide.ts";

test("tonight is 8 PM, and the next day after that", () => {
  const afternoon = new Date(2026, 8, 26, 15, 0, 0).getTime();
  const eight = new Date(primeTime(afternoon));
  assert.equal(eight.getFullYear(), 2026);
  assert.equal(eight.getMonth(), 8);
  assert.equal(eight.getDate(), 26);
  assert.equal(eight.getHours(), 20);
  assert.equal(eight.getMinutes(), 0);

  const exactly = new Date(2026, 8, 26, 20, 0, 0).getTime();
  assert.equal(new Date(primeTime(exactly)).getDate(), 26);

  const later = new Date(2026, 8, 26, 23, 30, 0).getTime();
  const next = new Date(primeTime(later));
  assert.equal(next.getDate(), 27);
  assert.equal(next.getHours(), 20);
});

test("the guide cell the keyboard is on has an id the grid can name", () => {
  assert.equal(guideCellId(4, 18), "guide-cell-18");
  assert.equal(guideCellId(4), "guide-empty-4");
  assert.notEqual(guideCellId(4, 18), guideCellId(4));
});

test("keptRow follows the channel when the list shrinks", () => {
  const rows = [{ id: 1 }, { id: 2 }, { id: 3 }];
  assert.deepEqual(keptRow(rows, { row: 2, id: 1 }), { row: 0, id: 1 });
});

test("keptRow adopts the clamped row when that channel is gone", () => {
  const rows = [{ id: 4 }, { id: 5 }];
  assert.deepEqual(keptRow(rows, { row: 9, id: 8 }), { row: 1, id: 5 });
});

test("keptRow treats id 0 as no channel and adopts the clamped row", () => {
  assert.deepEqual(keptRow([{ id: 0 }, { id: 4 }], { row: 1, id: 0 }), { row: 1, id: 4 });
  assert.deepEqual(keptRow([{ id: 4 }, { id: 5 }], { row: 0, id: 0 }), { row: 0, id: 4 });
});

test("keptRow remembers the channel while no rows are on screen", () => {
  assert.deepEqual(keptRow([], { row: 3, id: 5 }), { row: 0, id: 5 });
  assert.deepEqual(keptRow([], { row: 3, id: 0 }), { row: 0, id: 0 });
});

test("the guide grid is phone landscape as well as desktop and tv", () => {
  assert.equal(guideShowsGrid("phone", false), false);
  assert.equal(guideShowsGrid("phone", true), true);
  assert.equal(guideShowsGrid("desktop", false), true);
  assert.equal(guideShowsGrid("tv", false), true);
});

test("a measure after cleanup or detach does not write the viewport", () => {
  const el = { isConnected: true, scrollTop: 40, scrollLeft: 80, clientHeight: 200, clientWidth: 320 };
  assert.deepEqual(guideViewBox(el, true), { top: 40, left: 80, height: 200, width: 320 });
  assert.equal(guideViewBox(el, false), null);
  assert.equal(guideViewBox({ ...el, isConnected: false }, true), null);
  assert.equal(guideViewBox(null, true), null);
});

test("liveChannel uses the refreshed row, including one the guide list hides", () => {
  const opened = { id: 7, favorite: false, hidden: true };
  const guide = [{ id: 4, favorite: false, hidden: false }];
  const all = [
    { id: 4, favorite: false, hidden: false },
    { id: 7, favorite: true, hidden: true },
  ];
  assert.equal(liveChannel(all, opened), all[1]);
  assert.equal(liveChannel(all, opened).favorite, true);
  assert.equal(liveChannel(guide, opened), opened);
  assert.equal(liveChannel(all, { id: 9, favorite: false, hidden: false }).id, 9);
});

test("a refresh keeps the two weeks the guide already loaded", () => {
  const now = Date.UTC(2026, 0, 15, 12, 0, 0);
  const span = guideSpan(now);
  assert.equal(span.from, new Date(now - 30 * 60_000).toISOString());
  assert.equal(span.to, new Date(now + 4 * 60 * 60_000).toISOString());
  assert.equal(span.restTo, new Date(now + 14 * 24 * 60 * 60_000).toISOString());
  assert.deepEqual(keptGuideWindow(now), { from: span.from, to: span.restTo });
});

test("dragging one visible channel leaves the hidden ones where they were", () => {
  assert.deepEqual(reorderChannels([5, 4, 3, 2, 1], 5, 3), [4, 3, 5, 2, 1]);
  assert.deepEqual(reorderChannels([5, 4, 3, 2, 1], 1, 4), [5, 1, 4, 3, 2]);
  assert.equal(reorderChannels([1, 2, 3], 2, 2), null);
  assert.equal(reorderChannels([1, 2, 3], 9, 1), null);
});

test("a search that is loading or failed does not keep rows", () => {
  assert.deepEqual(searchRows({ status: "pending" }), { airings: [], recordings: [] });
  assert.deepEqual(searchRows({ status: "error" }), { airings: [], recordings: [] });
  assert.deepEqual(searchRows({ status: "done", airings: [{ id: 3 }], recordings: [{ id: 4 }] }), {
    airings: [{ id: 3 }],
    recordings: [{ id: 4 }],
  });
});
