// Broadwave direct: trusted HTTPS names for servers on home networks.
//
// A server registers an Ed25519 key and gets a random label. With signed
// calls it points lan.<label>.<base> at its home network addresses (private
// ranges only) and sets the ACME DNS-01 record, so it can get a Let's Encrypt
// certificate for that name without anyone opening a port.

import { checkAddresses } from "./ip.ts";
import { freshTime, signedMessage, verify, decodeBase64 } from "./sign.ts";
import type { Registration, Store } from "./store.ts";
import { D1Store } from "./store.ts";
import type { DNS } from "./dns.ts";
import { CloudflareDNS, sync } from "./dns.ts";

export interface Env {
  DB: unknown;
  CF_API_TOKEN: string;
  ZONE_ID: string;
  BASE_DOMAIN: string;
  // Servers that may register from one address in an hour.
  REGISTER_PER_HOUR?: string;
  // Registrations a day from everyone.
  REGISTER_PER_DAY?: string;
  // New names a week that may ask for a first certificate. Let's Encrypt
  // allows 50 new certificates a week per registered domain.
  FIRST_CERTS_PER_WEEK?: string;
}

export interface Deps {
  store: Store;
  dns: DNS;
  base: string;
  now: () => number;
  random: (n: number) => Uint8Array;
  registerPerHour: number;
  registerPerDay: number;
  firstCertsPerWeek: number;
}

const ttl = 300;
const maxBody = 4096;
// ACME record sets a day for one name: an issue and a few retries.
const acmePerDay = 10;

const alphabet = "abcdefghijklmnopqrstuvwxyz234567";

export function newLabel(random: (n: number) => Uint8Array): string {
  const bytes = random(10);
  let out = "";
  for (const b of bytes) out += alphabet[b & 31];
  return out;
}

function json(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json", "Cache-Control": "no-store" },
  });
}

function fail(status: number, error: string): Response {
  return json(status, { error });
}

export function lanName(id: string, base: string): string {
  return `lan.${id}.${base}`;
}

export function acmeName(id: string, base: string): string {
  return `_acme-challenge.lan.${id}.${base}`;
}

async function readBody(req: Request): Promise<string | null> {
  const text = await req.text();
  return text.length > maxBody ? null : text;
}

async function signedBy(req: Request, path: string, body: string, publicKey: string, now: number): Promise<boolean> {
  const time = req.headers.get("X-Broadwave-Time");
  const sig = req.headers.get("X-Broadwave-Signature") ?? "";
  if (!freshTime(time, now)) return false;
  return verify(publicKey, sig, signedMessage(req.method, path, time as string, body));
}

async function register(req: Request, deps: Deps, path: string): Promise<Response> {
  const body = await readBody(req);
  if (body === null) return fail(413, "too large");
  let parsed: { publicKey?: unknown; serverId?: unknown };
  try {
    parsed = JSON.parse(body);
  } catch {
    return fail(400, "invalid json");
  }
  const publicKey = typeof parsed.publicKey === "string" ? parsed.publicKey : "";
  const serverId = typeof parsed.serverId === "string" ? parsed.serverId.slice(0, 64) : "";
  const raw = decodeBase64(publicKey);
  if (!raw || raw.length !== 32) return fail(400, "publicKey must be a base64 Ed25519 key");
  const now = deps.now();
  // The call is signed with the key it registers, so nobody registers a key they do not hold.
  if (!(await signedBy(req, path, body, publicKey, now))) return fail(401, "bad signature");
  const known = await deps.store.byKey(publicKey);
  if (known) return json(200, { id: known.id, name: lanName(known.id, deps.base) });
  const ip = req.headers.get("CF-Connecting-IP") ?? "";
  if ((await deps.store.registeredSince(ip, now - 3600)) >= deps.registerPerHour) return fail(429, "too many registrations from this address");
  if ((await deps.store.allSince(now - 86400)) >= deps.registerPerDay) return fail(429, "too many registrations today");
  for (let tries = 0; tries < 3; tries++) {
    const id = newLabel(deps.random);
    if (await deps.store.byId(id)) continue;
    const reg: Registration = {
      id,
      publicKey,
      serverId,
      created: now,
      ip,
      firstAcme: 0,
      acmeDay: "",
      acmeCount: 0,
      seen: now,
    };
    await deps.store.insert(reg);
    return json(201, { id, name: lanName(id, deps.base) });
  }
  return fail(503, "try again");
}

async function addresses(req: Request, deps: Deps, reg: Registration, body: string): Promise<Response> {
  let parsed: unknown;
  try {
    parsed = JSON.parse(body);
  } catch {
    return fail(400, "invalid json");
  }
  const list = checkAddresses(parsed);
  if (typeof list === "string") return fail(400, list);
  const name = lanName(reg.id, deps.base);
  await sync(deps.dns, name, "A", list.ipv4, ttl);
  await sync(deps.dns, name, "AAAA", list.ipv6, ttl);
  return json(200, { name, ipv4: list.ipv4, ipv6: list.ipv6 });
}

async function acme(req: Request, deps: Deps, reg: Registration, body: string): Promise<Response> {
  const name = acmeName(reg.id, deps.base);
  if (req.method === "DELETE") {
    await sync(deps.dns, name, "TXT", [], ttl);
    return json(200, { name });
  }
  let parsed: { value?: unknown };
  try {
    parsed = JSON.parse(body);
  } catch {
    return fail(400, "invalid json");
  }
  const value = typeof parsed.value === "string" ? parsed.value : "";
  // An ACME DNS-01 value is a base64url SHA-256: 43 characters.
  if (!/^[A-Za-z0-9_-]{43}$/.test(value)) return fail(400, "value must be an ACME DNS-01 value");
  const now = deps.now();
  const day = new Date(now * 1000).toISOString().slice(0, 10);
  const count = reg.acmeDay === day ? reg.acmeCount : 0;
  if (count >= acmePerDay) return fail(429, "too many certificate requests today");
  let first = reg.firstAcme;
  if (first === 0) {
    if ((await deps.store.firstAcmeSince(now - 7 * 86400)) >= deps.firstCertsPerWeek) {
      return fail(429, "too many new certificates this week; try again in a day");
    }
    first = now;
  }
  await deps.store.noteAcme(reg.id, first, day, count + 1);
  await sync(deps.dns, name, "TXT", [`"${value}"`], 60);
  return json(200, { name });
}

export async function handle(req: Request, deps: Deps): Promise<Response> {
  const url = new URL(req.url);
  const path = url.pathname;
  if (path === "/" || path === "/v1/health") {
    return json(200, { ok: true, service: "broadwave-direct", base: deps.base });
  }
  if (path === "/v1/register") {
    if (req.method !== "POST") return fail(405, "method not allowed");
    return register(req, deps, path);
  }
  const m = /^\/v1\/([a-z2-7]{10})\/(addresses|acme)$/.exec(path);
  if (!m) return fail(404, "not found");
  const [, id, what] = m;
  const allowed = what === "addresses" ? ["PUT"] : ["PUT", "DELETE"];
  if (!allowed.includes(req.method)) return fail(405, "method not allowed");
  const body = await readBody(req);
  if (body === null) return fail(413, "too large");
  const reg = await deps.store.byId(id);
  if (!reg) return fail(404, "unknown server");
  const now = deps.now();
  if (!(await signedBy(req, path, body, reg.publicKey, now))) return fail(401, "bad signature");
  await deps.store.touch(id, now);
  return what === "addresses" ? addresses(req, deps, reg, body) : acme(req, deps, reg, body);
}

function num(raw: string | undefined, fallback: number): number {
  const n = Number(raw);
  return Number.isFinite(n) && n > 0 ? n : fallback;
}

export default {
  async fetch(req: Request, env: Env): Promise<Response> {
    const deps: Deps = {
      store: new D1Store(env.DB as ConstructorParameters<typeof D1Store>[0]),
      dns: new CloudflareDNS(env.CF_API_TOKEN, env.ZONE_ID),
      base: env.BASE_DOMAIN,
      now: () => Math.floor(Date.now() / 1000),
      random: (n) => crypto.getRandomValues(new Uint8Array(n)),
      registerPerHour: num(env.REGISTER_PER_HOUR, 5),
      registerPerDay: num(env.REGISTER_PER_DAY, 500),
      firstCertsPerWeek: num(env.FIRST_CERTS_PER_WEEK, 40),
    };
    try {
      return await handle(req, deps);
    } catch (err) {
      console.error(String(err));
      return fail(502, "the DNS service did not answer; try again");
    }
  },
};
