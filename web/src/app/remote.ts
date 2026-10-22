import { alignedAhead, groupAxis, stepInGroup } from "../lib/focusStep";
import { isTextField } from "../lib/textField";
import { arrowScope, firstBelow, nearest, type Box, type Direction } from "../lib/remote";
import { canonicalKey } from "../lib/tvKey";
import { inAppDepth } from "./router";

const selector = "button, a[href], input, select, textarea, [tabindex]";

// The page the last key press or touch happened on. The press that opens a page
// happens on the page before it. This loads with the shell, so a press made
// before a page's own code arrives still counts, and the page does not take
// the focus back when its list lands.
let inputPath = "";
const mark = () => {
  inputPath = window.location.pathname;
};
window.addEventListener("keydown", mark, true);
window.addEventListener("pointerdown", mark, true);

/** The person has pressed a key or touched the screen on this page. */
export function inputOnThisPage() {
  return inputPath === window.location.pathname;
}

// Rows and lists the remote should walk in order. A tile on the multiview is
// also role=group, and trapping arrows inside it would skip the next tile.
// A game card wraps its buttons, so it is not a row: Down has to reach the next line.
const groupSelector = ".shelf-row, .library-chips, .search-list > li, .source-list > li, .pass-list > li, .setup-list > li, .together-list > li, .guide-chips, .segmented, [role='listbox'], [role='menu'], [role='radiogroup']";

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

/** The picture fills the screen. While its buttons are up, it must not take the arrow. */
function pictureWithChrome(node: HTMLElement) {
  if (!node.matches("section.stage:not(.mini)")) return false;
  for (const el of node.querySelectorAll<HTMLElement>("button, a[href], input, select, textarea")) {
    if (el.tabIndex < 0 || (el as HTMLButtonElement).disabled || el.closest("[inert], [aria-hidden='true']")) continue;
    const rect = el.getBoundingClientRect();
    if (rect.width >= 2 && rect.height >= 2) return true;
  }
  return false;
}

function boxes(scope: ParentNode) {
  const out: { el: HTMLElement; box: Box }[] = [];
  for (const node of scope.querySelectorAll<HTMLElement>(selector)) {
    // An inert control sits behind the player. Focusing it would drop the keys.
    if (node.tabIndex < 0 || node.closest("[inert], [aria-hidden='true']")) continue;
    if ((node as HTMLButtonElement).disabled) continue;
    // The skip link is for Tab. Once an arrow lands on it, nothing is above or beside it to leave by.
    if (node.matches(".skip-link")) continue;
    if (pictureWithChrome(node)) continue;
    // The multiview page fills the screen. Its center sits on the tiles, so
    // Down would land on the page instead of the buttons under the grid.
    if (node.matches("section.mv") && node.querySelector("button:not([disabled]), [tabindex='0']")) continue;
    const rect = node.getBoundingClientRect();
    if (rect.width < 2 || rect.height < 2) continue;
    // A notice across the top keeps its buttons at the far right. Up from the page
    // has to land on it, not skip past it to the tab bar.
    const notice = node.closest<HTMLElement>(".banner-update, .banner-home");
    const row = notice ? notice.getBoundingClientRect() : null;
    out.push({ el: node, box: { id: out.length, ...onScreen(node, rect), ...(row ? { rowLeft: row.left, rowRight: row.right } : {}) } });
  }
  return out;
}

/**
 * A notice closed from the keyboard or a remote takes its focused button with it,
 * and the keys would land nowhere. The ring moves to the page's first control.
 * A mouse click (detail > 0) leaves focus to the pointer, except on a TV,
 * where a pointer remote still needs a ring for the arrows that follow.
 */
export function focusPastNotice(event: { detail: number; currentTarget: EventTarget | null }) {
  const notice = event.currentTarget instanceof HTMLElement ? event.currentTarget.closest(".banner-update, .banner-home") : null;
  const tv = document.documentElement.dataset.layout === "tv";
  if (!notice || (event.detail !== 0 && !tv)) return;
  window.setTimeout(() => {
    const active = document.activeElement;
    if (active && active !== document.body && document.contains(active)) return;
    const next = [...document.querySelectorAll<HTMLElement>("main button:not([disabled]), main a[href]")].find(
      (el) => !el.closest(".banner-update, .banner-home, [inert]") && el.getBoundingClientRect().height > 1,
    );
    focusRing(next);
  }, 0);
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
/** Past the end of a row or list. A control beside it, outside the group, is still this way. */
function pastEnd(group: Element, from: Box, dir: Direction, items: { el: HTMLElement; box: Box }[]): HTMLElement | "stay" {
  const outside = items.filter((item) => !group.contains(item.el));
  const hit = alignedAhead(from, outside.map((item) => item.box), dir);
  if (hit != null) return outside[hit].el;
  // The shelf's own heading action (Guide, All sports) and the mini player in
  // the corner are still reachable. Nothing else is: Right at the end of a
  // shelf must not drop into the next one.
  const heading = group.closest(".shelf")?.querySelector(":scope > .section-header");
  const lone = outside.filter((item) => item.el.closest(".stage.mini") || heading?.contains(item.el));
  const pool = lone.map((item) => item.box);
  const id = nearest(from, pool, dir) ?? nearest(from, pool, dir, true);
  return lone.find((item) => item.box.id === id)?.el ?? "stay";
}

function withinGroup(current: HTMLElement, dir: Direction, items: { el: HTMLElement; box: Box }[]): HTMLElement | "stay" | null {
  const group = current.closest(groupSelector);
  if (!group) return null;
  const inside = items.filter((item) => group.contains(item.el));
  const index = inside.findIndex((item) => item.el === current);
  if (index < 0) return null;
  // A shelf with one card is still a row. Left and Right end there.
  if (inside.length < 2) return dir === "left" || dir === "right" ? pastEnd(group, inside[index].box, dir, items) : null;
  const boxes = inside.map((item) => item.box);
  const stepped = stepInGroup(index, inside.length, dir, groupAxis(boxes), boxes);
  if (stepped === "stay") return pastEnd(group, inside[index].box, dir, items);
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
    const inDialog = dialog ? boxes(dialog) : [];
    const items = arrowScope(dialog, inDialog.length) == null ? boxes(document) : inDialog;
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
      // A notice across the top is not the page. Its Not now sits at the far end,
      // and landing there throws the ring across the screen; Up from the page reaches it.
      const inMain = items.filter((item) => item.el.closest("main"));
      const content = inMain.filter((item) => !item.el.closest(".banner-update, .banner-home"));
      const page = (content.length > 0 ? content : inMain).map((item) => item.box);
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
