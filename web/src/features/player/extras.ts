const soundKey = "broadwave-volume";

export type Sound = { volume: number; muted: boolean };

export function parseSound(raw: string | null): Sound {
  try {
    const parsed = JSON.parse(raw || "{}") as { volume?: unknown; muted?: unknown };
    const volume = typeof parsed.volume === "number" && Number.isFinite(parsed.volume) ? Math.min(1, Math.max(0, parsed.volume)) : 1;
    return { volume, muted: parsed.muted === true };
  } catch {
    return { volume: 1, muted: false };
  }
}

export function soundRecord(video: { volume: number; muted: boolean }): string {
  const volume = Number.isFinite(video.volume) ? Math.min(1, Math.max(0, video.volume)) : 1;
  return JSON.stringify({ volume, muted: video.muted });
}

export function readSound(): Sound {
  try {
    return parseSound(localStorage.getItem(soundKey));
  } catch {
    return { volume: 1, muted: false };
  }
}

export function saveSound(video: { volume: number; muted: boolean }) {
  try {
    localStorage.setItem(soundKey, soundRecord(video));
  } catch {
    // A private window can refuse the write. The picture keeps the level for this visit.
  }
}

export function applySound(video: HTMLVideoElement) {
  const sound = readSound();
  video.volume = sound.volume;
  video.muted = sound.muted;
}

export function sleepUntilFrom(minutes: number, now: number): number | null {
  if (minutes !== 30 && minutes !== 60 && minutes !== 90) return null;
  return now + minutes * 60_000;
}

export function sleepDue(until: number | null, now: number): boolean {
  return until != null && now >= until;
}

export function sleepSentence(minutes: number): string {
  if (minutes === 30) return "Stops in 30 minutes.";
  if (minutes === 60) return "Stops in 1 hour.";
  if (minutes === 90) return "Stops in 90 minutes.";
  return "";
}
