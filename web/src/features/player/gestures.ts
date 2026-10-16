/** What one finger did on the picture. */
export type TouchKind = "tap" | "swipe-up" | "swipe-down" | "none";

/** A short still touch is a tap. A long, mostly vertical, quick drag is a swipe. */
export function classifyTouch(dx: number, dy: number, ms: number): TouchKind {
  const ax = Math.abs(dx);
  const ay = Math.abs(dy);
  if (ax < 14 && ay < 14 && ms < 450) return "tap";
  if (ay >= 56 && ay > ax * 1.5 && ms < 900) return dy < 0 ? "swipe-up" : "swipe-down";
  return "none";
}

export type TapZone = "left" | "center" | "right";

/** The outer thirds of the picture seek on a double tap. The middle never does. */
export function tapZone(x: number, width: number): TapZone {
  if (width <= 0) return "center";
  if (x < width / 3) return "left";
  if (x > (width * 2) / 3) return "right";
  return "center";
}

/** A second tap on the same edge, soon after the first, seeks. Each further tap seeks again. */
export function seeksOnTap(prev: { at: number; zone: TapZone } | null, at: number, zone: TapZone, gap = 320): boolean {
  return Boolean(prev) && zone !== "center" && prev!.zone === zone && at - prev!.at <= gap;
}

/** Swipe up is channel up, the next number; swipe down goes back down the list. */
export function swipeStep(kind: TouchKind): 1 | -1 | 0 {
  return kind === "swipe-up" ? 1 : kind === "swipe-down" ? -1 : 0;
}

/** A pointer event that a touch already handled. Browsers send a click, and a dblclick, after it. */
export function touchEcho(lastTouch: number, now: number): boolean {
  return now - lastTouch < 800;
}
