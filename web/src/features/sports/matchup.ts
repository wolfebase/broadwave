import { sportsMessages } from "../../i18n/messages/sports.ts";
import { bind } from "../../i18n/runtime.ts";

const t = bind(sportsMessages);

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
  if (sides) return t("sports.matchup", { away: sides[0], home: sides[1] });
  return a.subtitle || a.title;
}

/** Chip and eyebrow text. League acronyms and listing titles stay as stored. */
export function leagueLabel(id: string): string {
  switch (id) {
    case "College Football":
      return t("sports.league.collegeFootball");
    case "College Basketball":
      return t("sports.league.collegeBasketball");
    case "Soccer":
      return t("sports.league.soccer");
    case "Golf":
      return t("sports.league.golf");
    case "Tennis":
      return t("sports.league.tennis");
    case "Olympics":
      return t("sports.league.olympics");
    default:
      return id;
  }
}
