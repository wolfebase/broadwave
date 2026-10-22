import { useEffect, type RefObject } from "react";
import { useLayout } from "../app/layout";
import { focusRing, inputOnThisPage } from "../app/remote";

/**
 * On a TV the remote starts on the page's content, not on the tab it came from
 * or the first button the shell found while the page was still loading.
 * Once the person presses a key or touches the screen on this page, focus is theirs.
 */
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
      const target = root.current?.querySelector<HTMLElement>(selector);
      // The page's lists can arrive after the first frame.
      if (!target) {
        if (tries++ < 15) timer = window.setTimeout(place, 100);
        return;
      }
      if (target !== active) focusRing(target);
    };
    timer = window.setTimeout(place, 16);
    return () => window.clearTimeout(timer);
  }, [layout, root, selector, key]);
}
