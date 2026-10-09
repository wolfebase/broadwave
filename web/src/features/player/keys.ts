const held = new Set([" ", "r", "c", "i", "t", "m", "g", "l", "?"]);

/** A channel number typed on the full player must not tune after it docks. */
export function pendingTuneFires(mode: string): boolean {
  return mode === "full";
}

/** Escape while the stage is fullscreen only leaves fullscreen. */
export function escapeAction(fullscreen: boolean, panelOpen: boolean): "exit-fullscreen" | "close-panel" | "minimize" {
  if (fullscreen) return "exit-fullscreen";
  if (panelOpen) return "close-panel";
  return "minimize";
}

/** A held toggle would flip twice. Arrows keep repeating. Ctrl/Cmd/Alt+C is Copy. */
export function ignoreHeldKey(
  key: string,
  event: { repeat: boolean; metaKey: boolean; ctrlKey: boolean; altKey: boolean },
): boolean {
  const k = key.toLowerCase();
  if (k === "c" && (event.metaKey || event.ctrlKey || event.altKey)) return true;
  return event.repeat && held.has(k);
}

/** Focus selects the row the pointer would select. Out of range keeps the old row. */
export function guideRowOnFocus(index: number, count: number, current: number): number {
  if (!Number.isInteger(index) || index < 0 || index >= count) return current;
  return index;
}

/** Close keys inside the open mini-guide. A repeated g must not undo the open. */
export function guideListAction(key: string, repeat: boolean): "close" | "up" | "down" | "tune" | "ignore" {
  const k = key.toLowerCase();
  if (k === "g" && repeat) return "ignore";
  if (k === "escape" || k === "backspace" || k === "g") return "close";
  if (k === "arrowup") return "up";
  if (k === "arrowdown") return "down";
  if (k === "enter") return "tune";
  return "ignore";
}
