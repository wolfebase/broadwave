import { useEffect, useState } from "react";

export type ScoreGame = {
  id: string;
  state?: string;
  teams?: { name: string; short?: string; abbr?: string; score?: string; home?: boolean }[];
};

const cache: { at: number; games: ScoreGame[] } = { at: 0, games: [] };
let pending: Promise<ScoreGame[]> | null = null;

export function loadScores(): Promise<ScoreGame[]> {
  if (Date.now() - cache.at < 30_000) return Promise.resolve(cache.games);
  if (!pending) {
    pending = fetch("/api/v1/sports/scoreboard")
      .then((res) => (res.ok ? res.json() : { games: [] }))
      .then((body: { games?: ScoreGame[] }) => {
        cache.games = body.games ?? [];
        cache.at = Date.now();
        return cache.games;
      })
      .catch(() => cache.games)
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
