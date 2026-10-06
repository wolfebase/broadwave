// Home loads with the first paint, so what it needs stays out of the library model.
import type { Recording } from "../../types";

/** Marked watched or unwatched wins; otherwise a playhead in the last 15 s or 10 % counts. */
export function watched(rec: Recording) {
  if (rec.watched === 1) return true;
  if (rec.watched === 2) return false;
  const position = rec.position ?? 0;
  const duration = rec.durationSec ?? 0;
  if (duration < 10 || position < 1) return false;
  return position >= duration - 15 || position / duration >= 0.9;
}

/** Started and not finished, most recently played first. */
export function continueWatching(recordings: Recording[], limit = 10) {
  return recordings
    .filter((rec) => rec.status !== "recording" && !rec.missing && (rec.position ?? 0) > 30 && !watched(rec))
    .sort((a, b) => Date.parse(b.progressAt ?? b.startedAt) - Date.parse(a.progressAt ?? a.startedAt))
    .slice(0, limit);
}
