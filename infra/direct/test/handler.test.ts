import { test } from "node:test";
import assert from "node:assert/strict";
import { generateKeyPairSync, sign as edSign, type KeyObject } from "node:crypto";
import { handle, type Deps } from "../src/index.ts";
import { MemoryStore } from "../src/store.ts";
import { MemoryDNS } from "../src/dns.ts";
import { signedMessage, verify, encodeBase64 } from "../src/sign.ts";

const base = "direct.example.com";

function keys() {
  const { publicKey, privateKey } = generateKeyPairSync("ed25519");
  const raw = publicKey.export({ format: "der", type: "spki" }).subarray(12);
  return { pub: Buffer.from(raw).toString("base64"), priv: privateKey };
}

function deps(over: Partial<Deps> = {}) {
  let n = 0;
  const d: Deps = {
    store: new MemoryStore(),
    dns: new MemoryDNS(),
    base,
    now: () => 1_800_000_000,
    random: (len) => {
      n++;
      return new Uint8Array(len).fill(n);
    },
    registerPerHour: 5,
    registerPerDay: 100,
    firstCertsPerWeek: 2,
    ...over,
  };
  return d;
}

function req(method: string, path: string, body: string, priv: KeyObject | null, time = 1_800_000_000, ip = "203.0.113.9") {
  const headers: Record<string, string> = { "CF-Connecting-IP": ip, "X-Broadwave-Time": String(time) };
  if (priv) {
    headers["X-Broadwave-Signature"] = edSign(null, signedMessage(method, path, String(time), body), priv).toString("base64");
  }
  return new Request("https://api.direct.example.com" + path, { method, headers, body: method === "GET" ? undefined : body });
}

async function registered(d: Deps) {
  const k = keys();
  const body = JSON.stringify({ publicKey: k.pub, serverId: "srv-1" });
  const res = await handle(req("POST", "/v1/register", body, k.priv), d);
  assert.equal(res.status, 201);
  const out = (await res.json()) as { id: string; name: string };
  return { ...k, ...out };
}

test("Go and the Worker sign the same bytes", async () => {
  // From server/internal/direct/sign_test.go (seed 0x01 x 32).
  const pub = "iojj3XQJ8ZX9UtstPLpdcspnCb8dlBIb83SIAbQPb1w=";
  const sig = "yV6lV+uJjlRL48NsMOC1RR6MkJJbjwsuu5eLt8l5nMvuddpQSj76ljZeRVM1NfTdaMEcAaQxK6BMT3BZCgbyBw==";
  const msg = signedMessage("PUT", "/v1/abcdefghij/addresses", "1800000000", '{"ipv4":["192.168.1.2"]}');
  assert.equal(await verify(pub, sig, msg), true);
  assert.equal(await verify(pub, sig, signedMessage("PUT", "/v1/abcdefghij/addresses", "1800000001", '{"ipv4":["192.168.1.2"]}')), false);
});

test("register gives a stable label for a key", async () => {
  const d = deps();
  const r = await registered(d);
  assert.match(r.id, /^[a-z2-7]{10}$/);
  assert.equal(r.name, `lan.${r.id}.${base}`);
  const again = await handle(req("POST", "/v1/register", JSON.stringify({ publicKey: r.pub, serverId: "srv-1" }), r.priv), d);
  assert.equal(again.status, 200);
  assert.equal(((await again.json()) as { id: string }).id, r.id);
});

test("register needs a signature by the key it registers", async () => {
  const d = deps();
  const a = keys();
  const b = keys();
  const body = JSON.stringify({ publicKey: a.pub, serverId: "x" });
  assert.equal((await handle(req("POST", "/v1/register", body, b.priv), d)).status, 401);
  assert.equal((await handle(req("POST", "/v1/register", body, null), d)).status, 401);
  assert.equal((await handle(req("POST", "/v1/register", body, a.priv, 1_800_000_000 - 400), d)).status, 401);
  assert.equal((await handle(req("POST", "/v1/register", JSON.stringify({ publicKey: "abc" }), a.priv), d)).status, 400);
});

test("register is rate limited per address", async () => {
  const d = deps({ registerPerHour: 2 });
  await registered(d);
  await registered(d);
  const k = keys();
  const res = await handle(req("POST", "/v1/register", JSON.stringify({ publicKey: k.pub, serverId: "x" }), k.priv), d);
  assert.equal(res.status, 429);
  const other = await handle(req("POST", "/v1/register", JSON.stringify({ publicKey: k.pub, serverId: "x" }), k.priv, 1_800_000_000, "198.51.100.1"), d);
  assert.equal(other.status, 201);
});

test("addresses point the name at home addresses only", async () => {
  const dns = new MemoryDNS();
  const d = deps({ dns });
  const r = await registered(d);
  const path = `/v1/${r.id}/addresses`;
  let body = JSON.stringify({ ipv4: ["192.168.1.2"], ipv6: ["fd00::2"] });
  let res = await handle(req("PUT", path, body, r.priv), d);
  assert.equal(res.status, 200);
  assert.deepEqual(
    dns.records.map((x) => `${x.type} ${x.name} ${x.content}`),
    [`A ${r.name} 192.168.1.2`, `AAAA ${r.name} fd00:0:0:0:0:0:0:2`],
  );
  // A change replaces the old address; the same list again changes nothing.
  body = JSON.stringify({ ipv4: ["192.168.1.3"], ipv6: ["fd00::2"] });
  await handle(req("PUT", path, body, r.priv), d);
  dns.calls = [];
  await handle(req("PUT", path, body, r.priv), d);
  assert.deepEqual(dns.calls, []);
  assert.deepEqual(dns.records.filter((x) => x.type === "A").map((x) => x.content), ["192.168.1.3"]);
  // Public addresses are refused and nothing changes.
  res = await handle(req("PUT", path, JSON.stringify({ ipv4: ["93.184.216.34"] }), r.priv), d);
  assert.equal(res.status, 400);
  assert.deepEqual(dns.records.filter((x) => x.type === "A").map((x) => x.content), ["192.168.1.3"]);
});

test("another server's key cannot change a name", async () => {
  const d = deps();
  const r = await registered(d);
  const other = keys();
  const res = await handle(req("PUT", `/v1/${r.id}/addresses`, JSON.stringify({ ipv4: ["10.0.0.9"] }), other.priv), d);
  assert.equal(res.status, 401);
  assert.equal((await handle(req("PUT", `/v1/aaaaaaaaaa/addresses`, "{}", other.priv), d)).status, 404);
});

test("acme sets and clears the challenge record", async () => {
  const dns = new MemoryDNS();
  const d = deps({ dns });
  const r = await registered(d);
  const path = `/v1/${r.id}/acme`;
  const value = "a".repeat(43);
  assert.equal((await handle(req("PUT", path, JSON.stringify({ value }), r.priv), d)).status, 200);
  assert.deepEqual(dns.records.map((x) => `${x.type} ${x.name} ${x.content}`), [`TXT _acme-challenge.${r.name} "${value}"`]);
  assert.equal((await handle(req("DELETE", path, "", r.priv), d)).status, 200);
  assert.deepEqual(dns.records, []);
  assert.equal((await handle(req("PUT", path, JSON.stringify({ value: "short" }), r.priv), d)).status, 400);
});

test("first certificates are capped a week; renewals are not", async () => {
  const d = deps({ firstCertsPerWeek: 1 });
  const a = await registered(d);
  const b = await registered(d);
  const value = "b".repeat(43);
  assert.equal((await handle(req("PUT", `/v1/${a.id}/acme`, JSON.stringify({ value }), a.priv), d)).status, 200);
  assert.equal((await handle(req("PUT", `/v1/${b.id}/acme`, JSON.stringify({ value }), b.priv), d)).status, 429);
  // a already has its first certificate: asking again is allowed.
  assert.equal((await handle(req("PUT", `/v1/${a.id}/acme`, JSON.stringify({ value }), a.priv), d)).status, 200);
});

test("acme is capped a day per name", async () => {
  const d = deps({ firstCertsPerWeek: 10 });
  const a = await registered(d);
  const value = "c".repeat(43);
  for (let i = 0; i < 10; i++) {
    assert.equal((await handle(req("PUT", `/v1/${a.id}/acme`, JSON.stringify({ value }), a.priv), d)).status, 200);
  }
  assert.equal((await handle(req("PUT", `/v1/${a.id}/acme`, JSON.stringify({ value }), a.priv), d)).status, 429);
});

test("address updates are capped an hour per name", async () => {
  const d = deps();
  const r = await registered(d);
  const path = `/v1/${r.id}/addresses`;
  for (let i = 0; i < 20; i++) {
    const body = JSON.stringify({ ipv4: [`192.168.1.${i + 2}`] });
    assert.equal((await handle(req("PUT", path, body, r.priv), d)).status, 200);
  }
  const body = JSON.stringify({ ipv4: ["10.0.0.2"] });
  assert.equal((await handle(req("PUT", path, body, r.priv), d)).status, 429);
  d.now = () => 1_800_000_000 + 3600;
  assert.equal((await handle(req("PUT", path, body, r.priv, 1_800_000_000 + 3600), d)).status, 200);
});

test("wrong methods and paths", async () => {
  const d = deps();
  assert.equal((await handle(new Request("https://x/v1/register"), d)).status, 405);
  assert.equal((await handle(new Request("https://x/v1/health"), d)).status, 200);
  assert.equal((await handle(new Request("https://x/v1/ABCDEFGHIJ/addresses", { method: "PUT", body: "{}" }), d)).status, 404);
  assert.equal((await handle(new Request("https://x/v1/abcdefghij/addresses", { method: "DELETE" }), d)).status, 405);
  assert.equal(encodeBase64(new Uint8Array([1, 2, 3])), "AQID");
});

test("the Cloudflare client calls fetch unbound", async () => {
  const { CloudflareDNS } = await import("../src/dns.ts");
  const seen: string[] = [];
  const original = globalThis.fetch;
  globalThis.fetch = async function (this: unknown, input: RequestInfo | URL, init?: RequestInit) {
    // Workers throw "Illegal invocation" when fetch is called with another `this`.
    if (this !== undefined && this !== globalThis) throw new TypeError("Illegal invocation");
    seen.push(`${init?.method} ${String(input)}`);
    return new Response(JSON.stringify({ success: true, result: [] }));
  } as typeof fetch;
  try {
    const dns = new CloudflareDNS("t", "zone");
    assert.deepEqual(await dns.list("lan.x.example.com", "A"), []);
    assert.equal(seen.length, 1);
    assert.match(seen[0], /^GET https:\/\/api\.cloudflare\.com\/client\/v4\/zones\/zone\/dns_records\?name=lan\.x\.example\.com&type=A/);
  } finally {
    globalThis.fetch = original;
  }
});
