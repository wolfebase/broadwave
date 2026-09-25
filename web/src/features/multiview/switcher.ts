// Matches sports.PickFocus. The clock is injected so tests can freeze it.

export type SwitchTeam = {
  name: string;
  short?: string;
  abbr?: string;
  score?: string;
  home?: boolean;
};

export type SwitchGame = {
  id: string;
  league?: string;
  name?: string;
  shortName?: string;
  state?: string;
  detail?: string;
  clock?: string;
  period?: number;
  redZone?: boolean;
  powerPlay?: boolean;
  teams?: SwitchTeam[];
};

export type SwitchChoice = {
  keepManual: boolean;
  gameId: string;
  banner: string;
};

const HOLD_MS = 2 * 60 * 1000;

const SPORT: Record<string, string> = {
  nfl: "football",
  ncaaf: "football",
  nba: "basketball",
  wnba: "basketball",
  ncaab: "basketball",
  mlb: "baseball",
  nhl: "hockey",
  mls: "soccer",
  nwsl: "soccer",
  epl: "soccer",
};

const lastQuarter = /\b(?:4th|fourth|ot|overtime)\b/i;
const lastHockey = /\b(?:3rd|third|ot|overtime)\b/i;
const lastHalf = /\b(?:2nd half|second half|2h)\b/i;
const overtime = /\b(?:ot|overtime|extra time|et)\b/i;
const extraSoccer = /\b(?:extra time|stoppage|penalties|aet|et)\b/i;
const soccerMinute = /(\d{1,3})(?:\s*\+\s*\d{1,2})?\s*'/;
const clockRE = /(?:^|[^0-9])(\d{1,2}):(\d{2})(?:[^0-9]|$)/;

type Rank = 0 | 1 | 2 | 3;

export function pickFocus(now: number, manualAt: number, games: SwitchGame[], previous: SwitchGame[]): SwitchChoice {
  if (manualAt > 0 && now >= manualAt && now - manualAt < HOLD_MS) {
    return { keepManual: true, gameId: "", banner: "" };
  }
  const prev = new Map(previous.map((game) => [game.id, game]));
  let best: Rank = 0;
  let chosen: SwitchGame | null = null;
  let why = "";
  for (const game of games) {
    if (!actionable(game)) continue;
    const earlier = prev.get(game.id);
    const [label, rank] = classify(game, earlier, earlier != null);
    if (rank > best) {
      best = rank;
      chosen = game;
      why = label;
    }
  }
  if (!chosen || best === 0) return { keepManual: false, gameId: "", banner: "" };
  return { keepManual: false, gameId: chosen.id, banner: banner(why, chosen) };
}

function sportOf(league: string): string {
  return SPORT[league] ?? "";
}

function actionable(game: SwitchGame): boolean {
  return game.state === "in" && !betweenPeriods(game.detail ?? "");
}

function classify(game: SwitchGame, prev: SwitchGame | undefined, hasPrev: boolean): [string, Rank] {
  if (game.redZone && sportOf(game.league ?? "") === "football") return ["Red zone", 3];
  if (game.powerPlay && sportOf(game.league ?? "") === "hockey") return ["Power play", 3];
  if (hasPrev && prev && leadChanged(game, prev)) return ["Lead change", 2];
  if (closeFinish(game)) return ["Final minutes", 1];
  return ["", 0];
}

function banner(why: string, game: SwitchGame): string {
  const who = matchup(game);
  return who ? `${why}: ${who}` : why;
}

function matchup(game: SwitchGame): string {
  const teams = game.teams ?? [];
  const home = teams.find((team) => team.home);
  const away = home ? teams.find((team) => team.abbr !== home.abbr || team.name !== home.name) : undefined;
  if (home && away) return `${label(away)} at ${label(home)}`;
  if (game.shortName) return game.shortName.replace(" @ ", " at ");
  return game.name ?? "";
}

function label(team: SwitchTeam): string {
  return team.abbr || team.short || team.name;
}

function leadChanged(cur: SwitchGame, prev: SwitchGame): boolean {
  const [prevLead, prevOK] = leaderKey(prev);
  const [curLead, curOK] = leaderKey(cur);
  return prevOK && curOK && prevLead !== "" && curLead !== "" && prevLead !== curLead;
}

function leaderKey(game: SwitchGame): [string, boolean] {
  const teams = game.teams ?? [];
  if (teams.length < 2) return ["", false];
  let best = 0;
  let key = "";
  let tied = false;
  let seen = 0;
  for (const team of teams) {
    const n = intScore(team.score);
    if (n == null) return ["", false];
    if (seen === 0 || n > best) {
      best = n;
      key = label(team);
      tied = false;
    } else if (n === best) {
      tied = true;
    }
    seen++;
  }
  if (seen < 2) return ["", false];
  if (tied) return ["", true];
  return [key, true];
}

function closeFinish(game: SwitchGame): boolean {
  const margin = closeMargin(game.league ?? "");
  if (margin == null || !inFinalMinutes(game)) return false;
  const gap = scoreGap(game);
  return gap != null && gap <= margin;
}

function closeMargin(league: string): number | null {
  const sport = sportOf(league);
  if (sport === "football" || sport === "basketball") return 8;
  if (sport === "hockey" || sport === "soccer") return 1;
  return null;
}

function scoreGap(game: SwitchGame): number | null {
  const teams = game.teams ?? [];
  if (teams.length < 2) return null;
  let min = 0;
  let max = 0;
  for (let i = 0; i < teams.length; i++) {
    const n = intScore(teams[i].score);
    if (n == null) return null;
    if (i === 0 || n < min) min = n;
    if (i === 0 || n > max) max = n;
  }
  return max - min;
}

function intScore(raw: string | undefined): number | null {
  if (raw == null) return null;
  const text = raw.trim();
  if (!/^[+-]?\d+$/.test(text)) return null;
  return Number(text);
}

function inFinalMinutes(game: SwitchGame): boolean {
  if (game.state !== "in" || betweenPeriods(game.detail ?? "")) return false;
  if (soccerClockLate(game)) return true;
  const need = finalPeriod(game.league ?? "");
  if (need === 0) return false;
  const late = (game.period ?? 0) >= need || detailIsLastPeriod(game);
  if (!late) return false;
  const left = remainingClock(game);
  return left != null && left < 300;
}

function finalPeriod(league: string): number {
  if (league === "ncaab") return 2;
  const sport = sportOf(league);
  if (sport === "hockey") return 3;
  if (sport === "soccer") return 2;
  if (sport === "football" || sport === "basketball") return 4;
  return 0;
}

function betweenPeriods(detail: string): boolean {
  const d = detail.trim().toLowerCase();
  if (!d) return false;
  if (d === "ht" || d === "halftime" || d === "half time") return true;
  return d.includes("halftime") || d.includes("half time") || d.includes("end of") || d.includes("intermission") || d.includes("delay") || d.includes("suspended");
}

function detailIsLastPeriod(game: SwitchGame): boolean {
  const text = game.detail ?? "";
  const sport = sportOf(game.league ?? "");
  if (sport === "hockey") return lastHockey.test(text);
  if (game.league === "ncaab" || sport === "soccer") return lastHalf.test(text) || overtime.test(text);
  if (sport === "football" || sport === "basketball") return lastQuarter.test(text);
  return false;
}

function soccerClockLate(game: SwitchGame): boolean {
  if (sportOf(game.league ?? "") !== "soccer") return false;
  const text = `${game.detail ?? ""} ${game.clock ?? ""}`;
  if (extraSoccer.test(text)) return true;
  for (const src of [game.clock ?? "", game.detail ?? ""]) {
    const m = src.match(soccerMinute);
    if (m && Number(m[1]) >= 85) return true;
  }
  return false;
}

function remainingClock(game: SwitchGame): number | null {
  return clockSeconds(game.clock ?? "") ?? clockSeconds(game.detail ?? "");
}

function clockSeconds(raw: string): number | null {
  const direct = parseClock(raw.trim());
  if (direct != null) return direct;
  const m = raw.match(clockRE);
  if (!m) return null;
  return parseClock(`${m[1]}:${m[2]}`);
}

function parseClock(raw: string): number | null {
  const parts = raw.split(":");
  if (parts.length !== 2) return null;
  let secText = parts[1];
  const cut = secText.search(/[. ]/);
  if (cut >= 0) secText = secText.slice(0, cut);
  if (!/^\d+$/.test(parts[0].trim()) || !/^\d+$/.test(secText)) return null;
  const min = Number(parts[0].trim());
  const sec = Number(secText);
  if (min < 0 || sec < 0 || sec > 59) return null;
  return min * 60 + sec;
}
