/** Largest gap between buffered ranges that is a hole, not the live edge. Matches hls.js maxBufferHole. */
export const maxHoleS = 0.5;

/**
 * Where playback continues past a small hole just ahead of the playhead, or null.
 * An encode that starts again at a timestamp break leaves a few tens of
 * milliseconds unbuffered between its media and the last encode's.
 */
export function holeEnd(t: number, ranges: [number, number][]): number | null {
  for (let i = 0; i + 1 < ranges.length; i++) {
    const [start, end] = ranges[i];
    const next = ranges[i + 1][0];
    if (t < start - 0.05 || t > next) continue;
    if (end - t > maxHoleS || next - end > maxHoleS || next <= t) continue;
    return next;
  }
  return null;
}
