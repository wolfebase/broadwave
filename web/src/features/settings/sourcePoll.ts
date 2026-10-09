/** A status failure keeps the tuner list this attempt already got, or the list on screen. A slower poll writes nothing. */
export function applyTunerPoll<T, S>(
  mine: number,
  latest: number,
  current: T[],
  tuners: T[] | null,
  statuses: S[] | null,
): { tuners: T[]; statuses: S[] | null } | null {
  if (mine !== latest) return null;
  return { tuners: tuners ?? current, statuses };
}

export function applySignalPoll<T>(mine: number, latest: number, running: boolean, rows: T[]): { running: boolean; rows: T[] } | null {
  if (mine !== latest) return null;
  return { running, rows };
}

export function applyDiagnostics<T>(mine: number, latest: number, body: T | null): T | null {
  if (mine !== latest || body == null) return null;
  return body;
}
