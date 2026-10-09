import assert from "node:assert/strict";
import fs from "node:fs";
import { test } from "node:test";
import { channelWatchName, guideCellId, guideCellName, primeTime, recordingCountLabel } from "./src/lib/guide.ts";

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
