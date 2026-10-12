import Hls from "hls.js";
import { afterEach, describe, expect, test, vi } from "vitest";
import { masterConfig, openPartsOnly, primeLevel, primedLoader, type Primed } from "./src/lib/primeLevel.ts";

const masterUrl = "http://127.0.0.1/media/live/4/master.m3u8";
const levelUrl = "http://127.0.0.1/media/live/4/level.m3u8";

const completed =
  "#EXTM3U\n#EXT-X-VERSION:6\n#EXTINF:2.000,\nseg1.m4s\n#EXT-X-PART:DURATION=0.500,URI=\"old.m4s\"\n#EXTINF:2.000,\nseg2.m4s\n#EXT-X-PART:DURATION=0.500,URI=\"open.m4s\"\n";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("openPartsOnly", () => {
  test("drops parts of a finished segment and keeps the open one", () => {
    const out = openPartsOnly(completed);
    expect(out).not.toContain("old.m4s");
    expect(out).toContain("seg1.m4s");
    expect(out).toContain("seg2.m4s");
    expect(out).toContain('URI="open.m4s"');
  });

  test("keeps a part-target line and every open part", () => {
    const text = "#EXTM3U\n#EXT-X-PART-INF:PART-TARGET=0.5\n#EXTINF:2.0,\nseg.m4s\n#EXT-X-PART:DURATION=0.5,URI=\"a.m4s\"\n#EXT-X-PART:DURATION=0.5,URI=\"b.m4s\"\n";
    const out = openPartsOnly(text);
    expect(out).toContain("#EXT-X-PART-INF:PART-TARGET=0.5");
    expect(out).toContain('URI="a.m4s"');
    expect(out).toContain('URI="b.m4s"');
  });

  test("a playlist with no parts is returned unchanged, including CRLF", () => {
    const text = "#EXTM3U\r\n#EXTINF:2.0,\r\nseg.m4s\r\n";
    expect(openPartsOnly(text)).toBe(text);
    expect(openPartsOnly("")).toBe("");
  });

  test("a CRLF playlist still keeps the open part", () => {
    const text = "#EXTM3U\r\n#EXTINF:2.000,\r\nseg.m4s\r\n#EXT-X-PART:DURATION=0.500,URI=\"done.m4s\"\r\n#EXTINF:2.000,\r\nseg2.m4s\r\n#EXT-X-PART:DURATION=0.500,URI=\"open.m4s\"\r\n\r\n";
    const out = openPartsOnly(text);
    expect(out).toContain('URI="open.m4s"');
    expect(out).not.toContain("done.m4s");
    expect(out).not.toContain("\r");
  });

  test("a blank line after the open part does not count as a segment", () => {
    const text = "#EXTM3U\n#EXTINF:2.0,\nseg.m4s\n#EXT-X-PART:DURATION=0.5,URI=\"open.m4s\"\n   \n";
    expect(openPartsOnly(text)).toContain('URI="open.m4s"');
  });
});

function liveLevel() {
  return [
    "#EXTM3U",
    "#EXT-X-VERSION:6",
    "#EXT-X-TARGETDURATION:2",
    "#EXT-X-MEDIA-SEQUENCE:10",
    "#EXT-X-PROGRAM-DATE-TIME:2026-10-11T23:00:00.000Z",
    "#EXTINF:2.000,",
    "seg10.m4s",
    "#EXT-X-PART:DURATION=0.500,URI=\"old.m4s\"",
    "#EXTINF:2.000,",
    "seg11.m4s",
    "#EXT-X-PART:DURATION=0.500,URI=\"open.m4s\"",
    "",
  ].join("\n");
}

function masterBody() {
  return ["#EXTM3U", "#EXT-X-VERSION:6", '#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",NAME="English",LANGUAGE="en",DEFAULT=YES,URI="eng.m3u8"', "#EXT-X-STREAM-INF:BANDWIDTH=1000000,AUDIO=\"aud\"", "level.m3u8", ""].join("\n");
}

test("primeLevel keeps one live picture playlist and its open parts", async () => {
  const fetches: string[] = [];
  vi.stubGlobal("fetch", (url: string, init?: RequestInit) => {
    fetches.push(String(url));
    expect(init?.cache).toBe("no-store");
    const body = String(url) === masterUrl ? masterBody() : liveLevel();
    return Promise.resolve(new Response(body, { status: 200 }));
  });
  const primed = await primeLevel(masterUrl);
  expect(fetches).toEqual([masterUrl, levelUrl]);
  expect(primed).not.toBeNull();
  expect(primed?.url).toBe(masterUrl);
  expect(primed?.frags.length).toBeGreaterThan(0);
  expect(primed?.sounds).toEqual([{ role: "main", name: "English", lang: "en" }]);
  const stored = primed?.bodies.get(levelUrl)?.data ?? "";
  expect(stored).toContain('URI="open.m4s"');
  expect(stored).not.toContain("old.m4s");
});

test("primeLevel refuses a media playlist, two levels, and a finished recording", async () => {
  vi.stubGlobal("fetch", (url: string) => {
    const target = String(url);
    if (target.endsWith("media.m3u8")) return Promise.resolve(new Response("#EXTM3U\n#EXTINF:2.0,\nseg.m4s\n", { status: 200 }));
    if (target.endsWith("two.m3u8")) {
      return Promise.resolve(new Response("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\na.m3u8\n#EXT-X-STREAM-INF:BANDWIDTH=2\nb.m3u8\n", { status: 200 }));
    }
    if (target.endsWith("ended.m3u8")) return Promise.resolve(new Response("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\nended-level.m3u8\n", { status: 200 }));
    return Promise.resolve(new Response("#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXTINF:2.0,\nseg.m4s\n#EXT-X-ENDLIST\n", { status: 200 }));
  });
  expect(await primeLevel("http://127.0.0.1/media.m3u8")).toBeNull();
  expect(await primeLevel("http://127.0.0.1/two.m3u8")).toBeNull();
  expect(await primeLevel("http://127.0.0.1/ended.m3u8")).toBeNull();
});

test("primeLevel reports a failed fetch and passes the abort signal", async () => {
  const signal = new AbortController().signal;
  vi.stubGlobal("fetch", () => Promise.resolve(new Response("nope", { status: 404 })));
  await expect(primeLevel(masterUrl, signal)).rejects.toThrow(/404/);

  vi.stubGlobal("fetch", (_url: string, init?: RequestInit) => {
    expect(init?.signal).toBe(signal);
    return Promise.reject(new Error("down"));
  });
  await expect(primeLevel(masterUrl, signal)).rejects.toThrow("down");
});

test("a primed loader answers each playlist once, then asks the network", async () => {
  vi.useFakeTimers();
  const opened: string[] = [];
  class FakeXHR {
    responseURL = "";
    status = 200;
    responseText = "fresh";
    readyState = 0;
    onload: (() => void) | null = null;
    onreadystatechange: (() => void) | null = null;
    open(_method: string, url: string) {
      this.responseURL = url;
      opened.push(url);
    }
    setRequestHeader() {}
    send() {
      this.readyState = 4;
      this.onreadystatechange?.();
      this.onload?.();
    }
    abort() {}
    getAllResponseHeaders() {
      return "";
    }
    addEventListener() {}
  }
  vi.stubGlobal("XMLHttpRequest", FakeXHR);
  const primed: Primed = {
    url: masterUrl,
    bodies: new Map([
      [masterUrl, { data: "master", start: 1, first: 2, end: 3 }],
      [levelUrl, { data: "level", start: 4, first: 5, end: 6 }],
    ]),
    frags: [],
    sounds: [],
  };
  const Loader = primedLoader(primed) as unknown as new (config: object) => {
    load: (
      context: { url: string; responseType: string },
      config: object,
      callbacks: { onSuccess: (response: { data: string }, stats: { aborted?: boolean }, ctx: unknown, details: unknown) => void },
    ) => void;
    abort: () => void;
    destroy: () => void;
    stats: { aborted?: boolean };
  };
  const policy = { loadPolicy: Hls.DefaultConfig.playlistLoadPolicy, maxRetry: 0, timeout: 0 };
  const context = (url: string) => ({ url, responseType: "text" });
  const first = new Loader({});
  let got = "";
  first.load(context(masterUrl), policy, { onSuccess: (response) => (got = response.data) });
  expect(opened).toEqual([]);
  await vi.advanceTimersByTimeAsync(0);
  expect(got).toBe("master");

  new Loader({}).load(context(masterUrl), policy, { onSuccess: () => undefined });
  expect(opened).toContain(masterUrl);

  const late = new Loader({});
  let lateHit = false;
  late.load(context(levelUrl), policy, { onSuccess: () => (lateHit = true) });
  late.abort();
  await vi.advanceTimersByTimeAsync(10);
  expect(lateHit).toBe(false);
  expect(late.stats.aborted).toBe(true);

  let fresh = "";
  new Loader({}).load(context("http://127.0.0.1/other.m3u8"), policy, { onSuccess: (response) => (fresh = response.data) });
  expect(opened).toContain("http://127.0.0.1/other.m3u8");
  expect(fresh).toBe("fresh");

  primed.bodies.set(levelUrl, { data: "level", start: 4, first: 5, end: 6 });
  const dropped = new Loader({});
  let droppedHit = false;
  dropped.load(context(levelUrl), policy, { onSuccess: () => (droppedHit = true) });
  dropped.destroy();
  await vi.advanceTimersByTimeAsync(10);
  expect(droppedHit).toBe(false);
});

test("master config starts on the chosen sound and does not load a second caption track", () => {
  const primed: Primed = { url: masterUrl, bodies: new Map(), frags: [], sounds: [] };
  const plain = masterConfig(primed);
  expect(plain.pLoader).toBeTypeOf("function");
  expect(plain.subtitleTrackController).toBeUndefined();
  expect(plain.subtitleStreamController).toBeUndefined();
  expect(plain.audioPreference).toBeUndefined();
  const withSound = masterConfig(primed, { role: "main", name: "English", lang: "en" });
  expect(withSound.audioPreference).toEqual({ name: "English", lang: "en" });
});
