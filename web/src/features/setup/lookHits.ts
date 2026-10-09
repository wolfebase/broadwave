/** A failed or older look keeps the hits already shown. A look that answers replaces them. */
export function hitsAfterLook<T>(
  mine: number,
  latest: number,
  shown: T[],
  ok: boolean,
  found: T[] | null | undefined,
): T[] | null {
  if (mine !== latest) return null;
  if (!ok) return shown;
  return found ?? [];
}
