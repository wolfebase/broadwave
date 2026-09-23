import { useEffect, useSyncExternalStore } from "react";
import { useData } from "./data";

export type Layout = "desktop" | "tv" | "phone";

const phone = window.matchMedia("(max-width: 760px)");
const tv = window.matchMedia("(pointer: coarse) and (min-width: 1100px)");

function subscribe(fn: () => void) {
  phone.addEventListener("change", fn);
  tv.addEventListener("change", fn);
  return () => {
    phone.removeEventListener("change", fn);
    tv.removeEventListener("change", fn);
  };
}

function detect(): Layout {
  const forced = new URLSearchParams(window.location.search).get("layout");
  if (forced === "tv" || forced === "phone" || forced === "desktop") return forced;
  return phone.matches ? "phone" : tv.matches ? "tv" : "desktop";
}

/** The layout in effect: a saved choice wins, otherwise the window decides. */
export function useLayout(): Layout {
  const { settings } = useData();
  const auto = useSyncExternalStore(subscribe, detect);
  const saved = settings.layout;
  const layout: Layout = saved === "desktop" || saved === "tv" || saved === "phone" ? saved : auto;
  useEffect(() => {
    document.documentElement.dataset.layout = layout;
  }, [layout]);
  return layout;
}
