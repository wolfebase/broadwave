// Sign in with Cloudflare: what the card shows for the server's state. No React,
// so the node tests read it directly.

export type CfProblem = { code: string; message: string };

export type CfStatus = {
  state: string;
  purpose?: string;
  loginUrl?: string;
  expiresAt?: string;
  zone?: { id: string; name?: string };
  hostname?: string;
  step?: string;
  error?: CfProblem;
  dnsLeft?: string;
  managed: boolean;
  tunnel: { state: string; message?: string };
};

/** What the card shows. */
export type CfStage = "loading" | "notHome" | "idle" | "waiting" | "signedIn" | "working" | "connected" | "removing";

export function stageOf(status: CfStatus | null, notHome: boolean): CfStage {
  if (notHome) return "notHome";
  if (!status) return "loading";
  switch (status.state) {
    case "waiting":
      return status.purpose === "remove" ? "removing" : "waiting";
    case "authorized":
      // A remove signs in and deletes at once; until then it waits like one.
      return status.purpose === "remove" ? "removing" : "signedIn";
    case "provisioning":
      return "working";
    case "connected":
      return "connected";
    default:
      return "idle";
  }
}

export function secondsLeft(expiresAt: string | undefined, now: number): number {
  if (!expiresAt) return 0;
  const end = Date.parse(expiresAt);
  if (Number.isNaN(end)) return 0;
  return Math.max(0, Math.ceil((end - now) / 1000));
}

/** 8:05, for a countdown. */
export function countdown(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds));
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
}

function goodLabel(part: string): boolean {
  return /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/.test(part);
}

/** The name the owner typed, cleaned the way the server cleans it. */
export function cleanLabel(raw: string, zone: string): string {
  let label = raw.trim().toLowerCase().replace(/\.$/, "");
  if (zone && label.endsWith(`.${zone}`)) label = label.slice(0, -(zone.length + 1));
  return label;
}

/**
 * The full name setup would make, or "" when the label is not usable. With
 * the domain known, one label goes in front of it. Without, the whole name.
 */
export function hostFor(raw: string, zone: string): string {
  const label = cleanLabel(raw, zone);
  if (!label) return "";
  if (zone) return goodLabel(label) ? `${label}.${zone}` : "";
  const parts = label.split(".");
  if (parts.length < 3 || label.length > 253 || !parts.every(goodLabel)) return "";
  return label;
}

export type StepMark = "done" | "now" | "next";

export const setupSteps = ["tunnel", "dns", "connect"] as const;

export function stepMarks(step: string | undefined): Record<(typeof setupSteps)[number], StepMark> {
  const at = Math.max(0, setupSteps.indexOf((step ?? "tunnel") as (typeof setupSteps)[number]));
  const mark = (i: number): StepMark => (i < at ? "done" : i === at ? "now" : "next");
  return { tunnel: mark(0), dns: mark(1), connect: mark(2) };
}

/** The tunnel's dot: connected, still connecting, or down. */
export function linkState(tunnel: { state: string } | undefined): "connected" | "connecting" | "down" {
  if (tunnel?.state === "connected") return "connected";
  if (tunnel?.state === "starting") return "connecting";
  return "down";
}

/** Problems the app words for itself. Anything else shows the server's message. */
export const knownProblems = ["timeout", "no_zone", "login_failed", "not_permitted"] as const;

export type KnownProblem = (typeof knownProblems)[number];

export function knownProblem(code: string | undefined): KnownProblem | null {
  return (knownProblems as readonly string[]).includes(code ?? "") ? (code as KnownProblem) : null;
}

/** A finished sign-in that still shows on the card: an error, or a name left in DNS. */
export function noticeOf(status: CfStatus | null): { kind: "problem"; problem: CfProblem } | { kind: "dnsLeft"; host: string } | null {
  if (!status) return null;
  if (status.error && status.state !== "authorized") return { kind: "problem", problem: status.error };
  if (status.dnsLeft && status.state === "idle") return { kind: "dnsLeft", host: status.dnsLeft };
  return null;
}

/** https://tv.example.com for a hostname. */
export function addressOf(host: string): string {
  return host ? `https://${host}` : "";
}
