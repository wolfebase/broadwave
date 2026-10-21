import { useEffect, useSyncExternalStore } from "react";
import { useData } from "./data";
import { isLayout, pickLayout, type Layout } from "../lib/layoutPick";

export type { Layout };

const phone = window.matchMedia("(max-width: 760px)");
// Coarse or no pointer on a wide window. Touch is checked apart: an iPad matches this too.
const tv = window.matchMedia("(pointer: coarse) and (min-width: 1100px), (pointer: none) and (min-width: 1100px)");
const rememberKey = "broadwave-layout";

/** ?layout=tv (or desktop, phone) sticks to this browser; ?layout=auto forgets it. */
function remembered(forced: string): string {
  try {
    if (isLayout(forced)) localStorage.setItem(rememberKey, forced);
    else if (forced === "auto") localStorage.removeItem(rememberKey);
    return localStorage.getItem(rememberKey) ?? "";
  } catch {
    return "";
  }
}

function subscribe(fn: () => void) {
  phone.addEventListener("change", fn);
  tv.addEventListener("change", fn);
  return () => {
    phone.removeEventListener("change", fn);
    tv.removeEventListener("change", fn);
  };
}

/** Settings › Layout › Auto also forgets a ?layout= this browser was given. */
export function forgetLayoutChoice() {
  try {
    localStorage.removeItem(rememberKey);
  } catch {
    // Storage blocked: nothing was remembered.
  }
}

function detect(): Layout {
  const forced = new URLSearchParams(window.location.search).get("layout") ?? "";
  return pickLayout({
    forced,
    remembered: remembered(forced),
    narrow: phone.matches,
    wideCoarse: tv.matches,
    touchPoints: navigator.maxTouchPoints || 0,
    userAgent: navigator.userAgent,
  });
}

/** The layout in effect: a saved choice wins, otherwise the window decides. */
export function useLayout(): Layout {
  const { settings } = useData();
  const auto = useSyncExternalStore(subscribe, detect);
  const saved = settings.layout;
  const layout: Layout = saved === "desktop" || saved === "tv" || saved === "phone" ? saved : auto;
  useEffect(() => {
    const root = document.documentElement;
    root.dataset.layout = layout;
    // The player is rendered beside the shell, so the layout class has to live on <html>.
    root.classList.remove("layout-phone", "layout-desktop", "layout-tv");
    root.classList.add(`layout-${layout}`);
  }, [layout]);
  return layout;
}
