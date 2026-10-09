/** A failed fetch leaves its note. A later success clears that note and no other. */
export function noteAfterFetch(note: string, ok: boolean, failure: string): string {
  if (ok) return note === failure ? "" : note;
  return failure;
}
