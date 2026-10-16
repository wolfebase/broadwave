// Registrations. D1 in production; tests use the memory store.

export interface Registration {
  id: string;
  publicKey: string;
  serverId: string;
  created: number;
  ip: string;
  // When this name first asked for a certificate, 0 before. Only first
  // certificates count toward the weekly cap; renewals are free at the CA.
  firstAcme: number;
  acmeDay: string;
  acmeCount: number;
  // Address updates in the current hour (addrHour = unix hour).
  addrHour: number;
  addrCount: number;
  seen: number;
}

export interface Store {
  byId(id: string): Promise<Registration | null>;
  byKey(publicKey: string): Promise<Registration | null>;
  insert(r: Registration): Promise<void>;
  registeredSince(ip: string, since: number): Promise<number>;
  allSince(since: number): Promise<number>;
  firstAcmeSince(since: number): Promise<number>;
  noteAcme(id: string, firstAcme: number, day: string, count: number): Promise<void>;
  touch(id: string, at: number): Promise<void>;
  noteAddresses(id: string, hour: number, count: number): Promise<void>;
}

export class MemoryStore implements Store {
  rows = new Map<string, Registration>();
  async byId(id: string) {
    return this.rows.get(id) ?? null;
  }
  async byKey(publicKey: string) {
    for (const r of this.rows.values()) if (r.publicKey === publicKey) return r;
    return null;
  }
  async insert(r: Registration) {
    if (this.rows.has(r.id)) throw new Error("taken");
    this.rows.set(r.id, { ...r });
  }
  async registeredSince(ip: string, since: number) {
    return [...this.rows.values()].filter((r) => r.ip === ip && r.created >= since).length;
  }
  async allSince(since: number) {
    return [...this.rows.values()].filter((r) => r.created >= since).length;
  }
  async firstAcmeSince(since: number) {
    return [...this.rows.values()].filter((r) => r.firstAcme >= since && r.firstAcme > 0).length;
  }
  async noteAcme(id: string, firstAcme: number, day: string, count: number) {
    const r = this.rows.get(id);
    if (r) Object.assign(r, { firstAcme, acmeDay: day, acmeCount: count });
  }
  async touch(id: string, at: number) {
    const r = this.rows.get(id);
    if (r) r.seen = at;
  }
  async noteAddresses(id: string, hour: number, count: number) {
    const r = this.rows.get(id);
    if (r) Object.assign(r, { addrHour: hour, addrCount: count });
  }
}

interface Row {
  id: string;
  public_key: string;
  server_id: string;
  created: number;
  ip: string;
  first_acme: number;
  acme_day: string;
  acme_count: number;
  addr_hour: number | null;
  addr_count: number | null;
  seen: number;
}

function fromRow(r: Row | null): Registration | null {
  if (!r) return null;
  return {
    id: r.id,
    publicKey: r.public_key,
    serverId: r.server_id,
    created: r.created,
    ip: r.ip,
    firstAcme: r.first_acme,
    acmeDay: r.acme_day,
    acmeCount: r.acme_count,
    addrHour: r.addr_hour ?? 0,
    addrCount: r.addr_count ?? 0,
    seen: r.seen,
  };
}

// Minimal D1 surface, so this file needs no Workers types.
export interface D1Like {
  prepare(sql: string): {
    bind(...values: unknown[]): {
      first<T>(): Promise<T | null>;
      run(): Promise<unknown>;
    };
  };
}

export class D1Store implements Store {
  db: D1Like;
  constructor(db: D1Like) {
    this.db = db;
  }
  async byId(id: string) {
    return fromRow(await this.db.prepare("SELECT * FROM servers WHERE id = ?").bind(id).first<Row>());
  }
  async byKey(publicKey: string) {
    return fromRow(await this.db.prepare("SELECT * FROM servers WHERE public_key = ?").bind(publicKey).first<Row>());
  }
  async insert(r: Registration) {
    await this.db
      .prepare(
        "INSERT INTO servers (id, public_key, server_id, created, ip, first_acme, acme_day, acme_count, seen) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
      )
      .bind(r.id, r.publicKey, r.serverId, r.created, r.ip, r.firstAcme, r.acmeDay, r.acmeCount, r.seen)
      .run();
  }
  private async count(sql: string, ...values: unknown[]) {
    const row = await this.db.prepare(sql).bind(...values).first<{ n: number }>();
    return row?.n ?? 0;
  }
  registeredSince(ip: string, since: number) {
    return this.count("SELECT COUNT(*) AS n FROM servers WHERE ip = ? AND created >= ?", ip, since);
  }
  allSince(since: number) {
    return this.count("SELECT COUNT(*) AS n FROM servers WHERE created >= ?", since);
  }
  firstAcmeSince(since: number) {
    return this.count("SELECT COUNT(*) AS n FROM servers WHERE first_acme > 0 AND first_acme >= ?", since);
  }
  async noteAcme(id: string, firstAcme: number, day: string, count: number) {
    await this.db
      .prepare("UPDATE servers SET first_acme = ?, acme_day = ?, acme_count = ? WHERE id = ?")
      .bind(firstAcme, day, count, id)
      .run();
  }
  async touch(id: string, at: number) {
    await this.db.prepare("UPDATE servers SET seen = ? WHERE id = ?").bind(at, id).run();
  }
  async noteAddresses(id: string, hour: number, count: number) {
    await this.db.prepare("UPDATE servers SET addr_hour = ?, addr_count = ? WHERE id = ?").bind(hour, count, id).run();
  }
}
