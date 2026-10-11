export const TOKEN_KEY = "bw.deviceToken";

type ReadStore = { getItem(key: string): string | null };
type WriteStore = { setItem(key: string, value: string): void; removeItem(key: string): void };

// localStorage throws in private mode and when the browser blocks it.
function browserStorage(): (ReadStore & WriteStore) | null {
  try {
    const store = globalThis.localStorage;
    if (!store) return null;
    return store;
  } catch {
    return null;
  }
}

export function readToken(storage?: ReadStore | null): string {
  const store = storage === undefined ? browserStorage() : storage;
  if (!store) return "";
  try {
    return store.getItem(TOKEN_KEY) ?? "";
  } catch {
    return "";
  }
}

export function writeToken(token: string, storage?: WriteStore | null): void {
  const store = storage === undefined ? browserStorage() : storage;
  if (!store) return;
  try {
    if (token === "") store.removeItem(TOKEN_KEY);
    else store.setItem(TOKEN_KEY, token);
  } catch {
    // A blocked store should not stop the rest of pairing.
  }
}

export function authHeaders(): Record<string, string> {
  const token = readToken();
  if (!token) return {};
  return { Authorization: "Bearer " + token };
}

// A media element cannot set Authorization. The query is the same one the server accepts.
export function mediaURL(url: string, token = readToken()): string {
  if (!token || /[?&]access_token=/.test(url)) return url;
  return `${url}${url.includes("?") ? "&" : "?"}access_token=${encodeURIComponent(token)}`;
}

function statusOf(err: unknown): number | null {
  if (typeof err !== "object" || err === null || !("status" in err)) return null;
  const status = (err as { status: unknown }).status;
  return typeof status === "number" ? status : null;
}

export function isUnauthorized(err: unknown): boolean {
  return statusOf(err) === 401;
}

export function isForbidden(err: unknown): boolean {
  return statusOf(err) === 403;
}

type Denied = () => void;
let onDenied: Denied | null = null;

/** The app registers one listener. A 401 from any request opens pairing. */
export function onUnauthorized(fn: Denied): () => void {
  onDenied = fn;
  return () => {
    if (onDenied === fn) onDenied = null;
  };
}

export function noteUnauthorized() {
  onDenied?.();
}

export function noteIfUnauthorized(status: number) {
  if (status === 401) noteUnauthorized();
}

export function settingsForWatcher<T extends { pictureMode?: string; autoplay?: string; layout?: string; deviceAuth?: string }>(
  base: T,
  playback: { pictureMode?: string; autoplay?: string; layout?: string } | null,
  auth: string,
): T {
  return {
    ...base,
    pictureMode: playback?.pictureMode || base.pictureMode,
    autoplay: playback?.autoplay || base.autoplay,
    layout: playback?.layout || base.layout,
    deviceAuth: auth === "device" ? "1" : base.deviceAuth,
  };
}
