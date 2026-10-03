import { firstBelow, nearest, type Box, type Direction } from "../lib/remote";
import { inAppDepth } from "./router";

const selector = "button, a[href], input, select, textarea, [tabindex]";

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
    const text = type === "text" || type === "search" || type === "password" || type === "email" || type === "url" || type === "";
    if (text || type === "range") return along;
    return false;
  }
  return false;
}

function boxes(scope: ParentNode) {
  const out: { el: HTMLElement; box: Box }[] = [];
  for (const node of scope.querySelectorAll<HTMLElement>(selector)) {
    // An inert control sits behind the player. Focusing it would drop the keys.
    if (node.tabIndex < 0 || node.closest("[inert], [aria-hidden='true']")) continue;
    if ((node as HTMLButtonElement).disabled) continue;
    const rect = node.getBoundingClientRect();
    if (rect.width < 2 || rect.height < 2) continue;
    out.push({ el: node, box: { id: out.length, left: rect.left, top: rect.top, right: rect.right, bottom: rect.bottom } });
  }
  return out;
}

/** A plain focus() does not match :focus-visible, so the ring would not draw. */
export function focusRing(el: HTMLElement | null | undefined) {
  el?.focus({ focusVisible: true } as FocusOptions);
}

function editing(target: HTMLElement | null) {
  return Boolean(target && (target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.isContentEditable));
}

/** One level back. A text field keeps Backspace so it can delete a character. */
function goBack(event: KeyboardEvent, target: HTMLElement | null) {
  if (event.key !== "Escape" && event.key !== "Backspace") return false;
  if (editing(target)) {
    if (event.key === "Escape") {
      target?.blur();
      event.preventDefault();
    }
    return true;
  }
  // The open dialog closes itself. Leaving now would skip that level.
  if (document.querySelector("[role='dialog']")) return true;
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
    const target = event.target instanceof HTMLElement ? event.target : null;
    if (goBack(event, target)) return;
    const dir = arrow(event.key);
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
    const from: Box = current?.box ?? { id: -1, left: window.innerWidth / 2 - 1, top: -2, right: window.innerWidth / 2 + 1, bottom: 0 };
    const focused = current?.el ?? target;
    const inBar = Boolean(focused?.closest(".topbar"));
    // Left and Right stay on the tab bar. A scrolled field can sit under the sticky
    // bar and look closer than the next tab, and a text field then keeps the keys.
    const alongBar = (dir === "left" || dir === "right") && inBar;
    const pool = alongBar ? items.filter((item) => item.el.closest(".topbar")) : items;
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
