// The television counts as a viewer of the rendition it was told to play.
// A browser that leaves would otherwise drop the channel out from under it.
// The check matches live.ParseRenditionKey: a key that does not round-trip
// would make the server choose an encode, and a Cast message must not do that.

import { mediaPath } from "./protocol.ts";

const videos = new Set(["copy", "1080", "720", "540", "360"]);
const audios = new Set(["copy", "aac2", "aac6", "ac3", "none"]);

/** The rendition key the server would store, or null when the key is not one. */
export function canonicalRendition(key: string): string | null {
  const parts = key.split(".");
  if (parts.length < 2 || parts.length > 7) return null;
  const video = parts[0];
  const audio = parts[1];
  if (!videos.has(video) || !audios.has(audio)) return null;
  let mode = "";
  let track = "";
  let even = false;
  let full = false;
  let hevc = false;
  for (const part of parts.slice(2)) {
    if (part === "hevc") hevc = true;
    else if (part === "even") even = true;
    else if (part === "60") full = true;
    else if (part === "lang" || part === "vi") track = part;
    else if (part === "broadcast" || part === "smooth" || part === "film") mode = part;
    else return null;
  }
  if (video === "copy") {
    mode = "";
    hevc = false;
    full = false;
  } else if (mode !== "smooth" && mode !== "film") mode = "broadcast";
  if (track !== "lang" && track !== "vi") track = "";
  if (audio === "none") {
    track = "";
    even = false;
  }
  let out = `${video}.${audio}`;
  if (mode) out += `.${mode}`;
  if (track) out += `.${track}`;
  if (even) out += ".even";
  if (full) out += ".60";
  if (hevc) out += ".hevc";
  return out === key ? out : null;
}

/** Channel and rendition named by a live playlist, or null. */
export function renditionOf(playlist: string): { channelId: number; rendition: string } | null {
  const path = mediaPath(playlist);
  if (!path) return null;
  const parts = path.split("/");
  if (parts.length !== 6 || parts[1] !== "media" || parts[2] !== "live") return null;
  const file = parts[5];
  if (file !== "index.m3u8" && file !== "main.m3u8" && file !== "master.m3u8") return null;
  const channelId = Number(parts[3]);
  if (!Number.isInteger(channelId) || channelId <= 0 || String(channelId) !== parts[3]) return null;
  const rendition = canonicalRendition(parts[4] ?? "");
  if (!rendition) return null;
  return { channelId, rendition };
}

export type WatchGrant = { channelId: number; rendition: string; boot: string };

export type CastHoldInput = { channelId: number; playlist: string; token: string };

/** Watch and stop, on this server only. The token is a header, never the URL. */
export function watchEndpoint(page: string, channelId = 0): string {
  const url = new URL(page);
  url.pathname = channelId > 0 ? `/api/v1/watch/${channelId}/stop` : "/api/v1/watch";
  url.search = "";
  url.hash = "";
  return url.toString();
}

export function watchPayload(channelId: number, rendition: string) {
  return { channelId, rendition, confirmLive: true };
}

export function stopPayload(held: WatchGrant) {
  return { rendition: held.rendition, boot: held.boot };
}

export function watchHeaders(token: string): Record<string, string> {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (token) headers.Authorization = "Bearer " + token;
  return headers;
}

export type WatchPoster = {
  post(body: { channelId: number; rendition: string }, token: string): Promise<WatchGrant | null>;
  stop(held: WatchGrant, token: string): void;
};

function browserPoster(pageURL: string): WatchPoster {
  return {
    async post(body, token) {
      try {
        const res = await fetch(watchEndpoint(pageURL), {
          method: "POST",
          headers: watchHeaders(token),
          body: JSON.stringify(watchPayload(body.channelId, body.rendition)),
          credentials: "omit",
          cache: "no-store",
        });
        if (!res.ok) return null;
        const payload = (await res.json()) as { channelId?: unknown; rendition?: unknown; boot?: unknown };
        if (typeof payload.channelId !== "number" || typeof payload.rendition !== "string") return null;
        return { channelId: payload.channelId, rendition: payload.rendition, boot: typeof payload.boot === "string" ? payload.boot : "" };
      } catch {
        return null;
      }
    },
    stop(held, token) {
      void fetch(watchEndpoint(pageURL, held.channelId), {
        method: "POST",
        headers: watchHeaders(token),
        body: JSON.stringify(stopPayload(held)),
        credentials: "omit",
        keepalive: true,
      }).catch(() => undefined);
    },
  };
}

type Grant = WatchGrant & { token: string };

export type CastHold = {
  hold(input: CastHoldInput): void;
  release(): void;
  noteBoot(boot: string): void;
};

/**
 * One viewer. A second hold of the same rendition does not count twice.
 * A grant that is not the rendition we asked for is released, including one
 * that arrives after a retune or a server restart.
 */
export function createCastHold(pageURL: string, poster: WatchPoster = browserPoster(pageURL)): CastHold {
  let generation = 0;
  let current: Grant | null = null;
  let pending: { gen: number; channelId: number; rendition: string } | null = null;
  let last: CastHoldInput | null = null;
  let seenBoot = "";
  let bootRetries = 0;
  let failRetries = 0;

  function releaseGrant(grant: Grant) {
    poster.stop({ channelId: grant.channelId, rendition: grant.rendition, boot: grant.boot }, grant.token);
  }

  function hold(input: CastHoldInput, retry = false) {
    last = { channelId: input.channelId, playlist: input.playlist, token: input.token };
    const named = renditionOf(input.playlist);
    const match = named && named.channelId === input.channelId ? named : null;
    if (match && current && current.channelId === match.channelId && current.rendition === match.rendition) {
      current = { ...current, token: input.token };
      return;
    }
    if (match && pending && pending.channelId === match.channelId && pending.rendition === match.rendition) return;
    // A caller's hold starts clean. A retry must not, or a dead server is asked forever.
    if (!retry) {
      bootRetries = 0;
      failRetries = 0;
    }
    const gen = ++generation;
    const previous = current;
    current = null;
    pending = match ? { gen, channelId: match.channelId, rendition: match.rendition } : null;
    if (previous) releaseGrant(previous);
    if (!match) return;
    const asked = match;
    void poster.post({ channelId: asked.channelId, rendition: asked.rendition }, input.token).then((granted) => {
      if (pending?.gen === gen) pending = null;
      if (gen !== generation) {
        if (granted) releaseGrant({ ...granted, token: input.token });
        return;
      }
      if (!granted) {
        // Hello already named this process, and the request died with the old one.
        if (seenBoot && last && failRetries < 1) {
          failRetries = 1;
          hold(last, true);
        }
        return;
      }
      const token = last?.channelId === asked.channelId ? last.token : input.token;
      const grant: Grant = { ...granted, token };
      if (grant.channelId !== asked.channelId || grant.rendition !== asked.rendition) {
        releaseGrant(grant);
        return;
      }
      if (seenBoot && grant.boot && seenBoot !== grant.boot) {
        releaseGrant(grant);
        if (bootRetries < 1 && last) {
          bootRetries = 1;
          hold(last, true);
        }
        return;
      }
      bootRetries = 0;
      current = grant;
    });
  }

  return {
    hold,
    release() {
      generation += 1;
      pending = null;
      bootRetries = 0;
      failRetries = 0;
      const previous = current;
      current = null;
      last = null;
      if (previous) releaseGrant(previous);
    },
    noteBoot(boot) {
      if (!boot) return;
      const changed = boot !== seenBoot;
      seenBoot = boot;
      if (changed) failRetries = 0;
      if (current && current.boot && current.boot !== boot) {
        const again = last;
        const previous = current;
        current = null;
        releaseGrant(previous);
        if (again) hold(again);
        return;
      }
      // The watch died before this hello. Ask once for the process that is up now.
      if (changed && !current && !pending && last && failRetries < 1) {
        failRetries = 1;
        hold(last, true);
      }
    },
  };
}
