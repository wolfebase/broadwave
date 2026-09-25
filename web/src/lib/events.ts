// One socket per tab for live updates, clock sync, and Whole-Home Sync rooms.

export type RoomState = {
  room: string;
  channelId: number;
  mode: "follow" | "group";
  anchorServer: number;
  anchorMedia: number;
  rate: number;
  latency: "lowest" | "balanced" | "stable";
  version: number;
  members: number;
};

type Handler = (data: unknown) => void;

class EventSocket {
  private ws: WebSocket | null = null;
  private handlers = new Map<string, Set<Handler>>();
  private queue: string[] = [];
  private retry = 0;
  private rooms = new Map<string, number>();
  private roomRefs = new Map<string, number>();
  private latest = new Map<string, unknown>();
  /** Estimated server clock minus local clock, in ms. */
  offset = 0;
  private bestRtt = Infinity;
  connected = false;

  constructor() {
    this.connect();
    window.setInterval(() => this.sampleClock(), 15_000);
  }

  private connect() {
    const proto = location.protocol === "https:" ? "wss" : "ws";
    const ws = new WebSocket(`${proto}://${location.host}/api/v1/ws`);
    this.ws = ws;
    ws.onopen = () => {
      this.connected = true;
      this.retry = 0;
      this.bestRtt = Infinity;
      this.raw("here", { name: "This browser", kind: "web" });
      for (const [room, channelId] of this.rooms) this.raw("sync.join", { room, channelId });
      for (const msg of this.queue.splice(0)) ws.send(msg);
      for (let i = 0; i < 5; i++) window.setTimeout(() => this.sampleClock(), i * 300);
      this.emit("connection", true);
    };
    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data as string) as { type: string; data?: unknown };
        if (msg.type === "clock") this.onClock(msg.data as { t0: number; t1: number });
        if (msg.type === "sync.state" && msg.data && typeof msg.data === "object" && "room" in msg.data) {
          this.latest.set((msg.data as { room: string }).room, msg.data);
        }
        this.emit(msg.type, msg.data);
      } catch {
        // Ignore malformed frames.
      }
    };
    ws.onclose = () => {
      this.connected = false;
      this.emit("connection", false);
      const wait = Math.min(15_000, 500 * 2 ** this.retry++);
      window.setTimeout(() => this.connect(), wait);
    };
  }

  private raw(type: string, data: unknown) {
    const msg = JSON.stringify({ type, data });
    if (this.ws?.readyState === WebSocket.OPEN) this.ws.send(msg);
    else this.queue.push(msg);
  }

  private sampleClock() {
    if (this.ws?.readyState === WebSocket.OPEN) this.raw("clock", { t0: Date.now() });
  }

  private onClock({ t0, t1 }: { t0: number; t1: number }) {
    const t2 = Date.now();
    const rtt = t2 - t0;
    // Keep the sample with the shortest round trip; allow drift to reset it slowly.
    if (rtt <= this.bestRtt * 1.2) {
      this.bestRtt = Math.min(rtt, this.bestRtt);
      this.offset = t1 - (t0 + t2) / 2;
    }
    this.bestRtt *= 1.01;
  }

  serverNow() {
    return Date.now() + this.offset;
  }

  /** Last room state this tab has seen, so a second tile can catch up without another join. */
  roomState(room: string) {
    return this.latest.get(room);
  }

  private emit(type: string, data: unknown) {
    this.handlers.get(type)?.forEach((fn) => fn(data));
  }

  on(type: string, fn: Handler) {
    let set = this.handlers.get(type);
    if (!set) this.handlers.set(type, (set = new Set()));
    set.add(fn);
    return () => set!.delete(fn);
  }

  join(room: string, channelId: number) {
    const n = (this.roomRefs.get(room) ?? 0) + 1;
    this.roomRefs.set(room, n);
    this.rooms.set(room, channelId);
    if (n === 1) this.raw("sync.join", { room, channelId });
  }

  leave(room: string) {
    const n = (this.roomRefs.get(room) ?? 0) - 1;
    if (n > 0) {
      this.roomRefs.set(room, n);
      return;
    }
    this.roomRefs.delete(room);
    if (!this.rooms.delete(room)) return;
    this.raw("sync.leave", { room });
  }

  command(room: string, action: "play" | "pause" | "seek" | "live" | "latency", extra: { mediaTime?: number; latency?: string } = {}) {
    this.raw("sync.command", { room, action, ...extra });
  }
}

let socket: EventSocket | null = null;

export function events(): EventSocket {
  if (!socket) socket = new EventSocket();
  return socket;
}
