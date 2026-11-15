import assert from "node:assert/strict";
import { test } from "node:test";
import type { RoomState } from "./src/lib/events.ts";
import { SyncEngine, type SyncEnv, type SyncStatus } from "./src/lib/syncEngine.ts";

// The engine against a fake <video>, a fake hls.js playlist, and fake clocks.
// The playlist is one-second fragments; fragment i starts at media time i and
// is dated P0 + i s. Time moves in 10 ms steps and the video plays at its rate.

const WALL = 1_800_000_000_000;
const P0 = WALL - 40_000;

class Ranges {
  private r: [number, number][];
  constructor(r: [number, number][]) {
    this.r = r;
  }
  get length() {
    return this.r.length;
  }
  start(i: number) {
    return this.r[i][0];
  }
  end(i: number) {
    return this.r[i][1];
  }
}

class FakeVideo {
  t = 10;
  paused = false;
  playbackRate = 1;
  readyState = 4;
  seeking = false;
  ended = false;
  isConnected = true;
  dataset: Record<string, string> = {};
  ranges: [number, number][] = [[0, 30]];
  seeks: { from: number; to: number }[] = [];
  // Safari: the picture stops this long after every rate change.
  rateStallMs = 0;
  /** The picture stops for ms without pausing: a stall at the edge. */
  freeze(ms: number) {
    this.frozenFor = ms;
  }
  private frozenFor = 0;
  private rate = 1;
  private listeners = new Map<string, Set<() => void>>();
  get currentTime() {
    return this.t;
  }
  set currentTime(v: number) {
    this.seeks.push({ from: this.t, to: v });
    this.t = v;
  }
  get buffered() {
    return new Ranges(this.ranges);
  }
  get seekable() {
    return new Ranges(this.ranges);
  }
  pause() {
    if (this.paused) return;
    this.paused = true;
    this.fire("pause");
  }
  play() {
    this.paused = false;
    return Promise.resolve();
  }
  addEventListener(type: string, fn: () => void) {
    let set = this.listeners.get(type);
    if (!set) this.listeners.set(type, (set = new Set()));
    set.add(fn);
  }
  removeEventListener(type: string, fn: () => void) {
    this.listeners.get(type)?.delete(fn);
  }
  fire(type: string) {
    this.listeners.get(type)?.forEach((fn) => fn());
  }
  advance(ms: number) {
    if (this.playbackRate !== this.rate) {
      this.rate = this.playbackRate;
      this.frozenFor = this.rateStallMs;
    }
    if (this.paused) return;
    if (this.frozenFor > 0) {
      this.frozenFor -= ms;
      return;
    }
    this.t += (ms / 1000) * this.playbackRate;
  }
}

type Timer = { id: number; at: number; fn: () => void; every?: number };

function harness(room = "channel:1") {
  // A page open for ten seconds: the engine's waits count from page load.
  let now = 10_000;
  let offset = 0;
  let version = 1;
  let nextId = 1;
  let timers: Timer[] = [];
  const handlers = new Map<string, Set<(d: unknown) => void>>();
  const cached = new Map<string, RoomState>();
  const commands: { room: string; action: string }[] = [];
  const bus = {
    on(type: string, fn: (d: unknown) => void) {
      let set = handlers.get(type);
      if (!set) handlers.set(type, (set = new Set()));
      set.add(fn);
      return () => set.delete(fn);
    },
    join() {},
    leave() {},
    command(r: string, action: string) {
      commands.push({ room: r, action });
    },
    roomState(r: string) {
      return cached.get(r);
    },
    serverNow() {
      return WALL + now + offset;
    },
  };
  const add = (fn: () => void, ms: number, every?: number) => {
    const id = nextId++;
    timers.push({ id, at: now + ms, fn, every });
    return id;
  };
  const remove = (id: number) => {
    timers = timers.filter((t) => t.id !== id);
  };
  const env: SyncEnv = {
    bus: bus as unknown as SyncEnv["bus"],
    now: () => now,
    wall: () => WALL + now,
    every: (fn, ms) => add(fn, ms, ms),
    cancelEvery: remove,
    after: (fn, ms) => add(fn, ms),
    cancelAfter: remove,
  };
  const video = new FakeVideo();
  const frags = Array.from({ length: 60 }, (_, i) => ({ start: i, duration: 1, programDateTime: P0 + i * 1000 }));
  const hls = { levels: [{ details: { fragments: frags } }], currentLevel: 0 };
  const statuses: SyncStatus[] = [];
  let viewerPauses = 0;
  const engine = new SyncEngine(
    video as unknown as HTMLVideoElement,
    hls as never,
    room,
    1,
    (s) => statuses.push(s),
    () => viewerPauses++,
    env,
  );
  const local = () => P0 + video.t * 1000;
  return {
    engine,
    video,
    commands,
    statuses,
    viewerPauses: () => viewerPauses,
    /** A room state that puts this screen drift ms ahead of the room (negative: behind). */
    state(drift: number, extra: Partial<RoomState> = {}): RoomState {
      return {
        room,
        channelId: 1,
        mode: room.startsWith("group:") ? "group" : "follow",
        anchorServer: WALL + now + offset,
        anchorMedia: local() - drift,
        rate: 1,
        latency: "balanced",
        version: version++,
        members: 2,
        ...extra,
      };
    },
    cache(st: RoomState) {
      cached.set(st.room, st);
    },
    emit(st: RoomState) {
      cached.set(st.room, st);
      handlers.get("sync.state")?.forEach((fn) => fn(st));
    },
    /** Local minus the room's target now, in ms. */
    drift() {
      const st = cached.get(room)!;
      const target = st.rate === 0 ? st.anchorMedia : st.anchorMedia + (WALL + now + offset - st.anchorServer) * st.rate;
      return local() - target;
    },
    moveClock(ms: number) {
      offset += ms;
    },
    run(ms: number) {
      const end = now + ms;
      while (now < end) {
        now += 10;
        video.advance(10);
        for (;;) {
          const due = timers.filter((t) => t.at <= now).sort((a, b) => a.at - b.at)[0];
          if (!due) break;
          if (due.every) due.at += due.every;
          else remove(due.id);
          due.fn();
        }
      }
    },
    last() {
      return statuses[statuses.length - 1];
    },
  };
}

const near = (a: number, b: number, eps = 1e-9) => Math.abs(a - b) <= eps;
const backward = (v: FakeVideo) => v.seeks.filter((s) => s.to < s.from - 1e-9);

test("a screen within 25 ms of the room plays at the room's rate", () => {
  const h = harness();
  h.engine.start();
  h.emit(h.state(20));
  assert.equal(h.video.playbackRate, 1);
  assert.equal(h.last().state, "locked");
  h.run(2000);
  assert.equal(h.video.seeks.length, 0);
  assert.equal(h.video.paused, false);
});

test("a small drift trims the rate in half-percent steps, at most 3%, and stops on the target", () => {
  for (const [drift, rate] of [
    [-30, 1.015],
    [-100, 1.03],
    [60, 0.97],
    [-300, 1.03],
  ] as const) {
    const h = harness();
    h.engine.start();
    h.emit(h.state(drift));
    assert.ok(near(h.video.playbackRate, rate), `drift ${drift}: rate ${h.video.playbackRate}, want ${rate}`);
    assert.equal(h.last().state, "syncing");
    h.run(20_000);
    assert.equal(h.video.playbackRate, 1, `drift ${drift}: still trimming`);
    assert.ok(Math.abs(h.drift()) <= 25, `drift ${drift}: ended ${h.drift()} ms off`);
    assert.equal(h.video.seeks.length, 0, `drift ${drift}: a trim seeked`);
  }
});

test("a screen behind with a thin buffer does not speed up past the live edge", () => {
  const h = harness();
  h.video.ranges = [[0, 11]];
  h.engine.start();
  h.emit(h.state(-100));
  assert.equal(h.video.playbackRate, 1);
});

test("far behind seeks forward once, then waits for the landing", () => {
  const h = harness();
  h.engine.start();
  h.emit(h.state(-600));
  assert.equal(h.video.seeks.length, 1);
  const [s] = h.video.seeks;
  assert.ok(near(s.to - s.from, 0.6, 1e-6), `seeked ${s.to - s.from} s`);
  h.run(1900);
  assert.equal(h.video.seeks.length, 1, "seeked again before the landing was measured");
  assert.ok(Math.abs(h.drift()) <= 25);
});

test("far ahead pauses for exactly the drift and never seeks backward", () => {
  for (const drift of [600, 3000]) {
    const h = harness();
    h.engine.start();
    h.emit(h.state(drift));
    assert.equal(h.video.paused, true);
    h.run(drift - 20);
    assert.equal(h.video.paused, true, `${drift}: resumed early`);
    h.run(40);
    assert.equal(h.video.paused, false, `${drift}: did not resume`);
    assert.equal(h.video.seeks.length, 0, `${drift}: seeked`);
    assert.ok(Math.abs(h.drift()) <= 25, `${drift}: ended ${h.drift()} ms off`);
    assert.equal(h.viewerPauses(), 0, "the engine's own pause reached the viewer");
  }
});

test("a follow room never seeks backward in the live buffer, whatever the drift", () => {
  const h = harness();
  h.engine.start();
  let seed = 11;
  const rand = () => (seed = (seed * 16807) % 2147483647) / 2147483647;
  for (let i = 0; i < 40; i++) {
    h.emit(h.state(Math.round((rand() - 0.5) * 6000)));
    h.run(250 + Math.round(rand() * 3000));
  }
  assert.deepEqual(backward(h.video), []);
});

test("a player that stalls on rate changes is corrected by seeking instead", () => {
  const h = harness();
  h.video.rateStallMs = 250;
  h.engine.start();
  h.emit(h.state(-150));
  h.run(15_000);
  // Two rate changes that each cost a quarter second, then a seek from under
  // the 400 ms a trimming player waits for.
  assert.ok(h.video.seeks.length >= 1, "never fell back to seeking");
  assert.deepEqual(backward(h.video), []);
  assert.ok(Math.abs(h.drift()) <= 150, `ended ${h.drift()} ms off`);
});

test("a stall at the live edge tells the room once, and a small hole is stepped over", () => {
  const h = harness();
  h.engine.start();
  h.emit(h.state(0));
  h.video.ranges = [[0, 10.2]];
  h.video.fire("waiting");
  h.video.fire("waiting");
  assert.deepEqual(
    h.commands.map((c) => c.action),
    ["stalled"],
  );
  h.run(3100);
  h.video.t = 10.1;
  h.video.fire("waiting");
  assert.equal(h.commands.length, 2, "a later stall is told again");

  const g = harness();
  g.engine.start();
  g.emit(g.state(0));
  g.video.ranges = [
    [0, 10.1],
    [10.3, 30],
  ];
  g.video.t = 10.05;
  g.video.fire("waiting");
  assert.equal(g.video.t, 10.3);
  assert.equal(g.commands.length, 0);
});

test("after a stall leaves the screen behind on a thin buffer, it holds instead of seeking to the edge", () => {
  const h = harness();
  h.engine.start();
  h.emit(h.state(0));
  // The picture stopped for 800 ms; the buffer holds one second past it.
  h.video.ranges = [[0, h.video.t + 1]];
  h.video.freeze(800);
  h.run(1050);
  assert.equal(h.video.seeks.length, 0);
  assert.equal(h.video.playbackRate, 1);
  assert.equal(h.last().state, "syncing");
  // The buffer catches up, and the engine seeks forward to the room.
  h.video.ranges = [[0, 30]];
  h.run(500);
  assert.equal(h.video.seeks.length, 1);
  assert.deepEqual(backward(h.video), []);
  assert.ok(Math.abs(h.drift()) <= 25);
});

test("a change in the measured server clock moves the target, and the engine follows it forward only", () => {
  const h = harness();
  h.engine.start();
  h.emit(h.state(0));
  h.run(1000);
  // The server's clock reads 800 ms later than measured: this screen is behind.
  h.moveClock(800);
  h.run(500);
  assert.equal(h.video.seeks.length, 1);
  h.run(3000);
  assert.ok(Math.abs(h.drift()) <= 25, `${h.drift()} ms off after the forward move`);
  // And 800 ms earlier: this screen is ahead, and pauses.
  h.moveClock(-800);
  h.run(250);
  assert.equal(h.video.paused, true);
  h.run(3000);
  assert.ok(Math.abs(h.drift()) <= 25, `${h.drift()} ms off after the move back`);
  assert.deepEqual(backward(h.video), []);
});

test("joining mid-stream waits for the room, then lines up with it", () => {
  const h = harness();
  h.engine.start();
  h.run(1000);
  assert.equal(h.last().state, "waiting");
  assert.equal(h.video.seeks.length, 0);
  h.emit(h.state(-2000));
  assert.equal(h.video.seeks.length, 1);
  h.run(3000);
  assert.ok(Math.abs(h.drift()) <= 25);

  // A room this tab already knows is used at once.
  const c = harness();
  c.cache(c.state(-2000));
  c.engine.start();
  c.run(250);
  assert.equal(c.video.seeks.length, 1);

  // A room playing older than this screen's first frame: the pause lines it up.
  const old = harness();
  old.video.ranges = [[9.5, 30]];
  old.engine.start();
  old.emit(old.state(5000));
  assert.equal(old.video.paused, true);
  assert.equal(old.video.seeks.length, 0);
  old.run(5100);
  assert.ok(Math.abs(old.drift()) <= 25);
});

test("a group pause stops the screen on the room's frame, and resume plays on", () => {
  const h = harness("group:den");
  h.engine.start();
  h.emit(h.state(0));
  h.run(500);
  const st = h.state(150, { rate: 0 });
  h.emit(st);
  assert.equal(h.video.paused, true);
  assert.ok(near(P0 + h.video.t * 1000, st.anchorMedia, 1e-3), "not on the room's frame");
  h.run(2000);
  assert.equal(h.video.paused, true);
  h.emit(h.state(0));
  assert.equal(h.video.paused, false);
  h.run(2000);
  assert.ok(Math.abs(h.drift()) <= 25);
});

test("a group pause cancels the resume of a pause that was lining the screen up", () => {
  const h = harness("group:den");
  h.engine.start();
  h.emit(h.state(600));
  assert.equal(h.video.paused, true);
  h.run(200);
  h.emit(h.state(0, { rate: 0 }));
  h.run(1000);
  assert.equal(h.video.paused, true, "the old hold played the picture during a group pause");
});

test("another screen rewinding the group seeks back with it", () => {
  const h = harness("group:den");
  h.engine.start();
  h.emit(h.state(0));
  h.run(2500);
  h.emit(h.state(5000));
  assert.equal(h.video.paused, false);
  assert.equal(backward(h.video).length, 1);
  assert.ok(Math.abs(h.drift()) <= 25);
});

test("a pause the engine did not make is the viewer's", () => {
  const h = harness();
  h.engine.start();
  h.emit(h.state(0));
  h.video.pause();
  assert.equal(h.viewerPauses(), 1);
});
