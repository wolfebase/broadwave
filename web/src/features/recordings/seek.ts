// A recording's playlist only holds the part already transcoded. A seek
// inside that part moves the playhead. A seek outside it asks the server
// to start the encode there, so the player does not sit at the edge or
// request a segment that was never written.

export type RecordingSeek = { action: "within"; to: number } | { action: "reload"; to: number };

export function planRecordingSeek(target: number, encodedStart: number, seekEnd: number): RecordingSeek {
  const want = Number.isFinite(target) ? Math.max(0, target) : 0;
  const start = Number.isFinite(encodedStart) && encodedStart > 0 ? encodedStart : 0;
  const end = Number.isFinite(seekEnd) && seekEnd > 0 ? seekEnd : 0;
  const covered = end > start ? end : start;
  // A second and a half past the last segment is still this encode: the next
  // segment is about to be published. Further than that, or back before the
  // encode started, the server has to begin again at the new place.
  if (want + 0.05 >= start && want <= covered + 1.5) {
    let to = want;
    if (to < start) to = start;
    if (covered > 0 && to > covered) to = covered;
    return { action: "within", to };
  }
  return { action: "reload", to: want };
}
