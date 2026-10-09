/** Back and Forward show the query in the address. Letters still being typed stay in the box. */
export function searchFieldValue(urlQuery: string, typed: string, historyMove: boolean): string {
  return historyMove ? urlQuery : typed;
}

/** A record request finishes on the query that started it, or the note stays whatever the new query showed. */
export function noteAfterRecord(asked: string, current: string, title: string, existing: string): string {
  return asked === current ? `Recording every ${title}.` : existing;
}
