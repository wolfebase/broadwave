/**
 * The hour prime time starts on the viewer's clock. Networks air prime time
 * from 8 PM in the Eastern and Pacific zones (and Alaska, Hawaii, Atlantic
 * Canada) but from 7 PM in the Central and Mountain zones, so "Tonight"
 * lands where the evening shows begin wherever the viewer is.
 */
export function primeHour(now: number = Date.now()): number {
  const year = new Date(now).getFullYear();
  // The larger offset of January and July is standard time in North America.
  const standard = Math.max(new Date(year, 0, 1).getTimezoneOffset(), new Date(year, 6, 1).getTimezoneOffset());
  return primeHourFor(standard);
}

/** primeHour for a standard-time offset in minutes west of UTC (360 is Central). */
export function primeHourFor(standardOffset: number): number {
  return standardOffset === 360 || standardOffset === 420 ? 19 : 20;
}
