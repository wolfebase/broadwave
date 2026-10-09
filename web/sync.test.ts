import assert from "node:assert/strict";
import { test } from "node:test";
import { holeEnd } from "./src/lib/bufferHole.ts";
import { PauseHold } from "./src/lib/pauseHold.ts";
import { roomStart, roomTarget } from "./src/lib/roomStart.ts";
import { nextSeekLead } from "./src/lib/seekLead.ts";
import { newSettle, settleDue } from "./src/lib/settle.ts";

type Job = { id: number; fn: () => void; ms: number };

function pauses() {
  let n = 1;
  const jobs: Job[] = [];
  const cleared = new Set<number>();
  const hold = new PauseHold(
    (fn, ms) => {
      const id = n++;
      jobs.push({ id, fn, ms });
      return id;
    },
    (id) => {
      if (id) cleared.add(id);
    },
  );
  return { hold, jobs, cleared };
}

test("a seek that lands behind makes the next one lead by the loss", () => {
  // Safari lost 850 ms decoding up to the target; the next seek aims that far ahead.
  const lead = nextSeekLead(0, -850);
  assert.equal(lead, 0.85);
  // Leading by the loss lands on target, so the lead holds.
  assert.equal(nextSeekLead(lead, 0), 0.85);
  assert.ok(Math.abs(nextSeekLead(lead, -40) - 0.89) < 1e-9);
});

test("a seek that lands ahead shortens the lead, never below zero or past two seconds", () => {
  assert.ok(Math.abs(nextSeekLead(0.85, 300) - 0.55) < 1e-9);
  assert.equal(nextSeekLead(0.2, 900), 0);
  assert.equal(nextSeekLead(1.5, -4000), 2);
  // Chrome lands on target; it never gains a lead.
  assert.equal(nextSeekLead(0, 15), 0);
});

test("a playhead at a small hole continues at the next range", () => {
  // The buffer around a timestamp break: old encode to 19.97, new from 20.04.
  assert.equal(holeEnd(19.888, [[0, 19.97], [20.04, 22.02]]), 20.04);
  assert.equal(holeEnd(19.99, [[0, 19.97], [20.04, 22.02]]), 20.04);
  // Well before the hole, playback just continues.
  assert.equal(holeEnd(18, [[0, 19.97], [20.04, 22.02]]), null);
  // A gap wider than a hole is the edge of what has loaded.
  assert.equal(holeEnd(19.9, [[0, 19.97], [21, 22]]), null);
  // One range, no hole.
  assert.equal(holeEnd(19.9, [[0, 19.97]]), null);
  // Already past the hole.
  assert.equal(holeEnd(20.5, [[0, 19.97], [20.04, 22.02]]), null);
});

test("a screen that stopped trimming corrects a steady offset once, then waits longer", () => {
  // WebKit held -117 ms for minutes: under the 120 ms seek threshold, no trims.
  const s = newSettle();
  const at = (sec: number) => sec * 1000;
  assert.equal(settleDue(s, -117, at(0)), false);
  assert.equal(settleDue(s, -117, at(9)), false);
  assert.equal(settleDue(s, -117, at(10)), true);
  // It missed: the next one waits 30 s, then 60 s, then 120 s.
  const fires: number[] = [];
  for (let t = 11; t <= 400; t++) if (settleDue(s, -90, at(t))) fires.push(t);
  assert.deepEqual(fires, [40, 100, 220]);
  // Never more often than the cap, however long it misses.
  let fired = 0;
  for (let t = 101; t < 101 + 3600; t++) if (settleDue(s, -90, at(t))) fired++;
  assert.ok(fired <= 3600 / 300 + 1, `fired ${fired}`);
});

test("a screen inside the band is left alone, and a minute there starts the waits over", () => {
  const s = newSettle();
  const at = (sec: number) => sec * 1000;
  for (let t = 0; t < 120; t++) assert.equal(settleDue(s, 30, at(t)), false);
  assert.equal(settleDue(s, -60, at(120)), false);
  assert.equal(settleDue(s, -60, at(130)), true);
  // Landed inside: after a minute there, the next drift waits only its 10 s.
  for (let t = 131; t < 200; t++) assert.equal(settleDue(s, 5, at(t)), false);
  assert.equal(settleDue(s, 70, at(200)), false);
  assert.equal(settleDue(s, 70, at(210)), true);
  // A drift that comes and goes inside 10 s never corrects.
  const t2 = newSettle();
  for (let t = 0; t < 100; t++) assert.equal(settleDue(t2, t % 8 < 5 ? 80 : 10, at(t)), false);
  assert.equal(settleDue(t2, Number.NaN, at(200)), false);
});

test("a screen joining a room starts on the room's frame, a moment ahead", () => {
  // Six one-second fragments from 12:00:00; the playlist edge is 12:00:06.
  const t0 = Date.UTC(2026, 8, 28, 12, 0, 0);
  const frags = Array.from({ length: 6 }, (_, i) => ({ start: 100 + i, duration: 1, programDateTime: t0 + i * 1000 }));
  const room = { room: "channel:2", channelId: 2, mode: "follow" as const, anchorServer: t0 + 10_000, anchorMedia: t0 + 1_000, rate: 1, latency: "balanced" as const, version: 1, members: 1 };
  // At 12:00:11.5 the room plays 12:00:02.5; the first picture shows a moment later.
  assert.equal(roomTarget(room, t0 + 11_500), t0 + 2_500);
  assert.ok(Math.abs((roomStart(frags, room, t0 + 11_500) ?? 0) - 103) < 1e-9);
  // A room easing back at 0.975x leads by less.
  assert.ok(Math.abs((roomStart(frags, { ...room, rate: 0.975 }, t0 + 10_000) ?? 0) - (101 + 0.4875)) < 1e-9);
  // A paused group room starts on its frame exactly.
  assert.ok(Math.abs((roomStart(frags, { ...room, rate: 0 }, t0 + 60_000) ?? 0) - 101) < 1e-9);
});

test("a room whose frame the playlist does not hold leaves the start alone", () => {
  const t0 = Date.UTC(2026, 8, 28, 12, 0, 0);
  const frags = Array.from({ length: 6 }, (_, i) => ({ start: i, duration: 1, programDateTime: t0 + i * 1000 }));
  // A fresh tune: the room waits on its first frame until 1.5 s from now.
  const fresh = { room: "channel:2", channelId: 2, mode: "follow" as const, anchorServer: t0 + 7_500, anchorMedia: t0, rate: 1, latency: "balanced" as const, version: 1, members: 1 };
  assert.equal(roomStart(frags, fresh, t0 + 6_000), null);
  // Past the edge, and a playlist with no program date-times.
  assert.equal(roomStart(frags, { ...fresh, anchorServer: t0, anchorMedia: t0 + 5_500 }, t0), null);
  assert.equal(roomStart(frags.map((f) => ({ ...f, programDateTime: null })), { ...fresh, anchorServer: t0 }, t0 + 2_000), null);
});

test("stopping a pause for drift never resumes the picture", () => {
  const { hold, jobs, cleared } = pauses();
  let plays = 0;
  hold.arm(800, () => {
    plays++;
  });
  assert.equal(jobs[0]?.ms, 800);
  hold.stop();
  assert.ok(cleared.has(jobs[0].id));
  jobs[0].fn();
  assert.equal(plays, 0);
});

test("a newer pause replaces the one still waiting, and the one that elapses plays once", () => {
  const { hold, jobs, cleared } = pauses();
  let plays = 0;
  hold.arm(800, () => {
    plays++;
  });
  hold.arm(120, () => {
    plays++;
  });
  assert.ok(cleared.has(jobs[0].id));
  jobs[0].fn();
  assert.equal(plays, 0);
  jobs[1].fn();
  jobs[1].fn();
  assert.equal(plays, 1);
  assert.equal(jobs[1].ms, 120);
});

test("leaving the room does not play a picture that paused to catch up", async () => {
  const jobs: Job[] = [];
  const cleared = new Set<number>();
  let n = 1;
  const g = globalThis as typeof globalThis & {
    window: Window & typeof globalThis;
    document: Document;
    location: Location;
    WebSocket: typeof WebSocket;
    localStorage: Storage;
  };
  g.window = {
    setTimeout: ((fn: TimerHandler, ms?: number) => {
      const id = n++;
      if (typeof fn === "function") jobs.push({ id, fn, ms: ms ?? 0 });
      return id;
    }) as typeof setTimeout,
    clearTimeout: ((id?: number) => {
      if (id) cleared.add(id);
    }) as typeof clearTimeout,
    setInterval: (() => n++) as typeof setInterval,
    clearInterval: (() => undefined) as typeof clearInterval,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
  } as unknown as Window & typeof globalThis;
  g.document = { addEventListener: () => undefined } as unknown as Document;
  g.location = { protocol: "http:", host: "localhost" } as Location;
  g.localStorage = { getItem: () => null, setItem: () => undefined, removeItem: () => undefined, clear: () => undefined, length: 0, key: () => null };
  g.WebSocket = class {
    static CONNECTING = 0;
    static OPEN = 1;
    static CLOSING = 2;
    static CLOSED = 3;
    readyState = 0;
    onopen: ((ev: Event) => void) | null = null;
    onmessage: ((ev: MessageEvent) => void) | null = null;
    onclose: ((ev: CloseEvent) => void) | null = null;
    constructor(_url: string) {}
    send() {}
    close() {
      this.readyState = 3;
    }
  } as unknown as typeof WebSocket;

  const { SyncEngine } = await import("./src/lib/sync.ts");
  const { events } = await import("./src/lib/events.ts");
  const server = 1_700_000_000_000;
  const clock = Date.now;
  Date.now = () => server;
  try {
    const calls: string[] = [];
    const video = {
      paused: false,
      seeking: false,
      currentTime: 5,
      readyState: 4,
      playbackRate: 1,
      buffered: { length: 1, start: () => 0, end: () => 40 },
      seekable: { length: 1, start: () => 0, end: () => 40 },
      dataset: {} as DOMStringMap,
      pause() {
        this.paused = true;
        calls.push("pause");
      },
      play() {
        this.paused = false;
        calls.push("play");
        return Promise.resolve();
      },
      addEventListener() {},
      removeEventListener() {},
      getStartDate() {
        return new Date(1_700_000_000_000);
      },
    };
    const engine = new SyncEngine(video as unknown as HTMLVideoElement, null, "channel:2", 2, () => undefined);
    engine.start();
    events().offset = 0;
    const media = server + video.currentTime * 1000;
    const ws = (events() as unknown as { ws: { onmessage: ((ev: { data: string }) => void) | null } }).ws;
    ws.onmessage?.({
      data: JSON.stringify({
        type: "sync.state",
        data: {
          room: "channel:2",
          channelId: 2,
          mode: "follow",
          anchorServer: server,
          anchorMedia: media - 800,
          rate: 1,
          latency: "balanced",
          version: 1,
          members: 2,
        },
      }),
    });
    const pause = jobs.find((job) => job.ms === 800);
    assert.ok(pause, "a picture that is ahead pauses for the drift");
    assert.deepEqual(calls, ["pause"]);
    assert.equal(video.currentTime, 5);
    engine.stop();
    assert.ok(cleared.has(pause.id));
    pause.fn();
    assert.deepEqual(calls, ["pause"]);
  } finally {
    Date.now = clock;
  }
});

const SERVER = 1_700_000_000_000;

function installClock() {
  let nowMs = 20_000;
  let n = 1;
  const jobs: Job[] = [];
  const cleared = new Set<number>();
  const prevDate = Date.now;
  const prevPerf = performance.now;
  Date.now = () => SERVER;
  performance.now = () => nowMs;
  const g = globalThis as typeof globalThis & {
    window: Window & typeof globalThis;
    document: Document;
    location: Location;
    WebSocket: typeof WebSocket;
    localStorage: Storage;
  };
  g.window = {
    setTimeout: ((fn: TimerHandler, ms?: number) => {
      const id = n++;
      if (typeof fn === "function") jobs.push({ id, fn, ms: ms ?? 0 });
      return id;
    }) as typeof setTimeout,
    clearTimeout: ((id?: number) => {
      if (id) cleared.add(id);
    }) as typeof clearTimeout,
    setInterval: (() => n++) as typeof setInterval,
    clearInterval: (() => undefined) as typeof clearInterval,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
  } as unknown as Window & typeof globalThis;
  g.document = { addEventListener: () => undefined } as unknown as Document;
  g.location = { protocol: "http:", host: "localhost" } as Location;
  g.localStorage = { getItem: () => null, setItem: () => undefined, removeItem: () => undefined, clear: () => undefined, length: 0, key: () => null };
  g.WebSocket = class {
    static CONNECTING = 0;
    static OPEN = 1;
    static CLOSING = 2;
    static CLOSED = 3;
    readyState = 0;
    onopen: ((ev: Event) => void) | null = null;
    onmessage: ((ev: MessageEvent) => void) | null = null;
    onclose: ((ev: CloseEvent) => void) | null = null;
    constructor(_url: string) {}
    send() {}
    close() {
      this.readyState = 3;
    }
  } as unknown as typeof WebSocket;
  return {
    jobs,
    cleared,
    setNow: (ms: number) => {
      nowMs = ms;
    },
    restore: () => {
      Date.now = prevDate;
      performance.now = prevPerf;
    },
  };
}

function fakeVideo(currentTime: number, bufferedStart: number, bufferedEnd: number) {
  const calls: string[] = [];
  let end = bufferedEnd;
  const video = {
    paused: false,
    seeking: false,
    currentTime,
    readyState: 4,
    playbackRate: 1,
    buffered: { length: 1, start: () => bufferedStart, end: () => end },
    seekable: { length: 1, start: () => 0, end: () => 40 },
    dataset: {} as DOMStringMap,
    pause() {
      this.paused = true;
      calls.push("pause");
    },
    play() {
      this.paused = false;
      calls.push("play");
      return Promise.resolve();
    },
    addEventListener() {},
    removeEventListener() {},
    getStartDate() {
      return new Date(SERVER);
    },
  };
  return {
    video,
    calls,
    setBufferedEnd: (value: number) => {
      end = value;
    },
  };
}

async function postRoom(room: string, channelId: number, fields: { anchorMedia: number; rate: number }) {
  const { events } = await import("./src/lib/events.ts");
  events().offset = 0;
  const ws = (events() as unknown as { ws: { onmessage: ((ev: { data: string }) => void) | null } }).ws;
  ws.onmessage?.({
    data: JSON.stringify({
      type: "sync.state",
      data: {
        room,
        channelId,
        mode: room.startsWith("group:") ? "group" : "follow",
        anchorServer: SERVER,
        anchorMedia: fields.anchorMedia,
        rate: fields.rate,
        latency: "balanced",
        version: 1,
        members: 2,
      },
    }),
  });
}

test("a catch-up seek stays inside the buffer and does not spend its cooldown", async () => {
  const clock = installClock();
  try {
    const { SyncEngine } = await import("./src/lib/sync.ts");
    // Playhead 5, buffer ends at 8. The room is at 8. Landing needs 8.25.
    const { video, calls, setBufferedEnd } = fakeVideo(5, 0, 8);
    const engine = new SyncEngine(video as unknown as HTMLVideoElement, null, "channel:4", 4, () => undefined);
    engine.start();
    const local = SERVER + 5_000;
    await postRoom("channel:4", 4, { anchorMedia: local + 3_000, rate: 1 });
    assert.equal(video.currentTime, 5);
    assert.deepEqual(calls, []);
    // The refused seek must not start the two-second cooldown.
    setBufferedEnd(9);
    await postRoom("channel:4", 4, { anchorMedia: local + 3_000, rate: 1 });
    assert.equal(video.currentTime, 8);
    engine.stop();
  } finally {
    clock.restore();
  }
});

test("a drift pause does not play through a group pause", async () => {
  const clock = installClock();
  try {
    const { SyncEngine } = await import("./src/lib/sync.ts");
    const { video, calls } = fakeVideo(10, 8, 14);
    const engine = new SyncEngine(video as unknown as HTMLVideoElement, null, "group:ch4", 4, () => undefined);
    engine.start();
    const local = SERVER + 10_000;
    await postRoom("group:ch4", 4, { anchorMedia: local - 800, rate: 1 });
    const pause = clock.jobs.find((job) => job.ms === 800);
    assert.ok(pause, "a picture that is ahead pauses for the drift");
    assert.deepEqual(calls, ["pause"]);
    // The group is paused on the frame already on screen.
    await postRoom("group:ch4", 4, { anchorMedia: local, rate: 0 });
    assert.ok(clock.cleared.has(pause.id));
    pause.fn();
    assert.equal(video.paused, true);
    assert.equal(video.currentTime, 10);
    assert.deepEqual(calls, ["pause"]);
    engine.stop();
  } finally {
    clock.restore();
  }
});

test("a drift pause seeks back when the group rewinds", async () => {
  const clock = installClock();
  try {
    const { SyncEngine } = await import("./src/lib/sync.ts");
    // Buffer starts at 8, so the rewind landing at 4 is seekable and not buffered.
    const { video, calls } = fakeVideo(10, 8, 14);
    const engine = new SyncEngine(video as unknown as HTMLVideoElement, null, "group:ch5", 5, () => undefined);
    engine.start();
    const local = SERVER + 10_000;
    await postRoom("group:ch5", 5, { anchorMedia: local - 800, rate: 1 });
    const pause = clock.jobs.find((job) => job.ms === 800);
    assert.ok(pause);
    await postRoom("group:ch5", 5, { anchorMedia: SERVER + 4_000, rate: 1 });
    assert.equal(video.currentTime, 4);
    assert.ok(clock.cleared.has(pause.id));
    const n = calls.length;
    pause.fn();
    assert.equal(calls.length, n);
    engine.stop();
  } finally {
    clock.restore();
  }
});

test("a group rewind during the resume window seeks back", async () => {
  const clock = installClock();
  try {
    const { SyncEngine } = await import("./src/lib/sync.ts");
    const { video } = fakeVideo(10, 8, 14);
    const engine = new SyncEngine(video as unknown as HTMLVideoElement, null, "group:ch6", 6, () => undefined);
    engine.start();
    const local = SERVER + 10_000;
    await postRoom("group:ch6", 6, { anchorMedia: local - 800, rate: 1 });
    const pause = clock.jobs.find((job) => job.ms === 800);
    assert.ok(pause);
    clock.setNow(20_800);
    pause.fn();
    assert.equal(video.paused, false);
    assert.equal(video.currentTime, 10);
    // 200 ms into the second where a resume is not corrected yet.
    clock.setNow(21_000);
    await postRoom("group:ch6", 6, { anchorMedia: SERVER + 4_000, rate: 1 });
    assert.equal(video.currentTime, 4);
    engine.stop();
  } finally {
    clock.restore();
  }
});
