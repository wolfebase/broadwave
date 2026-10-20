// Reading a listing's words, with no copy attached, so Home and the guide can use it
// without loading the sports page's strings.

import { isFiction } from "../../lib/guide.ts";

export type Listing = { title: string; subtitle?: string; category?: string; gameId?: string };

const event = /\bsports? event\b/i;
// Scripted bouts ("Star vs. Star") and studio shows ("Postgame Live at the Garden")
// can read like a matchup. Neither has a result.
const staged = /\b(pro wrestling|entertainment)\b/i;
const studio = /\b(sports talk|talk|interview|magazine|sports non-event|documentary|reality|news)\b/i;

/** A game, a race, or a match: something with a result. Talk, magazine, and pro wrestling shows are not. */
export function isGame(a: Listing): boolean {
  if (a.gameId) return true;
  const category = a.category ?? "";
  if (isFiction(a) || staged.test(category) || studio.test(category)) return false;
  if (event.test(category)) return true;
  return matchup(a) != null;
}


/** Splits "Bears at Bills" or "Lakers vs. Celtics" into a matchup. */
export function matchup(a: { title: string; subtitle?: string }): [string, string] | null {
  const text = a.subtitle && / (at|vs\.?|@) /i.test(a.subtitle) ? a.subtitle : a.title;
  const m = text.match(/^(.*?)\s+(?:at|vs\.?|@)\s+(.*)$/i);
  if (!m) return null;
  const clean = (s: string) => s.replace(/^.*?:\s*/, "").trim();
  return [clean(m[1]), clean(m[2])];
}

const months = "(jan(uary)?|feb(ruary)?|mar(ch)?|apr(il)?|may|june?|july?|aug(ust)?|sep(t(ember)?)?|oct(ober)?|nov(ember)?|dec(ember)?)";
const bare = new RegExp(
  `^((mon|tue|wed|thu|fri|sat|sun)[a-z]*,?\\s+)?(${months}\\.?\\s+\\d{1,2}(st|nd|rd|th)?,?\\s+\\d{4}|\\d{1,2}/\\d{1,2}/\\d{2,4}|\\d{4}-\\d{2}-\\d{2})$`,
  "i",
);

/** A subtitle that is only the air date ("October 20th 2026") says nothing a card needs. */
export function bareDate(text: string | undefined): boolean {
  return bare.test((text ?? "").trim());
}
