import { formatDateTime } from "../../i18n/runtime.ts";
import type { Airing, Device } from "../../types";

/**
 * One entry per rail. A game is the same game at the same minute on any
 * station. Any other show is one entry however many airings it has: a
 * sitcom about football is not four cards.
 */
export function gameKey(airing: Pick<Airing, "title" | "start"> & { category?: string; subtitle?: string }): string {
  const title = airing.title.trim().toLowerCase();
  const game = /\bsports?\b/i.test(airing.category ?? "") || /\s(at|vs\.?|@)\s/i.test(`${airing.title} ${airing.subtitle ?? ""}`);
  return game ? `${title}|${Date.parse(airing.start)}` : title;
}

const day = 24 * 3600_000;

/**
 * Tuners that have not answered for more than a day. A tuner the owner retired
 * on purpose is not missing.
 */
export function missingTuners(devices: Pick<Device, "deviceId" | "friendlyName" | "lastSeen" | "offline" | "retired">[], now: number): { id: string; name: string; since: string }[] {
  const out: { id: string; name: string; since: string }[] = [];
  for (const d of devices) {
    if (d.retired || !d.offline || !d.lastSeen) continue;
    const seen = Date.parse(d.lastSeen);
    if (!Number.isFinite(seen) || now - seen <= day) continue;
    out.push({ id: d.deviceId, name: d.friendlyName || d.deviceId, since: formatDateTime(new Date(seen), { weekday: "short", month: "short", day: "numeric" }) });
  }
  return out;
}
