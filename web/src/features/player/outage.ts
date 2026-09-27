// What the player tells a viewer when live TV cannot keep going, and when it
// is worth asking again without reloading the page.

export type Recovery = "" | "busy" | "server" | "tuner" | "signal";

export const serverStopped = "The server stopped. It will try again when it's back.";
export const connectionDropped = "The connection dropped. It will try again when it's back.";
export const tunerStopped = "This tuner did not answer. Check that it is on.";
export const noSignal = "This channel isn't coming in. Check the antenna.";
export const pictureStopped = "The picture stopped. Trying again usually fixes it.";
export const noListing = "No listing for this channel.";

const busyFallback = "Every tuner is busy. Stop a recording or watch something already on.";

type Failed = { status?: number; code?: string; message?: string };

export function viewerFailure(err: unknown): { message: string; recovery: Recovery } {
  const failed = (err ?? {}) as Failed;
  if (failed.code === "tuners_busy") {
    return { message: failed.message || busyFallback, recovery: "busy" };
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
  return { message: message || "This channel did not start.", recovery: "" };
}

export type RecoverySnap = {
  health: boolean;
  freeTuner: boolean;
  tunerAnswers: boolean;
  online: boolean;
  signalLost: boolean;
};

export function classifySnap(snap: RecoverySnap): { message: string; recovery: Recovery } {
  if (!snap.health) {
    if (!snap.online) return { message: connectionDropped, recovery: "server" };
    return { message: serverStopped, recovery: "server" };
  }
  if (!snap.tunerAnswers) return { message: tunerStopped, recovery: "tuner" };
  if (snap.signalLost) return { message: noSignal, recovery: "signal" };
  return { message: pictureStopped, recovery: "" };
}

export function aTunerIsFree(tuners: { target?: string; guide?: string }[]): boolean {
  return tuners.some((tuner) => !tuner.target && !tuner.guide);
}

export function recoveryReady(kind: Recovery, snap: RecoverySnap): boolean {
  if (kind === "server") return snap.health && snap.online;
  if (kind === "busy") return snap.freeTuner;
  if (kind === "tuner") return snap.tunerAnswers;
  if (kind === "signal") return snap.health && !snap.signalLost;
  return false;
}
