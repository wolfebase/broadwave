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

/**
 * The rows show the order being saved, with any pass added since at the end and
 * any removed left out. A list read while the save was under way still has the
 * old order; showing it moved the rows back and lost the focused button.
 */
export function heldOrder(server: number[], held: number[]): number[] {
  const kept = held.filter((id) => server.includes(id));
  return [...kept, ...server.filter((id) => !kept.includes(id))];
}

/** How long after its save a list with another order can still be from before it. */
export const heldGrace = 5000;

/**
 * The held order gives way once the last save is done and a new list has come:
 * one that has the saved order, or any one after the save failed. A list with
 * the old order can still land just after the save; it does not count, until
 * `heldGrace` has passed: by then another order is someone else's change.
 */
export function releaseHeld<T>(
  held: { ids: number[]; saved: boolean; failed: boolean; list: T; savedAt?: number } | null,
  list: T,
  server: number[],
  now = Date.now(),
): boolean {
  if (held == null || !held.saved || held.list === list) return false;
  if (held.failed || (held.savedAt != null && now - held.savedAt > heldGrace)) return true;
  const kept = held.ids.filter((id) => server.includes(id));
  return server.filter((id) => kept.includes(id)).every((id, i) => id === kept[i]);
}
