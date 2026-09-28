import assert from "node:assert/strict";
import { mock, test } from "node:test";

type Sent = { type: string; data?: Record<string, unknown> };

class FakeSocket {
  static OPEN = 1;
  static CLOSING = 2;
  static CLOSED = 3;
  static all: FakeSocket[] = [];
  readyState = 0;
  sent: Sent[] = [];
  onopen: (() => void) | null = null;
  onmessage: ((e: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  url: string;
  constructor(url: string) {
    this.url = url;
    FakeSocket.all.push(this);
  }
  send(msg: string) {
    this.sent.push(JSON.parse(msg) as Sent);
  }
  close() {
    this.readyState = FakeSocket.CLOSED;
  }
  open() {
    this.readyState = FakeSocket.OPEN;
    this.onopen?.();
  }
  hear(type: string, data?: unknown) {
    this.onmessage?.({ data: JSON.stringify({ type, data }) });
  }
  lose() {
    this.readyState = FakeSocket.CLOSED;
    this.onclose?.();
  }
}

const listeners = new Map<string, () => void>();
mock.timers.enable({ apis: ["setTimeout", "setInterval", "Date"] });
Object.assign(globalThis, {
  WebSocket: FakeSocket,
  location: { protocol: "http:", host: "tv.local:8477" },
  document: { visibilityState: "visible", addEventListener: (t: string, fn: () => void) => listeners.set(t, fn) },
  window: Object.assign(globalThis, { addEventListener: (t: string, fn: () => void) => listeners.set(t, fn) }),
});
const { EventSocket, reconnectWait } = await import("./src/lib/events.ts");

function latest() {
  return FakeSocket.all[FakeSocket.all.length - 1];
}

test("reconnect waits back off with jitter and never past 5 s", () => {
  assert.equal(reconnectWait(0, () => 0), 250);
  assert.equal(reconnectWait(0, () => 1), 500);
  assert.equal(reconnectWait(3, () => 0.5), 3000);
  for (let retry = 0; retry < 30; retry++) {
    assert.ok(reconnectWait(retry, () => 0.999) <= 5000);
    assert.ok(reconnectWait(retry, () => 0) >= 250);
  }
});

test("a restarted server is named once, and the room is joined again", () => {
  FakeSocket.all = [];
  const bus = new EventSocket();
  const seen: string[] = [];
  bus.on("restarted", () => seen.push("restarted"));
  bus.on("reconnected", () => seen.push("reconnected"));
  latest().open();
  latest().hear("hello", { boot: "a", serverTime: 1 });
  bus.join("channel:4", 4);
  assert.deepEqual(seen, []);

  // The network drops and comes back to the same process.
  latest().lose();
  mock.timers.tick(5000);
  latest().open();
  latest().hear("hello", { boot: "a", serverTime: 2 });
  assert.deepEqual(seen, ["reconnected"]);
  assert.ok(latest().sent.some((m) => m.type === "sync.join" && m.data?.room === "channel:4"));

  // The server restarts.
  latest().lose();
  mock.timers.tick(5000);
  latest().open();
  latest().hear("hello", { boot: "b", serverTime: 3 });
  assert.deepEqual(seen, ["reconnected", "reconnected", "restarted"]);
});

test("a command from before the drop is not replayed later", () => {
  FakeSocket.all = [];
  const bus = new EventSocket();
  latest().open();
  bus.join("group:ch4", 4);
  latest().lose();
  bus.command("group:ch4", "pause");
  mock.timers.tick(5000);
  latest().open();
  const sent = latest().sent.map((m) => m.type);
  assert.ok(sent.includes("sync.join"));
  assert.ok(!sent.includes("sync.command"));
  // Joins are not queued twice: the room list sends them on open.
  assert.equal(sent.filter((t) => t === "sync.join").length, 1);
});

test("a command from a moment ago is sent once the socket opens", () => {
  FakeSocket.all = [];
  const bus = new EventSocket();
  latest().open();
  bus.join("group:ch4", 4);
  latest().lose();
  mock.timers.tick(400);
  bus.command("group:ch4", "pause");
  mock.timers.tick(600);
  latest().open();
  assert.ok(latest().sent.some((m) => m.type === "sync.command" && m.data?.action === "pause"));
});

test("waking with a socket that went quiet opens a new one", () => {
  FakeSocket.all = [];
  const bus = new EventSocket();
  const quiet = latest();
  quiet.open();
  bus.wake();
  mock.timers.tick(1);
  assert.ok(quiet.sent.filter((m) => m.type === "clock").length >= 1);
  // Nothing answers the clock burst.
  mock.timers.tick(4100);
  assert.notEqual(latest(), quiet);
  latest().open();
  assert.equal(bus.connected, true);
});

test("waking with a live socket keeps it and measures the clock again", () => {
  FakeSocket.all = [];
  const bus = new EventSocket();
  const live = latest();
  live.open();
  bus.wake();
  mock.timers.tick(1500);
  live.hear("clock", { t0: Date.now() - 10, t1: Date.now() + 500 });
  mock.timers.tick(3000);
  assert.equal(latest(), live);
  assert.ok(Math.abs(bus.offset - 505) < 1);
});

test("a clock reply whose round trip is negative is ignored", () => {
  FakeSocket.all = [];
  const bus = new EventSocket();
  latest().open();
  latest().hear("clock", { t0: Date.now() + 60_000, t1: Date.now() });
  assert.equal(bus.offset, 0);
});

test("a room left and joined again forgets its old state", () => {
  FakeSocket.all = [];
  const bus = new EventSocket();
  latest().open();
  bus.join("channel:4", 4);
  latest().hear("sync.state", { room: "channel:4", anchorMedia: 1 });
  assert.deepEqual(bus.roomState("channel:4"), { room: "channel:4", anchorMedia: 1 });
  // A second player in the tab keeps it.
  bus.join("channel:4", 4);
  bus.leave("channel:4");
  assert.ok(bus.roomState("channel:4"));
  bus.leave("channel:4");
  assert.equal(bus.roomState("channel:4"), undefined);

  // A restarted server's rooms start over too.
  latest().hear("hello", { boot: "a" });
  bus.join("channel:5", 5);
  latest().hear("sync.state", { room: "channel:5", anchorMedia: 2 });
  latest().hear("hello", { boot: "b" });
  assert.equal(bus.roomState("channel:5"), undefined);
});
