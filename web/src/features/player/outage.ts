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
  if (snap.watchGone) return { message: pictureRestarting, recovery: "restart" };
  if (!snap.tunerAnswers) return { message: tunerStopped, recovery: "tuner" };
  if (snap.signalLost) return { message: noSignal, recovery: "signal" };
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

// A quiet retry that fails without a named cause keeps the picture message.
// No signal stays the viewer's call, including a tune that never locked.
export function holdPictureMessage(next: { message: string; recovery: Recovery }): boolean {
  return next.recovery === "" && next.message !== noSignal;
}
