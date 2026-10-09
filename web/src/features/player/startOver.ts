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

export type DatedFrag = { start: number; duration: number; programDateTime: number | null };

/** Broadcast time at the start of the seekable window, from any dated fragment. */
export function windowFromFragments(seekStart: number, frags: readonly DatedFrag[]): number | null {
  if (!Number.isFinite(seekStart)) return null;
  for (const f of frags) {
    if (f.programDateTime == null || !(f.duration > 0) || f.duration > 30 || !Number.isFinite(f.start)) continue;
    const at = f.programDateTime - (f.start - seekStart) * 1000;
    if (!Number.isFinite(at)) continue;
    return Math.round(at / 1000) * 1000;
  }
  return null;
}

/**
 * Broadcast time of the live window's start. A local seek drops the room
 * clock, so the playhead's time can be missing while the playlist still
 * dates the window.
 */
export function liveWindowFrom(input: {
  seekable: boolean;
  seekStart: number;
  currentTime: number;
  media: number | null;
  frags: readonly DatedFrag[];
}): number | null {
  if (!input.seekable) return null;
  if (input.media !== null && Number.isFinite(input.media)) {
    return Math.round((input.media - (input.currentTime - input.seekStart) * 1000) / 1000) * 1000;
  }
  return windowFromFragments(input.seekStart, input.frags);
}

/** Playhead for the show's start, inside the seekable window. */
export function startOverAt(showStart: number, windowFrom: number | null, seekStart: number, seekEnd: number): number | null {
  if (windowFrom === null || !(windowFrom <= showStart)) return null;
  const at = seekStart + (showStart - windowFrom) / 1000;
  if (!Number.isFinite(at) || at > seekEnd) return null;
  return Math.max(seekStart + 0.5, at);
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
