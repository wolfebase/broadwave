/** Letters still in the box. A settings read that returns the previous text does not replace them. */
export function shownSettingText(server: string, typed: string | null): string {
  if (typed == null || typed === server) return server;
  return typed;
}

/**
 * Drop the draft once it matches the server.
 * Until then an echo of the previous text must not replace it.
 * Returns the same `typed` reference while the edit is still different.
 */
export function settingTextDraft(server: string, typed: string | null): string | null {
  if (typed != null && typed === server) return null;
  return typed;
}

/** Saved when the box is left. Null means the text already matches the server. */
export function settingTextCommit(server: string, typed: string): string | null {
  return typed === server ? null : typed;
}
