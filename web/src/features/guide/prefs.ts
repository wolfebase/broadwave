import type { Category } from "../../lib/guide";
import { primeHour } from "../../lib/primeHour.ts";

export type GuideFilter = "all" | "favorites" | Category | "recording";

const filterKey = "broadwave-guide-filter";
const filters: readonly GuideFilter[] = ["all", "favorites", "sports", "news", "movies", "kids", "series", "other", "recording"];

/** A saved filter, or All for anything this version does not know. */
export function parseGuideFilter(raw: string | null | undefined): GuideFilter {
  return filters.find((f) => f === raw) ?? "all";
}

/** The filter this screen used last. Leaving the guide or reloading keeps it. */
export function readGuideFilter(): GuideFilter {
  try {
    return parseGuideFilter(globalThis.localStorage?.getItem(filterKey));
  } catch {
    return "all";
  }
}

export function saveGuideFilter(filter: GuideFilter) {
  try {
    if (filter === "all") localStorage.removeItem(filterKey);
    else localStorage.setItem(filterKey, filter);
  } catch {
    // A private window can refuse the write. The filter still holds for this visit.
  }
}

const MIN = 60_000;

/**
 * Where a day chip lands: half an hour before prime time that day (8 PM, or
 * 7 PM in the Central and Mountain zones). A guide with
 * only a day or two of listings ends before then on most channels, so the
 * jump stops where most channels still have listings, without leaving the day.
 */
export function dayJumpTarget(midnight: number, lastEnds: readonly number[], visibleMs: number, hour: number = primeHour(midnight)): number {
  const prime = new Date(midnight);
  prime.setHours(hour, 0, 0, 0);
  const target = prime.getTime() - 30 * MIN;
  if (lastEnds.length === 0) return target;
  const sorted = [...lastEnds].sort((a, b) => a - b);
  const covered = sorted[Math.floor((sorted.length - 1) / 2)];
  if (covered <= midnight || target + visibleMs <= covered) return target;
  const at = new Date(covered - visibleMs);
  at.setMinutes(at.getMinutes() < 30 ? 0 : 30, 0, 0);
  return Math.max(midnight, at.getTime());
}
