/** A failed scoreboard is not an empty one. Only a successful body replaces the board. */
export function gamesFromScoreboard<T>(ok: boolean, games: T[] | undefined): T[] | null {
  if (!ok) return null;
  return games ?? [];
}

/** A live or final zero is a score. A blank, and a zero before the game, is not. */
export function teamScoreShown(score: string | undefined, state: string | undefined): boolean {
  if (!score) return false;
  if (score !== "0") return true;
  return state === "in" || state === "post";
}
