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

// Spaces and dashes only group the six digits.
export function normalizePairCode(raw: string): string | null {
  const digits = raw.replace(/[\s-]/g, "");
  if (!/^\d{6}$/.test(digits)) return null;
  return digits;
}

export function formatPairCode(code: string): string {
  if (!/^\d{6}$/.test(code)) return code;
  return `${code.slice(0, 3)} ${code.slice(3)}`;
}

const scopeOrder = ["watch", "record", "admin"] as const;

const scopeNames: Record<(typeof scopeOrder)[number], string> = {
  watch: "Watch",
  record: "Record",
  admin: "Admin",
};

export function scopeLabel(scopes: string[]): string {
  const names = scopeOrder.filter((scope) => scopes.includes(scope)).map((scope) => scopeNames[scope]);
  if (names.length === 0) return "Watch";
  return names.join(", ");
}

export function kindLabel(kind: string): string {
  if (kind === "phone") return "Phone";
  if (kind === "tv") return "TV";
  if (kind === "web") return "Browser";
  return "Device";
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

export type PairPollStep = {
  action: "wait" | "ready" | "miss" | "expired" | "denied";
  /** True after an approved poll that did not include the token. */
  missed: boolean;
};

// The first approved poll with no token can be the one that raced the handoff,
// or a response the browser has not applied yet. The next empty poll is a miss.
export function pairPollStep(state: string, token: string | undefined, missed: boolean): PairPollStep {
  if (state === "approved" && token) return { action: "ready", missed: false };
  if (state === "approved") return missed ? { action: "miss", missed: true } : { action: "wait", missed: true };
  if (state === "expired") return { action: "expired", missed };
  if (state === "denied") return { action: "denied", missed };
  return { action: "wait", missed: false };
}
