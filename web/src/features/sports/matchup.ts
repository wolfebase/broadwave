/** Splits "Bears at Bills" or "Lakers vs. Celtics" into a matchup. */
export function matchup(a: { title: string; subtitle?: string }): [string, string] | null {
  const text = a.subtitle && / (at|vs\.?|@) /i.test(a.subtitle) ? a.subtitle : a.title;
  const m = text.match(/^(.*?)\s+(?:at|vs\.?|@)\s+(.*)$/i);
  if (!m) return null;
  const clean = (s: string) => s.replace(/^.*?:\s*/, "").trim();
  return [clean(m[1]), clean(m[2])];
}

/** The name on the card. Two games can share a program title and still be different matchups. */
export function cardLabel(a: { title: string; subtitle?: string }): string {
  const sides = matchup(a);
  if (sides) return `${sides[0]} at ${sides[1]}`;
  return a.subtitle || a.title;
}
