import assert from "node:assert/strict";
import { test } from "node:test";
import { isFiction, isGame, leagueOf, playsIn } from "./src/features/sports/kind.ts";

test("a game has two sides or a scoreboard match; a talk show does not", () => {
  assert.equal(isGame({ title: "College Football", subtitle: "Washington at Purdue", category: "Sport, Football" }), true);
  assert.equal(isGame({ title: "High School Football", subtitle: "Lee's Summit at Blue Springs", category: "Sport, Football" }), true);
  assert.equal(isGame({ title: "NFL Football", category: "Sports event", gameId: "" }), true);
  assert.equal(isGame({ title: "Golf", category: "Sport", gameId: "401" }), true);
  assert.equal(isGame({ title: "SportsPulse", category: "Series, Sports talk" }), false);
  assert.equal(isGame({ title: "In Depth With Graham Bensinger", category: "Series, Interview, Sports talk" }), false);
  assert.equal(isGame({ title: "KC End Zone", category: "Sport, Football" }), false);
});

test("a sitcom about football is not on the sports page", () => {
  const sitcom = { title: "The Game", subtitle: "Stay Fierce, Malik", category: "Series, Comedy drama, Football" };
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
  assert.equal(leagueOf({ title: "Bears at Bills" }, "nfl"), "NFL");
  assert.equal(leagueOf({ title: "Game" }, "ncaaf"), "College Football");
  assert.equal(leagueOf({ title: "Royals at Tigers", category: "Sport, Baseball" }), "Baseball");
  assert.equal(leagueOf({ title: "El pelotazo", category: "Series, Sports talk" }), "");
});

test("a followed team is found by its name or its short name", () => {
  assert.equal(playsIn(["Chiefs", "Raiders"], { name: "Kansas City Chiefs", short: "Chiefs" }), true);
  assert.equal(playsIn(["Kansas City", "Denver"], { name: "Kansas City Chiefs", short: "Chiefs" }), true);
  assert.equal(playsIn(["Washington", "Purdue"], { name: "Kansas City Chiefs", short: "Chiefs" }), false);
  assert.equal(playsIn(null, { name: "Chiefs" }), false);
});
