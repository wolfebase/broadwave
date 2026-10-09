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

/** A failed free-channel look keeps the guide text. Feeds from an answer clear it. */
export function guideAfterFree(ok: boolean, found: readonly unknown[] | null | undefined, guide: string | null | undefined): string | null {
  if (!ok) return null;
  if (found && found.length > 0) return "";
  return guide ?? "";
}
