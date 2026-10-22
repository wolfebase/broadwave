import { useEffect, type RefObject } from "react";
import { useLayout } from "../app/layout";
import { focusRing, inputOnThisPage, setPageStart } from "../app/remote";

/**
 * On a TV the remote starts on the page's content, not on the tab it came from
 * or the first button the shell found while the page was still loading.
 * Once the person presses a key or touches the screen on this page, focus is theirs.
 */
/**
 * The first selector, in the order given, that matches. querySelector with a list
 * returns the first in the page, so the filter chips above the list would win over
 * the newest recording.
 */
export function startTarget(root: ParentNode | null | undefined, selector: string): HTMLElement | null {
  if (!root) return null;
  for (const one of selector.split(",")) {
    const found = root.querySelector<HTMLElement>(one.trim());
    if (found) return found;
  }
  return null;
}

export function useTvStartFocus(root: RefObject<HTMLElement | null>, selector: string, key: unknown) {
  const layout = useLayout();
  useEffect(() => {
    if (layout !== "tv") return;
    let tries = 0;
    let timer = 0;
    const place = () => {
      const active = document.activeElement;
      const idle = !(active instanceof HTMLElement) || active === document.body || active === document.documentElement;
      if (!idle && (inputOnThisPage() || active.closest("[role='dialog'], .stage"))) return;
      const target = startTarget(root.current, selector);
      // The page's lists can arrive after the first frame.
      if (!target) {
        if (tries++ < 15) timer = window.setTimeout(place, 100);
        return;
      }
      if (target !== active) focusRing(target);
    };
    timer = window.setTimeout(place, 16);
    const unregister = setPageStart(() => startTarget(root.current, selector));
    return () => {
      window.clearTimeout(timer);
      unregister();
    };
  }, [layout, root, selector, key]);
}
