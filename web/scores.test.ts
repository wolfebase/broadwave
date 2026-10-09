import assert from "node:assert/strict";
import { test } from "node:test";
import { gamesFromScoreboard, teamScoreShown } from "./src/features/sports/board.ts";

test("a failed scoreboard does not clear the board", () => {
  assert.equal(gamesFromScoreboard(false, []), null);
  assert.equal(gamesFromScoreboard(false, undefined), null);
  assert.deepEqual(gamesFromScoreboard(true, []), []);
  assert.deepEqual(gamesFromScoreboard(true, undefined), []);
  assert.equal(gamesFromScoreboard(true, [{ id: "g1" }])?.length, 1);
});

test("a live zero is a score and a pregame zero is not", () => {
  assert.equal(teamScoreShown("0", "in"), true);
  assert.equal(teamScoreShown("0", "post"), true);
  assert.equal(teamScoreShown("0", "pre"), false);
  assert.equal(teamScoreShown("7", "pre"), true);
  assert.equal(teamScoreShown("", "in"), false);
  assert.equal(teamScoreShown(undefined, "in"), false);
});
