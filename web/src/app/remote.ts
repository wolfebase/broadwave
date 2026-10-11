import { groupAxis, stepInGroup } from "../lib/focusStep";
import { isTextField } from "../lib/textField";
import { firstBelow, nearest, type Box, type Direction } from "../lib/remote";
import { canonicalKey } from "../lib/tvKey";
import { inAppDepth } from "./router";

const selector = "button, a[href], input, select, textarea, [tabindex]";

// Rows and lists the remote should walk in order. A tile on the multiview is
// also role=group, and trapping arrows inside it would skip the next tile.
// A game card wraps its buttons, so it is not a row: Down has to reach the next line.
const groupSelector = ".shelf-row, .search-list > li, .source-list > li, .pass-list > li, .setup-list > li, .together-list > li, .guide-chips, .segmented, [role='listbox'], [role='menu'], [role='radiogroup']";

function arrow(key: string): Direction | null {
  if (key === "ArrowLeft") return "left";
  if (key === "ArrowRight") return "right";
  if (key === "ArrowUp") return "up";
  if (key === "ArrowDown") return "down";
  return null;
}

function keepsArrows(target: HTMLElement, dir: Direction): boolean {
  if (target.isContentEditable) return true;
  const tag = target.tagName;
  if (tag === "TEXTAREA") return true;
  // Up and Down leave a closed menu, a checkbox, and a file field. Left and Right
  // still edit a line of text or a menu. A remote that stops on the first menu
  // cannot reach the rest of the page.
  const along = dir === "left" || dir === "right";
  if (tag === "SELECT") return along;
  if (tag === "INPUT") {
    const type = (target as HTMLInputElement).type;
    const text = type === "text" || type === "search" || type === "password" || type === "email" || type === "url" || type === "tel" || type === "";
    if (text || type === "range" || type === "number" || type === "time" || type === "date" || type === "datetime-local") return along;
    return false;
  }
  return false;
}

/**
 * The part of a box the viewer can see. The guide grid is far bigger than the
 * screen, so its center sits below the mini player and Up would skip it. A box
 * inside a scroller is cut to the scroller. A box wholly off screen keeps its
 * place, so Down still reaches it.
 */
function onScreen(node: HTMLElement, rect: DOMRect) {
  let left = Math.max(rect.left, 0);
  let top = Math.max(rect.top, 0);
  let right = Math.min(rect.right, window.innerWidth);
  let bottom = Math.min(rect.bottom, window.innerHeight);
  const parent = node.parentElement;
  if (parent && (parent.scrollHeight > parent.clientHeight || parent.scrollWidth > parent.clientWidth)) {
    const view = parent.getBoundingClientRect();
    left = Math.max(left, view.left);
    top = Math.max(top, view.top);
    right = Math.min(right, view.right);
    bottom = Math.min(bottom, view.bottom);
  }
  if (right - left < 2 || bottom - top < 2) return { left: rect.left, top: rect.top, right: rect.right, bottom: rect.bottom };
  return { left, top, right, bottom };
}

function boxes(scope: ParentNode) {
  const out: { el: HTMLElement; box: Box }[] = [];
  for (const node of scope.querySelectorAll<HTMLElement>(selector)) {
    // An inert control sits behind the player. Focusing it would drop the keys.
    if (node.tabIndex < 0 || node.closest("[inert], [aria-hidden='true']")) continue;
    if ((node as HTMLButtonElement).disabled) continue;
    const rect = node.getBoundingClientRect();
    if (rect.width < 2 || rect.height < 2) continue;
    out.push({ el: node, box: { id: out.length, ...onScreen(node, rect) } });
  }
  return out;
}

/** A plain focus() does not match :focus-visible, so the ring would not draw. */
export function focusRing(el: HTMLElement | null | undefined) {
  el?.focus({ focusVisible: true } as FocusOptions);
}

function editing(target: HTMLElement | null) {
  return isTextField(target);
}

/**
 * The next control in the same row or list, in order.
 * "stay" means this direction ends here. Null lets the page-wide search run.
 */
function withinGroup(current: HTMLElement, dir: Direction, items: { el: HTMLElement; box: Box }[]): HTMLElement | "stay" | null {
  const group = current.closest(groupSelector);
  if (!group) return null;
  const inside = items.filter((item) => group.contains(item.el));
  if (inside.length < 2) return null;
  const index = inside.findIndex((item) => item.el === current);
  if (index < 0) return null;
  const boxes = inside.map((item) => item.box);
  const stepped = stepInGroup(index, inside.length, dir, groupAxis(boxes), boxes);
  if (stepped === "stay") return "stay";
  if (typeof stepped === "number") return inside[stepped].el;
  // The next line of a wrapped row starts past this control. A sibling on this
  // line can sit a little lower, and Down must not stop there.
  const from = inside[index].box;
  const onward = inside.filter((item) => {
    if (item.el === current) return false;
    if (dir === "down") return item.box.top >= from.bottom - 1;
    if (dir === "up") return item.box.bottom <= from.top + 1;
    if (dir === "right") return item.box.left >= from.right - 1;
    return item.box.right <= from.left + 1;
  });
  if (onward.length === 0) return null;
  const pool = onward.map((item) => item.box);
  const id = nearest(from, pool, dir) ?? nearest(from, pool, dir, true);
  return onward.find((item) => item.box.id === id)?.el ?? null;
}

/** One level back. A text field keeps Backspace so it can delete a character. */
function goBack(event: KeyboardEvent, target: HTMLElement | null, key: string) {
  if (key !== "Escape" && key !== "Backspace") return false;
  if (editing(target)) {
    if (key === "Escape") {
      target?.blur();
      event.preventDefault();
    }
    return true;
  }
  // The open dialog closes itself. Leaving now would skip that level.
  if (document.querySelector("[role='dialog']")) return true;
  const offer = target?.closest(".install-offer");
  if (offer) {
    event.preventDefault();
    offer.querySelector<HTMLButtonElement>("[data-install-dismiss]")?.click();
    return true;
  }
  if (inAppDepth() > 0) {
    event.preventDefault();
    window.history.back();
  }
  return true;
}

/** Arrow keys move focus. Enter still activates the focused control. Escape and Backspace go back one level. */
export function installTvRemote(): () => void {
  const onKey = (event: KeyboardEvent) => {
    if (event.defaultPrevented || event.metaKey || event.ctrlKey || event.altKey) return;
    const key = canonicalKey(event);
    const target = event.target instanceof HTMLElement ? event.target : null;
    if (goBack(event, target, key)) return;
    const dir = arrow(key);
    if (!dir) return;
    // A closed menu does not move on Left or Right. Step it here so a remote can change it.
    if (target instanceof HTMLSelectElement && (dir === "left" || dir === "right")) {
      const next = target.selectedIndex + (dir === "right" ? 1 : -1);
      if (next >= 0 && next < target.options.length) {
        target.selectedIndex = next;
        target.dispatchEvent(new Event("change", { bubbles: true }));
      }
      event.preventDefault();
      return;
    }
    if (target && keepsArrows(target, dir)) return;
    const dialog = document.querySelector<HTMLElement>("[role='dialog']");
    const items = boxes(dialog ?? document);
    if (items.length === 0) return;
    const current = items.find((item) => item.el === document.activeElement);
    const focused = current?.el ?? target;
    let leaving: HTMLElement | null = null;
    if (focused) {
      const grouped = withinGroup(focused, dir, items);
      if (grouped === "stay") {
        event.preventDefault();
        return;
      }
      if (grouped) {
        event.preventDefault();
        focusRing(grouped);
        return;
      }
      leaving = focused.closest(groupSelector);
    }
    const from: Box = current?.box ?? { id: -1, left: window.innerWidth / 2 - 1, top: -2, right: window.innerWidth / 2 + 1, bottom: 0 };
    const inBar = Boolean(focused?.closest(".topbar"));
    // Left and Right stay on the tab bar. A scrolled field can sit under the sticky
    // bar and look closer than the next tab, and a text field then keeps the keys.
    const alongBar = (dir === "left" || dir === "right") && inBar;
    const pool = (alongBar ? items.filter((item) => item.el.closest(".topbar")) : items).filter((item) => !leaving || !leaving.contains(item.el));
    const all = pool.map((item) => item.box);
    // Down from the bar enters at the top of the page. A tab that happens to sit
    // over Delete would otherwise skip Play.
    let nextId: number | null = null;
    if (dir === "down" && inBar) {
      const page = items.filter((item) => item.el.closest("main")).map((item) => item.box);
      nextId = firstBelow(from, page);
    } else {
      nextId = nearest(from, all, dir);
      // Nothing shares this row or column. Still move, so a corner control
      // (the mini player) is reachable.
      if (nextId == null) nextId = nearest(from, all, dir, true);
    }
    const next = items.find((item) => item.box.id === nextId);
    if (!next) return;
    event.preventDefault();
    focusRing(next.el);
  };
  window.addEventListener("keydown", onKey);
  return () => window.removeEventListener("keydown", onKey);
}
