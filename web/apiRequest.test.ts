import assert from "node:assert/strict";
import { afterEach, test } from "node:test";
import { addPass, addPlaylistFile, apiFailure, getAirings, getHome, playVirtual, removeDevice, request, restoreBackup, search, startScan, stopWatchTicket, unreadBody, watchChannel, watchTicket } from "./src/api.ts";
import { onUnauthorized } from "./src/lib/deviceToken.ts";

type Call = { url: string; init?: RequestInit };

const calls: Call[] = [];

function storage() {
  const store = new Map<string, string>();
  Object.assign(globalThis, {
    localStorage: {
      getItem: (key: string) => store.get(key) ?? null,
      setItem: (key: string, value: string) => store.set(key, value),
      removeItem: (key: string) => store.delete(key),
    },
  });
  return store;
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
}

function stub(answer: (url: string, init?: RequestInit) => Response | Promise<Response>) {
  calls.length = 0;
  globalThis.fetch = (async (url: string, init?: RequestInit) => {
    calls.push({ url: String(url), init });
    return answer(String(url), init);
  }) as typeof fetch;
}

function sentBody() {
  return JSON.parse(String(calls.at(-1)?.init?.body)) as Record<string, unknown>;
}

afterEach(() => {
  calls.length = 0;
  storage();
});

test("an error body becomes a failure, and a body that is not JSON uses the shared words", () => {
  const parsed = apiFailure(409, JSON.stringify({ message: "That channel is already recording.", code: "recording" }));
  assert.equal(parsed.message, "That channel is already recording.");
  assert.equal(parsed.status, 409);
  assert.equal(parsed.code, "recording");

  const coded = apiFailure(400, JSON.stringify({ code: "bad" }));
  assert.equal(coded.message, unreadBody());
  assert.equal(coded.code, "bad");

  const blank = apiFailure(500, JSON.stringify({ message: "" }));
  assert.equal(blank.message, unreadBody());
  assert.equal(blank.code, undefined);

  const junk = apiFailure(502, "nope");
  assert.equal(junk.message, unreadBody());
  assert.equal(junk.status, 502);
});

test("a request sends JSON, keeps the caller's headers, and opens pairing only on 401", async () => {
  const store = storage();
  store.set("bw.deviceToken", "den-token");
  let denied = 0;
  const stop = onUnauthorized(() => {
    denied += 1;
  });
  stub(() => jsonResponse({ ok: true }));
  const signal = new AbortController().signal;
  assert.deepEqual(await request<{ ok: boolean }>("/api/v1/ping", { signal, headers: { "Content-Type": "text/plain", "X-Den": "1" } }), { ok: true });
  assert.equal(calls[0].url, "/api/v1/ping");
  assert.equal(calls[0].init?.signal, signal);
  const headers = calls[0].init?.headers as Record<string, string>;
  assert.equal(headers["Content-Type"], "text/plain");
  assert.equal(headers["X-Den"], "1");
  assert.equal(headers.Authorization, "Bearer den-token");

  stub(() => jsonResponse({ message: "Sign in again.", code: "unauthorized" }, 401));
  await assert.rejects(request("/api/v1/ping"), (err: unknown) => {
    assert.equal((err as { status: number }).status, 401);
    assert.equal((err as Error).message, "Sign in again.");
    return true;
  });
  assert.equal(denied, 1);

  stub(() => jsonResponse({ message: "no" }, 403));
  await assert.rejects(request("/api/v1/ping"));
  stub(() => jsonResponse({ message: "down" }, 500));
  await assert.rejects(request("/api/v1/ping"));
  assert.equal(denied, 1);
  stop();

  stub(() => jsonResponse({ message: "Sign in again." }, 401));
  await assert.rejects(request("/api/v1/ping"));
  assert.equal(denied, 1);

  stub(() => new Response("not-json", { status: 200 }));
  await assert.rejects(request("/api/v1/ping"), SyntaxError);
});

test("watch, search, and guide reads build the path the server expects", async () => {
  stub(() => jsonResponse({}));
  await watchChannel(4, { platform: "web", video: ["h264"], audio: ["aac"], network: "lan" }, { quality: "auto" } as never);
  assert.equal(calls[0].url, "/api/v1/watch");
  assert.equal(calls[0].init?.method, "POST");
  assert.equal("confirmLive" in sentBody(), false);
  assert.equal("room" in sentBody(), false);
  assert.equal(sentBody().channelId, 4);

  await watchChannel(4, { platform: "web", video: ["h264"], audio: ["aac"], network: "lan" }, { quality: "auto" } as never, "720", true, undefined, "den");
  const again = sentBody();
  assert.equal(again.confirmLive, true);
  assert.equal(again.room, "den");
  assert.equal(again.rendition, "720");
  assert.equal("ticket" in again, false);

  await watchChannel(4, { platform: "web", video: ["h264"], audio: ["aac"], network: "lan" }, { quality: "auto" } as never, "", false, undefined, "", "t1");
  assert.equal(sentBody().ticket, "t1");

  await search("News & 4");
  assert.equal(calls.at(-1)?.url, "/api/v1/search?q=News%20%26%204");

  await getAirings();
  assert.equal(calls.at(-1)?.url, "/api/v1/airings");
  await getAirings({ from: "2026-10-11T00:00:00Z", to: "2026-10-12T00:00:00Z", channels: "4,7" });
  const params = new URL(calls.at(-1)!.url, "http://127.0.0.1").searchParams;
  assert.equal(params.get("from"), "2026-10-11T00:00:00Z");
  assert.equal(params.get("to"), "2026-10-12T00:00:00Z");
  assert.equal(params.get("channels"), "4,7");

  await getHome();
  assert.equal(calls.at(-1)?.url, "/api/v1/home");
  await getHome(true);
  assert.equal(calls.at(-1)?.url, "/api/v1/home?fresh=1");

  await startScan("duo / a");
  assert.equal(calls.at(-1)?.url, "/api/v1/devices/duo%20%2F%20a/scan");
  assert.equal(calls.at(-1)?.init?.method, "POST");
  await removeDevice("duo / a");
  assert.equal(calls.at(-1)?.url, "/api/v1/devices/duo%20%2F%20a");
  assert.equal(calls.at(-1)?.init?.method, "DELETE");

  await restoreBackup("nightly backup");
  assert.equal(calls.at(-1)?.url, "/api/v1/backups/nightly%20backup/restore");

  await addPass("Night Show", 4);
  assert.equal("airingStart" in sentBody(), false);
  await addPass("Night Show", 4, "2026-10-11T23:00:00Z");
  assert.equal(sentBody().airingStart, "2026-10-11T23:00:00Z");

  await playVirtual(3, 1);
  assert.equal(calls.at(-1)?.url, "/api/v1/virtuals/3/play");
  assert.equal(sentBody().pictureMode, "broadcast");
  assert.equal(sentBody().index, 1);
});

test("a playlist file uploads as form data, and a 401 still opens pairing", async () => {
  const store = storage();
  let denied = 0;
  const stop = onUnauthorized(() => {
    denied += 1;
  });
  const file = new File(["#EXTM3U\n"], "den.m3u");
  stub(() => jsonResponse({ id: 8, message: "Added." }));
  const added = await addPlaylistFile("Den TV", "news", file, "30");
  assert.equal(added.id, 8);
  assert.equal(calls[0].url, "/api/v1/sources");
  assert.equal(calls[0].init?.method, "POST");
  const body = calls[0].init?.body as FormData;
  assert.equal(body.get("name"), "Den TV");
  assert.equal(body.get("groups"), "news");
  assert.equal(body.get("keep"), "30");
  assert.ok(body.get("file"));
  const headers = calls[0].init?.headers as Record<string, string>;
  assert.equal(headers["Content-Type"], undefined);
  assert.equal(headers.Authorization, undefined);

  store.set("bw.deviceToken", "den-token");
  stub(() => jsonResponse({ message: "Sign in again." }, 401));
  await assert.rejects(addPlaylistFile("Den TV", "", file), (err: unknown) => {
    assert.equal((err as Error).message, "Sign in again.");
    assert.equal(err instanceof Error, true);
    return true;
  });
  assert.equal((calls[0].init?.headers as Record<string, string>).Authorization, "Bearer den-token");
  assert.equal(denied, 1);

  stub(() => new Response("nope", { status: 500 }));
  await assert.rejects(addPlaylistFile("Den TV", "", file), (err: unknown) => {
    assert.equal((err as Error).message, unreadBody());
    return true;
  });
  assert.equal(denied, 1);
  stop();
});

test("a watch ticket is a fresh name, and its stop names it", async () => {
  const a = watchTicket();
  const b = watchTicket();
  assert.match(a, /^[0-9a-f]{24}$/);
  assert.notEqual(a, b);

  // No beacon (as in Node): the stop goes as a kept-alive request.
  stub(() => jsonResponse({ ok: true }));
  stopWatchTicket(4, a);
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(calls.at(-1)?.url, "/api/v1/watch/4/stop");
  assert.equal(calls.at(-1)?.init?.keepalive, true);
  assert.deepEqual(JSON.parse(String(calls.at(-1)?.init?.body)), { ticket: a });
});
