import assert from "node:assert/strict";
import { test } from "node:test";
import type { MediaLink } from "./src/api/generated.ts";
import { forgetMediaLinks, linkURL, mediaLinkFor, type LinkPoster } from "./src/cast/link.ts";
import { castLoad, mediaKey, mediaURL, parseLoad } from "./src/cast/protocol.ts";
import { openCastSession, type CastPlayer } from "./src/cast/session.ts";

const origin = "https://tv.example.net";
const playlist = "/media/live/28/1080.aac2.broadcast/index.m3u8";
const later = "2026-10-16T20:00:00Z";
const nowMs = Date.parse("2026-10-15T20:00:00Z");

function poster(answer: MediaLink | null | Error) {
  const calls: { path: string; token: string }[] = [];
  const post: LinkPoster = async (path, token) => {
    calls.push({ path, token });
    if (answer instanceof Error) throw answer;
    return answer;
  };
  return { post, calls };
}

test("no token asks for no link", async () => {
  forgetMediaLinks();
  const fake = poster({ url: `${playlist}?mk=abc`, expiresAt: later });
  assert.equal(await mediaLinkFor(origin, playlist, "", fake.post, () => nowMs), undefined);
  assert.deepEqual(fake.calls, []);
});

test("a stored token gets an absolute link on the server's origin", async () => {
  forgetMediaLinks();
  const fake = poster({ url: `${playlist}?mk=abc_123`, expiresAt: later });
  const url = await mediaLinkFor(origin, playlist, "bw_tok", fake.post, () => nowMs);
  assert.equal(url, `${origin}${playlist}?mk=abc_123`);
  assert.deepEqual(fake.calls, [{ path: playlist, token: "bw_tok" }]);
});

test("an older server without /media/link keeps the old load", async () => {
  forgetMediaLinks();
  assert.equal(await mediaLinkFor(origin, playlist, "bw_tok", poster(null).post, () => nowMs), undefined);
  assert.equal(await mediaLinkFor(origin, playlist, "bw_tok", poster(new Error("network")).post, () => nowMs), undefined);
});

test("a link is reused until an hour before it ends", async () => {
  forgetMediaLinks();
  const fake = poster({ url: `${playlist}?mk=one`, expiresAt: later });
  let now = nowMs;
  await mediaLinkFor(origin, playlist, "bw_tok", fake.post, () => now);
  await mediaLinkFor(origin, playlist, "bw_tok", fake.post, () => now);
  assert.equal(fake.calls.length, 1);
  now = Date.parse(later) - 30 * 60 * 1000;
  await mediaLinkFor(origin, playlist, "bw_tok", fake.post, () => now);
  assert.equal(fake.calls.length, 2);
  await mediaLinkFor(origin, playlist, "bw_other", fake.post, () => nowMs);
  assert.equal(fake.calls.length, 3);
});

test("a link that is not this server's /media/ path is not used", () => {
  assert.equal(linkURL(origin, { url: "https://evil.example/media/x.m3u8?mk=a", expiresAt: later }), null);
  assert.equal(linkURL(origin, { url: "//evil.example/media/x.m3u8?mk=a", expiresAt: later }), null);
  assert.equal(linkURL(origin, { url: "/api/v1/settings", expiresAt: later }), null);
  assert.equal(linkURL(origin, null), null);
});

test("the load carries the link, and the receiver keeps only its mk", () => {
  const media = `${origin}${playlist}?mk=abc-123.x`;
  const message = castLoad({ channelId: 28, playlist, room: "channel:28", latency: "balanced", token: "bw_tok", media });
  assert.equal(message.media, media);
  assert.equal(message.playlist, playlist);
  const load = parseLoad(message);
  assert.equal(load?.mediaKey, "abc-123.x");
  assert.equal(load?.token, "bw_tok");
  assert.equal(castLoad({ channelId: 28, playlist, room: "channel:28", latency: "balanced" }).media, undefined);
});

test("a link to another playlist, another query, or a bad mk is dropped, and the load still plays", () => {
  const base = { type: "load", channelId: 28, playlist, room: "channel:28", latency: "balanced" };
  for (const media of [
    `${origin}/media/live/29/1080.aac2.broadcast/index.m3u8?mk=a`,
    `${origin}${playlist}?mk=a&x=1`,
    `${origin}${playlist}?other=a`,
    `${origin}${playlist}?mk=`,
    `${origin}${playlist}?mk=a#frag`,
    `javascript:alert(1)`,
    `${origin}${playlist}?mk=${"a".repeat(1100)}`,
  ]) {
    const load = parseLoad({ ...base, media });
    assert.ok(load, media);
    assert.equal(load.mediaKey, undefined, media);
  }
  assert.equal(mediaKey(`${playlist}?mk=k`, playlist), "k");
  assert.equal(mediaKey(`${playlist}?mk=k`, "https://evil.example/a.m3u8"), null);
});

test("the receiver plays the link on its own server", () => {
  assert.equal(mediaURL("http://192.0.2.10:8477/cast", playlist, "k/+="), `http://192.0.2.10:8477${playlist}?mk=k%2F%2B%3D`);
  assert.equal(mediaURL("http://192.0.2.10:8477/cast", playlist), `http://192.0.2.10:8477${playlist}`);
});

function recorder() {
  const loads: { url: string; token: string }[] = [];
  const player: CastPlayer = {
    load(url, token) {
      loads.push({ url, token });
    },
    startAt() {},
    stop() {},
    fragments: () => [],
    mediaNow: () => null,
    timeFor: () => null,
    forwardSec: () => 0,
    paused: () => true,
    pause() {},
    play() {},
    seek() {},
    setRate() {},
  };
  return { player, loads };
}

test("a session loads the mk link, and a fresh link to the same playlist does not reload", () => {
  const fake = recorder();
  const session = openCastSession({
    pageURL: "http://192.0.2.10:8477/cast",
    channelId: 28,
    playlist,
    room: "channel:28",
    latency: "balanced",
    token: "bw_tok",
    mediaKey: "one",
    player: fake.player,
    transport: { send() {}, close() {} },
  });
  assert.ok(session);
  assert.deepEqual(fake.loads, [{ url: `http://192.0.2.10:8477${playlist}?mk=one`, token: "bw_tok" }]);
  session.retune({ channelId: 28, playlist, room: "channel:28", latency: "balanced", token: "bw_tok", mediaKey: "two", title: "", subtitle: "" });
  assert.equal(fake.loads.length, 1);
  const next = "/media/live/29/1080.aac2.broadcast/index.m3u8";
  session.retune({ channelId: 29, playlist: next, room: "channel:29", latency: "balanced", token: "bw_tok", mediaKey: "three", title: "", subtitle: "" });
  assert.deepEqual(fake.loads.at(-1), { url: `http://192.0.2.10:8477${next}?mk=three`, token: "bw_tok" });
  session.retune({ channelId: 30, playlist: "/media/live/30/1080.aac2.broadcast/index.m3u8", room: "channel:30", latency: "balanced", token: "bw_tok", title: "", subtitle: "" });
  assert.equal(fake.loads.at(-1)?.url, "http://192.0.2.10:8477/media/live/30/1080.aac2.broadcast/index.m3u8");
});
