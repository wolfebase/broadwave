import assert from "node:assert/strict";
import { test } from "node:test";
import { LatestReads, takeGames } from "./src/lib/latest.ts";

test("a later read wins, and a different list is left alone", () => {
  const reads = new LatestReads();
  const channels = reads.start(["channels"]);
  const oldRec = reads.start(["recordings"]);
  const fresh = reads.start(["recordings"]);
  assert.equal(channels("channels"), true);
  assert.equal(oldRec("recordings"), false);
  assert.equal(fresh("recordings"), true);
  assert.equal(oldRec("channels"), false);
  assert.equal(reads.generation("recordings"), 2);
  assert.equal(reads.generation("channels"), 1);
  assert.equal(reads.generation("airings"), 0);
});

test("one list refreshed again does not drop the other list from the same read", () => {
  const reads = new LatestReads();
  const first = reads.start(["channels", "recordings"]);
  const second = reads.start(["recordings"]);
  assert.equal(first("channels"), true);
  assert.equal(first("recordings"), false);
  assert.equal(second("recordings"), true);
  assert.equal(second("channels"), false);
});

test("an older scoreboard does not replace the newer one", () => {
  const seq = { n: 2 };
  const cache: { at: number; games: { id: string }[] } = { at: 5, games: [{ id: "new" }] };
  assert.equal(takeGames(seq, 1, [{ id: "old" }], cache, 9), null);
  assert.equal(cache.games[0].id, "new");
  assert.equal(cache.at, 5);
  const next = takeGames(seq, 2, [{ id: "newer" }], cache, 9);
  assert.equal(next?.[0].id, "newer");
  assert.equal(cache.at, 9);
  assert.equal(takeGames(seq, 2, null, cache, 11)?.[0].id, "newer");
  assert.equal(cache.at, 9);
  assert.equal(takeGames(seq, 2, [], cache, 12)?.length, 0);
  assert.equal(cache.at, 12);
});
