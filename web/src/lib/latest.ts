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

/** One number per settings save. A later save makes an earlier response stale. */
export type SaveSeq = { n: number };

export function beginSave(seq: SaveSeq): number {
  seq.n += 1;
  return seq.n;
}

type ReadClock = {
  start(keys: readonly string[]): (key: string) => boolean;
};

/**
 * Apply this save only if no later save has started.
 * Bumping the read clock here drops a GET that started while the PUT was in flight.
 * A failed write (null) is not applied and does not bump. A stale write does not bump either.
 */
export function commitSave<T>(
  seq: SaveSeq,
  mine: number,
  saved: T | null,
  reads: ReadClock,
  keys: readonly string[],
): { saved: T; still: (key: string) => boolean } | null {
  if (saved == null || mine !== seq.n) return null;
  return { saved, still: reads.start(keys) };
}

/** Null means do not write. An older save, a stale read, or a failed read keeps what is on screen. */
export function followUpRead<T>(seq: SaveSeq, mine: number, still: boolean, incoming: T | null): T | null {
  if (!still || mine !== seq.n || incoming == null) return null;
  return incoming;
}
