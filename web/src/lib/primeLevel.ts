import Hls, {
  LoadStats,
  M3U8Parser,
  PlaylistLevelType,
  type HlsConfig,
  type Loader,
  type LoaderCallbacks,
  type LoaderConfiguration,
  type LoaderContext,
} from "hls.js";
import { soundsOf, type Sound } from "../features/player/sounds";
import type { Frag } from "./roomStart";

type Body = { data: string; start: number; first: number; end: number };

/** A master and its picture playlist, fetched once, for a start on the room's frame. */
export type Primed = { url: string; bodies: Map<string, Body>; frags: Frag[]; sounds: Sound[] };

const key = (url: string) => new URL(url, window.location.href).href;

async function load(url: string, signal?: AbortSignal): Promise<Body> {
  const start = performance.now();
  const res = await fetch(url, { signal, cache: "no-store" });
  const first = performance.now();
  if (!res.ok) throw new Error(`${res.status} ${url}`);
  const data = await res.text();
  return { data, start, first, end: performance.now() };
}

/**
 * hls.js loads no level playlist from a master until startLoad, and a start on
 * the room's frame needs that playlist's fragments before it. This fetches the
 * master and its one picture playlist and parses the fragments with hls.js's own
 * parser. primedLoader then answers hls.js's first requests with the same
 * bodies, so the fragment starts it plays by are the ones the start was computed on.
 */
export async function primeLevel(master: string, signal?: AbortSignal): Promise<Primed | null> {
  const masterUrl = key(master);
  const top = await load(masterUrl, signal);
  if (M3U8Parser.isMediaPlaylist(top.data)) return null;
  const parsed = M3U8Parser.parseMasterPlaylist(top.data, masterUrl);
  const { levels } = parsed;
  const urls = new Set(levels.map((level) => level.url));
  if (urls.size !== 1) return null;
  const [levelUrl] = urls;
  const level = await load(levelUrl, signal);
  level.data = openPartsOnly(level.data);
  const details = M3U8Parser.parseLevelPlaylist(level.data, levelUrl, 0, PlaylistLevelType.MAIN, 0, null);
  if (!details.live) return null;
  const frags = details.fragments.map((f) => ({ start: f.start, duration: f.duration, programDateTime: f.programDateTime }));
  const sounds = soundsOf(M3U8Parser.parseMasterPlaylistMedia(top.data, masterUrl, parsed).AUDIO ?? []);
  return {
    url: masterUrl,
    bodies: new Map([
      [masterUrl, top],
      [key(levelUrl), level],
    ]),
    frags,
    sounds,
  };
}

/**
 * The server lists a recent segment's parts before it, as LL-HLS asks, and
 * AVPlayer needs them. hls.js 1.7 mis-times a playlist that ends on such a
 * segment with nothing open after it, then rejects the next playlist and stops
 * loading. It played whole segments and the open segment's parts before, so
 * those are all it gets.
 */
export function openPartsOnly(text: string): string {
  if (!text.includes("#EXT-X-PART:")) return text;
  const lines = text.split("\n");
  let last = -1;
  lines.forEach((line, i) => {
    if (line && !line.startsWith("#")) last = i;
  });
  return lines.filter((line, i) => i > last || !line.startsWith("#EXT-X-PART:")).join("\n");
}

/** hls.js's playlist loader, with each playlist passed through openPartsOnly. */
export function livePlaylistLoader(): HlsConfig["pLoader"] {
  const Base = Hls.DefaultConfig.loader;
  class OpenPartsLoader extends Base {
    load(context: LoaderContext, config: LoaderConfiguration, callbacks: LoaderCallbacks<LoaderContext>) {
      super.load(context, config, {
        ...callbacks,
        onSuccess: (response, stats, ctx, details) => {
          if (typeof response.data === "string") response.data = openPartsOnly(response.data);
          callbacks.onSuccess(response, stats, ctx, details);
        },
      });
    }
  }
  return OpenPartsLoader as unknown as HlsConfig["pLoader"];
}

/** A playlist loader that answers each primed URL once from its body and fetches the rest. */
export function primedLoader(primed: Primed): HlsConfig["pLoader"] {
  const Base = livePlaylistLoader() as unknown as typeof Hls.DefaultConfig.loader;
  class PrimedLoader implements Loader<LoaderContext> {
    private inner: Loader<LoaderContext> | null = null;
    private own = new LoadStats();
    private owned: LoaderContext | null = null;
    private timer = 0;

    constructor(private config: HlsConfig) {}

    get stats() {
      return this.inner?.stats ?? this.own;
    }

    get context() {
      return this.inner?.context ?? this.owned;
    }

    load(context: LoaderContext, config: LoaderConfiguration, callbacks: LoaderCallbacks<LoaderContext>) {
      const body = primed.bodies.get(key(context.url));
      if (!body) {
        this.inner = new Base(this.config);
        this.inner.load(context, config, callbacks);
        return;
      }
      primed.bodies.delete(key(context.url));
      this.owned = context;
      this.own.loading = { start: body.start, first: body.first, end: body.end };
      this.own.loaded = this.own.total = body.data.length;
      this.timer = window.setTimeout(() => callbacks.onSuccess({ url: context.url, data: body.data }, this.own, context, null));
    }

    abort() {
      window.clearTimeout(this.timer);
      this.own.aborted = true;
      this.inner?.abort();
    }

    destroy() {
      window.clearTimeout(this.timer);
      this.inner?.destroy();
      this.inner = null;
    }

    getCacheAge() {
      return this.inner?.getCacheAge?.() ?? null;
    }

    getResponseHeader(name: string) {
      return this.inner?.getResponseHeader?.(name) ?? null;
    }
  }
  return PrimedLoader as unknown as HlsConfig["pLoader"];
}

/**
 * hls.js settings for a primed master, starting on the given sound. Captions
 * come from liveCaptions, so hls.js does not load the master's subtitle group
 * as a second track.
 */
export function masterConfig(primed: Primed, sound?: Sound): Partial<HlsConfig> {
  return {
    pLoader: primedLoader(primed),
    subtitleTrackController: undefined,
    subtitleStreamController: undefined,
    ...(sound ? { audioPreference: { name: sound.name, lang: sound.lang } } : {}),
  };
}
