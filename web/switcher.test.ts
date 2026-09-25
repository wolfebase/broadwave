import assert from "node:assert/strict";
import { test } from "node:test";
import { pickFocus, type SwitchGame } from "./src/features/multiview/switcher.ts";

function game(partial: SwitchGame): SwitchGame {
  return { state: "in", teams: [], ...partial, teams: partial.teams ?? [] };
}

const zone = game({
  id: "nfl-redzone",
  league: "nfl",
  period: 3,
  clock: "8:12",
  detail: "3rd 8:12",
  redZone: true,
  teams: [
    { name: "Raiders", abbr: "LV", score: "14", home: true },
    { name: "Chiefs", abbr: "KC", score: "17" },
  ],
});

const power = game({
  id: "nhl-pp",
  league: "nhl",
  period: 2,
  clock: "12:04",
  detail: "2nd 12:04",
  powerPlay: true,
  teams: [
    { name: "Rangers", abbr: "NYR", score: "1", home: true },
    { name: "Bruins", abbr: "BOS", score: "1" },
  ],
});

const close = game({
  id: "nfl-close",
  league: "nfl",
  period: 4,
  clock: "3:20",
  detail: "4th 3:20",
  teams: [
    { name: "Dolphins", abbr: "MIA", score: "24", home: true },
    { name: "Bills", abbr: "BUF", score: "21" },
  ],
});

const lead = game({
  id: "nfl-lead",
  league: "nfl",
  period: 2,
  clock: "10:00",
  detail: "2nd 10:00",
  teams: [
    { name: "Eagles", abbr: "PHI", score: "10", home: true },
    { name: "Cowboys", abbr: "DAL", score: "14" },
  ],
});

const now = Date.parse("2026-09-25T20:00:00Z");

test("red zone wins over a close finish", () => {
  const got = pickFocus(now, 0, [close, zone], []);
  assert.equal(got.keepManual, false);
  assert.equal(got.gameId, "nfl-redzone");
  assert.equal(got.banner, "Red zone: KC at LV");
});

test("power play wins over a lead change", () => {
  const prev = game({ ...lead, teams: [
    { name: "Eagles", abbr: "PHI", score: "14", home: true },
    { name: "Cowboys", abbr: "DAL", score: "7" },
  ] });
  const got = pickFocus(now, 0, [lead, power], [prev]);
  assert.equal(got.gameId, "nhl-pp");
  assert.equal(got.banner, "Power play: BOS at NYR");
});

test("a flipped lead beats the final minutes", () => {
  const prev = game({ ...lead, teams: [
    { name: "Eagles", abbr: "PHI", score: "14", home: true },
    { name: "Cowboys", abbr: "DAL", score: "7" },
  ] });
  const got = pickFocus(now, 0, [close, lead], [prev]);
  assert.equal(got.gameId, "nfl-lead");
  assert.equal(got.banner, "Lead change: DAL at PHI");
});

test("final minutes use the clock or the detail line", () => {
  const got = pickFocus(now, 0, [close], []);
  assert.equal(got.banner, "Final minutes: BUF at MIA");
  const named = game({ ...close, period: 0, clock: "", detail: "4th 2:05" });
  assert.equal(pickFocus(now, 0, [named], []).banner, "Final minutes: BUF at MIA");
  const five = game({ ...close, clock: "5:00", detail: "4th 5:00" });
  assert.equal(pickFocus(now, 0, [five], []).gameId, "");
});

test("a manual choice holds for under 2 minutes", () => {
  const held = pickFocus(now, now - 119_000, [close, zone], []);
  assert.equal(held.keepManual, true);
  assert.equal(held.gameId, "");
  assert.equal(held.banner, "");
  const free = pickFocus(now, now - 120_000, [close, zone], []);
  assert.equal(free.keepManual, false);
  assert.equal(free.gameId, "nfl-redzone");
});
