import assert from "node:assert/strict";
import fs from "node:fs";
import { test } from "node:test";
import { channelWatchName, guideCellId, guideCellName, guideShowsGrid, guideViewBox, keptRow, liveChannel, primeTime, recordingCountLabel, searchRows } from "./src/lib/guide.ts";

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

test("a guide cell says when the show is recording, new, or scored", () => {
  const span = "8:00 PM – 9:00 PM";
  assert.equal(guideCellName("Night Show", span, "KBWV"), `Night Show, ${span}, KBWV`);
  assert.equal(
    guideCellName("Bears at Bills", span, "WTST", { recording: "recording", fresh: true, score: "14–7" }),
    `Bears at Bills, Recording, New, 14–7, ${span}, WTST`,
  );
  assert.equal(
    guideCellName("Late Local News", span, "KBWV2", { recording: "scheduled" }),
    `Late Local News, Will record, ${span}, KBWV2`,
  );
});

test("a channel button names a favorite", () => {
  assert.equal(channelWatchName("4.1", "KBWV"), "Watch 4.1 KBWV");
  assert.equal(channelWatchName("4.1", "KBWV", { atsc3: true }), "Watch 4.1 KBWV, ATSC 3.0");
  assert.equal(channelWatchName("4.1", "KBWV", { atsc3: true, favorite: true }), "Watch 4.1 KBWV, ATSC 3.0, Favorite");
});

test("the recordings tab names how many are in progress", () => {
  assert.equal(recordingCountLabel(1), "1 recording");
  assert.equal(recordingCountLabel(4), "4 recordings");
});

test("the guide, the tab, and the recording dot use those names", () => {
  const guide = fs.readFileSync(new URL("./src/features/guide/Guide.tsx", import.meta.url), "utf8");
  const app = fs.readFileSync(new URL("./src/app/App.tsx", import.meta.url), "utf8");
  const dot = fs.readFileSync(new URL("./src/ui/primitives.tsx", import.meta.url), "utf8");
  assert.match(guide, /aria-label=\{guideCellName\(/);
  assert.match(guide, /aria-label=\{channelWatchName\(/);
  assert.match(app, /recordingCountLabel\(recordingCount\)/);
  assert.match(dot, /function RecDot[\s\S]*role="img"/);
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

test("a search that is loading or failed does not keep rows", () => {
  assert.deepEqual(searchRows({ status: "pending" }), { airings: [], recordings: [] });
  assert.deepEqual(searchRows({ status: "error" }), { airings: [], recordings: [] });
  assert.deepEqual(searchRows({ status: "done", airings: [{ id: 3 }], recordings: [{ id: 4 }] }), {
    airings: [{ id: 3 }],
    recordings: [{ id: 4 }],
  });
});
