import type Hls from "hls.js";

// Live captions ride beside the video playlist instead of through a
// multivariant one: hls.js loads no media playlist from a multivariant URL
// until startLoad, and starting on the room's frame needs that playlist first.

export type VttCue = { start: number; end: number; text: string; settings: string };
export type CaptionSegment = { sn: number; cc: number; url: string; date: number };

const WRAP = 2 ** 33;

function vttTime(value: string): number | null {
  const m = /^(?:(\d+):)?(\d{2}):(\d{2})\.(\d{3})$/.exec(value.trim());
  if (!m) return null;
  return Number(m[1] ?? 0) * 3600 + Number(m[2]) * 60 + Number(m[3]) + Number(m[4]) / 1000;
}

export function unescapeVtt(text: string): string {
  return text.replace(/&(?:amp|lt|gt);/g, (entity) => (entity === "&amp;" ? "&" : entity === "&lt;" ? "<" : ">"));
}

/** One WebVTT segment: the 90 kHz time its local zero maps to, and its cues in local seconds. */
export function parseVtt(body: string): { mpegts: number; cues: VttCue[] } {
  let mpegts = 0;
  let local = 0;
  const cues: VttCue[] = [];
  for (const block of body.replace(/\r\n?/g, "\n").split(/\n{2,}/)) {
    const lines = block.split("\n");
    for (const line of lines) {
      const map = /^X-TIMESTAMP-MAP=(.*)$/.exec(line);
      if (!map) continue;
      for (const part of map[1].split(",")) {
        const [key, value = ""] = part.split(/:(.*)/s);
        if (key === "MPEGTS") mpegts = Number(value) || 0;
        if (key === "LOCAL") local = vttTime(value) ?? 0;
      }
    }
    const at = lines.findIndex((line) => line.includes("-->"));
    if (at < 0) continue;
    const [from, rest = ""] = lines[at].split("-->");
    const [to = "", ...settings] = rest.trim().split(/\s+/);
    const start = vttTime(from);
    const end = vttTime(to);
    const text = unescapeVtt(lines.slice(at + 1).join("\n").trim());
    if (start == null || end == null || end <= start || !text) continue;
    cues.push({ start: start - local, end: end - local, text, settings: settings.join(" ") });
  }
  return { mpegts, cues };
}

/**
 * Seconds on the player's timeline for a 90 kHz time. zero90k is the 90 kHz
 * time at the timeline's zero. Near is where the time is expected, which picks
 * the 33-bit wrap when the player's clock has run past it.
 */
export function mediaTime(mpegts: number, zero90k: number, near?: number): number {
  let ts = mpegts;
  const around = near == null ? zero90k : zero90k + near * 90000;
  while (ts - around > WRAP / 2) ts -= WRAP;
  while (around - ts > WRAP / 2) ts += WRAP;
  return (ts - zero90k) / 90000;
}

/** Applies the cue settings the server writes (line, position, align, size). */
export function applySettings(cue: VTTCue, settings: string) {
  for (const part of settings.split(/\s+/)) {
    const [key, raw = ""] = part.split(":");
    const value = raw.split(",")[0];
    const num = Number.parseFloat(value);
    if (key === "line" && Number.isFinite(num)) {
      cue.snapToLines = !value.endsWith("%");
      cue.line = num;
    } else if (key === "position" && Number.isFinite(num)) cue.position = num;
    else if (key === "size" && Number.isFinite(num)) cue.size = num;
    else if (key === "align" && ["start", "center", "end", "left", "right"].includes(value)) cue.align = value as AlignSetting;
  }
}

/** The captions playlist: each segment's sequence number, break count, URL, and date. */
export function parseCaptionPlaylist(body: string, base: string): { target: number; segments: CaptionSegment[] } {
  let target = 2;
  let sn = 0;
  let cc = 0;
  let date = 0;
  const segments: CaptionSegment[] = [];
  for (const raw of body.split("\n")) {
    const line = raw.trim();
    if (!line) continue;
    if (line.startsWith("#EXT-X-TARGETDURATION:")) target = Number(line.slice(22)) || target;
    else if (line.startsWith("#EXT-X-MEDIA-SEQUENCE:")) sn = Number(line.slice(22)) || 0;
    else if (line.startsWith("#EXT-X-DISCONTINUITY-SEQUENCE:")) cc = Number(line.slice(30)) || 0;
    else if (line === "#EXT-X-DISCONTINUITY") cc++;
    else if (line.startsWith("#EXT-X-PROGRAM-DATE-TIME:")) date = Date.parse(line.slice(25)) || 0;
    else if (!line.startsWith("#")) {
      segments.push({ sn, cc, url: new URL(line, base).toString(), date });
      sn++;
      date = 0;
    }
  }
  return { target, segments };
}

const zeros = new WeakMap<Hls, Map<number, number>>();

/** Records each break's timeline zero in 90 kHz. Call it before loadSource so none is missed. */
export function watchTimeline(hls: Hls, events: typeof Hls.Events) {
  const byBreak = new Map<number, number>();
  zeros.set(hls, byBreak);
  hls.on(events.INIT_PTS_FOUND, (_e, data) => {
    byBreak.set(data.frag.cc, (data.initPTS / data.timescale) * 90000);
  });
}

function trackFor(video: HTMLVideoElement): TextTrack {
  const own = video as HTMLVideoElement & { captionTrack?: TextTrack };
  own.captionTrack ??= video.addTextTrack("captions", "English CC", "en");
  return own.captionTrack;
}

function clear(track: TextTrack) {
  for (const cue of Array.from(track.cues ?? [])) track.removeCue(cue);
}

/** Where hls.js has each video segment, by sequence number. */
function fragmentStarts(hls: Hls): Map<number, number> {
  const out = new Map<number, number>();
  for (const level of hls.levels ?? []) {
    for (const frag of level.details?.fragments ?? []) if (typeof frag.sn === "number") out.set(frag.sn, frag.start);
  }
  return out;
}

// Segments are fetched from a little behind the playhead to a little past the
// buffer, and dropped once well behind, so a rewind fetches them again.
const FETCH_BEHIND_S = 10;
const FETCH_AHEAD_S = 45;
const KEEP_BEHIND_S = 30;
const WAIT_MS = 30_000;

type Placed = { cues: VTTCue[]; end: number };

/** Shows the server's captions for the playing channel until the returned stop runs or hls.js goes. */
export function followCaptions(hls: Hls, events: typeof Hls.Events, video: HTMLVideoElement, url: string): () => void {
  const track = trackFor(video);
  clear(track);
  track.mode = "showing";
  const abort = new AbortController();
  const placed = new Map<string, Placed>();
  const loading = new Set<string>();
  const waiting = new Map<string, { sn: number; cc: number; vtt: ReturnType<typeof parseVtt>; at: number }>();
  let timer = 0;

  const place = (starts: Map<number, number>) => {
    const byBreak = zeros.get(hls);
    const now = performance.now();
    for (const [key, item] of waiting) {
      const zero = byBreak?.get(item.cc);
      if (zero == null) {
        if (now - item.at > WAIT_MS) waiting.delete(key);
        continue;
      }
      const at = mediaTime(item.vtt.mpegts, zero, starts.get(item.sn));
      const cues: VTTCue[] = [];
      let end = at;
      for (const c of item.vtt.cues) {
        const cue = new VTTCue(at + c.start, at + c.end, c.text);
        applySettings(cue, c.settings);
        track.addCue(cue);
        cues.push(cue);
        end = Math.max(end, at + c.end);
      }
      placed.set(key, { cues, end: Math.max(end, starts.get(item.sn + 1) ?? at) });
      video.dataset.captionSegment = `${item.sn} ${at.toFixed(3)}`;
      waiting.delete(key);
    }
    const t = video.currentTime;
    for (const [key, seg] of placed) {
      if (seg.end >= t - KEEP_BEHIND_S) continue;
      for (const cue of seg.cues) track.removeCue(cue);
      placed.delete(key);
    }
  };

  const load = async (key: string, sn: number, cc: number, segUrl: string) => {
    loading.add(key);
    try {
      const res = await fetch(segUrl, { cache: "no-store", signal: abort.signal });
      if (res.ok) waiting.set(key, { sn, cc, vtt: parseVtt(await res.text()), at: performance.now() });
    } finally {
      loading.delete(key);
    }
  };

  const tick = async () => {
    let wait = 2000;
    try {
      const res = await fetch(url, { cache: "no-store", signal: abort.signal });
      if (res.ok) {
        const list = parseCaptionPlaylist(await res.text(), url);
        wait = Math.min(4000, Math.max(1000, (list.target * 1000) / 2));
        const starts = fragmentStarts(hls);
        const t = video.currentTime;
        for (const seg of list.segments) {
          const key = `${seg.cc}:${seg.sn}`;
          const start = starts.get(seg.sn);
          if (start == null || placed.has(key) || waiting.has(key) || loading.has(key)) continue;
          if (start < t - FETCH_BEHIND_S || start > t + FETCH_AHEAD_S) continue;
          await load(key, seg.sn, seg.cc, seg.url).catch(() => undefined);
          if (abort.signal.aborted) return;
        }
        place(starts);
      }
    } catch {
      if (abort.signal.aborted) return;
    }
    if (!abort.signal.aborted) timer = window.setTimeout(() => void tick(), wait);
  };
  void tick();

  const stop = () => {
    if (abort.signal.aborted) return;
    abort.abort();
    window.clearTimeout(timer);
    hls.off(events.DESTROYING, stop);
    clear(track);
    track.mode = "disabled";
    delete video.dataset.captionSegment;
  };
  hls.once(events.DESTROYING, stop);
  return stop;
}
