/** The newest follow or team list wins. A failed read leaves the list on screen. */
export function takeTeams<T>(mine: number, latest: number, teams: T[] | null): T[] | null {
  if (mine !== latest || teams == null) return null;
  return teams;
}
