import assert from "node:assert/strict";
import { test } from "node:test";
import { isFiction, isGame, leagueOf, playsIn } from "./src/features/sports/kind.ts";
import { bareDate, cardLabel } from "./src/features/sports/matchup.ts";

test("a game has two sides or a scoreboard match; a talk show does not", () => {
  assert.equal(isGame({ title: "College Football", subtitle: "Washington at Purdue", category: "Sport, Football" }), true);
  assert.equal(isGame({ title: "High School Football", subtitle: "Northfield at Riverton", category: "Sport, Football" }), true);
  assert.equal(isGame({ title: "NFL Football", category: "Sports event", gameId: "" }), true);
  assert.equal(isGame({ title: "Golf", category: "Sport", gameId: "401" }), true);
  assert.equal(isGame({ title: "SportsPulse", category: "Series, Sports talk" }), false);
  assert.equal(isGame({ title: "In Depth With Graham Bensinger", category: "Series, Interview, Sports talk" }), false);
  assert.equal(isGame({ title: "Friday Night Highlights", category: "Sport, Football" }), false);
  assert.equal(isGame({ title: "Postgame Live at the Arena", category: "Sport, Basketball, Sports talk" }), false);
});

test("pro wrestling is entertainment, not a game, even with a versus", () => {
  assert.equal(isGame({ title: "Wrestling Night", subtitle: "Iron Ace vs. The Comet", category: "Sport, Pro wrestling" }), false);
  assert.equal(isGame({ title: "Wrestling Night", subtitle: "October 20th 2026", category: "Sport, Pro wrestling" }), false);
});

test("a subtitle that is only the air date is not a card title", () => {
  assert.equal(bareDate("October 20th 2026"), true);
  assert.equal(bareDate("Tuesday, Oct. 20, 2026"), true);
  assert.equal(bareDate("10/20/2026"), true);
  assert.equal(bareDate("2026-10-20"), true);
  assert.equal(bareDate("October Surprise"), false);
  assert.equal(bareDate("Game 7"), false);
  assert.equal(bareDate(undefined), false);
  assert.equal(cardLabel({ title: "Wrestling Night", subtitle: "October 20th 2026" }), "Wrestling Night");
  assert.equal(cardLabel({ title: "Golf", subtitle: "The Lakeside Open" }), "The Lakeside Open");
});

test("a sitcom about football is not on the sports page", () => {
  const sitcom = { title: "Mudtown Mavericks", subtitle: "The Wedding Episode", category: "Series, Comedy drama, Football" };
  assert.equal(isFiction(sitcom), true);
  assert.equal(isGame(sitcom), false);
  assert.equal(isFiction({ title: "College Football", category: "Sport, Football" }), false);
  assert.equal(isFiction({ title: "Any", category: "Drama", gameId: "9" }), false);
});

test("chips name a league, or the sport, never a show", () => {
  assert.equal(leagueOf({ title: "College Football", category: "Sport, Football" }), "College Football");
  assert.equal(leagueOf({ title: "High School Football", category: "Sport, Football" }), "High School Football");
  assert.equal(leagueOf({ title: "NFL: Bears at Bills" }), "NFL");
  assert.equal(leagueOf({ title: "WNBA Basketball" }), "WNBA");
  assert.equal(leagueOf({ title: "Women's College Basketball", subtitle: "Ridge at Lakeside" }), "Women's College Basketball");
  assert.equal(leagueOf({ title: "College Basketball" }), "College Basketball");
  assert.equal(leagueOf({ title: "Fútbol", subtitle: "Liga MX: Norte vs. Sur" }), "Liga MX");
  assert.equal(leagueOf({ title: "Racing" }, "indycar"), "IndyCar");
  assert.equal(leagueOf({ title: "Football" }, "fcs"), "College Football");
  assert.equal(leagueOf({ title: "Bears at Bills" }, "nfl"), "NFL");
  assert.equal(leagueOf({ title: "Game" }, "ncaaf"), "College Football");
  assert.equal(leagueOf({ title: "Comets at Tigers", category: "Sport, Baseball" }), "Baseball");
  assert.equal(leagueOf({ title: "El pelotazo", category: "Series, Sports talk" }), "");
});

test("a followed team is found by its name or its short name", () => {
  assert.equal(playsIn(["Comets", "Raiders"], { name: "Riverton Comets", short: "Comets" }), true);
  assert.equal(playsIn(["Riverton", "Denver"], { name: "Riverton Comets", short: "Comets" }), true);
  assert.equal(playsIn(["Washington", "Purdue"], { name: "Riverton Comets", short: "Comets" }), false);
  assert.equal(playsIn(null, { name: "Comets" }), false);
});
