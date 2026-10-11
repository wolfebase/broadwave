import { formatBytes } from "../../lib/format.ts";

/** now is a clean-up that runs on its own. next waits until another episode of that show is saved. */
export function removalWhen(when: string | undefined): string {
  if (when === "now") return "Removed on its own.";
  if (when === "next") return "Removed the next time another episode of this show is saved.";
  return "";
}

type SpaceFacts = {
  freeBytes: number;
  reserveBytes: number;
  makeRoom: boolean;
  deleteWatchedDays: number;
};

/** Free space against the reserve, then whether making room and deleting watched recordings are on. */
export function spaceLine(plan: SpaceFacts): string {
  return `${freeLine(plan.freeBytes, plan.reserveBytes)} ${roomLine(plan.makeRoom)} ${daysLine(plan.deleteWatchedDays)}`;
}

function freeLine(freeBytes: number, reserveBytes: number): string {
  const free = formatBytes(freeBytes);
  if (reserveBytes <= 0) return `${free} free. The free-space reserve is off.`;
  const reserve = formatBytes(reserveBytes);
  if (freeBytes < reserveBytes) return `${free} free, under the ${reserve} reserve.`;
  if (freeBytes > reserveBytes) return `${free} free, above the ${reserve} reserve.`;
  return `${free} free, at the ${reserve} reserve.`;
}

function roomLine(makeRoom: boolean): string {
  return makeRoom ? "Making room is on." : "Making room is off.";
}

function daysLine(days: number): string {
  if (days === 1) return "Watched recordings are deleted after 1 day.";
  if (days > 1) return `Watched recordings are deleted after ${days} days.`;
  return "Deleting watched recordings is off.";
}
