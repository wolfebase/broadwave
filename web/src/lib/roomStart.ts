import type { RoomState } from "./events";

export type Frag = { start: number; duration: number; programDateTime: number | null };

// A player started on a fragment shows its first picture a moment later: a
// few ms on the same machine, a few hundred on a slow phone. Landing a little
// ahead or behind is trimmed away without a pause or a seek.
const START_LEAD_MS = 500;

/** The program date-time (Unix ms) the room plays at server time now. */
export function roomTarget(st: RoomState, now: number): number {
  return st.rate === 0 ? st.anchorMedia : st.anchorMedia + (now - st.anchorServer) * st.rate;
}

/** Playhead position for a program date-time, or null when no fragment holds it. */
export function fragTime(frags: Frag[], media: number): number | null {
  for (const f of frags) {
    if (f.programDateTime == null || f.duration <= 0 || f.duration > 30) continue;
    const s = f.programDateTime;
    if (media >= s && media < s + f.duration * 1000) return f.start + (media - s) / 1000;
  }
  return null;
}

/**
 * Where a player joining the room starts loading: the room's frame at the
 * moment its first picture shows. Null when the playlist does not hold that
 * frame yet (a fresh tune), so the player starts where it would have.
 */
export function roomStart(frags: Frag[], st: RoomState, now: number): number | null {
  return fragTime(frags, roomTarget(st, now) + START_LEAD_MS * st.rate);
}
