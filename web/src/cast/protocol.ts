// Decisions the Cast receiver shares with the web player. No DOM, no socket.
// The Chromium path only: a Cast device does not pay Safari's rate-change stall.

import type { RoomState } from "../lib/events.ts";
import type { Frag } from "../lib/roomStart.ts";

export const receiverSDK = "https://www.gstatic.com/cast/sdk/libs/caf_receiver/v3/cast_receiver_framework.js";
export const senderSDK = "https://www.gstatic.com/cv/js/sender/v1/cast_sender.js?loadCastFramework=1";

/** Custom Cast namespace. It has to start with urn:x-cast:. */
export const castNamespace = "urn:x-cast:app.broadwave.watch";

export type Latency = "lowest" | "balanced" | "stable";

export type CastLoad = {
  channelId: number;
  playlist: string;
  room: string;
  latency: Latency;
  token?: string;
  /** The mk of a media link for this playlist. The picture plays without a header with it. */
  mediaKey?: string;
  title: string;
  subtitle: string;
};

const TRIM_START_MS = 25;
const TRIM_STOP_MS = 5;
const SEEK_MS = 400;
const MAX_TRIM = 0.03;
const TRIM_STEP = 0.005;
const CUSHION_S = 1.5;
const GROUP_REWIND_MS = 2000;
/** A paused room seeks when it is farther than this from the anchor. Two trims. */
const PAUSE_SEEK_MS = 40;
const SEEK_GAP_MS = 2000;

export type SyncMemory = { holdUntil: number; trimBy: number; lastSeek: number };

export type SyncAction =
  | { kind: "wait" }
  | { kind: "pause" }
  | { kind: "hold"; ms: number }
  | { kind: "seek"; to: number }
  | { kind: "rate"; rate: number }
  | { kind: "lock"; rate: number };

export type SyncInput = {
  now: number;
  serverNow: number;
  /** Program date-time at the playhead, or null while the picture is not ready. */
  mediaNow: number | null;
  room: RoomState;
  /** Playhead seconds for the target, or null when the playlist does not hold it. */
  timeFor: number | null;
  forwardSec: number;
};

export function freshSync(): SyncMemory {
  return { holdUntil: 0, trimBy: 0, lastSeek: 0 };
}

export function freshClock(): Clock {
  return { offset: 0, bestRtt: Infinity };
}

export type Clock = { offset: number; bestRtt: number };

/** Same offset the web player's socket keeps. A long or negative round trip is dropped. */
export function noteClock(clock: Clock, sample: { t0: number; t1: number; t2: number }): Clock {
  const rtt = sample.t2 - sample.t0;
  if (rtt < 0 || rtt > 10_000) return clock;
  let { offset, bestRtt } = clock;
  if (rtt <= bestRtt * 1.2) {
    bestRtt = Math.min(Math.max(rtt, 1), bestRtt);
    offset = sample.t1 - (sample.t0 + sample.t2) / 2;
  }
  bestRtt *= 1.01;
  return { offset, bestRtt };
}

/** Program date-time the room plays at serverNow. Matches roomTarget. */
export function targetMedia(room: { anchorMedia: number; anchorServer: number; rate: number }, serverNow: number): number {
  return room.rate === 0 ? room.anchorMedia : room.anchorMedia + (serverNow - room.anchorServer) * room.rate;
}

/** Playhead program date-time, or null when no fragment holds currentTime. */
export function mediaTime(frags: Frag[], currentTime: number): number | null {
  for (const frag of frags) {
    if (frag.programDateTime == null || frag.duration <= 0 || frag.duration > 30) continue;
    if (currentTime >= frag.start && currentTime < frag.start + frag.duration) {
      return frag.programDateTime + (currentTime - frag.start) * 1000;
    }
  }
  return null;
}

/**
 * One correction. Ahead of a follow room pauses; it does not seek backward.
 * A group room ahead by more than two seconds seeks back to the rewind.
 */
export function syncStep(memory: SyncMemory, input: SyncInput): { memory: SyncMemory; action: SyncAction } {
  // A missing frame cannot cancel a hold. Playing here would cut the pause short.
  // A paused room drops the hold, the same as the web player.
  if (input.mediaNow == null) {
    if (input.now < memory.holdUntil && input.room.rate !== 0) {
      return { memory, action: { kind: "hold", ms: memory.holdUntil - input.now } };
    }
    const next = input.now < memory.holdUntil ? { ...memory, holdUntil: 0 } : memory;
    return { memory: next, action: { kind: "wait" } };
  }
  const target = targetMedia(input.room, input.serverNow);
  const drift = input.mediaNow - target;
  const groupRewind = drift > GROUP_REWIND_MS && input.room.room.startsWith("group:");
  if (input.now < memory.holdUntil) {
    if (input.room.rate !== 0 && !groupRewind) return { memory, action: { kind: "wait" } };
    memory = { ...memory, holdUntil: 0 };
  }
  const seek = (to: number, next: SyncMemory): { memory: SyncMemory; action: SyncAction } => {
    if (input.now - next.lastSeek < SEEK_GAP_MS) {
      if (input.room.rate === 0) return { memory: next, action: { kind: "pause" } };
      return { memory: next, action: { kind: "lock", rate: input.room.rate } };
    }
    return { memory: { ...next, lastSeek: input.now, trimBy: 0, holdUntil: 0 }, action: { kind: "seek", to } };
  };
  if (input.room.rate === 0) {
    const next = { ...memory, holdUntil: 0, trimBy: 0 };
    if (Math.abs(drift) > PAUSE_SEEK_MS && input.timeFor != null) return seek(input.timeFor, next);
    return { memory: next, action: { kind: "pause" } };
  }
  if (groupRewind && input.timeFor != null) return seek(input.timeFor, memory);
  if (drift > SEEK_MS) {
    return { memory: { ...memory, holdUntil: input.now + drift, trimBy: 0 }, action: { kind: "hold", ms: drift } };
  }
  if (drift < -SEEK_MS) {
    if (input.timeFor == null || input.forwardSec < CUSHION_S) {
      return { memory: { ...memory, trimBy: 0 }, action: { kind: "lock", rate: input.room.rate } };
    }
    return seek(input.timeFor, memory);
  }
  const base = input.room.rate;
  if (memory.trimBy !== 0 || Math.abs(drift) > TRIM_START_MS) {
    const want = Math.round(Math.max(-MAX_TRIM, Math.min(MAX_TRIM, -drift / 2000)) / TRIM_STEP) * TRIM_STEP;
    const crossed = memory.trimBy > 0 ? drift >= -TRIM_STOP_MS : memory.trimBy < 0 && drift <= TRIM_STOP_MS;
    let trimBy = memory.trimBy;
    if (!trimBy || (Math.sign(want) === Math.sign(trimBy) && Math.abs(want) > Math.abs(trimBy))) trimBy = want;
    if (crossed || (trimBy > 0 && input.forwardSec < CUSHION_S)) trimBy = 0;
    if (trimBy) return { memory: { ...memory, trimBy }, action: { kind: "rate", rate: base + trimBy } };
    return { memory: { ...memory, trimBy: 0 }, action: { kind: "lock", rate: base } };
  }
  return { memory: { ...memory, trimBy: 0 }, action: { kind: "lock", rate: base } };
}

/** A playlist path on this server, or null. The receiver will not load anything else. */
export function mediaPath(playlist: string): string | null {
  if (typeof playlist !== "string" || !playlist.startsWith("/media/") || playlist.includes("\\") || playlist.includes("..")) return null;
  let url: URL;
  try {
    url = new URL(playlist, "http://broadwave.local");
  } catch {
    return null;
  }
  if (url.hostname !== "broadwave.local" || url.search || url.hash) return null;
  if (!url.pathname.startsWith("/media/")) return null;
  return url.pathname;
}

/** Absolute playlist URL on the page's own origin. The token is not in it; a media link's mk can be. */
export function mediaURL(page: string, playlist: string, key = ""): string | null {
  const path = mediaPath(playlist);
  if (!path) return null;
  const url = new URL(page);
  url.pathname = path;
  url.search = "";
  url.hash = "";
  if (key) url.searchParams.set("mk", key);
  return url.toString();
}

/**
 * The mk of a media link to this same playlist, or null. The link may name
 * another host; only its mk is kept, so the receiver still plays from its own server.
 */
export function mediaKey(link: string, playlist: string): string | null {
  const path = mediaPath(playlist);
  if (!path || typeof link !== "string" || link.length > 2048) return null;
  let url: URL;
  try {
    url = new URL(link, "http://broadwave.local");
  } catch {
    return null;
  }
  if ((url.protocol !== "http:" && url.protocol !== "https:") || url.hash || url.username || url.password) return null;
  if (url.pathname !== path) return null;
  const keys = [...url.searchParams.keys()];
  if (keys.length !== 1 || keys[0] !== "mk") return null;
  const key = url.searchParams.get("mk") ?? "";
  if (!/^[\x21-\x7e]{1,1024}$/.test(key)) return null;
  return key;
}

/** The same WebSocket the web player opens. The token is the access_token query, when there is one. */
export function socketURL(page: string, token = ""): string {
  const url = new URL(page);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  url.pathname = "/api/v1/ws";
  url.search = "";
  url.hash = "";
  if (token) url.searchParams.set("access_token", token);
  return url.toString();
}

export function hereMessage(id: string, channelId: number) {
  return { type: "here", data: { id, name: "Cast", kind: "web", channelId } };
}

export function joinMessage(room: string, channelId: number, latency: Latency) {
  return { type: "sync.join", data: { room, channelId, latency } };
}

export function leaveMessage(room: string) {
  return { type: "sync.leave", data: { room } };
}

export function clockMessage(t0: number) {
  return { type: "clock", data: { t0 } };
}

/** A sync.state payload, or null when the anchors are missing. */
export function readRoom(data: unknown): RoomState | null {
  if (!data || typeof data !== "object") return null;
  const row = data as Record<string, unknown>;
  if (typeof row.room !== "string") return null;
  if (typeof row.anchorServer !== "number" || typeof row.anchorMedia !== "number" || typeof row.rate !== "number") return null;
  if (!Number.isFinite(row.anchorServer) || !Number.isFinite(row.anchorMedia) || !Number.isFinite(row.rate)) return null;
  const latency: Latency = row.latency === "lowest" || row.latency === "stable" ? row.latency : "balanced";
  return {
    room: row.room,
    channelId: typeof row.channelId === "number" ? row.channelId : 0,
    mode: row.mode === "group" ? "group" : "follow",
    anchorServer: row.anchorServer,
    anchorMedia: row.anchorMedia,
    rate: row.rate,
    latency,
    latencyMs: typeof row.latencyMs === "number" ? row.latencyMs : undefined,
    version: typeof row.version === "number" ? row.version : 0,
    members: typeof row.members === "number" ? row.members : 0,
  };
}

function clip(value: unknown): string {
  if (typeof value !== "string") return "";
  return value.trim().slice(0, 120);
}

/** A load the receiver will play, or null. */
export function parseLoad(data: unknown): CastLoad | null {
  if (!data || typeof data !== "object") return null;
  const row = data as Record<string, unknown>;
  if (row.type !== "load") return null;
  const channelId = row.channelId;
  if (typeof channelId !== "number" || !Number.isInteger(channelId) || channelId <= 0) return null;
  const playlist = mediaPath(typeof row.playlist === "string" ? row.playlist : "");
  if (!playlist) return null;
  const room = row.room;
  if (room !== `channel:${channelId}` && room !== `group:ch${channelId}`) return null;
  let latency: Latency = "balanced";
  if (row.latency !== undefined && row.latency !== "") {
    if (row.latency !== "lowest" && row.latency !== "balanced" && row.latency !== "stable") return null;
    latency = row.latency;
  }
  let token: string | undefined;
  if (row.token !== undefined && row.token !== "") {
    if (typeof row.token !== "string" || row.token.length > 256 || !/^[A-Za-z0-9_-]+$/.test(row.token)) return null;
    token = row.token;
  }
  // A link this receiver cannot read is dropped. The token still plays the channel.
  const key = typeof row.media === "string" && row.media !== "" ? mediaKey(row.media, playlist) : null;
  const load: CastLoad = { channelId, playlist, room, latency, token, title: clip(row.title), subtitle: clip(row.subtitle) };
  if (key) load.mediaKey = key;
  return load;
}

/** The sender's load. An empty token or media link is left out. */
export function castLoad(input: { channelId: number; playlist: string; room: string; latency: Latency; token?: string; media?: string; title?: string; subtitle?: string }) {
  const message: Record<string, unknown> = {
    type: "load",
    channelId: input.channelId,
    playlist: input.playlist,
    room: input.room,
    latency: input.latency,
    title: clip(input.title),
    subtitle: clip(input.subtitle),
  };
  if (input.token) message.token = input.token;
  if (input.media) message.media = input.media;
  return message;
}

/** Cast is in the player only when this server published an id and there is a playlist to send. */
export function castAvailable(appId: string, playlist: string): boolean {
  return appId.trim().length > 0 && mediaPath(playlist) !== null;
}

/**
 * Hidden until there is an id and a playlist. An open session stays visible so
 * it can be stopped while the next channel's playlist is not ready yet.
 */
export function castControl(appId: string, playlist: string, linked: boolean): "hidden" | "cast" | "casting" {
  if (linked) return "casting";
  return castAvailable(appId, playlist) ? "cast" : "hidden";
}

/** A load goes out only for a playlist the receiver will play, and only while a session is open. */
export function castOutgoing(appId: string, playlist: string, linked: boolean): boolean {
  return linked && castAvailable(appId, playlist);
}

/** The line on the television before the picture. The channel is left out when it repeats the title. */
export function castLine(title: string, subtitle: string): string {
  const program = title.trim();
  const channel = subtitle.trim();
  if (program === "") return channel;
  if (channel === "" || channel === program) return program;
  return `${program} · ${channel}`;
}
