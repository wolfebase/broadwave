import { nearest, type Box, type Direction } from "../lib/remote";
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
  if (tag === "TEXTAREA" || tag === "SELECT") return true;
  if (tag === "INPUT") {
    const type = (target as HTMLInputElement).type;
    const text = type === "text" || type === "search" || type === "password" || type === "email" || type === "url" || type === "";
    return text ? dir === "left" || dir === "right" : true;
  }
  return false;
}

function boxes(scope: ParentNode) {
  const out: { el: HTMLElement; box: Box }[] = [];
  for (const node of scope.querySelectorAll<HTMLElement>(selector)) {
    if (node.tabIndex < 0 || node.closest("[aria-hidden='true']")) continue;
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
    if (target && keepsArrows(target, dir)) return;
    const dialog = document.querySelector<HTMLElement>("[role='dialog']");
    const items = boxes(dialog ?? document);
    if (items.length === 0) return;
    const current = items.find((item) => item.el === document.activeElement);
    const from: Box = current?.box ?? { id: -1, left: window.innerWidth / 2 - 1, top: -2, right: window.innerWidth / 2 + 1, bottom: 0 };
    const nextId = nearest(from, items.map((item) => item.box), dir);
    const next = items.find((item) => item.box.id === nextId);
    if (!next) return;
    event.preventDefault();
    focusRing(next.el);
  };
  window.addEventListener("keydown", onKey);
  return () => window.removeEventListener("keydown", onKey);
}
