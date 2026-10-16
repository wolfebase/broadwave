// Which addresses a name under the direct domain may point at. Only home
// network ranges: a name that could point at a public address would be a free,
// trusted name for any site, phishing included.

export function privateIPv4(raw: string): boolean {
  const parts = raw.split(".");
  if (parts.length !== 4) return false;
  const n: number[] = [];
  for (const p of parts) {
    if (!/^(0|[1-9][0-9]{0,2})$/.test(p)) return false;
    const v = Number(p);
    if (v > 255) return false;
    n.push(v);
  }
  const [a, b] = n;
  if (a === 10) return true;
  if (a === 172 && b >= 16 && b <= 31) return true;
  if (a === 192 && b === 168) return true;
  // Carrier-grade NAT space, which Tailscale uses.
  if (a === 100 && b >= 64 && b <= 127) return true;
  return false;
}

// expandIPv6 returns the eight groups of an address, or null.
export function expandIPv6(raw: string): number[] | null {
  if (!/^[0-9a-fA-F:]+$/.test(raw) || raw.length > 39) return null;
  const halves = raw.split("::");
  if (halves.length > 2) return null;
  const parse = (s: string): number[] | null => {
    if (s === "") return [];
    const out: number[] = [];
    for (const g of s.split(":")) {
      if (!/^[0-9a-fA-F]{1,4}$/.test(g)) return null;
      out.push(parseInt(g, 16));
    }
    return out;
  };
  const head = parse(halves[0]);
  const tail = halves.length === 2 ? parse(halves[1]) : [];
  if (!head || !tail) return null;
  if (halves.length === 1) return head.length === 8 ? head : null;
  const fill = 8 - head.length - tail.length;
  if (fill < 1) return null;
  return [...head, ...new Array(fill).fill(0), ...tail];
}

// Unique local addresses (fd00::/8). fc00::/8 is reserved and unassigned.
export function privateIPv6(raw: string): boolean {
  const g = expandIPv6(raw);
  return g !== null && g[0] >> 8 === 0xfd;
}

export function canonicalIPv6(raw: string): string {
  const g = expandIPv6(raw);
  if (!g) return raw;
  return g.map((x) => x.toString(16)).join(":");
}

export interface AddressList {
  ipv4: string[];
  ipv6: string[];
}

export const maxPerFamily = 4;

// checkAddresses returns the cleaned lists, or a reason the request is refused.
export function checkAddresses(body: unknown): AddressList | string {
  if (typeof body !== "object" || body === null) return "body must be an object";
  const b = body as Record<string, unknown>;
  const v4 = b.ipv4 ?? [];
  const v6 = b.ipv6 ?? [];
  if (!Array.isArray(v4) || !Array.isArray(v6)) return "ipv4 and ipv6 must be lists";
  if (v4.length > maxPerFamily || v6.length > maxPerFamily) return `at most ${maxPerFamily} addresses of each kind`;
  const ipv4: string[] = [];
  for (const a of v4) {
    if (typeof a !== "string" || !privateIPv4(a)) return `not a home network IPv4 address: ${String(a).slice(0, 45)}`;
    if (!ipv4.includes(a)) ipv4.push(a);
  }
  const ipv6: string[] = [];
  for (const a of v6) {
    if (typeof a !== "string" || !privateIPv6(a)) return `not a unique local IPv6 address: ${String(a).slice(0, 45)}`;
    const c = canonicalIPv6(a);
    if (!ipv6.includes(c)) ipv6.push(c);
  }
  if (ipv4.length + ipv6.length === 0) return "no addresses";
  return { ipv4, ipv6 };
}
