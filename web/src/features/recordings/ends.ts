import type { Recording } from "../../types";

/** Seconds before the next episode plays once Up next shows. */
export const upNextCountdown = 10;

/** Where Skip intro jumps to, while the playhead is in the intro. */
export function introSkip(rec: Recording, t: number) {
  const start = rec.introStart ?? 0;
  const end = rec.introEnd ?? 0;
  return end > start && t >= start && t < end - 1 ? end : null;
}

/**
 * Autoplay advances only after a countdown the viewer actually saw. A resume
 * or a new episode that opens already at the end (`counted` false) does not.
 */
export function takeUpNext(counted: boolean, left: number | null | undefined): { counted: boolean; play: boolean } {
  if (left == null) return { counted: false, play: false };
  if (left > 0) return { counted: true, play: false };
  if (!counted) return { counted: false, play: false };
  return { counted: false, play: true };
}

/**
 * Up next shows from the end titles, or the last seconds without them. With
 * autoplay the next episode plays once the playhead is upNextCountdown
 * seconds past that; pausing pauses the count and seeking back hides it.
 */
export function upNext(t: number, duration: number, credits: number | undefined, autoplay: boolean) {
  if (duration <= upNextCountdown * 2) return null;
  const from = credits && credits > 0 && credits < duration ? credits : duration - upNextCountdown;
  if (t < from) return null;
  return { left: autoplay ? Math.max(0, Math.ceil(from + upNextCountdown - t)) : null };
}
