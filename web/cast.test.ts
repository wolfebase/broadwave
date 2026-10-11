import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { roomTarget } from "./src/lib/roomStart.ts";
import {
  castAvailable,
  castLoad,
  castNamespace,
  clockMessage,
  freshClock,
  freshSync,
  hereMessage,
  joinMessage,
  mediaURL,
  noteClock,
  parseLoad,
  receiverSDK,
  senderSDK,
  socketURL,
  syncStep,
  targetMedia,
  type SyncAction,
  type SyncInput,
} from "./src/cast/protocol.ts";
import { openCastSession, type CastPlayer } from "./src/cast/session.ts";
import { ensureSender, requestCast } from "./src/cast/sender.ts";

const page = "http://broadwave.local:8477/cast";

function rateOf(action: SyncAction): number {
  if (action.kind === "rate" || action.kind === "lock") return action.rate;
  throw new Error(action.kind);
}

function input(over: { drift: number; forward?: number; timeFor?: number | null; now?: number; rate?: number; room?: string; mediaNow?: number | null }): SyncInput {
  const now = over.now ?? 10_000;
  const rate = over.rate ?? 1;
  const roomName = over.room ?? "channel:1";
  const anchorMedia = 5_000_000;
  const room = {
    room: roomName,
    channelId: 1,
    mode: roomName.startsWith("group:") ? ("group" as const) : ("follow" as const),
    anchorServer: now,
    anchorMedia,
    rate,
    latency: "balanced" as const,
    version: 1,
    members: 1,
  };
  const target = targetMedia(room, now);
  return {
    now,
    serverNow: now,
    mediaNow: over.mediaNow === undefined ? target + over.drift : over.mediaNow,
    room,
    timeFor: over.timeFor === undefined ? null : over.timeFor,
    forwardSec: over.forward ?? 10,
  };
}

function fakePlayer() {
  const calls: string[] = [];
  let frags: { start: number; duration: number; programDateTime: number | null }[] = [];
  let media: number | null = null;
  let at: number | null = null;
  let ahead = 0;
  let isPaused = false;
  const player: CastPlayer = {
    load(url, token) {
      calls.push(`load ${url}|${token}`);
    },
    startAt(seconds) {
      calls.push(`start ${seconds}`);
    },
    stop() {
      calls.push("stop");
    },
    fragments() {
      return frags;
    },
    mediaNow() {
      return media;
    },
    timeFor() {
      return at;
    },
    forwardSec() {
      return ahead;
    },
    paused() {
      return isPaused;
    },
    pause() {
      isPaused = true;
      calls.push("pause");
    },
    play() {
      isPaused = false;
      calls.push("play");
    },
    seek(seconds) {
      calls.push(`seek ${seconds}`);
    },
    setRate(rate) {
      calls.push(`rate ${rate}`);
    },
  };
  return {
    player,
    calls,
    setFrags(next: typeof frags) {
      frags = next;
    },
    setMedia(next: number | null) {
      media = next;
    },
    setTimeFor(next: number | null) {
      at = next;
    },
    setAhead(next: number) {
      ahead = next;
    },
  };
}

function open(player: CastPlayer, sent: unknown[] = [], token = "") {
  const session = openCastSession({
    pageURL: page,
    channelId: 1,
    playlist: "/media/live/1/index.m3u8",
    room: "channel:1",
    latency: "balanced",
    token: token || undefined,
    title: "Evening news",
    subtitle: "Channel 4",
    player,
    transport: {
      send(message) {
        sent.push(message);
      },
      close() {
        sent.push("close");
      },
    },
    now: () => 10_000,
    screenId: "screen-1",
  });
  assert.ok(session);
  return session;
}

test("receiver page loads the Cast SDK from Google and not from this repo", () => {
  const html = readFileSync(new URL("../server/internal/httpapi/castpage/receiver.html", import.meta.url), "utf8");
  const index = readFileSync(new URL("./index.html", import.meta.url), "utf8");
  const receiver = readFileSync(new URL("./src/cast/receiver.ts", import.meta.url), "utf8");
  assert.equal(html.split(receiverSDK).length - 1, 1);
  assert.match(html, /src="\/cast\/receiver\.js"/);
  assert.equal(html.includes(senderSDK), false);
  assert.ok(html.length < 8000);
  assert.equal(index.includes("gstatic.com/cast"), false);
  assert.match(receiver, /skipPlayersLoad\s*=\s*true/);
  assert.match(receiver, /disableIdleTimeout\s*=\s*true/);
  assert.match(receiver, /maxInactivity\s*=\s*600/);
  assert.doesNotMatch(receiver, /from ["'][^"']*sync/);
  assert.doesNotMatch(receiver, /events\(\)/);
  const player = readFileSync(new URL("./src/cast/hlsPlayer.ts", import.meta.url), "utf8");
  assert.match(player, /liveHlsConfig\("tv"\)/);
  assert.match(player, /autoStartLoad:\s*false/);
  assert.match(player, /Authorization/);
});

test("parseLoad keeps a channel playlist and refuses anything else", () => {
  const good = parseLoad({
    type: "load",
    channelId: 1,
    playlist: "/media/live/1/index.m3u8",
    room: "channel:1",
    title: "  Evening news  ",
    subtitle: "x".repeat(200),
  });
  assert.ok(good);
  assert.equal(good.latency, "balanced");
  assert.equal(good.token, undefined);
  assert.equal(good.title, "Evening news");
  assert.equal(good.subtitle.length, 120);
  assert.equal(parseLoad({ ...good, type: "load", channelId: 1, playlist: good.playlist, room: "group:ch1", latency: "lowest" })?.room, "group:ch1");
  assert.equal(parseLoad({ type: "load", channelId: 1, playlist: good.playlist, room: "channel:1", token: "bw_abc-def" })?.token, "bw_abc-def");

  const rejected = [
    { type: "stop", channelId: 1, playlist: "/media/live/1/index.m3u8", room: "channel:1" },
    { type: "load", channelId: 1.5, playlist: "/media/live/1/index.m3u8", room: "channel:1" },
    { type: "load", channelId: 0, playlist: "/media/live/1/index.m3u8", room: "channel:1" },
    { type: "load", channelId: "1", playlist: "/media/live/1/index.m3u8", room: "channel:1" },
    { type: "load", channelId: 1, playlist: "/media/../secret", room: "channel:1" },
    { type: "load", channelId: 1, playlist: "https://evil.example/a.m3u8", room: "channel:1" },
    { type: "load", channelId: 1, playlist: "/media/live/1/index.m3u8?x=1", room: "channel:1" },
    { type: "load", channelId: 1, playlist: "/media/live/1/index.m3u8#f", room: "channel:1" },
    { type: "load", channelId: 1, playlist: "/media/live/1\\index.m3u8", room: "channel:1" },
    { type: "load", channelId: 1, playlist: "http://other.example/media/live/1/index.m3u8", room: "channel:1" },
    { type: "load", channelId: 1, playlist: "/media/live/1/index.m3u8", room: "channel:2" },
    { type: "load", channelId: 1, playlist: "/media/live/1/index.m3u8", room: "group:1" },
    { type: "load", channelId: 1, playlist: "/media/live/1/index.m3u8", room: "channel:1", latency: "fast" },
    { type: "load", channelId: 1, playlist: "/media/live/1/index.m3u8", room: "channel:1", token: "bad token" },
    { type: "load", channelId: 1, playlist: "/media/live/1/index.m3u8", room: "channel:1", token: "a".repeat(257) },
    { type: "load", channelId: 1, playlist: "/media/live/1/index.m3u8", room: "channel:1", token: "ok.token" },
  ];
  for (const row of rejected) assert.equal(parseLoad(row), null, JSON.stringify(row));
});

test("the token rides the socket, not the playlist", () => {
  const token = "bw_abc-def";
  const media = mediaURL(page, "/media/live/1/index.m3u8");
  assert.equal(media, "http://broadwave.local:8477/media/live/1/index.m3u8");
  assert.equal(media?.includes("access_token"), false);
  assert.equal(socketURL(page, token), "ws://broadwave.local:8477/api/v1/ws?access_token=bw_abc-def");
  assert.equal(socketURL("https://broadwave.local/cast"), "wss://broadwave.local/api/v1/ws");
  const bodies = [hereMessage("screen-1", 1), joinMessage("channel:1", 1, "balanced"), clockMessage(10)];
  assert.equal(JSON.stringify(bodies).includes(token), false);
  const load = castLoad({ channelId: 1, playlist: "/media/live/1/index.m3u8", room: "channel:1", latency: "balanced", token });
  assert.equal(load.token, token);
  assert.equal(String(load.playlist).includes("access_token"), false);
  assert.equal(castAvailable("", "/media/live/1/index.m3u8"), false);
  assert.equal(castAvailable("4F8C2A1B", ""), false);
  assert.equal(castAvailable("4F8C2A1B", "https://evil.example/a.m3u8"), false);
  assert.equal(castAvailable("4F8C2A1B", "/media/live/1/index.m3u8"), true);
});

test("clock offset matches the web player", () => {
  let clock = freshClock();
  clock = noteClock(clock, { t0: 1000, t1: 1100, t2: 1020 });
  assert.equal(clock.offset, 90);
  assert.ok(Math.abs(clock.bestRtt - 20.2) < 1e-9);
  const stuck = noteClock(clock, { t0: 1000, t1: 5000, t2: 20_000 });
  assert.equal(stuck.offset, 90);
  const rejected = noteClock(clock, { t0: 5000, t1: 9000, t2: 1000 });
  assert.equal(rejected.offset, 90);
});

test("target media matches the web player's room target", () => {
  const room = {
    room: "channel:1",
    channelId: 1,
    mode: "follow" as const,
    anchorServer: 1000,
    anchorMedia: 5000,
    rate: 1,
    latency: "balanced" as const,
    version: 1,
    members: 1,
  };
  assert.equal(targetMedia(room, 1000 + 16_000), roomTarget(room, 1000 + 16_000));
  assert.equal(targetMedia({ ...room, rate: 0 }, 50_000), room.anchorMedia);
});

test("sync step pauses when ahead, seeks when behind, and trims the rest", () => {
  const ahead = syncStep(freshSync(), input({ drift: 500 }));
  assert.deepEqual(ahead.action, { kind: "hold", ms: 500 });
  const waiting = syncStep(ahead.memory, input({ drift: 500, now: 10_100 }));
  assert.equal(waiting.action.kind, "wait");

  assert.equal(syncStep(freshSync(), input({ drift: 10 })).action.kind, "lock");
  assert.equal(rateOf(syncStep(freshSync(), input({ drift: 10 })).action), 1);

  const hundred = syncStep(freshSync(), input({ drift: 100 }));
  assert.equal(hundred.action.kind, "rate");
  assert.ok(Math.abs(rateOf(hundred.action) - 0.97) < 1e-9);
  const forty = syncStep(freshSync(), input({ drift: 40 }));
  assert.ok(Math.abs(rateOf(forty.action) - 0.98) < 1e-9);

  const kept = syncStep(hundred.memory, input({ drift: 40, now: 11_000 }));
  assert.ok(Math.abs(rateOf(kept.action) - 0.97) < 1e-9);
  const crossed = syncStep(kept.memory, input({ drift: 4, now: 12_000 }));
  assert.equal(crossed.action.kind, "lock");
  assert.equal(rateOf(crossed.action), 1);

  const seek = syncStep(freshSync(), input({ drift: -500, forward: 2, timeFor: 12 }));
  assert.deepEqual(seek.action, { kind: "seek", to: 12 });
  const thin = syncStep(freshSync(), input({ drift: -500, forward: 1, timeFor: 12 }));
  assert.equal(thin.action.kind, "lock");
  assert.equal(rateOf(thin.action), 1);

  const group = syncStep(freshSync(), input({ room: "group:ch1", drift: 3000, timeFor: 4 }));
  assert.deepEqual(group.action, { kind: "seek", to: 4 });
  const follow = syncStep(freshSync(), input({ room: "channel:1", drift: 3000, timeFor: 4 }));
  assert.equal(follow.action.kind, "hold");
  assert.notEqual(follow.action.kind, "seek");

  const paused = syncStep(freshSync(), input({ rate: 0, drift: 100, timeFor: 3 }));
  assert.equal(paused.action.kind, "seek");
  const near = syncStep(freshSync(), input({ rate: 0, drift: 10, timeFor: 3 }));
  assert.equal(near.action.kind, "pause");

  const again = syncStep(seek.memory, input({ drift: -500, forward: 2, timeFor: 12.4, now: 11_000 }));
  assert.equal(again.action.kind, "lock");
  const pausedAgain = syncStep(paused.memory, input({ rate: 0, drift: 100, timeFor: 3, now: 10_500 }));
  assert.equal(pausedAgain.action.kind, "pause");

  const speeding = syncStep(freshSync(), input({ drift: -40, forward: 10 }));
  assert.ok(Math.abs(rateOf(speeding.action) - 1.02) < 1e-9);
  const cushion = syncStep(speeding.memory, input({ drift: -40, forward: 1, now: 11_000 }));
  assert.equal(cushion.action.kind, "lock");

  const held = syncStep(freshSync(), input({ drift: 500 }));
  const rewound = syncStep(held.memory, input({ room: "group:ch1", drift: 3000, timeFor: 2, now: 10_100 }));
  assert.equal(rewound.action.kind, "seek");
  assert.equal(syncStep(freshSync(), { ...input({ drift: 500 }), mediaNow: null }).action.kind, "wait");
});

test("a session joins on open, starts on the room, and ignores a bad playlist", () => {
  const sent: unknown[] = [];
  const fake = fakePlayer();
  const session = open(fake.player, sent);
  assert.deepEqual(fake.calls, ["load http://broadwave.local:8477/media/live/1/index.m3u8|"]);
  session.onOpen();
  assert.deepEqual(
    sent.map((message) => (message as { type: string }).type),
    ["here", "sync.join", "clock"],
  );
  const here = sent[0] as { data: { name: string; kind: string; channelId: number } };
  assert.equal(here.data.name, "Cast");
  assert.equal(here.data.kind, "web");
  assert.equal(here.data.channelId, 1);
  assert.equal(JSON.stringify(sent).includes("bw_abc-def"), false);

  const refused = fakePlayer();
  assert.equal(
    openCastSession({
      pageURL: page,
      channelId: 1,
      playlist: "https://evil.example/a.m3u8",
      room: "channel:1",
      latency: "balanced",
      player: refused.player,
      transport: { send() {}, close() {} },
    }),
    null,
  );
  assert.deepEqual(refused.calls, []);
});

test("playback starts at the room frame, or at the edge when the wait ends first", () => {
  const fake = fakePlayer();
  const session = open(fake.player);
  fake.setFrags([
    { programDateTime: 5_000_000, start: 0, duration: 2 },
    { programDateTime: 5_002_000, start: 2, duration: 2 },
  ]);
  session.onMessage({
    type: "sync.state",
    data: { room: "channel:1", channelId: 1, anchorServer: 10_000, anchorMedia: 5_000_000, rate: 1, mode: "follow", latency: "balanced", version: 1, members: 1 },
  });
  session.considerStart();
  assert.ok(fake.calls.includes("start 0.5"));

  const edge = fakePlayer();
  const waiting = open(edge.player);
  waiting.markWaitOver(waiting.generation);
  assert.ok(edge.calls.includes("start null"));
  const before = edge.calls.length;
  waiting.retune(parseLoad({ type: "load", channelId: 2, playlist: "/media/live/2/index.m3u8", room: "channel:2", latency: "balanced" })!);
  waiting.markWaitOver(0);
  assert.equal(edge.calls.filter((call) => call.startsWith("start")).length, 1);
  assert.ok(edge.calls.length > before);
});

test("a tick ahead pauses, stop leaves, and a repeat load does not restart", () => {
  const sent: { type?: string; data?: { room?: string } }[] = [];
  const fake = fakePlayer();
  const session = open(fake.player, sent);
  session.onMessage({
    type: "sync.state",
    data: { room: "channel:1", channelId: 1, anchorServer: 10_000, anchorMedia: 5_000_000, rate: 1 },
  });
  fake.setMedia(5_000_500);
  fake.setAhead(10);
  session.tick();
  assert.ok(fake.calls.includes("pause"));

  sent.length = 0;
  const same = parseLoad({ type: "load", channelId: 1, playlist: "/media/live/1/index.m3u8", room: "channel:1", latency: "balanced", title: "Later title" })!;
  assert.equal(session.retune(same), false);
  assert.equal(fake.calls.filter((call) => call.startsWith("load")).length, 1);
  assert.equal(sent.length, 0);
  assert.equal(session.title, "Later title");

  const moved = parseLoad({ type: "load", channelId: 2, playlist: "/media/live/2/index.m3u8", room: "channel:2", latency: "stable" })!;
  assert.equal(session.retune(moved), false);
  assert.ok(sent.some((message) => message.type === "sync.leave" && message.data?.room === "channel:1"));
  assert.ok(sent.some((message) => message.type === "sync.join" && message.data?.room === "channel:2"));
  assert.equal(fake.calls.filter((call) => call.startsWith("load")).length, 2);

  sent.length = 0;
  const authed = parseLoad({ type: "load", channelId: 2, playlist: "/media/live/2/index.m3u8", room: "channel:2", latency: "stable", token: "bw_abc-def" })!;
  assert.equal(session.retune(authed), true);
  assert.ok(sent.some((message) => message.type === "sync.leave"));
  assert.equal(sent.some((message) => message.type === "sync.join"), false);

  session.stop();
  assert.ok(sent.some((message) => message.type === "sync.leave"));
  assert.ok(fake.calls.includes("stop"));
  assert.equal(session.status, "idle");
});

test("the sender script is Google's, and an existing framework is left alone", async () => {
  let created = false;
  const ready = await ensureSender({
    cast: { framework: {} },
    document: {
      createElement() {
        created = true;
        return { src: "", onerror: null };
      },
      head: { appendChild() {} },
    },
  });
  assert.equal(ready, true);
  assert.equal(created, false);

  const scripts: { src: string; onerror: (() => void) | null }[] = [];
  const pending = ensureSender(
    {
      document: {
        createElement() {
          const script = { src: "", onerror: null as (() => void) | null };
          scripts.push(script);
          return script;
        },
        head: { appendChild() {} },
      },
      window: {},
    },
    50,
  );
  assert.equal(scripts[0]?.src, senderSDK);
  scripts[0]?.onerror?.();
  assert.equal(await pending, false);
});

test("requestCast addresses this app and this namespace", async () => {
  const sent: { namespace: string; data: unknown }[] = [];
  let ended = false;
  let options: { receiverApplicationId?: string; autoJoinPolicy?: string } = {};
  const link = await requestCast("4F8C2A1B", {
    cast: {
      framework: {
        CastContext: {
          getInstance: () => ({
            setOptions(next) {
              options = next;
            },
            requestSession: () => Promise.resolve(null),
            getCurrentSession: () => ({
              sendMessage(namespace, data) {
                sent.push({ namespace, data });
                return Promise.resolve(null);
              },
              endSession(stop) {
                ended = stop;
              },
            }),
          }),
        },
      },
    },
    chrome: { cast: { AutoJoinPolicy: { ORIGIN_SCOPED: "origin_scoped" } } },
  });
  assert.equal(options.receiverApplicationId, "4F8C2A1B");
  assert.equal(options.autoJoinPolicy, "origin_scoped");
  link.send({ type: "load" });
  assert.equal(sent[0]?.namespace, castNamespace);
  link.end();
  assert.equal(ended, true);
  await assert.rejects(() => requestCast("4F8C2A1B", {}), /Cast didn't load/);
});
