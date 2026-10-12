// A scripted viewer: it opens a watch, polls the live playlist, and fetches
// each new segment. It does not send a blocking reload, so a stalled edge
// shows up as a playlist that stops moving instead of a request that waits.

import { noteFetch, noteStall, noteTransportFailure, noteWindow, parsePlaylist } from "./analyze.mjs";

const caps = { platform: "web", video: ["h264"], audio: ["aac", "ac3"], network: "lan" };

export function createViewer(id) {
  return {
    id,
    channelId: 0,
    rendition: "",
    playlist: "",
    mapFetched: false,
    window: null,
    stall: null,
    paused: false,
    aligned: true,
    quietUntil: 0,
    edgePdt: null,
    fetched: new Set(),
  };
}

export async function retune(viewer, base, channelId, quality) {
  if (viewer.channelId) await stopWatch(base, viewer.channelId, viewer.rendition);
  const session = await watch(base, channelId, quality);
  viewer.channelId = session.channelId ?? channelId;
  viewer.rendition = session.rendition || "";
  viewer.playlist = session.playlist || "";
  viewer.mapFetched = false;
  viewer.window = null;
  viewer.stall = null;
  viewer.edgePdt = null;
  viewer.fetched = new Set();
  viewer.paused = false;
  viewer.aligned = true;
  viewer.quietUntil = Date.now() + 15_000;
  if (!viewer.playlist) throw new Error(`${viewer.id} watched ${channelId} without a playlist`);
}

export async function stopViewer(viewer, base) {
  if (!viewer.channelId) return;
  await stopWatch(base, viewer.channelId, viewer.rendition);
  viewer.channelId = 0;
  viewer.playlist = "";
  viewer.aligned = false;
}

export async function tick(viewer, base, now) {
  const events = [];
  if (!viewer.playlist) return events;
  if (viewer.paused) {
    const stalled = noteStall(viewer.stall, viewer.edgePdt, now, {
      paused: true,
      quietUntil: viewer.quietUntil,
      stallMs: 8_000,
    });
    viewer.stall = stalled.state;
    return events;
  }
  let res;
  let playlistErr = "";
  for (let attempt = 0; attempt < 2 && !res; attempt++) {
    try {
      res = await fetch(new URL(viewer.playlist, base), { cache: "no-store", signal: AbortSignal.timeout(5_000) });
    } catch (err) {
      playlistErr = err.name || "error";
      if (noteTransportFailure("playlist", attempt)) break;
    }
  }
  if (!res) {
    const drop = noteTransportFailure("playlist", 1);
    if (drop) events.push({ ...drop, detail: `playlist ${playlistErr}` });
    return events;
  }
  if (!res.ok) {
    events.push({ kind: "drop", detail: `playlist ${res.status}` });
    return events;
  }
  const parsed = parsePlaylist(await res.text());
  events.push(...onPlaylist(viewer, parsed));
  const stallMs = Math.max(8_000, (parsed.target || 2) * 3_000);
  const stalled = noteStall(viewer.stall, parsed.edgePdt, now, { paused: false, quietUntil: viewer.quietUntil, stallMs });
  viewer.stall = stalled.state;
  events.push(...stalled.events);
  const root = new URL(viewer.playlist, base);
  if (parsed.map && !viewer.mapFetched) {
    events.push(...(await fetchOne(new URL(parsed.map, root), parsed.map)));
    viewer.mapFetched = true;
  }
  for (const uri of freshSegments(viewer.fetched, parsed.segments.map((seg) => seg.uri))) {
    events.push(...(await fetchOne(new URL(uri, root), uri)));
  }
  return events;
}

// onPlaylist remembers the live edge. A sequence that starts over is a new
// directory, so names from the old one have to be fetched again.
export function onPlaylist(viewer, parsed) {
  const noted = noteWindow(viewer.window, parsed);
  const restarted = noted.events.some((event) => event.detail === "playlist sequence restarted");
  if (restarted) {
    viewer.fetched = new Set();
    viewer.mapFetched = false;
  }
  viewer.window = noted.state;
  viewer.edgePdt = parsed.edgePdt;
  return noted.events;
}

// freshSegments returns URIs this viewer has not fetched yet.
// The live playlist keeps about 90 minutes, so the set is not trimmed:
// forgetting an early name would download that whole window on the next poll.
export function freshSegments(seen, uris) {
  const fresh = [];
  for (const uri of uris) {
    if (seen.has(uri)) continue;
    seen.add(uri);
    fresh.push(uri);
  }
  return fresh;
}

async function fetchOne(url, name) {
  let failure = "";
  for (let attempt = 0; attempt < 2; attempt++) {
    try {
      const res = await fetch(url, { cache: "no-store", signal: AbortSignal.timeout(8_000) });
      await res.body?.cancel();
      const drop = noteFetch(res.status, name);
      return drop ? [drop] : [];
    } catch (err) {
      failure = err.name || "error";
      if (noteTransportFailure(name, attempt)) break;
    }
  }
  const drop = noteTransportFailure(name, 1);
  return drop ? [{ ...drop, detail: `${name} ${failure}` }] : [];
}

async function watch(base, channelId, quality) {
  const res = await fetch(new URL("/api/v1/watch", base), {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({
      channelId,
      confirmLive: true,
      caps,
      prefs: { quality, audio: "auto", picture: "broadcast" },
    }),
    signal: AbortSignal.timeout(20_000),
  });
  if (!res.ok) throw new Error(`watch ${channelId} ${res.status} ${(await res.text()).slice(0, 180)}`);
  return res.json();
}

async function stopWatch(base, channelId, rendition) {
  await fetch(new URL(`/api/v1/watch/${channelId}/stop`, base), {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ rendition }),
    signal: AbortSignal.timeout(8_000),
  }).catch(() => {});
}

export async function listRecordings(base) {
  const res = await fetch(new URL("/api/v1/recordings", base), { signal: AbortSignal.timeout(15_000) });
  if (!res.ok) throw new Error(`recordings ${res.status} ${(await res.text()).slice(0, 180)}`);
  return res.json();
}

export async function startRecording(base, channelId, minutes, title) {
  const res = await fetch(new URL("/api/v1/recordings", base), {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ channelId, minutes, title }),
    signal: AbortSignal.timeout(15_000),
  });
  if (!res.ok) throw new Error(`record ${res.status} ${(await res.text()).slice(0, 180)}`);
  return res.json();
}

export async function stopRecording(base, id) {
  const res = await fetch(new URL(`/api/v1/recordings/${id}/stop`, base), {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: "{}",
    signal: AbortSignal.timeout(15_000),
  }).catch(() => null);
  return Boolean(res?.ok);
}
