// One socket per tab for live updates, clock sync, and Whole-Home Sync rooms.

export type RoomState = {
  room: string;
  channelId: number;
  mode: "follow" | "group";
  anchorServer: number;
  anchorMedia: number;
  rate: number;
  latency: "lowest" | "balanced" | "stable";
  latencyMs?: number;
  version: number;
  members: number;
  /** Group rooms only: who is in the room. */
  people?: { name: string; kind: string }[];
};

/** What other screens call this one, such as "Chrome on Mac". */
export function browserName(ua = navigator.userAgent, touch = navigator.maxTouchPoints ?? 0): string {
  const browser = /Edg/.test(ua) ? "Edge" : /Firefox\/|FxiOS/.test(ua) ? "Firefox" : /Chrome\/|CriOS/.test(ua) ? "Chrome" : /Safari\//.test(ua) ? "Safari" : "Browser";
  // iPadOS asks for desktop pages and names itself a Mac, one with a touch screen.
  const ipad = /iPad/.test(ua) || (/Macintosh/.test(ua) && touch > 1);
  const os = /iPhone/.test(ua) ? "iPhone" : ipad ? "iPad" : /Android/.test(ua) ? "Android" : /CrOS/.test(ua) ? "Chromebook" : /Mac OS X/.test(ua) ? "Mac" : /Windows/.test(ua) ? "Windows" : /Linux/.test(ua) ? "Linux" : "";
  return os ? `${browser} on ${os}` : browser;
}

type Handler = (data: unknown) => void;

export class EventSocket {
  private ws: WebSocket | null = null;
  private handlers = new Map<string, Set<Handler>>();
  private queue: { msg: string; at: number }[] = [];
  private retry = 0;
  private retryTimer = 0;
  private boot = "";
  private opened = false;
  private heard = 0;
  private tickAt = Date.now();
  private rooms = new Map<string, number>();
  private roomRefs = new Map<string, number>();
  private latest = new Map<string, unknown>();
  /** Estimated server clock minus local clock, in ms. */
  offset = 0;
  private bestRtt = Infinity;
  connected = false;

  constructor() {
    this.connect();
    window.setInterval(() => {
      // A tick that comes late means the tab or the computer slept.
      const gap = Date.now() - this.tickAt;
      this.tickAt = Date.now();
      if (gap > 30_000) this.wake();
      else this.sampleClock();
    }, 15_000);
    window.addEventListener("online", () => this.wake());
    window.addEventListener("pageshow", () => this.wake());
    document.addEventListener("visibilitychange", () => {
      if (document.visibilityState === "visible") this.wake();
    });
  }

  private connect() {
    window.clearTimeout(this.retryTimer);
    const proto = location.protocol === "https:" ? "wss" : "ws";
    const ws = new WebSocket(`${proto}://${location.host}/api/v1/ws`);
    this.ws = ws;
    ws.onopen = () => {
      if (this.ws !== ws) return;
      this.connected = true;
      this.retry = 0;
      this.raw("here", { name: browserName(), kind: "web" });
      for (const [room, channelId] of this.rooms) this.raw("sync.join", { room, channelId, latency: readLiveDelay() });
      // A command older than a few seconds would move the room somewhere nobody asked for now.
      for (const { msg, at } of this.queue.splice(0)) if (Date.now() - at < 3_000) ws.send(msg);
      this.burst();
      this.emit("connection", true);
      if (this.opened) this.emit("reconnected", true);
      this.opened = true;
    };
    ws.onmessage = (event) => {
      this.heard++;
      try {
        const msg = JSON.parse(event.data as string) as { type: string; data?: unknown };
        if (msg.type === "hello") this.onHello(msg.data as { boot?: string });
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
      if (this.ws !== ws) return;
      this.connected = false;
      this.emit("connection", false);
      this.retryTimer = window.setTimeout(() => this.connect(), reconnectWait(this.retry++));
    };
  }

  /** After a sleep, going online, or coming back to the tab: reconnect now and measure the clock again. */
  wake() {
    const ws = this.ws;
    if (!ws || ws.readyState === WebSocket.CLOSED || ws.readyState === WebSocket.CLOSING) {
      this.retry = 0;
      this.connect();
    } else if (ws.readyState === WebSocket.OPEN) this.burst();
    // A socket that went quiet across a sleep can look open and be dead.
    // One we just opened can stay connecting until the browser gives up.
    const current = this.ws;
    if (!current) return;
    const heard = this.heard;
    window.setTimeout(() => {
      if (this.ws !== current) return;
      if (current.readyState !== WebSocket.OPEN || this.heard === heard) this.drop(current);
    }, 4_000);
  }

  private drop(ws: WebSocket) {
    ws.onopen = null;
    ws.onclose = null;
    ws.onmessage = null;
    ws.close();
    this.connected = false;
    this.emit("connection", false);
    this.retry = 0;
    this.connect();
  }

  private burst() {
    this.bestRtt = Infinity;
    for (let i = 0; i < 5; i++) window.setTimeout(() => this.sampleClock(), i * 300);
  }

  private onHello({ boot }: { boot?: string }) {
    if (!boot) return;
    // Every watch and room the old process had is gone.
    if (this.boot && boot !== this.boot) {
      this.latest.clear();
      this.emit("restarted", boot);
    }
    this.boot = boot;
  }

  private raw(type: string, data: unknown) {
    const msg = JSON.stringify({ type, data });
    if (this.ws?.readyState === WebSocket.OPEN) this.ws.send(msg);
    // Joins and the announcement are sent again on open, so only commands wait.
    else if (type === "sync.command") this.queue.push({ msg, at: Date.now() });
  }

  private sampleClock() {
    if (this.ws?.readyState === WebSocket.OPEN) this.raw("clock", { t0: Date.now() });
  }

  private onClock({ t0, t1 }: { t0: number; t1: number }) {
    const t2 = Date.now();
    const rtt = t2 - t0;
    // The local clock moved while the sample was out.
    if (rtt < 0 || rtt > 10_000) return;
    // Keep the sample with the shortest round trip; allow drift to reset it slowly.
    // A round trip of 0 would stick: multiplying it never lets a later sample in.
    if (rtt <= this.bestRtt * 1.2) {
      this.bestRtt = Math.min(Math.max(rtt, 1), this.bestRtt);
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
    if (n === 1) this.raw("sync.join", { room, channelId, latency: readLiveDelay() });
  }

  leave(room: string) {
    const n = (this.roomRefs.get(room) ?? 0) - 1;
    if (n > 0) {
      this.roomRefs.set(room, n);
      return;
    }
    this.roomRefs.delete(room);
    // A room joined again gets its state again; the old one may be long out of date.
    this.latest.delete(room);
    if (!this.rooms.delete(room)) return;
    this.raw("sync.leave", { room });
  }

  command(room: string, action: "play" | "pause" | "seek" | "live" | "latency" | "stalled", extra: { mediaTime?: number; latency?: string } = {}) {
    this.raw("sync.command", { room, action, ...extra });
  }
}

/** Backoff with jitter, so a restarted server is not hit by every screen in the same instant. Capped at 5 s so a screen is back within 5 s of the server. */
export function reconnectWait(retry: number, random = Math.random) {
  const ceiling = Math.min(5_000, 500 * 2 ** retry);
  return Math.round(ceiling / 2 + (random() * ceiling) / 2);
}

export type LiveDelay = "lowest" | "balanced" | "stable";

const delayKey = "broadwave-live-delay";

/** This device's live delay. It sets a channel's delay when this device is the first screen on it. */
export function readLiveDelay(): LiveDelay {
  try {
    const value = localStorage.getItem(delayKey);
    return value === "lowest" || value === "stable" ? value : "balanced";
  } catch {
    return "balanced";
  }
}

export function saveLiveDelay(value: LiveDelay) {
  localStorage.setItem(delayKey, value);
}

let socket: EventSocket | null = null;

export function events(): EventSocket {
  if (!socket) socket = new EventSocket();
  return socket;
}
