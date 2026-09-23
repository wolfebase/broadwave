import type { Airing } from "../types";

export function formatClockPoint(seconds: number) {
  const total = Math.max(0, Math.floor(seconds));
  const minutes = Math.floor(total / 60);
  const rest = total % 60;
  return `${minutes}:${rest.toString().padStart(2, "0")}`;
}

export function formatBytes(bytes: number) {
  if (bytes >= 1e12) return `${(bytes / 1e12).toFixed(1)} TB`;
  if (bytes >= 1e9) return `${(bytes / 1e9).toFixed(1)} GB`;
  if (bytes >= 1e6) return `${(bytes / 1e6).toFixed(1)} MB`;
  if (bytes >= 1e3) return `${(bytes / 1e3).toFixed(0)} KB`;
  return `${bytes} B`;
}

export function currentTitle(airings: Airing[], channelId: number) {
  const now = Date.now();
  const hit = airings.find((airing) => airing.channelId === channelId && new Date(airing.start).getTime() <= now && new Date(airing.end).getTime() > now);
  return hit?.title ?? "";
}
