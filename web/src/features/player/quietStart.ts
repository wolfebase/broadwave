// Quiet tiles wait until the sound tile's watch has answered. A page the
// browser kept for Back starts them with the sound tile only when every tile
// fits: the last diagnostics said so, or every tile already had a picture.
// A smaller budget keeps the sound tile first. The socket dies while the page
// is frozen and that drop would otherwise make the other tiles wait out a
// new sound-tile watch even when they all fit.
export function pageFitsTiles(slots: number | null, tileCount: number, granted: number): boolean {
  if (tileCount <= 1) return true;
  if (slots != null && slots >= tileCount) return true;
  return granted >= tileCount;
}

export function holdQuietStart(after: boolean, kept: boolean, fits: boolean, quiet = false): boolean {
  if (kept) return quiet && !fits;
  return after;
}
