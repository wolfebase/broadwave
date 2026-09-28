export type Box = { id: number; left: number; top: number; right: number; bottom: number };

export type Direction = "up" | "down" | "left" | "right";

function center(box: Box) {
  return { x: (box.left + box.right) / 2, y: (box.top + box.bottom) / 2 };
}

/**
 * The nearest box in a direction. Aligned boxes beat nearer boxes that sit off to the side,
 * which is how a remote walks a row of tabs and then drops into the page under them.
 */
export function nearest(from: Box, items: Box[], dir: Direction): number | null {
  const origin = center(from);
  let best: { id: number; score: number } | null = null;
  for (const item of items) {
    if (item.id === from.id) continue;
    const point = center(item);
    const dx = point.x - origin.x;
    const dy = point.y - origin.y;
    const primary = dir === "left" ? -dx : dir === "right" ? dx : dir === "up" ? -dy : dy;
    if (primary <= 1) continue;
    const cross = dir === "left" || dir === "right" ? Math.abs(dy) : Math.abs(dx);
    const overlap =
      dir === "up" || dir === "down"
        ? Math.min(from.right, item.right) - Math.max(from.left, item.left)
        : Math.min(from.bottom, item.bottom) - Math.max(from.top, item.top);
    if (overlap <= 0 && cross > primary * 2) continue;
    const score = primary + cross * 2;
    if (!best || score < best.score) best = { id: item.id, score };
  }
  return best?.id ?? null;
}

function digitsOf(value: string) {
  return value.replaceAll(".", "");
}

/** True when another digit could still be part of a channel number, dot or not. */
export function channelNumberContinues<T extends { displayNumber: string }>(channels: T[], typed: string): boolean {
  const digits = digitsOf(typed);
  return channels.some((c) => c.displayNumber.startsWith(typed) || (digits.length > 0 && digitsOf(c.displayNumber).startsWith(digits)));
}

/**
 * The channel a typed number points to. "51" is 5.1. A single channel whose number
 * starts with what was typed ("5" when only 5.1 does) tunes at once. Digits with the
 * dot left out wait until the viewer stops, then an exact number wins.
 */
export function typedChannel<T extends { displayNumber: string }>(channels: T[], typed: string, done: boolean): T | null {
  const digits = digitsOf(typed);
  const matches = channels.filter((c) => c.displayNumber.startsWith(typed) || (digits.length > 0 && digitsOf(c.displayNumber).startsWith(digits)));
  const unique = matches.filter((c, i) => matches.findIndex((other) => other.displayNumber === c.displayNumber) === i);
  if (!done) {
    const byName = channels.filter((c) => c.displayNumber.startsWith(typed));
    const one = byName.filter((c, i) => byName.findIndex((other) => other.displayNumber === c.displayNumber) === i);
    return one.length === 1 ? one[0] : null;
  }
  return unique.find((c) => c.displayNumber === typed) ?? unique.find((c) => digitsOf(c.displayNumber) === digits) ?? unique[0] ?? null;
}
