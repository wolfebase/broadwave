/**
 * A page's file could not be fetched (the server went away after the app
 * loaded), as opposed to a bug in the page. Browsers word it differently.
 */
export function isChunkError(error: unknown): boolean {
  const text = error instanceof Error ? `${error.name} ${error.message}` : String(error ?? "");
  return /dynamically imported module|importing a module script failed|unable to preload css|loading (css )?chunk|chunkloaderror/i.test(text);
}
