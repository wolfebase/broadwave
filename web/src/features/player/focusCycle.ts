/** Where Tab goes inside the player. Null lets the browser move to the next control. */
export function playerTabTarget(count: number, index: number, shift: boolean): number | null {
  if (count <= 0) return null;
  // Focus is on the picture, not a control. The first Tab enters the bar.
  if (index < 0) return shift ? count - 1 : 0;
  const next = index + (shift ? -1 : 1);
  if (next < 0 || next >= count) return (next + count) % count;
  return null;
}

/**
 * Buttons and fields a viewer can Tab to. The stage itself is left out: it
 * takes the keys when the bar is hidden, and focusing it draws no ring.
 * A control inside an inert bar is hidden with it, so it is not a stop.
 */
export function playerControls(root: ParentNode | null): HTMLElement[] {
  if (!root) return [];
  const out: HTMLElement[] = [];
  for (const el of root.querySelectorAll<HTMLElement>("button, a[href], input, select, textarea")) {
    if (el.tabIndex < 0 || el.hasAttribute("disabled") || el.hidden) continue;
    if (el.closest("[inert], [hidden]")) continue;
    if (el.getAttribute("aria-hidden") === "true") continue;
    if (el instanceof HTMLInputElement && el.type === "hidden") continue;
    const style = getComputedStyle(el);
    if (style.display === "none" || style.visibility === "hidden") continue;
    if (el.getClientRects().length === 0) continue;
    out.push(el);
  }
  return out;
}
