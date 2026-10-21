/** The zone's offset from UTC in minutes at that time, or null for a name Intl does not know. */
export function zoneOffset(zone: string, at: Date): number | null {
  try {
    const part = new Intl.DateTimeFormat("en-US", { timeZone: zone, timeZoneName: "longOffset" }).formatToParts(at).find((p) => p.type === "timeZoneName")?.value ?? "";
    if (part === "GMT") return 0;
    const m = /GMT([+-])(\d{1,2})(?::(\d{2}))?/.exec(part);
    if (!m) return null;
    const minutes = Number(m[2]) * 60 + Number(m[3] ?? 0);
    return m[1] === "-" ? -minutes : minutes;
  } catch {
    return null;
  }
}

/**
 * Whether the server and this browser read clock times the same way. Two
 * named zones agree when their offsets match in winter and in summer. A
 * server without a named zone (only an abbreviation such as CDT) is judged
 * by its offset now.
 */
export function sameZone(serverZone: string, serverOffsetSeconds: number, browserZone: string, now: Date): boolean {
  if (serverZone === browserZone) return true;
  const year = now.getUTCFullYear();
  const dates = [new Date(Date.UTC(year, 0, 15, 12)), new Date(Date.UTC(year, 6, 15, 12))];
  if (zoneOffset(serverZone, now) !== null && serverZone.includes("/")) {
    return dates.every((d) => zoneOffset(serverZone, d) === zoneOffset(browserZone, d));
  }
  return zoneOffset(browserZone, now) === Math.round(serverOffsetSeconds / 60);
}
