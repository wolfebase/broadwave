export type PictureChoice = "broadcast" | "smooth" | "film";

/** Saved checkbox, unless this visit has paused or seeked away. */
export function visitSync(saved: boolean, detached: boolean): boolean {
  return saved && !detached;
}

/** The checkbox is what gets stored. A detach is not written. */
export function storedSync(saved: boolean, detached: boolean): boolean {
  return detached ? saved : saved;
}

export function shownPicture(pick: PictureChoice | null, saved: string | undefined): PictureChoice {
  if (pick) return pick;
  return saved === "smooth" || saved === "film" || saved === "broadcast" ? saved : "broadcast";
}

/** Drop the local pick once the saved setting matches it, so a later change applies. */
export function clearPicturePick(pick: PictureChoice | null, saved: string | undefined): PictureChoice | null {
  if (pick && saved === pick) return null;
  return pick;
}

/** Keep a pick until a newer settings object echoes it. The boot default is not an echo. */
export function nextPicturePick(pick: PictureChoice | null, saved: string | undefined, echo: boolean): PictureChoice | null {
  if (!echo) return pick;
  return clearPicturePick(pick, saved);
}
