import { useSyncExternalStore } from "react";
import { getFrames } from "../api";
import { events } from "../lib/events";

// One read for every card. A preview is requested only after the server lists it.
const every = 60_000;
let ids = new Set<number>();
const listeners = new Set<() => void>();
let started = false;
let timer = 0;
let offLive: (() => void) | null = null;

function emit() {
  for (const fn of listeners) fn();
}

function same(a: Set<number>, b: Set<number>) {
  if (a.size !== b.size) return false;
  for (const id of a) if (!b.has(id)) return false;
  return true;
}

async function refresh() {
  try {
    const body = await getFrames();
    const next = new Set(body.channels);
    if (same(ids, next)) return;
    ids = next;
    emit();
  } catch {
    // A missed read keeps the last list. The next minute tries again.
  }
}

function arm() {
  window.clearInterval(timer);
  timer = 0;
  if (document.visibilityState === "hidden") return;
  timer = window.setInterval(() => void refresh(), every);
}

function onVisible() {
  if (document.visibilityState !== "visible") {
    window.clearInterval(timer);
    timer = 0;
    return;
  }
  arm();
  void refresh();
}

function start() {
  if (started) return;
  started = true;
  void refresh();
  offLive = events().on("live.changed", () => void refresh());
  document.addEventListener("visibilitychange", onVisible);
  arm();
}

function stop() {
  if (listeners.size > 0) return;
  started = false;
  offLive?.();
  offLive = null;
  window.clearInterval(timer);
  timer = 0;
  document.removeEventListener("visibilitychange", onVisible);
}

function subscribe(fn: () => void) {
  listeners.add(fn);
  start();
  return () => {
    listeners.delete(fn);
    stop();
  };
}

export function useHasFrame(id: number) {
  return useSyncExternalStore(subscribe, () => ids.has(id), () => false);
}
