import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { roomTarget } from "./src/lib/roomStart.ts";
import {
  castAvailable,
  castControl,
  castLine,
  castLoad,
  castNamespace,
  castOutgoing,
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
import { castSessionLive, openCastSession, type CastPlayer } from "./src/cast/session.ts";
import { ensureSender, requestCast } from "./src/cast/sender.ts";
import { canonicalRendition, createCastHold, renditionOf, stopPayload, watchEndpoint, watchHeaders, watchPayload, type CastHold, type WatchGrant, type WatchPoster } from "./src/cast/watch.ts";

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
    setPaused(next: boolean) {
      isPaused = next;
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
  assert.match(receiver, /castSessionLive/);
  assert.match(receiver, /castLine/);
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
  assert.equal(castControl("", "/media/live/1/index.m3u8", false), "hidden");
  assert.equal(castControl("4F8C2A1B", "/media/live/1/index.m3u8", false), "cast");
  assert.equal(castControl("4F8C2A1B", "", true), "casting");
  assert.equal(castOutgoing("4F8C2A1B", "/media/live/1/index.m3u8", true), true);
  assert.equal(castOutgoing("4F8C2A1B", "https://evil.example/a.m3u8", true), false);
  assert.equal(castOutgoing("4F8C2A1B", "/media/live/1/index.m3u8", false), false);
  assert.equal(castLine("Evening news", "Channel 4"), "Evening news · Channel 4");
  assert.equal(castLine("Channel 4", "Channel 4"), "Channel 4");
  assert.equal(castLine("  ", "Channel 4"), "Channel 4");
  assert.equal(castLine("Evening news", ""), "Evening news");
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
  const dropped = syncStep(held.memory, input({ mediaNow: null, now: 10_100 }));
  assert.equal(dropped.action.kind, "hold");
  assert.equal(dropped.memory.holdUntil, held.memory.holdUntil);
  const roomPaused = syncStep(held.memory, input({ rate: 0, mediaNow: null, now: 10_100 }));
  assert.equal(roomPaused.action.kind, "wait");
  assert.equal(roomPaused.memory.holdUntil, 0);
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
  assert.ok(fake.calls.includes("play"));

  const held = fakePlayer();
  const paused = open(held.player);
  held.setFrags([{ programDateTime: 5_000_000, start: 0, duration: 2 }]);
  paused.onMessage({
    type: "sync.state",
    data: { room: "channel:1", channelId: 1, anchorServer: 10_000, anchorMedia: 5_000_000, rate: 0, mode: "follow", latency: "balanced", version: 1, members: 1 },
  });
  paused.considerStart();
  assert.ok(held.calls.includes("start 0"));
  assert.ok(held.calls.includes("play"));
  assert.equal(held.calls.includes("pause"), false);

  const edge = fakePlayer();
  const waiting = open(edge.player);
  waiting.markWaitOver(waiting.generation);
  assert.ok(edge.calls.includes("start null"));
  assert.ok(edge.calls.includes("play"));
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
  const before = fake.calls.length;
  sent.length = 0;
  session.tick();
  session.sampleClock();
  session.onOpen();
  assert.equal(session.retune(moved), false);
  assert.equal(fake.calls.length, before);
  assert.deepEqual(sent, []);
  assert.equal(castSessionLive(session), false);
  assert.equal(castSessionLive(null), false);
});

test("play is tried again until a frame is ready, and a hold is not played through", () => {
  const fake = fakePlayer();
  const session = open(fake.player);
  session.markWaitOver(session.generation);
  assert.ok(fake.calls.includes("play"));
  fake.calls.length = 0;
  fake.setPaused(true);
  session.tick();
  assert.ok(fake.calls.includes("play"));

  fake.calls.length = 0;
  fake.setMedia(5_000_500);
  fake.setPaused(false);
  session.onMessage({
    type: "sync.state",
    data: { room: "channel:1", channelId: 1, anchorServer: 10_000, anchorMedia: 5_000_000, rate: 1 },
  });
  session.tick();
  assert.ok(fake.calls.includes("pause"));
  fake.calls.length = 0;
  fake.setPaused(true);
  session.tick();
  assert.equal(fake.calls.includes("play"), false);
  fake.setMedia(null);
  session.tick();
  assert.equal(fake.calls.includes("play"), false);
  assert.ok(fake.calls.includes("pause"));
});

test("a paused room plays until it has a frame, then seeks to the anchor", () => {
  const fake = fakePlayer();
  const session = open(fake.player);
  fake.setFrags([{ programDateTime: 5_000_000, start: 0, duration: 2 }]);
  session.onMessage({
    type: "sync.state",
    data: { room: "channel:1", channelId: 1, anchorServer: 10_000, anchorMedia: 5_000_000, rate: 0, mode: "follow", latency: "balanced", version: 1, members: 1 },
  });
  fake.setPaused(true);
  session.tick();
  session.tick();
  assert.ok(fake.calls.includes("play"));
  assert.equal(fake.calls.some((call) => call.startsWith("seek")), false);

  fake.calls.length = 0;
  fake.setMedia(5_000_500);
  fake.setTimeFor(0.2);
  fake.setPaused(false);
  session.tick();
  assert.ok(fake.calls.includes("seek 0.2"));
  assert.ok(fake.calls.includes("pause"));
  assert.equal(fake.calls.includes("play"), false);

  fake.calls.length = 0;
  fake.setMedia(null);
  fake.setPaused(true);
  session.tick();
  assert.equal(fake.calls.includes("play"), false);
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
  let asked = false;
  let options: { receiverApplicationId?: string; autoJoinPolicy?: string } = {};
  const current = {
    sendMessage(namespace: string, data: unknown) {
      sent.push({ namespace, data });
      return Promise.resolve(null);
    },
    endSession(stop: boolean) {
      ended = stop;
    },
  };
  const link = await requestCast("4F8C2A1B", {
    cast: {
      framework: {
        CastContext: {
          getInstance: () => ({
            setOptions(next) {
              options = next;
            },
            requestSession: () => {
              asked = true;
              return Promise.resolve(null);
            },
            getCurrentSession: () => (asked ? current : null),
          }),
        },
      },
    },
    chrome: { cast: { AutoJoinPolicy: { ORIGIN_SCOPED: "origin_scoped" } } },
  });
  assert.ok(link);
  assert.equal(asked, true);
  assert.equal(options.receiverApplicationId, "4F8C2A1B");
  assert.equal(options.autoJoinPolicy, "origin_scoped");
  link.send({ type: "load" });
  assert.equal(sent[0]?.namespace, castNamespace);
  link.end();
  assert.equal(ended, true);
  await assert.rejects(() => requestCast("4F8C2A1B", {}), /Cast didn't load/);
});

test("an open Cast session is reused, and the button clears when the television ends it", async () => {
  let asked = false;
  let fire: ((state: string) => void) | null = null;
  let cleared = false;
  const link = await requestCast(
    "4F8C2A1B",
    {
      cast: {
        framework: {
          CastContextEventType: { SESSION_STATE_CHANGED: "sessionstatechanged" },
          SessionState: { SESSION_ENDED: "sessionended" },
          CastContext: {
            getInstance: () => ({
              setOptions() {},
              requestSession() {
                asked = true;
                return Promise.reject(new Error("picker"));
              },
              getCurrentSession: () => ({
                sendMessage() {
                  return Promise.resolve(null);
                },
                endSession() {},
              }),
              addEventListener(type: string, listener: (event: { sessionState?: string }) => void) {
                assert.equal(type, "sessionstatechanged");
                fire = (state) => listener({ sessionState: state });
              },
            }),
          },
        },
      },
    },
    () => {
      cleared = true;
    },
  );
  assert.equal(asked, false);
  assert.ok(link);
  link.send({ type: "load" });
  fire?.("sessionstarted");
  assert.equal(cleared, false);
  fire?.("sessionended");
  assert.equal(cleared, true);
});

test("closing the picker is not an error, and a device that fails is", async () => {
  const env = (reason: unknown) => ({
    cast: {
      framework: {
        CastContext: {
          getInstance: () => ({
            setOptions() {},
            requestSession: () => Promise.reject(reason),
            getCurrentSession: () => null,
          }),
        },
      },
    },
  });
  assert.equal(await requestCast("4F8C2A1B", env("cancel")), null);
  await assert.rejects(() => requestCast("4F8C2A1B", env("receiver_unavailable")), /No Cast devices found/);
});

test("a live playlist names one rendition, and anything else is not a watch", () => {
  assert.equal(canonicalRendition("1080.aac2.broadcast"), "1080.aac2.broadcast");
  assert.equal(canonicalRendition("copy.copy"), "copy.copy");
  assert.equal(canonicalRendition("720.aac2.broadcast.hevc"), "720.aac2.broadcast.hevc");
  assert.equal(canonicalRendition("540.aac2.smooth.60"), "540.aac2.smooth.60");
  assert.equal(canonicalRendition("360.none.film"), "360.none.film");
  assert.equal(canonicalRendition("1080.aac6.broadcast.lang.even"), "1080.aac6.broadcast.lang.even");
  // These would not round-trip, so the server would pick an encode of its own.
  for (const key of ["1080.aac2", "copy.copy.broadcast", "720.aac2.hevc.broadcast", "1080.aac2.nope", "1080.aac2.broadcast.broadcast"]) {
    assert.equal(canonicalRendition(key), null, key);
  }
  assert.deepEqual(renditionOf("/media/live/4/1080.aac2.broadcast/index.m3u8"), { channelId: 4, rendition: "1080.aac2.broadcast" });
  assert.deepEqual(renditionOf("/media/live/4/1080.aac2.broadcast/master.m3u8"), { channelId: 4, rendition: "1080.aac2.broadcast" });
  assert.deepEqual(renditionOf("/media/live/4/copy.copy/main.m3u8"), { channelId: 4, rendition: "copy.copy" });
  assert.equal(renditionOf("/media/live/1/index.m3u8"), null);
  assert.equal(renditionOf("/media/live/01/1080.aac2.broadcast/index.m3u8"), null);
  assert.equal(renditionOf("/media/mosaic/1-2/index.m3u8"), null);
  assert.equal(renditionOf("https://evil.example/a.m3u8"), null);
  assert.equal(renditionOf("/media/live/4/1080.aac2.broadcast/index.m3u8?access_token=bw_secret"), null);
});

test("the watch request stays on this server and keeps the token out of the URL", () => {
  const pageURL = "http://broadwave.local:8477/cast?access_token=bw_secret";
  assert.equal(watchEndpoint(pageURL), "http://broadwave.local:8477/api/v1/watch");
  assert.equal(watchEndpoint(pageURL, 4), "http://broadwave.local:8477/api/v1/watch/4/stop");
  assert.equal(JSON.stringify(watchEndpoint(pageURL, 4)).includes("bw_secret"), false);
  assert.deepEqual(watchPayload(4, "1080.aac2.broadcast"), { channelId: 4, rendition: "1080.aac2.broadcast", confirmLive: true });
  assert.equal("token" in watchPayload(4, "1080.aac2.broadcast"), false);
  assert.deepEqual(stopPayload({ channelId: 4, rendition: "1080.aac2.broadcast", boot: "boot-1" }), { rendition: "1080.aac2.broadcast", boot: "boot-1" });
  assert.equal(watchHeaders("bw_casttoken").Authorization, "Bearer bw_casttoken");
  assert.deepEqual(watchHeaders(""), { "Content-Type": "application/json" });
});

function gate() {
  let resolve: (value: WatchGrant | null) => void = () => {};
  const promise = new Promise<WatchGrant | null>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function scriptedPoster() {
  const posts: { channelId: number; rendition: string; token: string }[] = [];
  const stops: { channelId: number; rendition: string; boot: string; token: string }[] = [];
  const gates: ReturnType<typeof gate>[] = [];
  const poster: WatchPoster = {
    post(body, token) {
      posts.push({ channelId: body.channelId, rendition: body.rendition, token });
      const next = gate();
      gates.push(next);
      return next.promise;
    },
    stop(held, token) {
      stops.push({ ...held, token });
    },
  };
  return { posts, stops, gates, poster };
}

const castPlaylist = "/media/live/1/1080.aac2.broadcast/index.m3u8";

test("the television holds its rendition once, and releases it on stop", async () => {
  const script = scriptedPoster();
  const hold = createCastHold("http://broadwave.local:8477/cast", script.poster);
  hold.hold({ channelId: 1, playlist: castPlaylist, token: "bw_casttoken" });
  hold.hold({ channelId: 1, playlist: castPlaylist, token: "bw_casttoken" });
  assert.equal(script.posts.length, 1);
  assert.equal(script.posts[0]?.rendition, "1080.aac2.broadcast");
  assert.equal(script.posts[0]?.token, "bw_casttoken");
  assert.equal(JSON.stringify(script.posts[0]).includes("confirmLive"), false);
  script.gates[0]?.resolve({ channelId: 1, rendition: "1080.aac2.broadcast", boot: "boot-1" });
  await script.gates[0]?.promise;
  hold.hold({ channelId: 1, playlist: "/media/live/1/1080.aac2.broadcast/master.m3u8", token: "bw_next" });
  assert.equal(script.posts.length, 1);
  hold.release();
  assert.equal(script.stops.length, 1);
  assert.equal(script.stops[0]?.rendition, "1080.aac2.broadcast");
  assert.equal(script.stops[0]?.boot, "boot-1");
  assert.equal(script.stops[0]?.token, "bw_next");
  hold.release();
  assert.equal(script.stops.length, 1);
});

test("a different rendition releases the old viewer, and a stranger grant is not kept", async () => {
  const script = scriptedPoster();
  const hold = createCastHold("http://broadwave.local:8477/cast", script.poster);
  hold.hold({ channelId: 1, playlist: castPlaylist, token: "" });
  script.gates[0]?.resolve({ channelId: 1, rendition: "720.aac2.broadcast", boot: "boot-1" });
  await script.gates[0]?.promise;
  assert.equal(script.stops.length, 1);
  assert.equal(script.stops[0]?.rendition, "720.aac2.broadcast");

  hold.hold({ channelId: 2, playlist: "/media/live/2/720.aac2.broadcast/index.m3u8", token: "bw_two" });
  assert.equal(script.posts.at(-1)?.channelId, 2);
  script.gates[1]?.resolve({ channelId: 2, rendition: "720.aac2.broadcast", boot: "boot-1" });
  await script.gates[1]?.promise;
  hold.hold({ channelId: 2, playlist: "/media/live/9/720.aac2.broadcast/index.m3u8", token: "bw_two" });
  assert.equal(script.posts.length, 2);
  assert.equal(script.stops.at(-1)?.channelId, 2);
  hold.hold({ channelId: 1, playlist: "/media/live/1/index.m3u8", token: "" });
  assert.equal(script.posts.length, 2);
});

test("a restarted server is watched again, and the old boot is not kept", async () => {
  const script = scriptedPoster();
  const hold = createCastHold("http://broadwave.local:8477/cast", script.poster);
  hold.noteBoot("boot-2");
  hold.hold({ channelId: 1, playlist: castPlaylist, token: "bw_casttoken" });
  script.gates[0]?.resolve({ channelId: 1, rendition: "1080.aac2.broadcast", boot: "boot-1" });
  await script.gates[0]?.promise;
  assert.equal(script.stops[0]?.boot, "boot-1");
  assert.equal(script.posts.length, 2);
  script.gates[1]?.resolve({ channelId: 1, rendition: "1080.aac2.broadcast", boot: "boot-2" });
  await script.gates[1]?.promise;
  assert.equal(script.posts.length, 2);
  hold.noteBoot("boot-2");
  assert.equal(script.posts.length, 2);

  hold.noteBoot("boot-3");
  assert.equal(script.stops.at(-1)?.boot, "boot-2");
  assert.equal(script.posts.length, 3);
  script.gates[2]?.resolve({ channelId: 1, rendition: "1080.aac2.broadcast", boot: "boot-3" });
  await script.gates[2]?.promise;
  assert.equal(script.posts.length, 3);
});

test("a failed watch does not release a viewer, and stop drops one still in flight", async () => {
  const script = scriptedPoster();
  const hold = createCastHold("http://broadwave.local:8477/cast", script.poster);
  hold.hold({ channelId: 1, playlist: castPlaylist, token: "bw_casttoken" });
  script.gates[0]?.resolve(null);
  await script.gates[0]?.promise;
  assert.equal(script.stops.length, 0);

  hold.hold({ channelId: 1, playlist: castPlaylist, token: "bw_casttoken" });
  hold.release();
  script.gates[1]?.resolve({ channelId: 1, rendition: "1080.aac2.broadcast", boot: "boot-1" });
  await script.gates[1]?.promise;
  assert.equal(script.stops.length, 1);
  assert.equal(script.stops[0]?.boot, "boot-1");
});

test("the session asks the television to hold the channel, and stop lets it go", () => {
  const calls: string[] = [];
  const watch: CastHold = {
    hold(input) {
      calls.push(`hold ${input.channelId} ${input.playlist} ${input.token}`);
    },
    release() {
      calls.push("release");
    },
    noteBoot(boot) {
      calls.push(`boot ${boot}`);
    },
  };
  const fake = fakePlayer();
  const session = openCastSession({
    pageURL: page,
    channelId: 1,
    playlist: castPlaylist,
    room: "channel:1",
    latency: "balanced",
    token: "bw_casttoken",
    player: fake.player,
    transport: { send() {}, close() {} },
    watch,
  });
  assert.ok(session);
  assert.deepEqual(calls, [`hold 1 ${castPlaylist} bw_casttoken`]);
  const same = parseLoad({ type: "load", channelId: 1, playlist: castPlaylist, room: "channel:1", latency: "balanced", token: "bw_casttoken" });
  assert.equal(session.retune(same!), false);
  assert.equal(calls.length, 1);
  session.onMessage({ type: "hello", data: { boot: "boot-9" } });
  assert.equal(calls.at(-1), "boot boot-9");
  session.onMessage({ type: "hello", data: {} });
  assert.equal(calls.length, 2);
  const moved = parseLoad({ type: "load", channelId: 2, playlist: "/media/live/2/720.aac2.broadcast/index.m3u8", room: "channel:2", latency: "stable" });
  session.retune(moved!);
  assert.equal(calls.at(-1), "hold 2 /media/live/2/720.aac2.broadcast/index.m3u8 ");
  session.stop();
  assert.equal(calls.at(-1), "release");
  session.onMessage({ type: "hello", data: { boot: "boot-10" } });
  assert.equal(calls.at(-1), "release");

  calls.length = 0;
  assert.equal(
    openCastSession({
      pageURL: page,
      channelId: 1,
      playlist: "https://evil.example/a.m3u8",
      room: "channel:1",
      latency: "balanced",
      player: fakePlayer().player,
      transport: { send() {}, close() {} },
      watch,
    }),
    null,
  );
  assert.deepEqual(calls, []);
});
