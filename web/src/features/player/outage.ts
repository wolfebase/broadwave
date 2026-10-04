// What the player tells a viewer when live TV cannot keep going, and when it
// is worth asking again without reloading the page.

export type Recovery = "" | "busy" | "server" | "tuner" | "signal" | "restart";

export const serverStopped = "The server stopped. It will try again when it's back.";
export const connectionDropped = "The connection dropped. It will try again when it's back.";
export const tunerStopped = "This tuner did not answer. Check that it is on.";
export const noSignal = "This channel isn't coming in. Check the antenna.";
export const pictureStopped = "The picture stopped. Trying again usually fixes it.";
export const pictureRestarting = "The picture stopped. Starting it again.";
export const noListing = "No listing for this channel.";
export const noListingChecked = "Still no listing for this channel.";
export const channelDidNotStart = "This channel did not start. Try again.";
export const requestFailed = "That did not work. Try again.";

// A response that is not the error envelope, or a system string from the player.
// The server's own sentences pass through.
export function viewerMessage(message: string): string {
  const text = message.trim();
  if (!text || unreadable(text)) return channelDidNotStart;
  return text;
}

function unreadable(text: string): boolean {
  if (/^The server answered \d+/.test(text)) return true;
  if (/^(Internal Server Error|Bad Gateway|Service Unavailable|Gateway Timeout)$/i.test(text)) return true;
  const lower = text.toLowerCase();
  if (lower.includes("unexpected token") || lower.includes("is not valid json")) return true;
  if (lower.includes("couldn't be read") || lower.includes("couldn’t be read")) return true;
  if (lower.includes("couldn't be completed") || lower.includes("couldn’t be completed")) return true;
  if (lower.includes("nsurlerror")) return true;
  return false;
}

/** The note under the player. A check that finds nothing has to change the words, or the status line stays quiet. */
export function listingNote(checked: boolean): string {
  return checked ? noListingChecked : noListing;
}

const busyFallback = "Every tuner is busy. Stop a recording or watch something already on.";

type Failed = { status?: number; code?: string; message?: string };

export function viewerFailure(err: unknown): { message: string; recovery: Recovery } {
  const failed = (err ?? {}) as Failed;
  if (failed.code === "tuners_busy") {
    return { message: failed.message || busyFallback, recovery: "busy" };
  }
  // Tried once and not coming in. Try again is the viewer's call: nothing
  // here would tell the player the antenna changed.
  if (failed.code === "no_signal") {
    return { message: noSignal, recovery: "" };
  }
  // A source that stopped sending, like a playlist's upstream. The quiet
  // clock asks again, since nothing the player can read will change.
  if (failed.code === "stream_down") {
    return { message: pictureStopped, recovery: "" };
  }
  const message = err instanceof Error ? err.message : failed.message || "";
  if (failed.status === 0 || /failed to fetch|networkerror|load failed/i.test(message)) {
    if (typeof navigator !== "undefined" && navigator.onLine === false) {
      return { message: connectionDropped, recovery: "server" };
    }
    return { message: serverStopped, recovery: "server" };
  }
  if (/did not answer/i.test(message)) {
    return { message: tunerStopped, recovery: "tuner" };
  }
  return { message: viewerMessage(message), recovery: "" };
}

export type RecoverySnap = {
  health: boolean;
  freeTuner: boolean;
  tunerAnswers: boolean;
  online: boolean;
  signalLost: boolean;
  // A restarted server answers 404 for every playlist it had, with /health fine.
  watchGone?: boolean;
};

export function classifySnap(snap: RecoverySnap): { message: string; recovery: Recovery } {
  if (!snap.health) {
    if (!snap.online) return { message: connectionDropped, recovery: "server" };
    return { message: serverStopped, recovery: "server" };
  }
  // A dark tune the server gave back also leaves its playlist gone.
  if (snap.signalLost && snap.tunerAnswers) return { message: noSignal, recovery: "signal" };
  if (snap.watchGone) return { message: pictureRestarting, recovery: "restart" };
  if (!snap.tunerAnswers) return { message: tunerStopped, recovery: "tuner" };
  return { message: pictureStopped, recovery: "" };
}

export function aTunerIsFree(tuners: { target?: string; guide?: string }[]): boolean {
  return tuners.some((tuner) => !tuner.target && !tuner.guide);
}

// A home with no HDHomeRun (only playlists) has no tuner to blame.
export function aTunerAnswers(devices: { error?: string }[]): boolean {
  return devices.length === 0 || devices.some((device) => !device.error);
}

export function recoveryReady(kind: Recovery, snap: RecoverySnap): boolean {
  if (kind === "server" || kind === "restart") return snap.health && snap.online;
  if (kind === "busy") return snap.freeTuner;
  if (kind === "tuner") return snap.tunerAnswers;
  if (kind === "signal") return snap.health && !snap.signalLost;
  return false;
}

// A full picture budget often still holds the layout just left, and those
// encodes free one at a time. A page that left before its watch answered holds
// one until the server sees nobody fetching it (15 s), so ask for 20 s. Same
// count and pace as the Apple apps.
export const startRetryMs = 2000;
export function startAttempts(code?: string): number {
  return code === "pictures_full" ? 10 : 1;
}

// A stopped picture with no named cause has nothing to wait for: health stays
// up, an empty device list counts as a tuner answering, and signals have no
// Lost row. The player starts a new watch on this clock instead.
export const pictureRetryEveryMs = 10_000;
export const pictureRetryForMs = 2 * 60_000;

// Delay until the next quiet watch. Null when this outage is not on that
// clock, or the two minutes are over. The last watch starts at two minutes.
export function pictureRetryDelay(message: string, recovery: Recovery, elapsedMs: number): number | null {
  if (message !== pictureStopped || recovery !== "") return null;
  if (!Number.isFinite(elapsedMs) || elapsedMs < 0 || elapsedMs >= pictureRetryForMs) return null;
  const into = elapsedMs % pictureRetryEveryMs;
  const wait = into === 0 ? pictureRetryEveryMs : pictureRetryEveryMs - into;
  if (elapsedMs + wait > pictureRetryForMs) return null;
  return wait;
}

// Wait before starting the picture again after `restarts` restarts that never
// played. A watch can answer while its playlist is still gone (a proxy, a server
// mid-restart), and the new stream then fails at once.
export function restartDelayMs(restarts: number): number {
  if (!(restarts > 0)) return 0;
  return Math.min(10_000, 1000 * 2 ** (Math.min(restarts, 8) - 1));
}

// A quiet retry that fails without a named cause keeps the picture message.
// No signal stays the viewer's call, including a tune that never locked.
export function holdPictureMessage(next: { message: string; recovery: Recovery }): boolean {
  return next.recovery === "" && next.message !== noSignal;
}

// A picture that should be moving and is not. A player can stop on a live
// stream with seconds buffered and never fire an error. Eight seconds of that
// reloads the stream at the live edge; the next step starts a new watch, and
// they alternate until the picture has played 20 s or the cap. A reload can
// play the few seconds the server still lists, so a short run of picture does
// not start the count over.
export const frozenMs = 8000;
const settledMs = 20_000;
// After this many steps with no settled picture, the outage clock has the last word.
const maxFrozenSteps = 6;
// One sample is about a second apart. A larger step is a seek or a new
// stream, not the picture playing.
const largestStep = 3;
export type FrozenStep = "reload" | "retune";

export class FrozenPicture {
  private last: number | null = null;
  private since = 0;
  private movingSince = 0;
  private tries = 0;
  private moved = false;
  // True from the first step until the picture moves again.
  reconnecting = false;

  // time is null with nothing to watch. playing is false while the viewer or
  // the sync engine holds the picture.
  note(time: number | null, playing: boolean, now: number): FrozenStep | null {
    if (time == null || !Number.isFinite(time) || !playing) {
      this.last = null;
      this.since = 0;
      this.movingSince = 0;
      return null;
    }
    const last = this.last;
    this.last = time;
    if (last == null) {
      this.since = now;
      return null;
    }
    const step = time - last;
    if (step > 0.04 && step < largestStep) {
      this.moved = true;
      this.reconnecting = false;
      this.since = now;
      if (!this.movingSince) this.movingSince = now;
      if (now - this.movingSince >= settledMs) this.tries = 0;
      return null;
    }
    this.movingSince = 0;
    if (Math.abs(step) >= largestStep) {
      this.since = now;
      return null;
    }
    if (!this.moved || now - this.since < frozenMs) return null;
    if (this.tries >= maxFrozenSteps) {
      this.reconnecting = false;
      return null;
    }
    this.since = now;
    this.tries++;
    this.reconnecting = true;
    return this.tries % 2 === 1 ? "reload" : "retune";
  }
}
