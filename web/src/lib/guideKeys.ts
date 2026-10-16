/** Rows a Page Up or Page Down moves: a screenful, less one so the last row stays in sight. */
export function guidePageRow(row: number, dir: 1 | -1, viewHeight: number, rowH: number, headH: number, count: number): number {
  const visible = Math.max(1, Math.floor((viewHeight - headH) / rowH) - 1);
  return Math.max(0, Math.min(count - 1, row + dir * visible));
}

/** A half-hour label the "now" pill would sit on. The pill already says the time. */
export function slotUnderNow(slotX: number, nowX: number, pillHalf: number): boolean {
  return slotX > nowX - pillHalf - 6 && slotX < nowX + pillHalf + 4;
}

/**
 * OK on a TV tunes the show that is on now, the way a cable box does. Holding
 * OK, or Info, opens the details. Anywhere else Enter opens the details and
 * Space watches.
 */
export function guideEnterAction(tv: boolean, airingNow: boolean, held: boolean): "watch" | "sheet" {
  if (!tv || held || !airingNow) return "sheet";
  return "watch";
}

/** Keys that open a show's details on a TV remote without tuning. */
export function isInfoKey(key: string): boolean {
  return key === "i" || key === "I" || key === "Info" || key === "ContextMenu" || key === "F1";
}
