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
  return date.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
}

// How long ago a device answered, for the offline line on its card.
// Empty when the stamp is missing or not a time.
export function lastSeenPhrase(iso: string | undefined, now = Date.now()): string {
  if (!iso) return "";
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return "";
  const sec = Math.max(0, Math.round((now - then) / 1000));
  if (sec < 45) return "just now";
  const min = Math.round(sec / 60);
  if (min < 60) return min === 1 ? "1 minute ago" : `${min} minutes ago`;
  const hours = Math.round(min / 60);
  if (hours < 24) return hours === 1 ? "1 hour ago" : `${hours} hours ago`;
  const days = Math.round(hours / 24);
  return days === 1 ? "1 day ago" : `${days} days ago`;
}

export const SLOT_MINUTES = 30;
export const SLOT_COUNT = 48;
