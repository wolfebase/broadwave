import { timeMessages } from "./i18n/messages/time.ts";
import { bind, formatRelative, formatTime } from "./i18n/runtime.ts";

const t = bind(timeMessages);

export function halfHourFloor(date: Date): Date {
  const next = new Date(date);
  next.setSeconds(0, 0);
  next.setMinutes(next.getMinutes() < 30 ? 0 : 30);
  return next;
}

export function addMinutes(date: Date, minutes: number): Date {
  return new Date(date.getTime() + minutes * 60_000);
}

export function formatClock(date: Date): string {
  return formatTime(date);
}

// How long ago a device answered, for the offline line on its card.
// Empty when the stamp is missing or not a time.
export function lastSeenPhrase(iso: string | undefined, now = Date.now()): string {
  if (!iso) return "";
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return "";
  const sec = Math.max(0, Math.round((now - then) / 1000));
  if (sec < 45) return t("time.justNow");
  const min = Math.round(sec / 60);
  if (min < 60) return formatRelative(-min, "minute");
  const hours = Math.round(min / 60);
  if (hours < 24) return formatRelative(-hours, "hour");
  const days = Math.round(hours / 24);
  return formatRelative(-days, "day");
}

export const SLOT_MINUTES = 30;
export const SLOT_COUNT = 48;
