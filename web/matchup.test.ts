import assert from "node:assert/strict";
import fs from "node:fs";
import { test } from "node:test";
import { setLocaleMode } from "./src/i18n/runtime.ts";
import { cardLabel, leagueLabel } from "./src/features/sports/matchup.ts";

test("a record button names the matchup, not the shared program title", () => {
  assert.equal(cardLabel({ title: "NFL Football", subtitle: "Chicago at Buffalo" }), "Chicago at Buffalo");
  assert.equal(cardLabel({ title: "NFL: Bears at Bills", subtitle: "Sunday night" }), "Bears at Bills");
  assert.equal(cardLabel({ title: "Golf", subtitle: "Final round" }), "Final round");
  assert.equal(cardLabel({ title: "Tennis" }), "Tennis");
});

test("a matchup name and a sport chip come from the catalog", () => {
  const naming = fs.readFileSync(new URL("./src/features/sports/matchup.ts", import.meta.url), "utf8");
  const page = fs.readFileSync(new URL("./src/features/sports/Sports.tsx", import.meta.url), "utf8");
  assert.equal(naming.includes("${sides[0]} at ${sides[1]}"), false);
  assert.match(page, /gc-league">\{leagueLabel\(league\(airing\)\)\}/);
  assert.match(page, /\{leagueLabel\(l\)\}/);
  assert.equal(leagueLabel("Soccer"), "Soccer");
  assert.equal(leagueLabel("NFL"), "NFL");
  setLocaleMode("pseudo");
  try {
    assert.equal(cardLabel({ title: "NFL: Bears at Bills", subtitle: "Sunday night" }), "Bears at BillsBears at Bills");
    assert.equal(cardLabel({ title: "Golf", subtitle: "Final round" }), "Final round");
    assert.equal(leagueLabel("College Football"), "College FootballCollege Football");
    assert.equal(leagueLabel("College Basketball"), "College BasketballCollege Basketball");
    assert.equal(leagueLabel("Soccer"), "SoccerSoccer");
    assert.equal(leagueLabel("Golf"), "GolfGolf");
    assert.equal(leagueLabel("Tennis"), "TennisTennis");
    assert.equal(leagueLabel("Olympics"), "OlympicsOlympics");
    assert.equal(leagueLabel("NFL"), "NFL");
    assert.equal(leagueLabel("UFC"), "UFC");
  } finally {
    setLocaleMode("en");
  }
});
