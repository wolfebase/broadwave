/** "4.1 KBWV": the channel a tuner is on, without repeating a name that is the number. */
export function tunedLabel(guide?: string, name?: string): string {
  const g = (guide ?? "").trim();
  const n = (name ?? "").trim();
  return n && n !== g ? `${g} ${n}`.trim() : g;
}

/** Who holds a tuner and what it is on, leaving out a part the tuner didn't report. */
export function tunerHolder(who: string, guide?: string, name?: string): string {
  return [who, tunedLabel(guide, name)].filter(Boolean).join(" · ");
}
