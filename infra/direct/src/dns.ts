import { canonicalIPv6 } from "./ip.ts";

// The few Cloudflare DNS calls the service makes, behind an interface so tests
// can watch them.

export interface DNSRecord {
  id: string;
  type: string;
  name: string;
  content: string;
}

export interface DNS {
  list(name: string, type: string): Promise<DNSRecord[]>;
  create(name: string, type: string, content: string, ttl: number): Promise<void>;
  remove(id: string): Promise<void>;
}

export class CloudflareDNS implements DNS {
  token: string;
  zone: string;
  fetcher: typeof fetch;
  // A bare `fetch` kept in a field loses its binding in Workers ("Illegal invocation").
  constructor(token: string, zone: string, fetcher: typeof fetch = (input, init) => fetch(input, init)) {
    this.token = token;
    this.zone = zone;
    this.fetcher = fetcher;
  }

  private async call(method: string, path: string, body?: unknown): Promise<unknown> {
    const res = await this.fetcher(`https://api.cloudflare.com/client/v4/zones/${this.zone}${path}`, {
      method,
      headers: { Authorization: `Bearer ${this.token}`, "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    const out = (await res.json().catch(() => null)) as { success?: boolean; result?: unknown; errors?: unknown } | null;
    if (!res.ok || !out?.success) {
      throw new Error(`dns ${method} ${res.status} ${JSON.stringify(out?.errors ?? null).slice(0, 200)}`);
    }
    return out.result;
  }

  async list(name: string, type: string): Promise<DNSRecord[]> {
    const q = new URLSearchParams({ name, type, per_page: "50" });
    const rows = (await this.call("GET", `/dns_records?${q}`)) as DNSRecord[];
    return rows.map((r) => ({ id: r.id, type: r.type, name: r.name, content: r.content }));
  }

  async create(name: string, type: string, content: string, ttl: number): Promise<void> {
    await this.call("POST", "/dns_records", { type, name, content, ttl, proxied: false, comment: "broadwave direct" });
  }

  async remove(id: string): Promise<void> {
    await this.call("DELETE", `/dns_records/${encodeURIComponent(id)}`);
  }
}

export class MemoryDNS implements DNS {
  records: DNSRecord[] = [];
  calls: string[] = [];
  next = 1;
  async list(name: string, type: string) {
    return this.records.filter((r) => r.name === name && r.type === type);
  }
  async create(name: string, type: string, content: string) {
    this.calls.push(`create ${type} ${name} ${content}`);
    this.records.push({ id: String(this.next++), type, name, content });
  }
  async remove(id: string) {
    const r = this.records.find((x) => x.id === id);
    this.calls.push(`remove ${r?.type} ${r?.name} ${r?.content}`);
    this.records = this.records.filter((x) => x.id !== id);
  }
}

// sync makes the records of one type for a name exactly the wanted contents.
export async function sync(dns: DNS, name: string, type: string, wanted: string[], ttl: number): Promise<void> {
  const have = await dns.list(name, type);
  const norm = (s: string) => {
    const t = s.replace(/^"|"$/g, "");
    return type === "AAAA" ? canonicalIPv6(t) : t.toLowerCase();
  };
  for (const r of have) {
    if (!wanted.some((w) => norm(w) === norm(r.content))) await dns.remove(r.id);
  }
  for (const w of wanted) {
    if (!have.some((r) => norm(r.content) === norm(w))) await dns.create(name, type, w, ttl);
  }
}
