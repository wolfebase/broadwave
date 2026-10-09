const held = new Set([" ", "r", "c", "i", "t", "m", "g", "l", "?"]);

/** A held toggle would flip twice. Arrows keep repeating. Ctrl/Cmd/Alt+C is Copy. */
export function ignoreHeldKey(
  key: string,
  event: { repeat: boolean; metaKey: boolean; ctrlKey: boolean; altKey: boolean },
): boolean {
  const k = key.toLowerCase();
  if (k === "c" && (event.metaKey || event.ctrlKey || event.altKey)) return true;
  return event.repeat && held.has(k);
}
