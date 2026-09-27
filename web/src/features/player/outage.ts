// What the player tells a viewer when live TV cannot keep going, and when it
// is worth asking again without reloading the page.

export type Recovery = "" | "busy" | "server" | "tuner";

export const serverStopped = "The server stopped. It will try again when it's back.";
export const tunerStopped = "This tuner did not answer. Check that it is on.";
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
};

export function aTunerIsFree(tuners: { target?: string; guide?: string }[]): boolean {
  return tuners.some((tuner) => !tuner.target && !tuner.guide);
}

export function recoveryReady(kind: Recovery, snap: RecoverySnap): boolean {
  if (kind === "server") return snap.health;
  if (kind === "busy") return snap.freeTuner;
  if (kind === "tuner") return snap.tunerAnswers;
  return false;
}
