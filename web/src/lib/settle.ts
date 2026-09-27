// A screen that pays for every rate change stops trimming and holds any drift
// under the seek threshold. Next to a screen on the target, that much is an
// echo. The band keeps this screen and a trimming one (20 ms dead band) within
// 50 ms. One correction once it has sat off this long, then longer waits while
// corrections keep missing, so a screen that cannot land does not seek on a loop.
export const SETTLE_MS = 30;
const OFF_FOR_MS = 10_000;
const FIRST_GAP_MS = 30_000;
const MAX_GAP_MS = 5 * 60_000;
// Inside the band this long and the next miss starts the waits over.
const SETTLED_MS = 60_000;

export type Settle = { offSince: number | null; inSince: number | null; last: number; misses: number };

export function newSettle(): Settle {
  return { offSince: null, inSince: null, last: Number.NEGATIVE_INFINITY, misses: 0 };
}

/** Whether a screen that no longer trims should correct this drift (ms) now. Updates s. */
export function settleDue(s: Settle, drift: number, now: number): boolean {
  if (!Number.isFinite(drift)) return false;
  if (Math.abs(drift) <= SETTLE_MS) {
    s.offSince = null;
    s.inSince ??= now;
    if (now - s.inSince >= SETTLED_MS) s.misses = 0;
    return false;
  }
  s.inSince = null;
  s.offSince ??= now;
  if (now - s.offSince < OFF_FOR_MS) return false;
  const gap = s.misses ? Math.min(MAX_GAP_MS, FIRST_GAP_MS * 2 ** (s.misses - 1)) : 0;
  if (now - s.last < gap) return false;
  s.last = now;
  s.offSince = null;
  s.misses++;
  return true;
}
