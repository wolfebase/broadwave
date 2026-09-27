import type Hls from "hls.js";
import { events, type RoomState } from "./events";
import { holeEnd } from "./bufferHole";
import { nextSeekLead } from "./seekLead";
import { SETTLE_MS, newSettle, settleDue } from "./settle";

export type SyncStatus = {
  state: "off" | "waiting" | "syncing" | "locked";
  /** Local minus target, in ms. Positive means this screen is ahead. */
  drift: number;
  members: number;
  room?: RoomState;
};

const TRIM_MS = 20;
const SEEK_MS = 400;
const MAX_TRIM = 0.03;
// Speeding up, or seeking forward, with less media than this underruns the live edge.
const CUSHION_S = 1.5;
// Safari stops the picture for about a quarter second on every rate change. A
// player that loses this much after one is corrected by seeking instead, from a
// smaller drift.
const RATE_STALL_S = 0.12;
const STALL_SEEK_MS = 120;
// WebKit resumes about a tenth of a second late after a pause. Learned per screen.
const MAX_RESUME_LAG_S = 0.5;

type Frag = { start: number; duration: number; programDateTime: number | null };

/**
 * Holds a live <video> on a room's shared timeline. Every rendition stamps
 * segments with the same program date-time, so the target is the same frame on
 * every screen. Position is mapped through the playlist's segments in both
 * directions; small drift is corrected by rate, large drift by seeking.
 */
export class SyncEngine {
  private state: RoomState | null = null;
  private timer = 0;
  private unsubscribe: (() => void) | null = null;
  private status: SyncStatus = { state: "off", drift: 0, members: 0 };
  private lastSeek = 0;
  private holdUntil = 0;
  private seekLead = 0;
  private usedLead = 0;
  private leadCheck = false;
  private rateProbe: { at: number; t: number; rate: number } | null = null;
  private rateStalls = 0;
  private settle = newSettle();
  private resumeLag = 0;
  private resumeAt = 0;
  // Drift (ms) the last resume should land on. A step lands behind on purpose.
  private resumeExpect = 0;
  private stepped = false;
  private lastStallReport = 0;

  constructor(
    private video: HTMLVideoElement,
    private hls: Hls | null,
    private room: string,
    private channelId: number,
    private onStatus: (s: SyncStatus) => void,
  ) {}

  start() {
    const bus = events();
    this.unsubscribe = bus.on("sync.state", (data) => {
      const st = data as RoomState;
      if (st.room !== this.room) return;
      this.state = st;
      this.apply();
    });
    bus.join(this.room, this.channelId);
    const cached = bus.roomState(this.room) as RoomState | undefined;
    if (cached?.room === this.room) this.state = cached;
    this.setStatus({ state: "waiting", drift: 0, members: this.state?.members ?? 0 });
    this.timer = window.setInterval(() => this.apply(), 250);
    this.video.addEventListener("waiting", this.onWaiting);
  }

  /** Out of picture while playing: the room sits too close to this channel's live edge. */
  private onWaiting = () => {
    const video = this.video;
    if (video.seeking || !this.state || this.state.rate === 0 || this.forwardMedia() > 0.5) return;
    // A hole at a timestamp break is not the edge. Step over it now; hls.js
    // would too, but only after the room had stepped every screen back.
    const ranges: [number, number][] = [];
    for (let i = 0; i < video.buffered.length; i++) ranges.push([video.buffered.start(i), video.buffered.end(i)]);
    const past = holeEnd(video.currentTime, ranges);
    if (past != null) {
      video.currentTime = past;
      return;
    }
    const now = performance.now();
    if (now - this.lastStallReport < 3000) return;
    this.lastStallReport = now;
    events().command(this.room, "stalled");
  };

  stop() {
    window.clearInterval(this.timer);
    this.video.removeEventListener("waiting", this.onWaiting);
    this.unsubscribe?.();
    events().leave(this.room);
    this.video.playbackRate = 1;
    this.setStatus({ state: "off", drift: 0, members: 0 });
  }

  command(action: "play" | "pause" | "seek" | "live", mediaTime?: number) {
    events().command(this.room, action, mediaTime ? { mediaTime } : {});
  }

  private frags(): Frag[] {
    const h = this.hls as unknown as { levels?: { details?: { fragments?: Frag[] } }[]; currentLevel?: number; loadLevel?: number } | null;
    if (!h?.levels?.length) return [];
    const level = h.levels[Math.max(0, h.currentLevel ?? h.loadLevel ?? 0)] ?? h.levels[0];
    return level?.details?.fragments ?? [];
  }

  /** Program date-time (Unix ms) of the frame on screen. */
  mediaNow(): number | null {
    const t = this.video.currentTime;
    const frags = this.frags();
    if (frags.length) {
      for (const f of frags) {
        if (f.programDateTime == null || f.duration <= 0 || f.duration > 30) continue;
        if (t >= f.start && t < f.start + f.duration) return f.programDateTime + (t - f.start) * 1000;
      }
      return null;
    }
    const native = (this.video as HTMLVideoElement & { getStartDate?: () => Date }).getStartDate?.();
    if (native && !Number.isNaN(native.getTime())) return native.getTime() + t * 1000;
    return null;
  }

  /** Playhead position for a program date-time, or null when the playlist does not hold it yet. */
  private timeFor(media: number): number | null {
    const frags = this.frags().filter((f) => f.programDateTime != null && f.duration > 0 && f.duration <= 30);
    if (frags.length) {
      for (const f of frags) {
        const s = f.programDateTime as number;
        if (media >= s && media < s + f.duration * 1000) return f.start + (media - s) / 1000;
      }
      return null;
    }
    const native = (this.video as HTMLVideoElement & { getStartDate?: () => Date }).getStartDate?.();
    if (native && !Number.isNaN(native.getTime())) return (media - native.getTime()) / 1000;
    return null;
  }

  /** Seconds of media buffered past the playhead. Zero when the playhead is already past it. */
  private forwardMedia(): number {
    const t = this.video.currentTime;
    const ranges = this.video.buffered;
    let ahead = 0;
    for (let i = 0; i < ranges.length; i++) {
      const end = ranges.end(i);
      if (t >= ranges.start(i) - 0.05 && t <= end) ahead = Math.max(ahead, end - t);
    }
    return ahead;
  }

  /**
   * Moves this screen onto the target. Ahead: pause for exactly the drift while the
   * buffer keeps filling (backward seeks in a live buffer are fragile). Behind: seek forward.
   */
  private correct(drift: number, target: number) {
    if (drift > 0) {
      const now = performance.now();
      if (now < this.holdUntil) return;
      // The resume costs its own lag, so the pause is that much shorter. A
      // lead under that lag cannot be paused away: step behind with the resume
      // alone, then seek forward. A large lead pauses in full, since landing
      // behind is corrected and staying ahead is not.
      const lag = this.resumeLag * 1000;
      let hold = drift - lag;
      if (hold < TRIM_MS) {
        hold = drift > STALL_SEEK_MS ? drift : 0;
        this.stepped = hold === 0;
      }
      this.resumeExpect = drift - hold - lag;
      this.holdUntil = now + hold;
      this.rateProbe = null;
      this.video.pause();
      window.setTimeout(() => {
        this.holdUntil = 0;
        this.resumeAt = performance.now();
        void this.video.play().catch(() => undefined);
      }, hold);
      return;
    }
    this.seekTo(target, true);
  }

  /** Seeks sparingly: every seek flushes the player, and seeking into data it does not have stalls it. */
  private seekTo(media: number, playing: boolean): boolean {
    const now = performance.now();
    if (now - this.lastSeek < 2000) return false;
    let lead = playing ? this.seekLead : 0;
    let t = lead > 0 ? this.timeFor(media + lead * 1000) : null;
    if (t != null && !this.buffered(t, CUSHION_S / 2)) t = null;
    if (t == null) {
      lead = 0;
      t = this.timeFor(media);
    }
    if (t == null) return false;
    this.lastSeek = now;
    this.usedLead = lead;
    this.leadCheck = playing;
    this.rateProbe = null;
    this.video.currentTime = t;
    return true;
  }

  /** Changes speed and notes the playhead, so the next tick can tell whether the change stalled the picture. */
  private setRate(rate: number) {
    if (this.video.playbackRate === rate) return;
    this.video.playbackRate = rate;
    this.rateProbe = { at: performance.now(), t: this.video.currentTime, rate };
  }

  /** Whether t and the next margin seconds are in one buffered range. */
  private buffered(t: number, margin: number): boolean {
    const ranges = this.video.buffered;
    for (let i = 0; i < ranges.length; i++) if (t >= ranges.start(i) && t + margin <= ranges.end(i)) return true;
    return false;
  }

  private setStatus(s: SyncStatus) {
    this.status = s;
    this.onStatus(s);
  }

  private apply() {
    const st = this.state;
    if (!st) return;
    const video = this.video;
    if (performance.now() < this.holdUntil) return;
    const local = this.mediaNow();
    if (local == null || video.readyState < 2 || video.seeking) {
      this.setStatus({ ...this.status, state: "waiting", members: st.members, room: st });
      return;
    }
    const target = st.rate === 0 ? st.anchorMedia : st.anchorMedia + (events().serverNow() - st.anchorServer) * st.rate;
    const drift = local - target;
    video.dataset.syncOffset = String(Math.round(local - Date.now()));
    video.dataset.syncDrift = String(Math.round(drift));
    if (st.rate === 0) {
      this.rateProbe = null;
      if (!video.paused) video.pause();
      if (Math.abs(drift) > TRIM_MS * 2) this.seekTo(target, false);
      this.setStatus({ state: "locked", drift, members: st.members, room: st });
      return;
    }
    if (video.paused) void video.play().catch(() => undefined);
    const now = performance.now();
    // A resume has not shown its lag yet.
    if (this.resumeAt) {
      if (now - this.resumeAt < 1000) {
        this.setStatus({ state: "syncing", drift, members: st.members, room: st });
        return;
      }
      this.resumeAt = 0;
      this.resumeLag = Math.min(MAX_RESUME_LAG_S, nextSeekLead(this.resumeLag, drift - this.resumeExpect));
      if (this.stepped) {
        this.stepped = false;
        const ahead = this.forwardMedia();
        if (drift < -SETTLE_MS && this.timeFor(target) != null && ahead >= CUSHION_S) {
          video.dataset.syncFix = `step ${Math.round(drift)} lag ${Math.round(this.resumeLag * 1000)} lead ${Math.round(this.seekLead * 1000)}`;
          this.correct(drift, target);
          this.setStatus({ state: "syncing", drift, members: st.members, room: st });
          return;
        }
      }
    }
    // A seek has not paid its cost yet; correcting now would chase the landing.
    if (this.leadCheck) {
      if (now - this.lastSeek < 1000) {
        this.setStatus({ state: "syncing", drift, members: st.members, room: st });
        return;
      }
      this.leadCheck = false;
      this.seekLead = nextSeekLead(this.usedLead, drift);
    }
    const probe = this.rateProbe;
    if (probe && now - probe.at >= 200) {
      this.rateProbe = null;
      // Only a clean window counts: playing, with data, and short enough that
      // an underrun or a background tab cannot pass for a rate change.
      if (!video.paused && video.readyState >= 3 && now - probe.at <= 500) {
        const lost = ((now - probe.at) / 1000) * probe.rate - (video.currentTime - probe.t);
        if (lost > RATE_STALL_S) this.rateStalls++;
        else if (this.rateStalls < 2) this.rateStalls = 0;
      }
    }
    const trims = this.rateStalls < 2;
    // The room's own speed: 1, or slower while it eases back to its latency.
    // Trimming around it keeps a screen on the target instead of 50 ms off it.
    const base = st.rate;
    const ahead = this.forwardMedia();
    if (Math.abs(drift) > (trims ? SEEK_MS : STALL_SEEK_MS)) {
      this.setRate(base);
      // The target is not buffered, or the cushion is too thin to chase it.
      // Hold the room's rate; a seek past the edge stalls the picture.
      if (drift < 0 && (this.timeFor(target) == null || ahead < CUSHION_S)) {
        this.setStatus({ state: "syncing", drift, members: st.members, room: st });
        return;
      }
      this.correct(drift, target);
      this.setStatus({ state: "syncing", drift, members: st.members, room: st });
      return;
    }
    if (trims && Math.abs(drift) > TRIM_MS) {
      // Steps of 0.5%: every new rate is a rate change, which Safari pays for.
      let rate = base + Math.round(Math.max(-MAX_TRIM, Math.min(MAX_TRIM, -drift / 2000)) / 0.005) * 0.005;
      if (rate > base && ahead < CUSHION_S) rate = base;
      this.setRate(rate);
      this.setStatus({ state: "syncing", drift, members: st.members, room: st });
      return;
    }
    this.setRate(base);
    // No trims: a drift under the seek threshold would stay for good.
    if (!trims && settleDue(this.settle, drift, now)) {
      const thin = drift < 0 && (this.timeFor(target) == null || ahead < CUSHION_S);
      video.dataset.syncFix = `${thin ? "thin" : drift > 0 ? "pause" : "seek"} ${Math.round(drift)} lag ${Math.round(this.resumeLag * 1000)} lead ${Math.round(this.seekLead * 1000)}`;
      if (!thin) {
        this.correct(drift, target);
        this.setStatus({ state: "syncing", drift, members: st.members, room: st });
        return;
      }
    }
    this.setStatus({ state: "locked", drift, members: st.members, room: st });
  }
}
