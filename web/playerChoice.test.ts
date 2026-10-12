import assert from "node:assert/strict";
import { test } from "node:test";
import { goLiveAction } from "./src/features/player/goLive.ts";
import { clearPicturePick, nextPicturePick, shownPicture, visitSync } from "./src/features/player/livePref.ts";
import { pageFitsTiles } from "./src/features/player/quietStart.ts";
import { scrubHold, startOverChoice, swallowWakeClick } from "./src/features/player/startOver.ts";
import { gamesFromScoreboard, teamScoreShown } from "./src/features/sports/board.ts";

test("go live follows the group, arms sync, or plays", () => {
  assert.equal(goLiveAction(true, true), "group");
  assert.equal(goLiveAction(false, false), "arm");
  assert.equal(goLiveAction(false, true), "play");
});

test("start over prefers the live window, then a recording, then nothing", () => {
  assert.equal(startOverChoice(undefined, 0, 0), null);
  assert.equal(startOverChoice(Number.NaN, 0, 0), null);
  assert.equal(startOverChoice(100, 100, 50), "live");
  assert.equal(startOverChoice(100, 101, 100), "recording");
  assert.equal(startOverChoice(100, null, 101), null);
  assert.equal(scrubHold(null, 1), null);
  assert.equal(scrubHold(4, 5), 4);
  assert.equal(scrubHold(4, 4.2), null);
  assert.equal(scrubHold(4, Number.NaN), 4);
  assert.equal(swallowWakeClick(true, "touchstart"), true);
  assert.equal(swallowWakeClick(false, "touchstart"), false);
});

test("picture and visit choices keep a local pick until the server echoes it", () => {
  assert.equal(visitSync(true, false), true);
  assert.equal(visitSync(true, true), false);
  assert.equal(shownPicture(null, "nope"), "broadcast");
  assert.equal(shownPicture(null, "film"), "film");
  assert.equal(shownPicture("smooth", "film"), "smooth");
  assert.equal(clearPicturePick("film", "film"), null);
  assert.equal(nextPicturePick("film", "film", false), "film");
  assert.equal(nextPicturePick("film", "film", true), null);
});

test("a page fits when one tile, enough slots, or enough grants cover it", () => {
  assert.equal(pageFitsTiles(null, 1, 0), true);
  assert.equal(pageFitsTiles(2, 4, 0), false);
  assert.equal(pageFitsTiles(4, 4, 0), true);
  assert.equal(pageFitsTiles(null, 4, 4), true);
});

test("a failed scoreboard is not empty, and a zero counts only after the game starts", () => {
  assert.equal(gamesFromScoreboard(false, []), null);
  assert.deepEqual(gamesFromScoreboard(true, undefined), []);
  assert.equal(teamScoreShown("", "in"), false);
  assert.equal(teamScoreShown("0", "pre"), false);
  assert.equal(teamScoreShown("0", "in"), true);
  assert.equal(teamScoreShown("3", "pre"), true);
});
