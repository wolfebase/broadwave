import { isFiction } from "../../lib/guide.ts";
import { isGame, type Listing } from "./listing.ts";

export { isGame };

/** Listings the sports page leaves out altogether. */
export { isFiction };

// Board league ids (the scoreboard API) to the names listings use.
const boardLeagues: Record<string, string> = {
  nfl: "NFL",
  ncaaf: "College Football",
  fcs: "College Football",
  nba: "NBA",
  wnba: "WNBA",
  ncaab: "College Basketball",
  ncaaw: "Women's College Basketball",
  mlb: "MLB",
  nhl: "NHL",
  mls: "MLS",
  nwsl: "NWSL",
  epl: "Premier League",
  ligamx: "Liga MX",
  f1: "Formula 1",
  nascar: "NASCAR",
  indycar: "IndyCar",
};

const named: [string, RegExp][] = [
  ["NFL", /\bNFL\b/i],
  ["WNBA", /\bWNBA\b/i],
  ["NBA", /\bNBA\b/i],
  ["MLB", /\bMLB\b/i],
  ["NHL", /\bNHL\b/i],
  ["MLS", /\bMLS\b/i],
  ["NWSL", /\bNWSL\b/i],
  ["NASCAR", /\bNASCAR\b/i],
  ["IndyCar", /\bIndyCar\b/i],
  ["CFL", /\bCFL\b/i],
  ["Liga MX", /\bliga mx\b/i],
  ["Formula 1", /\b(formula (1|one)|F1)\b/i],
  ["PGA", /\bPGA\b/i],
  ["UFC", /\bUFC\b/i],
  ["Premier League", /\bpremier league\b/i],
  ["College Football", /\b(college|NCAA) football\b/i],
  ["Women's College Basketball", /\bwomen'?s (college|NCAA) basketball\b/i],
  ["College Basketball", /\b(college|NCAA) basketball\b/i],
  ["High School Football", /\bhigh school football\b/i],
  ["High School Basketball", /\bhigh school basketball\b/i],
  ["Olympics", /\bolympics?\b/i],
];

const sports: [string, RegExp][] = [
  ["Football", /\bfootball\b/i],
  ["Basketball", /\bbasketball\b/i],
  ["Baseball", /\bbaseball\b/i],
  ["Hockey", /\bhockey\b/i],
  ["Soccer", /\bsoccer\b/i],
  ["Golf", /\bgolf\b/i],
  ["Tennis", /\btennis\b/i],
  ["Racing", /\b(auto racing|motorsports?|racing)\b/i],
  ["Boxing", /\bboxing\b/i],
  ["MMA", /\b(MMA|mixed martial arts)\b/i],
  ["Wrestling", /\bwrestling\b/i],
];

/** The league a game belongs to, or its sport. Empty when nothing says. */
export function leagueOf(a: Listing, boardLeague?: string): string {
  const fromBoard = boardLeague ? boardLeagues[boardLeague.toLowerCase()] : undefined;
  if (fromBoard) return fromBoard;
  const text = `${a.title} ${a.subtitle ?? ""} ${a.category ?? ""}`;
  const league = named.find(([, re]) => re.test(text));
  if (league) return league[0];
  const sport = sports.find(([, re]) => re.test(a.category ?? "") || re.test(a.title));
  return sport ? sport[0] : "";
}

/** True when a followed team plays in this game. */
export function playsIn(sides: [string, string] | null, team: { name: string; short?: string }): boolean {
  if (!sides) return false;
  const names = [team.name, team.short].filter(Boolean).map((n) => n!.toLowerCase());
  return sides.some((side) => {
    const s = side.toLowerCase();
    return names.some((n) => s === n || s.endsWith(` ${n}`) || n.endsWith(` ${s}`) || n.startsWith(`${s} `));
  });
}
