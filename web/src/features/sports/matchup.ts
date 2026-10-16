import { sportsMessages } from "../../i18n/messages/sports.ts";
import { bind } from "../../i18n/runtime.ts";

const t = bind(sportsMessages);

/** Splits "Bears at Bills" or "Lakers vs. Celtics" into a matchup. */
export function matchup(a: { title: string; subtitle?: string }): [string, string] | null {
  const text = a.subtitle && / (at|vs\.?|@) /i.test(a.subtitle) ? a.subtitle : a.title;
  const m = text.match(/^(.*?)\s+(?:at|vs\.?|@)\s+(.*)$/i);
  if (!m) return null;
  const clean = (s: string) => s.replace(/^.*?:\s*/, "").trim();
  return [clean(m[1]), clean(m[2])];
}

/** The name on the card. Two games can share a program title and still be different matchups. */
export function cardLabel(a: { title: string; subtitle?: string }): string {
  const sides = matchup(a);
  if (sides) return t("sports.matchup", { away: sides[0], home: sides[1] });
  return a.subtitle || a.title;
}

const leagueKeys = {
  "College Football": "sports.league.collegeFootball",
  "College Basketball": "sports.league.collegeBasketball",
  "High School Football": "sports.league.highSchoolFootball",
  "High School Basketball": "sports.league.highSchoolBasketball",
  "Premier League": "sports.league.premierLeague",
  Olympics: "sports.league.olympics",
  Football: "sports.league.football",
  Basketball: "sports.league.basketball",
  Baseball: "sports.league.baseball",
  Hockey: "sports.league.hockey",
  Soccer: "sports.league.soccer",
  Golf: "sports.league.golf",
  Tennis: "sports.league.tennis",
  Racing: "sports.league.racing",
  Boxing: "sports.league.boxing",
  Wrestling: "sports.league.wrestling",
} as const;

/** Chip and eyebrow text. League acronyms stay as they are. */
export function leagueLabel(id: string): string {
  const key = (leagueKeys as Record<string, (typeof leagueKeys)[keyof typeof leagueKeys]>)[id];
  return key ? t(key) : id;
}
