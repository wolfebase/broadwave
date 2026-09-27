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

/**
 * The channel a typed number points to. While another digit could still change the answer
 * it returns null, unless the viewer is done typing; then an exact number beats the first
 * one that starts with what was typed.
 */
export function typedChannel<T extends { displayNumber: string }>(channels: T[], typed: string, done: boolean): T | null {
  const matches = channels.filter((c) => c.displayNumber.startsWith(typed));
  if (matches.length === 1) return matches[0];
  if (!done) return null;
  return matches.find((c) => c.displayNumber === typed) ?? matches[0] ?? null;
}
