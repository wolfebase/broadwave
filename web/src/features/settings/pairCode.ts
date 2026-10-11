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

export function scopesForAccess(access: string): string[] | null {
  if (access === "watch") return ["watch"];
  if (access === "record") return ["watch", "record"];
  if (access === "admin") return ["watch", "record", "admin"];
  return null;
}

export function canEditSignIn(seat: { auth?: string; device?: { scopes?: string[] } | null } | null): boolean {
  if (!seat || seat.auth !== "device") return true;
  return (seat.device?.scopes ?? []).includes("admin");
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

/** A missed, expired, or denied code must leave the screen. The digits are already dead. */
export function codeAfterPoll(action: PairPollStep["action"], issued: string): string {
  if (action === "miss" || action === "expired" || action === "denied") return "";
  return issued;
}
