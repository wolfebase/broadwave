import { useSyncExternalStore } from "react";

const listeners = new Set<() => void>();

function current() {
  return window.location.pathname + window.location.search;
}

window.addEventListener("popstate", () => listeners.forEach((fn) => fn()));

export function navigate(to: string, replace = false) {
  if (to === current()) return;
  if (replace) window.history.replaceState({}, "", to);
  else window.history.pushState({}, "", to);
  listeners.forEach((fn) => fn());
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
