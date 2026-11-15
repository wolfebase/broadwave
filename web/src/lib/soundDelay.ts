// How late this screen's sound reaches the room, in ms. A soundbar, AirPlay,
// or Bluetooth adds a delay the other screens do not have, so this screen
// plays that far ahead of the room and its sound lands with theirs. Negative
// is sound that comes early: this screen plays behind. It belongs to this
// device's speakers, so it is kept on the device, not on the server.

export const SOUND_DELAY_MAX_MS = 300;
export const SOUND_DELAY_STEP_MS = 25;
const key = "broadwave-sound-delay";

export function clampSoundDelay(ms: number): number {
  if (!Number.isFinite(ms)) return 0;
  const stepped = Math.round(ms / SOUND_DELAY_STEP_MS) * SOUND_DELAY_STEP_MS;
  return Math.max(-SOUND_DELAY_MAX_MS, Math.min(SOUND_DELAY_MAX_MS, stepped)) || 0;
}

/** Read on every sync tick, so a change in Settings or another tab applies at once. */
export function readSoundDelay(): number {
  try {
    return clampSoundDelay(Number(globalThis.localStorage?.getItem(key) ?? 0));
  } catch {
    return 0;
  }
}

export function saveSoundDelay(ms: number) {
  const value = clampSoundDelay(ms);
  try {
    if (value === 0) localStorage.removeItem(key);
    else localStorage.setItem(key, String(value));
  } catch {
    // Private windows can refuse storage; the screen stays at no delay.
  }
}

/** Every choice Settings offers, early to late. */
export function soundDelayChoices(): number[] {
  const out: number[] = [];
  for (let ms = -SOUND_DELAY_MAX_MS; ms <= SOUND_DELAY_MAX_MS; ms += SOUND_DELAY_STEP_MS) out.push(ms);
  return out;
}
