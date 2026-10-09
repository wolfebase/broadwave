// Where to put the playhead when a tab comes back from being hidden or frozen.
// The sync engine corrects small gaps. It cannot see a playhead the playlist
// has moved past, and hls.js is configured not to jump on its own.

/** Gone longer than this, the tab left the live window. */
export const awayBeforeSeekMs = 3000;

export type ComeBackAction = "ignore" | "play" | "resume";

/** A named outage already destroyed hls.js. startLoad will not load, so the message stays. */
export function comeBackAction(input: {
  tornDown: boolean;
  awayMs: number;
  syncing: boolean;
  paused: boolean;
}): ComeBackAction {
  if (input.tornDown) return "ignore";
  if (!(input.awayMs >= awayBeforeSeekMs)) return input.paused && input.syncing ? "play" : "ignore";
  if (!input.syncing && input.paused) return "ignore";
  return "resume";
}

/**
 * A room sitting at rate 0 is paused for everyone. Coming back must not play
 * this screen. A rate left over from a room this screen has left does not.
 */
export function returnHeld(syncing: boolean, paused: boolean, roomRate: number | null): boolean {
  if (syncing && roomRate === 0) return true;
  return !syncing && paused;
}

/** comeBackAction still says "play" for a short pause while syncing. A held room does not take it. */
export function playOnShortReturn(action: ComeBackAction, held: boolean): boolean {
  return action === "play" && !held;
}

/** How long to wait for a playlist that still shows the old edge. */
export const resumeWaitMs = 2200;

/** Still on the live sync point, within this many seconds. */
export const behindBeforeSeekS = 2;

export type ResumePlan = { action: "ignore" } | { action: "play" } | { action: "wait" } | { action: "seek"; to: number };

/**
 * The live sync point has moved about as long as the tab was gone.
 * A playlist parsed from a stale response stays near the edge we left.
 */
export function edgeCaughtUp(edgeAtLeave: number, liveSync: number | null, awayMs: number): boolean {
  if (liveSync == null || !Number.isFinite(liveSync) || !Number.isFinite(edgeAtLeave)) return false;
  return liveSync - edgeAtLeave > Math.max(2, awayMs / 1000 - 10);
}

/** Media time to seek to, or null to leave the playhead where it is. */
export function resumeSeek(input: { current: number; liveSync: number | null; awayMs: number; held: boolean }): number | null {
  if (input.held) return null;
  if (!(input.awayMs >= awayBeforeSeekMs)) return null;
  const edge = input.liveSync;
  if (edge == null || !Number.isFinite(edge)) return null;
  if (input.current >= edge - behindBeforeSeekS) return null;
  return edge;
}

export function resumePlan(input: {
  current: number;
  liveSync: number | null;
  awayMs: number;
  /** The viewer paused this screen. A tab the browser froze is not that. */
  held: boolean;
  waitedMs: number;
  /** hls.js live sync point when the tab left. */
  edgeAtLeave: number;
  /** Local minus the room, in ms, or null when the screen has no fix yet. */
  driftMs: number | null;
}): ResumePlan {
  if (input.held) return { action: "ignore" };
  if (!(input.awayMs >= awayBeforeSeekMs)) return { action: "play" };
  if (!edgeCaughtUp(input.edgeAtLeave, input.liveSync, input.awayMs) && input.waitedMs < resumeWaitMs) return { action: "wait" };
  const behindEdge = input.liveSync != null && Number.isFinite(input.liveSync) ? input.liveSync - input.current : Infinity;
  const drift = input.driftMs;
  // Already on the room, or ahead of it. The sync engine catches a lead by
  // pausing for the whole gap. Step back onto the room instead, so the picture
  // is moving. A playhead minutes off the edge is not this case: that drift
  // number is from before the tab froze.
  if (drift != null && Number.isFinite(drift) && behindEdge < 20 && drift > -2000) {
    if (drift > 400 && drift < 30_000) {
      const to = input.current - drift / 1000;
      if (Number.isFinite(to) && to >= 0) return { action: "seek", to };
    }
    return { action: "play" };
  }
  const to = resumeSeek(input);
  if (to == null) return { action: "play" };
  return { action: "seek", to };
}
