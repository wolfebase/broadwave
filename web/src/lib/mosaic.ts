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

/** The slots with key in the first empty one, and that slot's index.
 * A key already in the list keeps its slot. A full list of eight is left as it is
 * and slot is -1, so a ninth mosaic is never added.
 */
export function addMosaic(slots: string[], key: string): { slots: string[]; slot: number } {
  const next = slots.slice(0, mosaicShareMax);
  if (key !== "") {
    const existing = next.indexOf(key);
    if (existing >= 0) return { slots: next, slot: existing };
  }
  const empty = next.indexOf("");
  if (empty >= 0) {
    next[empty] = key;
    return { slots: next, slot: empty };
  }
  if (next.length >= mosaicShareMax) return { slots: next, slot: -1 };
  next.push(key);
  return { slots: next, slot: next.length - 1 };
}
