export type Box = { id: number; left: number; top: number; right: number; bottom: number };

export type Direction = "up" | "down" | "left" | "right";

function center(box: Box) {
  return { x: (box.left + box.right) / 2, y: (box.top + box.bottom) / 2 };
}

/** Empty space between two spans on one axis. Overlapping spans have no gap. */
function spanGap(a0: number, a1: number, b0: number, b1: number) {
  if (a1 < b0) return b0 - a1;
  if (b1 < a0) return a0 - b1;
  return 0;
}

/**
 * The nearest box in a direction. Aligned boxes beat nearer boxes that sit off to the side,
 * which is how a remote walks a row of tabs and then drops into the page under them.
 * A wide button counts as directly above whatever it covers, so Down stays on the next
 * control instead of leaping toward the button's center. A loose search still counts a
 * box that sits well off to the side, so a tab can reach a page that does not line up.
 */
export function nearest(from: Box, items: Box[], dir: Direction, loose = false): number | null {
  const origin = center(from);
  const vertical = dir === "up" || dir === "down";
  let best: { id: number; score: number } | null = null;
  for (const item of items) {
    if (item.id === from.id) continue;
    const point = center(item);
    const dx = point.x - origin.x;
    const dy = point.y - origin.y;
    const primary = dir === "left" ? -dx : dir === "right" ? dx : dir === "up" ? -dy : dy;
    if (primary <= 1) continue;
    const cross = vertical
      ? spanGap(from.left, from.right, item.left, item.right)
      : spanGap(from.top, from.bottom, item.top, item.bottom);
    if (!loose && cross > primary * 2) continue;
    const score = primary + cross * 2;
    if (!best || score < best.score) best = { id: item.id, score };
  }
  return best?.id ?? null;
}

/**
 * The control in the first row under a bar, the one nearest the focused tab across.
 * The first row is every box that starts before the topmost one ends.
 */
export function firstBelow(from: Box, items: Box[]): number | null {
  const below = items.filter((item) => item.id !== from.id && item.top >= from.bottom - 1);
  if (below.length === 0) return null;
  const top = below.reduce((a, b) => (b.top < a.top || (b.top === a.top && b.bottom < a.bottom) ? b : a));
  const x = center(from).x;
  let best: Box | null = null;
  for (const item of below) {
    if (item.top >= top.bottom) continue;
    const gap = Math.abs(center(item).x - x);
    if (!best || gap < Math.abs(center(best).x - x) || (gap === Math.abs(center(best).x - x) && item.left < best.left)) best = item;
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
