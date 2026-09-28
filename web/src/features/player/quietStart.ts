// Quiet tiles wait until the sound tile's watch has answered. A page the
// browser kept for Back already won those pictures. The socket dies while
// that page is frozen, and the drop would make the other tiles wait out a
// new sound-tile watch. They start with it instead.
export function holdQuietStart(after: boolean, kept: boolean): boolean {
  return after && !kept;
}
