/**
 * Mosaics other apps list as channels, by key: channel ids joined by dashes,
 * the sound first. A slot's place is its number (990.1 is the first), so a
 * removed mosaic leaves an empty slot and the others keep their numbers.
 */
export const mosaicShareMax = 8;

export function sharedMosaics(setting: string | undefined): string[] {
  return (setting ?? "").split(",");
}

export function mosaicKey(sound: number, ids: number[]): string {
  return [sound, ...ids.filter((id) => id !== sound)].join("-");
}

/** The slots with key in the first empty one, and that slot's index. */
export function addMosaic(slots: string[], key: string): { slots: string[]; slot: number } {
  const next = slots.filter((_, i) => i < mosaicShareMax);
  const empty = next.indexOf("");
  const slot = empty >= 0 ? empty : next.length;
  next[slot] = key;
  return { slots: next, slot };
}
