/** What the server reports about its clock in GET /server. Older servers omit all of it. */
export type ServerZone = { timeZone?: string; timeZoneValid?: boolean; utcOffsetMinutes?: number };

/** Why the server's time zone looks wrong from this browser, or null when it looks right. */
export type ZoneProblem =
  | { kind: "invalid"; zone: string; suggest: string }
  | { kind: "unset"; suggest: string }
  | { kind: "differs"; zone: string; hours: number; suggest: string };

/**
 * Compares the server's zone with this browser's. A mistyped or missing TZ
 * runs the server on UTC, which records at the wrong hour. A different
 * offset is only a hint: the browser may be away from home.
 * browserOffset is minutes east of UTC (-300 for Eastern daylight time).
 */
export function zoneProblem(server: ServerZone | null | undefined, browserZone: string, browserOffset: number): ZoneProblem | null {
  if (!server || server.utcOffsetMinutes === undefined) return null;
  const suggest = browserZone || "America/New_York";
  const zone = server.timeZone ?? "";
  if (server.timeZoneValid === false) return { kind: "invalid", zone, suggest };
  if (zone === "") return browserOffset === 0 ? null : { kind: "unset", suggest };
  const diff = browserOffset - server.utcOffsetMinutes;
  if (diff === 0) return null;
  return { kind: "differs", zone, hours: Math.round((Math.abs(diff) / 60) * 10) / 10, suggest };
}

/** This browser's zone name and its offset now, in minutes east of UTC. */
export function browserZone(now = new Date()): { zone: string; offset: number } {
  let zone = "";
  try {
    zone = Intl.DateTimeFormat().resolvedOptions().timeZone ?? "";
  } catch {
    // Old engines without zone names still give an offset.
  }
  return { zone, offset: -now.getTimezoneOffset() };
}

/** A stable key for "Not now", so the same problem stays dismissed and a new one shows. */
export function zoneProblemKey(problem: ZoneProblem): string {
  return problem.kind === "unset" ? "unset" : `${problem.kind}:${problem.zone}`;
}
