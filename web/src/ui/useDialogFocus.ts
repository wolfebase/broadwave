import { useEffect, useRef, type RefObject } from "react";
import { focusRing } from "../app/remote";
import { isTextField, trapTab } from "../lib/dialogFocus";

function items(root: ParentNode | null): HTMLElement[] {
  if (!root) return [];
  return [...root.querySelectorAll<HTMLElement>("button, a[href], input, select, textarea")].filter((el) => {
    if (el.hidden || el.hasAttribute("disabled") || el.getAttribute("aria-hidden") === "true") return false;
    if (el.tabIndex < 0 || el.closest("[inert], [hidden]")) return false;
    if (el instanceof HTMLInputElement && el.type === "hidden") return false;
    return true;
  });
}

/** Move focus into a dialog, keep Tab inside it, and put focus back when it closes. */
export function useDialogFocus(ref: RefObject<HTMLElement | null>, onEscape?: () => void) {
  const escapeRef = useRef(onEscape);
  useEffect(() => {
    escapeRef.current = onEscape;
  });
  useEffect(() => {
    const root = ref.current;
    const prev = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const open = items(root);
    const primary = open.find((el) => el.classList.contains("primary"));
    focusRing(primary ?? open[0] ?? root);
    const onKey = (event: KeyboardEvent) => {
      if ((event.key === "Escape" || event.key === "Backspace") && escapeRef.current) {
        const target = event.target;
        // A checkbox and a slider are inputs, and Back still closes.
        if (event.key === "Backspace" && isTextField(target)) return;
        event.preventDefault();
        event.stopPropagation();
        escapeRef.current();
        return;
      }
      // Ctrl+Tab and Alt+Tab belong to the browser.
      if (event.key !== "Tab" || event.altKey || event.metaKey || event.ctrlKey) return;
      const list = items(root);
      if (list.length === 0) {
        event.preventDefault();
        event.stopPropagation();
        focusRing(root);
        return;
      }
      const next = trapTab(list, document.activeElement instanceof HTMLElement ? document.activeElement : null, event.shiftKey);
      if (!next) return;
      event.preventDefault();
      event.stopPropagation();
      focusRing(next);
    };
    window.addEventListener("keydown", onKey, true);
    return () => {
      window.removeEventListener("keydown", onKey, true);
      if (!prev || !prev.isConnected || prev === document.body) return;
      // The control that opened the dialog may have faded inert with the chrome.
      if (prev.closest("[inert]")) {
        focusRing(document.querySelector<HTMLElement>(".stage:not(.mini)"));
        return;
      }
      focusRing(prev);
    };
  }, [ref]);
}
