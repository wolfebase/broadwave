import type { Airing, Recording } from "../../types";

/** The longest buffer is 4 hours; a file that started further back is another showing. */
const longestBuffer = 4 * 3_600_000;

/**
 * Where Start over plays the show from: the live window when it still holds
 * the show's start (it wins), else a recording of this showing that began at
 * or before it. Times are milliseconds since the epoch.
 */
export function startOverChoice(showStart: number | undefined, windowFrom: number | null, recordingStarted: number | undefined): "live" | "recording" | null {
  if (showStart === undefined || Number.isNaN(showStart)) return null;
  if (windowFrom !== null && windowFrom <= showStart) return "live";
  if (recordingStarted !== undefined && recordingStarted <= showStart) return "recording";
  return null;
}

/** A recording of this showing whose file holds its start, the earliest such. */
export function recordingHoldingStart(airing: Airing, recordings: Recording[]): Recording | undefined {
  const start = Date.parse(airing.start);
  const title = airing.title.trim().toLocaleLowerCase();
  return recordings
    .filter((rec) => {
      if (rec.channelId !== airing.channelId || rec.missing || rec.title.trim().toLocaleLowerCase() !== title) return false;
      if (rec.endedAt && Date.parse(rec.endedAt) < start) return false;
      if (rec.programId && airing.programId && rec.programId !== airing.programId) return false;
      const began = Date.parse(rec.startedAt);
      return began <= start && began >= start - longestBuffer;
    })
    .sort((a, b) => Date.parse(a.startedAt) - Date.parse(b.startedAt))[0];
}
