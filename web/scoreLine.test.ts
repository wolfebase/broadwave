import { expect, test } from "vitest";
import { scoreLine, type ScoreGame } from "./src/features/sports/scores.ts";

function game(extra: Partial<ScoreGame> = {}): ScoreGame {
  return {
    id: "city-den",
    state: "in",
    teams: [
      { name: "City News", abbr: "CNO", score: "3" },
      { name: "Den TV", abbr: "DEN", home: true, score: "7" },
    ],
    ...extra,
  };
}

test("a score line names the visitors first and prefers a short name", () => {
  expect(scoreLine(game())).toBe("CNO 3 · DEN 7");
  expect(scoreLine(game({ teams: [{ name: "City News", short: "City", score: "1" }, { name: "Den TV", home: true, score: "2" }] }))).toBe("City 1 · Den TV 2");
  expect(scoreLine(game({ teams: [{ name: "City News", score: "1" }, { name: "Den TV", home: true, score: "2" }] }))).toBe("City News 1 · Den TV 2");
});

test("a game that has not started, or a side with no score, has no line", () => {
  expect(scoreLine(undefined)).toBe("");
  expect(scoreLine(game({ state: "pre" }))).toBe("");
  expect(scoreLine(game({ teams: [] }))).toBe("");
  expect(scoreLine(game({ teams: [{ name: "Den TV", home: true, score: "7" }] }))).toBe("");
  expect(scoreLine(game({ teams: [{ name: "City News", score: "3" }, { name: "Den TV", home: true }] }))).toBe("");
  expect(scoreLine(game({ teams: [{ name: "City News", score: "" }, { name: "Den TV", home: true, score: "0" }] }))).toBe("");
});

test("a zero on the board is a score once the game is underway", () => {
  const tied = game({
    state: "in",
    teams: [
      { name: "City News", abbr: "CNO", score: "0" },
      { name: "Den TV", abbr: "DEN", home: true, score: "0" },
    ],
  });
  expect(scoreLine(tied)).toBe("CNO 0 · DEN 0");
  expect(scoreLine({ ...tied, state: "post" })).toBe("CNO 0 · DEN 0");
});
