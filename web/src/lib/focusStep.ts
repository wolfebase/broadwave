export type Bounds = { left: number; top: number; right: number; bottom: number };

export type StepDir = "left" | "right" | "up" | "down";

export type Axis = "row" | "column";

function mid(box: Bounds) {
  return { x: (box.left + box.right) / 2, y: (box.top + box.bottom) / 2 };
}

/**
 * Which way a group is arranged, from the path through its controls in order.
 * A wrapped grid still counts as a row when neighbors sit side by side.
 */
export function groupAxis(boxes: Bounds[]): Axis {
  if (boxes.length < 2) return "row";
  let dx = 0;
  let dy = 0;
  for (let i = 1; i < boxes.length; i++) {
    const a = mid(boxes[i - 1]);
    const b = mid(boxes[i]);
    dx += Math.abs(b.x - a.x);
    dy += Math.abs(b.y - a.y);
  }
  return dx >= dy ? "row" : "column";
}

/**
 * The next control along the group's axis.
 * "stay" is the end of the row or column: the key must not leap out sideways.
 * Null means this key leaves the group (Up from a row, Right from a column).
 */
export function stepInGroup(index: number, count: number, dir: StepDir, axis: Axis, boxes?: Bounds[]): number | "stay" | null {
  if (count < 2 || index < 0 || index >= count) return null;
  const along = axis === "row" ? dir === "left" || dir === "right" : dir === "up" || dir === "down";
  if (!along) return null;
  const next = index + (dir === "right" || dir === "down" ? 1 : -1);
  if (next < 0 || next >= count) return "stay";
  // A wrapped row's next control sits on the following line. Right stays on this line.
  if (boxes && boxes.length === count) {
    const a = mid(boxes[index]);
    const b = mid(boxes[next]);
    const forward = axis === "row" ? (dir === "right" ? b.x - a.x : a.x - b.x) : dir === "down" ? b.y - a.y : a.y - b.y;
    if (forward <= 1) return "stay";
  }
  return next;
}
