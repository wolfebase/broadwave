// A recording's playlist only holds the part already transcoded. A seek
// inside that part moves the playhead. A seek outside it asks the server
// to start the encode there, so the player does not sit at the edge or
// request a segment that was never written.

export type RecordingSeek = { action: "within"; to: number } | { action: "reload"; to: number };

// length, when known, is a finished recording's end: a seek past it lands there.
// pending is a reload already under way. The old player is gone, so the new
// spot replaces that reload instead of seeking the old one.
export function planRecordingSeek(target: number, encodedStart: number, seekEnd: number, length = 0, pending: number | null = null): RecordingSeek {
  let want = Number.isFinite(target) ? Math.max(0, target) : 0;
  if (Number.isFinite(length) && length > 0) want = Math.min(want, length);
  if (pending != null) return { action: "reload", to: want };
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
