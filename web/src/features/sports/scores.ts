import { useEffect, useState } from "react";
import { takeGames } from "../../lib/latest";
import { gamesFromScoreboard } from "./board";

export type ScoreTeam = {
  name: string;
  short?: string;
  abbr?: string;
  score?: string;
  home?: boolean;
  color?: string;
  altColor?: string;
  logo?: string;
};

export type ScoreGame = {
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
  situation?: string;
  teams?: ScoreTeam[];
};

const cache: { at: number; games: ScoreGame[] } = { at: 0, games: [] };
let pending: Promise<ScoreGame[]> | null = null;
const scoreSeq = { n: 0 };

export function refreshScores(): Promise<ScoreGame[]> {
  const mine = ++scoreSeq.n;
  return fetch("/api/v1/sports/scoreboard")
    .then(async (res) => {
      const body = res.ok ? ((await res.json()) as { games?: ScoreGame[] }) : null;
      return takeGames(scoreSeq, mine, gamesFromScoreboard(res.ok, body?.games), cache, Date.now()) ?? cache.games;
    })
    .catch(() => takeGames(scoreSeq, mine, null, cache, Date.now()) ?? cache.games);
}

export function loadScores(): Promise<ScoreGame[]> {
  if (Date.now() - cache.at < 30_000) return Promise.resolve(cache.games);
  if (!pending) {
    const mine = ++scoreSeq.n;
    pending = fetch("/api/v1/sports/scoreboard")
      .then(async (res) => {
        const body = res.ok ? ((await res.json()) as { games?: ScoreGame[] }) : null;
        return takeGames(scoreSeq, mine, gamesFromScoreboard(res.ok, body?.games), cache, Date.now()) ?? cache.games;
      })
      .catch(() => takeGames(scoreSeq, mine, null, cache, Date.now()) ?? cache.games)
      .finally(() => {
        pending = null;
      });
  }
  return pending;
}

export function scoreLine(game: ScoreGame | undefined): string {
  if (!game || game.state === "pre") return "";
  const teams = game.teams ?? [];
  const home = teams.find((team) => team.home);
  const away = teams.find((team) => team !== home);
  if (!home?.score || !away?.score) return "";
  const label = (team: { abbr?: string; short?: string; name: string }) => team.abbr || team.short || team.name;
  return `${label(away)} ${away.score} · ${label(home)} ${home.score}`;
}

export function useScoreboard() {
  const [games, setGames] = useState<ScoreGame[]>([]);
  useEffect(() => {
    let cancel = false;
    loadScores().then((next) => {
      if (!cancel) setGames(next);
    });
    return () => {
      cancel = true;
    };
  }, []);
  return games;
}

export function useScoreMap() {
  const [lines, setLines] = useState<Map<string, string>>(() => new Map());
  useEffect(() => {
    let cancel = false;
    loadScores().then((games) => {
      if (cancel) return;
      const next = new Map<string, string>();
      for (const game of games) {
        const line = scoreLine(game);
        if (game.id && line) next.set(game.id, line);
      }
      setLines(next);
    });
    return () => {
      cancel = true;
    };
  }, []);
  return lines;
}
