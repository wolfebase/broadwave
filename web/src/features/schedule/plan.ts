/** A schedule read applies only when it is still the latest read and no fix has started since. */
export function takeScheduleRead<T>(
  mine: number,
  latest: number,
  writeAt: number,
  writeNow: number,
  items: T[] | null,
  tunerCount: number,
): { items: T[]; tunerCount: number } | null {
  if (mine !== latest || writeAt !== writeNow || items == null) return null;
  return { items, tunerCount };
}

/** The fix the viewer just made wins over a read that started earlier. A newer fix wins over this one. */
export function takeScheduleWrite<T>(
  mine: number,
  latest: number,
  items: T[] | null,
  tunerCount: number,
): { items: T[]; tunerCount: number } | null {
  if (mine !== latest || items == null) return null;
  return { items, tunerCount };
}

/** Only the last reorder asks for the pass list again. An earlier save would put the old order back. */
export function reloadAfterOrder(mine: number, latest: number): boolean {
  return mine === latest;
}
