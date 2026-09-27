import { useSyncExternalStore } from "react";

const listeners = new Set<() => void>();

function current() {
  return window.location.pathname + window.location.search;
}

window.addEventListener("popstate", () => listeners.forEach((fn) => fn()));

export function navigate(to: string, replace = false) {
  if (to === current()) return;
  const depth = inAppDepth();
  if (replace) window.history.replaceState({ depth }, "", to);
  else window.history.pushState({ depth: depth + 1 }, "", to);
  listeners.forEach((fn) => fn());
}

/** How many entries back the app itself pushed; 0 on the page a viewer opened. */
export function inAppDepth(): number {
  const depth = (window.history.state as { depth?: unknown } | null)?.depth;
  return typeof depth === "number" ? depth : 0;
}

export function useRoute(): { path: string; params: URLSearchParams } {
  const full = useSyncExternalStore(
    (fn) => {
      listeners.add(fn);
      return () => listeners.delete(fn);
    },
    current,
  );
  const [path, search = ""] = full.split("?");
  return { path, params: new URLSearchParams(search) };
}
