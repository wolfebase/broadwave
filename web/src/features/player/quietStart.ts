// A page the browser kept for Back starts every tile at once only when every
// tile fits: the last diagnostics said so, or every tile already had a
// picture. A smaller budget keeps the start order below. The socket dies
// while the page is frozen, and that drop would otherwise make the other
// tiles wait out a new sound-tile watch even when they all fit.
export function pageFitsTiles(slots: number | null, tileCount: number, granted: number): boolean {
  if (tileCount <= 1) return true;
  if (slots != null && slots >= tileCount) return true;
  return granted >= tileCount;
}

// Multiview tiles ask for their pictures in rank order: the sound tile (0),
// then tiles that already showed a picture on their channel (1), then the
// rest (2). With fewer pictures than tiles, the ones asked for first keep
// theirs. A tile waits while a lower rank wants an answer, at most
// gateHoldMs once the server is reachable. A dropped socket counts every
// tile as wanting until the server says hello, since a tile can reach a
// restarted server before the socket says it restarted.
export const gateHoldMs = 8000;
export const gateDownMs = 30000;

type GateTile = { rank: number; want: boolean; socket: boolean; since: number };

export class StartGate {
  private tiles = new Map<number, GateTile>();
  private waiting = new Map<number, () => void>();
  private down = false;
  private timer: ReturnType<typeof setTimeout> | undefined;
  private now: () => number;

  constructor(now: () => number = () => performance.now()) {
    this.now = now;
  }

  rank(id: number, rank: number) {
    const tile = this.tiles.get(id);
    if (!tile) this.tiles.set(id, { rank, want: true, socket: this.down, since: this.now() });
    else if (tile.rank !== rank) {
      tile.rank = rank;
      this.check();
    }
  }

  // A start for this tile is on its way.
  want(id: number) {
    const tile = this.tiles.get(id);
    if (!tile) return;
    if (!tile.want) tile.since = this.now();
    tile.want = true;
  }

  // The server answered this tile's watch, with a picture or a refusal.
  answered(id: number) {
    const tile = this.tiles.get(id);
    if (!tile || (!tile.want && !tile.socket)) return;
    tile.want = false;
    tile.socket = false;
    this.check();
  }

  leave(id: number) {
    this.tiles.delete(id);
    this.waiting.delete(id);
    this.check();
  }

  dropped() {
    this.down = true;
    const now = this.now();
    for (const tile of this.tiles.values()) {
      if (!tile.socket && !tile.want) tile.since = now;
      tile.socket = true;
    }
  }

  // Every watch the old process had is gone. Called before back().
  restarted() {
    const now = this.now();
    for (const tile of this.tiles.values()) {
      tile.want = true;
      tile.since = now;
    }
  }

  // The server said hello. Each tile still wanting gets a full hold from now.
  back() {
    this.down = false;
    const now = this.now();
    for (const tile of this.tiles.values()) {
      tile.socket = false;
      if (tile.want) tile.since = now;
    }
    this.check();
  }

  blocked(id: number): boolean {
    const tile = this.tiles.get(id);
    if (!tile) return false;
    const now = this.now();
    for (const [other, o] of this.tiles) {
      if (other !== id && o.rank < tile.rank && this.pending(o, now)) return true;
    }
    return false;
  }

  // Runs start once nothing ranked lower is pending. Deferred, so every tile
  // that starts in the same render has said it wants an answer first.
  wait(id: number, start: () => void) {
    this.waiting.set(id, start);
    queueMicrotask(() => this.check());
  }

  cancel(id: number) {
    this.waiting.delete(id);
  }

  stop() {
    clearTimeout(this.timer);
    this.waiting.clear();
  }

  private limit() {
    return this.down ? gateDownMs : gateHoldMs;
  }

  private pending(tile: GateTile, now: number) {
    return (tile.want || tile.socket) && now - tile.since < this.limit();
  }

  private check() {
    clearTimeout(this.timer);
    for (const [id, start] of [...this.waiting]) {
      if (this.blocked(id)) continue;
      this.waiting.delete(id);
      start();
    }
    if (this.waiting.size === 0) return;
    const now = this.now();
    let next = Infinity;
    for (const tile of this.tiles.values()) {
      if (this.pending(tile, now)) next = Math.min(next, tile.since + this.limit() - now);
    }
    if (next < Infinity) this.timer = setTimeout(() => this.check(), next + 1);
  }
}
