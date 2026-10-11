import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { followCaptions, watchTimeline } from "./src/features/player/liveCaptions.ts";

// jsdom has no VTTCue. The follower constructs one for each caption.
class Cue {
  snapToLines = true;
  line: number | "auto" = "auto";
  position: number | "auto" = "auto";
  size = 100;
  align: AlignSetting = "center";
  constructor(
    public startTime: number,
    public endTime: number,
    public text: string,
  ) {}
}

const EV = { INIT_PTS_FOUND: "init-pts", DESTROYING: "destroying" };
const PLAYLIST = "http://127.0.0.1/media/live/4/captions.m3u8";

type Track = {
  mode: TextTrackMode;
  cues: Cue[];
  addCue: (cue: Cue) => void;
  removeCue: (cue: Cue) => void;
};

type Frag = { sn: number; start: number };

function makeVideo(start = 0) {
  let time = start;
  const cues: Cue[] = [];
  const track: Track = {
    mode: "disabled",
    cues,
    addCue(cue) {
      cues.push(cue);
    },
    removeCue(cue) {
      const at = cues.indexOf(cue);
      if (at >= 0) cues.splice(at, 1);
    },
  };
  const video = document.createElement("video") as HTMLVideoElement & { captionTrack?: Track };
  video.addTextTrack = () => track as unknown as TextTrack;
  Object.defineProperty(video, "currentTime", {
    configurable: true,
    get: () => time,
    set: (value: number) => {
      time = value;
    },
  });
  return {
    video,
    track,
    setTime(value: number) {
      time = value;
    },
  };
}

function makeHls(fragments: Frag[]) {
  const ons = new Map<string, Set<(event: string, data?: unknown) => void>>();
  const onces = new Map<string, Set<(event: string, data?: unknown) => void>>();
  return {
    levels: [{ details: { fragments } }] as { details?: { fragments: Frag[] } }[] | undefined,
    on(name: string, fn: (event: string, data?: unknown) => void) {
      const set = ons.get(name) ?? new Set();
      set.add(fn);
      ons.set(name, set);
    },
    once(name: string, fn: (event: string, data?: unknown) => void) {
      const set = onces.get(name) ?? new Set();
      set.add(fn);
      onces.set(name, set);
    },
    off(name: string, fn: (event: string, data?: unknown) => void) {
      ons.get(name)?.delete(fn);
      onces.get(name)?.delete(fn);
    },
    emit(name: string, data?: unknown) {
      for (const fn of [...(ons.get(name) ?? [])]) fn(name, data);
      const once = onces.get(name);
      if (!once) return;
      onces.delete(name);
      for (const fn of once) fn(name, data);
    },
    listening(name: string) {
      return (ons.get(name)?.size ?? 0) + (onces.get(name)?.size ?? 0);
    },
  };
}

function vtt(mpegts: number, text = "Den TV") {
  return `WEBVTT\nX-TIMESTAMP-MAP=LOCAL:00:00:00.000,MPEGTS:${mpegts}\n\n00:00:00.000 --> 00:00:01.000 line:90%\n${text}\n`;
}

type Hit = { url: string; authorization: string | null; cache: RequestCache | undefined };

function route(handler: (url: string, init?: RequestInit) => Response | Promise<Response>) {
  const hits: Hit[] = [];
  vi.stubGlobal("fetch", async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    hits.push({ url, authorization: new Headers(init?.headers).get("authorization"), cache: init?.cache });
    if (init?.signal?.aborted) throw new DOMException("aborted", "AbortError");
    return handler(url, init);
  });
  return hits;
}

async function flush() {
  for (let i = 0; i < 8; i++) await vi.advanceTimersByTimeAsync(0);
}

beforeEach(() => {
  vi.stubGlobal("VTTCue", Cue);
  localStorage.clear();
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  localStorage.clear();
});

test("a caption lands on the break's timeline and leaves once it is half a minute behind", async () => {
  localStorage.setItem("bw.deviceToken", "den-token");
  const { video, track, setTime } = makeVideo(40);
  const stale = new Cue(0, 1, "old");
  track.cues.push(stale);
  video.captionTrack = track;
  const hls = makeHls([{ sn: 11, start: 40 }]);
  // 900000 ticks at 90 kHz is 10 s, which is 90 kHz time 900000. A later init replaces the first.
  watchTimeline(hls as never, EV as never);
  hls.emit(EV.INIT_PTS_FOUND, { frag: { cc: 1 }, initPTS: 0, timescale: 1 });
  hls.emit(EV.INIT_PTS_FOUND, { frag: { cc: 1 }, initPTS: 900000, timescale: 90000 });
  const hits = route((url) => {
    if (url === PLAYLIST) {
      return new Response("#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXT-X-MEDIA-SEQUENCE:11\n#EXT-X-DISCONTINUITY-SEQUENCE:1\n11.vtt\n");
    }
    if (url === "http://127.0.0.1/media/live/4/11.vtt") return new Response(vtt(4_500_000));
    return new Response("missing", { status: 404 });
  });

  const stop = followCaptions(hls as never, EV as never, video, PLAYLIST);
  await flush();

  expect(track.cues).toHaveLength(1);
  expect(track.cues[0]).not.toBe(stale);
  expect(track.cues[0].startTime).toBe(40);
  expect(track.cues[0].endTime).toBe(41);
  expect(track.cues[0].text).toBe("Den TV");
  expect(track.cues[0].line).toBe(90);
  expect(track.cues[0].snapToLines).toBe(false);
  expect(track.mode).toBe("showing");
  expect(video.dataset.captionSegment).toBe("11 40.000");
  expect(hits.map((hit) => hit.url)).toEqual([PLAYLIST, "http://127.0.0.1/media/live/4/11.vtt"]);
  expect(hits.every((hit) => hit.authorization === "Bearer den-token" && hit.cache === "no-store")).toBe(true);

  setTime(71);
  await vi.advanceTimersByTimeAsync(2999);
  expect(hits.filter((hit) => hit.url === PLAYLIST)).toHaveLength(1);
  await vi.advanceTimersByTimeAsync(1);
  await flush();
  // The segment ends at 41. Thirty seconds later it is still kept; a moment past that it goes.
  expect(track.cues).toHaveLength(1);
  setTime(72);
  await vi.advanceTimersByTimeAsync(3000);
  await flush();
  expect(track.cues).toHaveLength(0);
  expect(hits.filter((hit) => hit.url.endsWith(".vtt"))).toHaveLength(1);
  stop();
});

test("only segments near the playhead are fetched, including both edges of the window", async () => {
  const { video } = makeVideo(50);
  const hls = makeHls([
    { sn: 1, start: 39 },
    { sn: 2, start: 40 },
    { sn: 3, start: 95 },
    { sn: 4, start: 96 },
  ]);
  const hits = route((url) => {
    if (url === PLAYLIST) {
      return new Response(
        ["#EXTM3U", "#EXT-X-TARGETDURATION:2", "#EXT-X-MEDIA-SEQUENCE:1", "1.vtt", "2.vtt", "3.vtt", "4.vtt"].join("\r\n"),
      );
    }
    return new Response(vtt(0, "City News"), { status: 200 });
  });
  const stop = followCaptions(hls as never, EV as never, video, PLAYLIST);
  await flush();
  const fetched = hits.filter((hit) => hit.url.endsWith(".vtt")).map((hit) => hit.url);
  expect(fetched).toEqual(["http://127.0.0.1/media/live/4/2.vtt", "http://127.0.0.1/media/live/4/3.vtt"]);
  expect(hits[0].authorization).toBeNull();
  stop();
});

test("a segment waits for its break's clock, then shows on that timeline", async () => {
  let now = 5_000;
  vi.spyOn(performance, "now").mockImplementation(() => now);
  const { video, track } = makeVideo(40);
  const hls = makeHls([{ sn: 11, start: 40 }]);
  watchTimeline(hls as never, EV as never);
  let segments = 0;
  const hits = route((url) => {
    if (url === PLAYLIST) {
      return new Response("#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:11\n#EXT-X-DISCONTINUITY-SEQUENCE:1\n11.vtt\n");
    }
    segments += 1;
    return new Response(vtt(4_500_000, "waiting"));
  });
  const stop = followCaptions(hls as never, EV as never, video, PLAYLIST);
  await flush();
  expect(track.cues).toHaveLength(0);
  expect(segments).toBe(1);

  hls.emit(EV.INIT_PTS_FOUND, { frag: { cc: 1 }, initPTS: 900000, timescale: 90000 });
  await vi.advanceTimersByTimeAsync(1000);
  await flush();
  expect(track.cues.map((cue) => cue.text)).toEqual(["waiting"]);
  expect(segments).toBe(1);
  stop();
  expect(hits.length).toBeGreaterThan(0);
});

test("a break that never reports its clock is dropped after 30 s and fetched again", async () => {
  let now = 1_000;
  vi.spyOn(performance, "now").mockImplementation(() => now);
  const { video, track } = makeVideo(40);
  const hls = makeHls([{ sn: 11, start: 40 }]);
  let bodies = 0;
  route((url) => {
    if (url !== PLAYLIST) {
      bodies += 1;
      return new Response(vtt(4_500_000, "late"));
    }
    return new Response("#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:11\n#EXT-X-DISCONTINUITY-SEQUENCE:1\n11.vtt\n");
  });
  const stop = followCaptions(hls as never, EV as never, video, PLAYLIST);
  await flush();
  expect(bodies).toBe(1);
  expect(track.cues).toHaveLength(0);

  now = 31_001;
  await vi.advanceTimersByTimeAsync(1000);
  await flush();
  expect(track.cues).toHaveLength(0);
  expect(bodies).toBe(1);

  await vi.advanceTimersByTimeAsync(1000);
  await flush();
  expect(bodies).toBe(2);
  expect(track.cues).toHaveLength(0);
  stop();
});

test("the playlist poll waits half the target duration, and stays inside 1 s to 4 s", async () => {
  for (const [target, wait] of [
    ["1", 1000],
    ["6", 3000],
    ["10", 4000],
  ] as const) {
    const hits: string[] = [];
    vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
      hits.push(String(input));
      return new Response(`#EXTM3U\n#EXT-X-TARGETDURATION:${target}\n`);
    });
    const { video } = makeVideo(0);
    const stop = followCaptions(makeHls([]) as never, EV as never, video, PLAYLIST);
    await flush();
    expect(hits).toEqual([PLAYLIST]);
    await vi.advanceTimersByTimeAsync(wait - 1);
    expect(hits).toEqual([PLAYLIST]);
    await vi.advanceTimersByTimeAsync(1);
    await flush();
    expect(hits).toEqual([PLAYLIST, PLAYLIST]);
    stop();
    await vi.advanceTimersByTimeAsync(wait);
    expect(hits).toHaveLength(2);
  }
});

test("a failed playlist or segment is tried again, and a miss is not shown", async () => {
  const { video, track } = makeVideo(40);
  const hls = makeHls([{ sn: 11, start: 40 }]);
  let attempt = 0;
  route((url) => {
    attempt += 1;
    if (attempt === 1) throw new Error("offline");
    if (url === PLAYLIST && attempt === 2) return new Response("no", { status: 500 });
    if (url === PLAYLIST) {
      return new Response("#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:11\n#EXT-X-DISCONTINUITY-SEQUENCE:1\n11.vtt\n");
    }
    if (attempt < 5) return new Response("gone", { status: 404 });
    return new Response(vtt(4_500_000, "back"));
  });
  watchTimeline(hls as never, EV as never);
  hls.emit(EV.INIT_PTS_FOUND, { frag: { cc: 1 }, initPTS: 900000, timescale: 90000 });
  const stop = followCaptions(hls as never, EV as never, video, PLAYLIST);
  await flush();
  expect(track.cues).toHaveLength(0);

  await vi.advanceTimersByTimeAsync(2000);
  await flush();
  expect(track.cues).toHaveLength(0);

  await vi.advanceTimersByTimeAsync(2000);
  await flush();
  expect(track.cues).toHaveLength(0);

  await vi.advanceTimersByTimeAsync(1000);
  await flush();
  expect(track.cues.map((cue) => cue.text)).toEqual(["back"]);
  stop();
});

test("stopping, or the player going away, clears the captions and the poll", async () => {
  const { video, track } = makeVideo(40);
  const hls = makeHls([{ sn: 11, start: 40 }]);
  watchTimeline(hls as never, EV as never);
  hls.emit(EV.INIT_PTS_FOUND, { frag: { cc: 1 }, initPTS: 900000, timescale: 90000 });
  let playlistHits = 0;
  route((url) => {
    if (url === PLAYLIST) {
      playlistHits += 1;
      return new Response("#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:11\n#EXT-X-DISCONTINUITY-SEQUENCE:1\n11.vtt\n");
    }
    return new Response(vtt(4_500_000));
  });
  const stop = followCaptions(hls as never, EV as never, video, PLAYLIST);
  await flush();
  expect(track.cues).toHaveLength(1);
  expect(hls.listening(EV.DESTROYING)).toBe(1);

  stop();
  stop();
  expect(track.cues).toHaveLength(0);
  expect(track.mode).toBe("disabled");
  expect(video.dataset.captionSegment).toBeUndefined();
  expect(hls.listening(EV.DESTROYING)).toBe(0);
  await vi.advanceTimersByTimeAsync(10_000);
  expect(playlistHits).toBe(1);

  const again = followCaptions(hls as never, EV as never, video, PLAYLIST);
  await flush();
  expect(track.cues).toHaveLength(1);
  hls.emit(EV.DESTROYING);
  expect(track.mode).toBe("disabled");
  expect(track.cues).toHaveLength(0);
  again();
  await vi.advanceTimersByTimeAsync(10_000);
  expect(playlistHits).toBe(2);
});

test("a playlist read that is stopped in flight does not poll again", async () => {
  let release: (value: Response) => void = () => undefined;
  const held = new Promise<Response>((resolve) => {
    release = resolve;
  });
  let hits = 0;
  vi.stubGlobal("fetch", () => {
    hits += 1;
    return held;
  });
  const { video } = makeVideo(0);
  const hls = makeHls([]);
  const stop = followCaptions(hls as never, EV as never, video, PLAYLIST);
  await flush();
  expect(hits).toBe(1);
  stop();
  release(new Response("#EXTM3U\n#EXT-X-TARGETDURATION:2\n"));
  await flush();
  await vi.advanceTimersByTimeAsync(10_000);
  expect(hits).toBe(1);
  expect(hls.listening(EV.DESTROYING)).toBe(0);
});

test("a level that is listed later is the one that decides where a segment sits", async () => {
  const { video } = makeVideo(40);
  const hls = makeHls([]);
  hls.levels = [{ details: { fragments: [{ sn: 11, start: 0 }] } }, { details: { fragments: [{ sn: 11, start: 40 }] } }];
  const hits = route((url) => {
    if (url === PLAYLIST) {
      return new Response("#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:11\n11.vtt\n");
    }
    return new Response("no", { status: 404 });
  });
  const stop = followCaptions(hls as never, EV as never, video, PLAYLIST);
  await flush();
  expect(hits.map((hit) => hit.url)).toContain("http://127.0.0.1/media/live/4/11.vtt");

  hls.levels = undefined;
  await vi.advanceTimersByTimeAsync(1000);
  await flush();
  expect(hits.filter((hit) => hit.url.endsWith(".vtt"))).toHaveLength(1);
  stop();
});
