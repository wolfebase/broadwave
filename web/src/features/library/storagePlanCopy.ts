import { libraryMessages } from "../../i18n/messages/library.ts";
import { bind } from "../../i18n/runtime.ts";
import { formatBytes } from "../../lib/format.ts";

const t = bind(libraryMessages);

/** now is a clean-up that runs on its own. next waits until another episode of that show is saved. */
export function removalWhen(when: string | undefined): string {
  if (when === "now") return t("storage.removedNow");
  if (when === "next") return t("storage.removedNext");
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
  if (reserveBytes <= 0) return t("storage.reserveOff", { free });
  const reserve = formatBytes(reserveBytes);
  if (freeBytes < reserveBytes) return t("storage.underReserve", { free, reserve });
  if (freeBytes > reserveBytes) return t("storage.aboveReserve", { free, reserve });
  return t("storage.atReserve", { free, reserve });
}

function roomLine(makeRoom: boolean): string {
  return makeRoom ? t("storage.roomOn") : t("storage.roomOff");
}

function daysLine(days: number): string {
  return t("storage.watchedDays", days);
}
