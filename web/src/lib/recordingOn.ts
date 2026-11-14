import type { Airing, Recording } from "../types";

/**
 * The recording in progress on a channel for the program shown. A channel
 * can record two at once, the last show in its padding and the next one: the
 * one whose program id, then title, is the program's, else the newest.
 */
export function recordingOn(recordings: Recording[], channelId: number, airing?: Airing): Recording | undefined {
  const on = recordings.filter((r) => r.status === "recording" && r.channelId === channelId);
  if (on.length < 2 || !airing) return newest(on);
  if (airing.programId) {
    const same = on.find((r) => r.programId === airing.programId);
    if (same) return same;
  }
  const title = airing.title.trim().toLocaleLowerCase();
  return newest(on.filter((r) => r.title.trim().toLocaleLowerCase() === title)) ?? newest(on);
}

function newest(recordings: Recording[]): Recording | undefined {
  return [...recordings].sort((a, b) => Date.parse(b.startedAt) - Date.parse(a.startedAt))[0];
}
