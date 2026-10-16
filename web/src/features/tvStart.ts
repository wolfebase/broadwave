import { useEffect, type RefObject } from "react";
import { useLayout } from "../app/layout";
import { focusRing } from "../app/remote";

/** On a TV the remote starts on the page's content, not on the tab it came from. */
export function useTvStartFocus(root: RefObject<HTMLElement | null>, selector: string, key: unknown) {
  const layout = useLayout();
  useEffect(() => {
    if (layout !== "tv") return;
    const frame = window.requestAnimationFrame(() => {
      const active = document.activeElement;
      if (active instanceof HTMLElement && active !== document.body && !active.closest(".topbar")) return;
      focusRing(root.current?.querySelector<HTMLElement>(selector));
    });
    return () => window.cancelAnimationFrame(frame);
  }, [layout, root, selector, key]);
}
