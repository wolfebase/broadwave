/**
 * A read that started earlier must not replace one that started later.
 * Lists are separate: a new recordings read leaves an older channels read alone.
 */
export class LatestReads {
  private n = new Map<string, number>();

  start(keys: readonly string[]): (key: string) => boolean {
    const mine = new Map<string, number>();
    for (const key of keys) {
      const next = (this.n.get(key) ?? 0) + 1;
      this.n.set(key, next);
      mine.set(key, next);
    }
    return (key) => this.n.get(key) === mine.get(key);
  }

  /** How many replacements of this list have started. A merge applies only while it is unchanged. */
  generation(key: string): number {
    return this.n.get(key) ?? 0;
  }
}

/** The newest scoreboard wins. A failed read keeps the board. An older read writes nothing. */
export function takeGames<T>(
  seq: { n: number },
  mine: number,
  games: T[] | null,
  board: { at: number; games: T[] },
  now: number,
): T[] | null {
  if (mine !== seq.n) return null;
  if (games == null) return board.games;
  board.games = games;
  board.at = now;
  return games;
}
