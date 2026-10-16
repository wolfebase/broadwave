// One Cast playback: the playlist, the sync room, and the corrections from protocol.ts.
// The page owns the WebSocket. This class only decides what to send and what the picture does.

import type { RoomState } from "../lib/events.ts";
import { roomStart, type Frag } from "../lib/roomStart.ts";
import {
  clockMessage,
  freshClock,
  freshSync,
  hereMessage,
  joinMessage,
  leaveMessage,
  mediaURL,
  noteClock,
  readRoom,
  socketURL,
  syncStep,
  targetMedia,
  type CastLoad,
  type Clock,
  type Latency,
  type SyncAction,
  type SyncMemory,
} from "./protocol.ts";
import type { CastHold } from "./watch.ts";

export type CastStatus = "idle" | "buffering" | "playing";

export type CastPlayer = {
  load(url: string, token: string): void;
  /** Null starts at the live edge. */
  startAt(seconds: number | null): void;
  stop(): void;
  fragments(): Frag[];
  /** Program date-time, or null while the element is not ready to show a frame. */
  mediaNow(): number | null;
  timeFor(mediaMs: number): number | null;
  forwardSec(): number;
  paused(): boolean;
  pause(): void;
  play(): void;
  seek(seconds: number): void;
  setRate(rate: number): void;
};

export type CastTransport = {
  send(message: unknown): void;
  close(): void;
};

type Open = {
  pageURL: string;
  channelId: number;
  playlist: string;
  room: string;
  latency: Latency;
  token?: string;
  mediaKey?: string;
  title?: string;
  subtitle?: string;
  player: CastPlayer;
  transport: CastTransport;
  now?: () => number;
  screenId?: string;
  onStatus?: (status: CastStatus) => void;
  /** Counts this television as a viewer. Omitted in tests that only check the picture. */
  watch?: CastHold;
};

export class CastSession {
  readonly pageURL: string;
  channelId: number;
  playlist: string;
  room: string;
  latency: Latency;
  token: string;
  mediaKey: string;
  title: string;
  subtitle: string;
  /** Bumps on a real retune, so a wait timer from the previous load cannot start the next one. */
  generation = 0;
  stopped = false;

  private readonly player: CastPlayer;
  private readonly transport: CastTransport;
  private readonly now: () => number;
  private readonly screenId: string;
  private readonly onStatus?: (status: CastStatus) => void;
  private readonly watch?: CastHold;
  private clock: Clock = freshClock();
  private memory: SyncMemory = freshSync();
  private state: RoomState | null = null;
  private started = false;
  private waited = false;
  /** True once a frame has been read. A paused room plays only until then. */
  private framed = false;
  private label: CastStatus = "buffering";

  constructor(input: Open, media: string) {
    this.pageURL = input.pageURL;
    this.channelId = input.channelId;
    this.playlist = input.playlist;
    this.room = input.room;
    this.latency = input.latency;
    this.token = input.token ?? "";
    this.mediaKey = input.mediaKey ?? "";
    this.title = input.title ?? "";
    this.subtitle = input.subtitle ?? "";
    this.player = input.player;
    this.transport = input.transport;
    this.now = input.now ?? (() => Date.now());
    this.screenId = input.screenId ?? "cast";
    this.onStatus = input.onStatus;
    this.watch = input.watch;
    this.player.load(media, this.token);
    this.watch?.hold(this.holdInput());
  }

  get status(): CastStatus {
    return this.label;
  }

  socketURL(): string {
    return socketURL(this.pageURL, this.token);
  }

  serverNow(): number {
    return this.now() + this.clock.offset;
  }

  /** here, join, and the first clock sample. The socket is already open. */
  onOpen() {
    if (this.stopped) return;
    this.clock = freshClock();
    this.transport.send(hereMessage(this.screenId, this.channelId));
    this.transport.send(joinMessage(this.room, this.channelId, this.latency));
    this.sampleClock();
  }

  sampleClock() {
    if (this.stopped) return;
    this.transport.send(clockMessage(this.now()));
  }

  onMessage(raw: unknown) {
    if (this.stopped || !raw || typeof raw !== "object") return;
    const message = raw as { type?: unknown; data?: unknown };
    if (message.type === "hello") {
      const data = message.data;
      const boot = data && typeof data === "object" ? (data as { boot?: unknown }).boot : "";
      if (typeof boot === "string" && boot) this.watch?.noteBoot(boot);
      return;
    }
    if (message.type === "sync.state") {
      const room = readRoom(message.data);
      if (!room || room.room !== this.room) return;
      this.state = room;
      return;
    }
    if (message.type !== "clock" || !message.data || typeof message.data !== "object") return;
    const data = message.data as { t0?: unknown; t1?: unknown };
    if (typeof data.t0 !== "number" || typeof data.t1 !== "number") return;
    this.clock = noteClock(this.clock, { t0: data.t0, t1: data.t1, t2: this.now() });
  }

  /**
   * Starts once. With a room and fragments, at the room's frame (the edge when
   * the playlist does not hold it). After the wait, at the edge.
   */
  considerStart() {
    if (this.stopped || this.started) return;
    const frags = this.player.fragments();
    const positioned = !!this.state && frags.length > 0;
    if (!positioned && !this.waited) return;
    this.started = true;
    // play() is what raises the picture. A Cast device can leave readyState at
    // 0 until then, including a paused room, and the corrections never run
    // without a frame. The room pauses on a later tick, once a frame exists.
    this.player.startAt(positioned ? roomStart(frags, this.state!, this.serverNow()) : null);
    this.player.play();
  }

  /** The page's 800 ms timer. A stale generation, from a retune, does nothing. */
  markWaitOver(generation: number) {
    if (this.stopped || generation !== this.generation) return;
    this.waited = true;
    this.considerStart();
  }

  tick() {
    if (this.stopped) return;
    this.considerStart();
    if (this.player.mediaNow() != null) this.framed = true;
    if (!this.state) {
      this.playUntilFrame();
      this.emit("buffering");
      return;
    }
    const mediaNow = this.player.mediaNow();
    const target = targetMedia(this.state, this.serverNow());
    const step = syncStep(this.memory, {
      now: this.now(),
      serverNow: this.serverNow(),
      mediaNow,
      room: this.state,
      timeFor: mediaNow == null ? null : this.player.timeFor(target),
      forwardSec: this.player.forwardSec(),
    });
    this.memory = step.memory;
    this.apply(step.action);
    this.emit(this.player.mediaNow() != null ? "playing" : "buffering");
  }

  /**
   * Another load. The same channel, playlist, and room does not reload.
   * True means the socket URL changed and the page should reconnect; join
   * goes out on the new socket's open. Otherwise this sends leave, here, and join.
   */
  retune(next: CastLoad): boolean {
    if (this.stopped) return false;
    const nextToken = next.token ?? "";
    // A fresh link to the same playlist does not reload the picture. The one playing lasts 24 hours.
    if (next.channelId === this.channelId && next.playlist === this.playlist && next.room === this.room && nextToken === this.token) {
      this.title = next.title;
      this.subtitle = next.subtitle;
      return false;
    }
    const reopen = socketURL(this.pageURL, nextToken) !== socketURL(this.pageURL, this.token);
    this.transport.send(leaveMessage(this.room));
    this.channelId = next.channelId;
    this.playlist = next.playlist;
    this.room = next.room;
    this.latency = next.latency;
    this.token = nextToken;
    this.mediaKey = next.mediaKey ?? "";
    this.title = next.title;
    this.subtitle = next.subtitle;
    this.generation += 1;
    this.started = false;
    this.waited = false;
    this.framed = false;
    this.state = null;
    this.memory = freshSync();
    const media = mediaURL(this.pageURL, next.playlist, this.mediaKey);
    if (media) this.player.load(media, nextToken);
    this.watch?.hold(this.holdInput());
    this.emit("buffering");
    if (reopen) return true;
    this.transport.send(hereMessage(this.screenId, this.channelId));
    this.transport.send(joinMessage(this.room, this.channelId, this.latency));
    return false;
  }

  stop() {
    if (this.stopped) return;
    this.stopped = true;
    this.watch?.release();
    this.transport.send(leaveMessage(this.room));
    this.player.stop();
    this.transport.close();
    this.emit("idle");
  }

  private apply(action: SyncAction) {
    const rate = this.state?.rate ?? 1;
    if (action.kind === "wait") {
      // No frame yet: play() may have been refused. A hold is not a wait, so
      // this does not play through that pause. After the first frame a paused
      // room stays paused, including while a seek hides the playhead.
      this.playUntilFrame();
      return;
    }
    if (action.kind === "seek") {
      this.player.seek(action.to);
      if (rate !== 0) this.player.play();
      else this.player.pause();
      return;
    }
    if (action.kind === "hold" || action.kind === "pause") {
      this.player.setRate(rate);
      this.player.pause();
      return;
    }
    if (action.rate !== 0 && this.player.paused()) this.player.play();
    this.player.setRate(action.rate);
  }

  private holdInput() {
    return { channelId: this.channelId, playlist: this.playlist, token: this.token };
  }

  /** Asks for a picture until one frame has been read. Later pauses are left alone. */
  private playUntilFrame() {
    if (!this.started || this.framed) return;
    if (this.player.mediaNow() != null) {
      this.framed = true;
      return;
    }
    if (this.player.paused()) this.player.play();
  }

  private emit(next: CastStatus) {
    if (this.label === next) return;
    this.label = next;
    this.onStatus?.(next);
  }
}

/** A session that can take another load. A stopped one cannot; the page opens another. */
export function castSessionLive<T extends { stopped: boolean }>(session: T | null): session is T {
  return session != null && !session.stopped;
}

/** Null when the playlist is not this server's /media/ path. The player is not touched. */
export function openCastSession(input: Open): CastSession | null {
  const media = mediaURL(input.pageURL, input.playlist, input.mediaKey);
  if (!media) return null;
  return new CastSession(input, media);
}
