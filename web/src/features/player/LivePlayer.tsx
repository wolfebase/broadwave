import { useEffect, useMemo, useRef, useState, type KeyboardEvent, type RefObject } from "react";
import { useData } from "../../app/data";
import { useLayout } from "../../app/layout";
import { navigate } from "../../app/router";
import { airingAt, categoryOf, minutesLeft, progress } from "../../lib/guide";
import type { SyncStatus } from "../../lib/sync";
import { readZoom, saveZoom, type PictureMode, type Zoom } from "../../picture";
import type { Channel } from "../../types";
import { InfoIcon, ListIcon, RecordIcon, SideBySideIcon, SyncIcon } from "../../ui/icons";
import { Progress } from "../../ui/primitives";
import { useScoreMap } from "../sports/scores";
import { Stage } from "./Stage";
import { useLiveStream } from "./useLiveStream";

type Quality = "auto" | "original" | "high" | "medium" | "saver";
type Sound = "auto" | "surround" | "stereo";
type Track = "main" | "language" | "described";
type Options = { quality: Quality; audio: Sound; track: Track; even: boolean; sync: boolean; shared: boolean };

function readOptions(): Options {
  try {
    return { quality: "auto", audio: "auto", track: "main", even: false, sync: true, shared: false, ...JSON.parse(localStorage.getItem("ota-live") || "{}") };
  } catch {
    return { quality: "auto", audio: "auto", track: "main", even: false, sync: true, shared: false };
  }
}

const qualityLabels: Record<Options["quality"], string> = { auto: "Auto", original: "Original", high: "High", medium: "Medium", saver: "Data saver" };

export function LivePlayer({
  channel,
  mode,
  onChannel,
  onMinimize,
  onClose,
  onExpand,
}: {
  channel: Channel;
  mode: "full" | "mini";
  onChannel: (channel: Channel) => void;
  onMinimize: () => void;
  onClose: () => void;
  onExpand: () => void;
}) {
  const { channels, index, now, recordings, settings, saveSettings, record, stopRecord } = useData();
  const layout = useLayout();
  const videoRef = useRef<HTMLVideoElement>(null);
  const rootRef = useRef<HTMLElement>(null);
  const [opts, setOpts] = useState<Options>(readOptions);
  const [picture, setPicture] = useState<PictureMode>(settings.pictureMode || "broadcast");
  const room = opts.shared ? `group:ch${channel.id}` : `channel:${channel.id}`;
  const stream = useLiveStream(videoRef, {
    channelId: channel.id,
    quality: opts.quality,
    audio: opts.audio,
    track: opts.track,
    even: opts.even,
    picture,
    room,
    sync: opts.sync,
    profile: mode === "mini" ? "tile" : layout,
    audible: true,
    remember: channel,
  });
  const session = stream.session;
  const error = stream.error;
  const sync: SyncStatus = stream.syncStatus;
  const [panel, setPanel] = useState<"none" | "guide" | "info" | "sync">("none");
  const playback = usePlaybackStats(videoRef, panel === "info");
  const [behind, setBehind] = useState(0);
  const [span, setSpan] = useState({ at: 0, len: 1 });
  const matchedRow = Math.max(0, channels.findIndex((c) => c.id === channel.id));
  const [guideRow, setGuideRow] = useState(matchedRow);
  const [rowFor, setRowFor] = useState(`${channel.id}:${channels.map((c) => c.id).join(",")}`);
  const rowKey = `${channel.id}:${channels.map((c) => c.id).join(",")}`;
  if (rowFor !== rowKey) {
    setRowFor(rowKey);
    setGuideRow(matchedRow);
  }
  const [zoom, setZoom] = useState<Zoom>(readZoom);
  const [sleepUntil, setSleepUntil] = useState<number | null>(null);
  const typed = useRef("");
  const typedTimer = useRef(0);

  const airing = airingAt(index, channel.id, now);
  const scores = useScoreMap();
  const active = recordings.find((r) => r.status === "recording" && r.channelId === channel.id);

  useEffect(() => localStorage.setItem("ota-live", JSON.stringify(opts)), [opts]);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    const tick = () => {
      const end = video.seekable.length ? video.seekable.end(video.seekable.length - 1) : video.currentTime;
      const start = video.seekable.length ? video.seekable.start(0) : 0;
      setBehind(Math.max(0, end - video.currentTime));
      setSpan({ at: Math.max(0, video.currentTime - start), len: Math.max(1, end - start) });
    };
    video.addEventListener("timeupdate", tick);
    return () => video.removeEventListener("timeupdate", tick);
  }, [channel.id]);

  useEffect(() => {
    if (!sleepUntil) return;
    const id = window.setInterval(() => {
      if (Date.now() >= sleepUntil) onClose();
    }, 1000);
    return () => window.clearInterval(id);
  }, [sleepUntil, onClose]);

  useEffect(() => {
    if (mode === "full") rootRef.current?.focus();
  }, [channel.id, mode]);

  function detachSync() {
    if (opts.sync && !opts.shared) setOpts((o) => ({ ...o, sync: false }));
  }

  function jump(delta: number) {
    const video = videoRef.current;
    if (!video) return;
    if (opts.sync && opts.shared) {
      const media = stream.mediaNow();
      if (media) stream.command("seek", media + delta * 1000);
      return;
    }
    detachSync();
    video.currentTime = Math.max(0, video.currentTime + delta);
  }

  function togglePlay() {
    const video = videoRef.current;
    if (!video) return;
    if (opts.sync && opts.shared) {
      stream.command(video.paused ? "play" : "pause");
      return;
    }
    if (!video.paused) detachSync();
    if (video.paused) void video.play();
    else video.pause();
  }

  function goLive() {
    const video = videoRef.current;
    if (!video) return;
    if (opts.sync && opts.shared) return stream.command("live");
    if (!opts.sync) return setOpts((o) => ({ ...o, sync: true }));
    if (video.seekable.length) video.currentTime = video.seekable.end(video.seekable.length - 1) - 10;
    void video.play();
  }

  function step(dir: number) {
    const i = channels.findIndex((c) => c.id === channel.id);
    const next = channels[(i + dir + channels.length) % channels.length];
    if (next) onChannel(next);
  }

  async function toggleRecord() {
    if (active) await stopRecord(active.id);
    else await record(channel, airing?.title || channel.displayName);
  }

  function onKey(event: KeyboardEvent) {
    const k = event.key;
    if (/^[0-9.]$/.test(k)) {
      event.preventDefault();
      typed.current += k;
      window.clearTimeout(typedTimer.current);
      typedTimer.current = window.setTimeout(() => (typed.current = ""), 1200);
      const hit = channels.find((c) => c.displayNumber === typed.current) ?? channels.find((c) => c.displayNumber.startsWith(typed.current));
      if (hit && hit.id !== channel.id) onChannel(hit);
      return;
    }
    if (panel === "guide") {
      if (k === "Escape" || k === "g") setPanel("none");
      else if (k === "ArrowDown") setGuideRow((r) => Math.min(channels.length - 1, r + 1));
      else if (k === "ArrowUp") setGuideRow((r) => Math.max(0, r - 1));
      else if (k === "Enter" && channels[guideRow]) {
        setPanel("none");
        onChannel(channels[guideRow]);
      } else return;
      event.preventDefault();
      return;
    }
    const actions: Record<string, () => void> = {
      Escape: () => (panel !== "none" ? setPanel("none") : onMinimize()),
      " ": togglePlay,
      ArrowLeft: () => jump(-15),
      ArrowRight: () => jump(30),
      ArrowUp: () => step(-1),
      ArrowDown: () => step(1),
      g: () => setPanel("guide"),
      m: () => navigate(`/multiview?ch=${channel.id}&layout=${localStorage.getItem("broadwave-mv-layout") || "2up"}&focus=${channel.id}&add=1`),
      i: () => setPanel((p) => (p === "info" ? "none" : "info")),
      r: () => void toggleRecord(),
      l: goLive,
    };
    const fn = actions[k] ?? actions[k.toLowerCase()];
    if (fn) {
      event.preventDefault();
      fn();
    }
  }

  const liveLabel = opts.sync && sync.state !== "off" ? "Live" : behind < 14 ? "Live" : `${formatBehind(behind)} behind`;
  const title = airing?.title || channel.displayName;
  const eyebrow = useMemo(
    () => (
      <>
        <span className="eyebrow-num">{channel.displayNumber}</span> {channel.displayName}
        {airing ? <span className="eyebrow-dim"> · {minutesLeft(airing, now)}</span> : null}
      </>
    ),
    [channel, airing, now],
  );

  const syncBadge =
    opts.sync && sync.state !== "off" ? (
      <button type="button" className={`sync-pill ${sync.state}`} onClick={() => setPanel((p) => (p === "sync" ? "none" : "sync"))} aria-label="Whole-Home Sync">
        <SyncIcon />
        {sync.members > 1 ? `${sync.members} screens` : "Synced"}
      </button>
    ) : null;

  return (
    <Stage
      videoRef={videoRef}
      rootRef={rootRef}
      mode={mode}
      onKeyDown={onKey}
      videoClass={zoom === "fit" ? "stage-video" : `stage-video ${zoom}`}
      eyebrow={eyebrow}
      title={title}
      score={airing?.gameId ? scores.get(airing.gameId) : undefined}
      onBack={onMinimize}
      backLabel="Back to browsing"
      onExpand={onExpand}
      onClose={onClose}
      liveLabel={liveLabel}
      onLive={goLive}
      position={span.at}
      duration={span.len}
      onSeek={(value) => {
        const video = videoRef.current;
        if (!video || !video.seekable.length) return;
        detachSync();
        video.currentTime = video.seekable.start(0) + value;
      }}
      onJump={jump}
      onTogglePlay={togglePlay}
      error={error}
      badge={syncBadge}
      tools={
        <>
          <button type="button" className={panel === "guide" ? "glass-icon on" : "glass-icon"} onClick={() => setPanel((p) => (p === "guide" ? "none" : "guide"))} aria-label="Channels">
            <ListIcon />
          </button>
          <button
            type="button"
            className="glass-icon"
            aria-label="Side by side"
            onClick={() => navigate(`/multiview?ch=${channel.id}&layout=${localStorage.getItem("broadwave-mv-layout") || "2up"}&focus=${channel.id}&add=1`)}
          >
            <SideBySideIcon />
          </button>
          <button type="button" className={active ? "record-btn on" : "record-btn"} onClick={() => void toggleRecord()} aria-pressed={!!active}>
            <RecordIcon />
            {active ? "Recording" : "Record"}
          </button>
          <button type="button" className={panel === "info" ? "glass-icon on" : "glass-icon"} onClick={() => setPanel((p) => (p === "info" ? "none" : "info"))} aria-label="Stream info">
            <InfoIcon />
          </button>
        </>
      }
      more={
        <div className="options-grid">
          <OptionRow label="Quality" value={opts.quality} options={Object.keys(qualityLabels) as Options["quality"][]} labels={qualityLabels} onChange={(quality) => setOpts((o) => ({ ...o, quality }))} />
          <OptionRow label="Sound" value={opts.audio} options={["auto", "surround", "stereo"]} labels={{ auto: "Auto", surround: "Surround", stereo: "Stereo" }} onChange={(audio) => setOpts((o) => ({ ...o, audio }))} />
          <OptionRow label="Audio" value={opts.track} options={["main", "language", "described"]} labels={{ main: "Main", language: "Second language", described: "Described video" }} onChange={(track) => setOpts((o) => ({ ...o, track }))} />
          <OptionRow label="Even volume" value={opts.even ? "on" : "off"} options={["off", "on"]} labels={{ off: "Off", on: "On" }} onChange={(v) => setOpts((o) => ({ ...o, even: v === "on" }))} />
          <OptionRow
            label="Motion"
            value={picture}
            options={["broadcast", "smooth", "film"]}
            labels={{ broadcast: "Broadcast 60", smooth: "Smooth", film: "Film 24" }}
            onChange={(p) => {
              setPicture(p);
              void saveSettings({ pictureMode: p });
            }}
          />
          <OptionRow
            label="Picture"
            value={zoom}
            options={["fit", "fill", "zoom"]}
            labels={{ fit: "Fit", fill: "Fill", zoom: "Zoom" }}
            onChange={(z) => {
              setZoom(z);
              saveZoom(z);
            }}
          />
          <OptionRow
            label="Sleep"
            value={sleepUntil ? "on" : "off"}
            options={["off", "30", "60", "90"]}
            labels={{ off: "Off", "30": "30 min", "60": "1 hr", "90": "90 min", on: "On" }}
            onChange={(v) => setSleepUntil(v === "off" ? null : Date.now() + Number(v) * 60_000)}
          />
        </div>
      }
    >
      {panel === "guide" ? (
        <div className="mini-guide glass" role="listbox" aria-label="Channels">
          {channels.map((c, i) => {
            const a = airingAt(index, c.id, now);
            return (
              <button
                key={c.id}
                type="button"
                role="option"
                aria-selected={i === guideRow}
                className={c.id === channel.id ? "mg-row current" : i === guideRow ? "mg-row on" : "mg-row"}
                onMouseEnter={() => setGuideRow(i)}
                onClick={() => {
                  setPanel("none");
                  onChannel(c);
                }}
              >
                <span className="mg-num">{c.displayNumber}</span>
                <span className="mg-body">
                  <span className="mg-name">{c.displayName}</span>
                  <span className="mg-title">{a?.title ?? "No listing"}</span>
                  <Progress value={progress(a, now)} category={categoryOf(a)} />
                </span>
              </button>
            );
          })}
        </div>
      ) : null}
      {panel === "info" ? (
        <div className="info-panel glass" role="dialog" aria-label="Stream info">
          <h3>Stream info</h3>
          {session ? (
            <dl>
              <dt>Playing</dt>
              <dd>{session.stream.reason}</dd>
              <dt>Source</dt>
              <dd>{sourceLine(session.stream)}</dd>
              <dt>Output</dt>
              <dd>{outputLine(session.stream, playback)}</dd>
              <dt>Dropped frames</dt>
              <dd>{playback.total > 0 ? `${playback.dropped} of ${playback.total}` : "0"}</dd>
              <dt>Buffer</dt>
              <dd>{`${playback.buffer.toFixed(1)}s`}</dd>
              <dt>Sound</dt>
              <dd>{session.stream.audio === "copy" ? `Original ${session.stream.sourceAudio ?? ""}` : session.stream.audio === "aac6" ? "5.1 AAC" : "Stereo AAC"}</dd>
              <dt>Tuner</dt>
              <dd>{session.shared ? `Shared · ${session.viewers} watching` : "This screen only"}</dd>
              <dt>Behind live</dt>
              <dd>{formatBehind(behind)}</dd>
              <dt>Sync</dt>
              <dd>{syncLine(sync)}</dd>
            </dl>
          ) : (
            <p>Tuning…</p>
          )}
        </div>
      ) : null}
      {panel === "sync" ? (
        <div className="info-panel glass" role="dialog" aria-label="Whole-Home Sync">
          <h3>Whole-Home Sync</h3>
          <p className="dim">Every screen on this channel shows the same moment{sync.members > 1 ? ` — ${sync.members} screens right now` : ""}.</p>
          <label className="switch-row">
            <input type="checkbox" checked={opts.sync} onChange={(e) => setOpts((o) => ({ ...o, sync: e.target.checked }))} />
            <span>Sync with other screens</span>
          </label>
          <label className="switch-row">
            <input type="checkbox" checked={opts.shared} onChange={(e) => setOpts((o) => ({ ...o, shared: e.target.checked, sync: true }))} />
            <span>Shared controls: pause and rewind for everyone</span>
          </label>
        </div>
      ) : null}
    </Stage>
  );
}

function OptionRow<T extends string>({ label, value, options, labels, onChange }: { label: string; value: T | string; options: T[]; labels: Record<string, string>; onChange: (v: T) => void }) {
  return (
    <div className="option-row">
      <span className="option-label">{label}</span>
      <div className="segmented" role="group" aria-label={label}>
        {options.map((o) => (
          <button key={o} type="button" className={value === o ? "seg on" : "seg"} onClick={() => onChange(o)}>
            {labels[o] ?? o}
          </button>
        ))}
      </div>
    </div>
  );
}

type PictureStats = { width: number; height: number; dropped: number; total: number; buffer: number; fps: number };

function usePlaybackStats(videoRef: RefObject<HTMLVideoElement | null>, on: boolean): PictureStats {
  const [stats, setStats] = useState<PictureStats>({ width: 0, height: 0, dropped: 0, total: 0, buffer: 0, fps: 0 });
  const prev = useRef({ frames: 0, at: 0 });
  useEffect(() => {
    if (!on) return;
    const id = window.setInterval(() => {
      const video = videoRef.current;
      if (!video) return;
      const quality = video.getVideoPlaybackQuality?.();
      const dropped = quality?.droppedVideoFrames ?? 0;
      const total = quality?.totalVideoFrames ?? 0;
      const buffer = bufferedAhead(video);
      const now = performance.now();
      let fps = 0;
      if (prev.current.at > 0 && now - prev.current.at > 400 && total >= prev.current.frames) {
        fps = ((total - prev.current.frames) * 1000) / (now - prev.current.at);
      }
      prev.current = { frames: total, at: now };
      setStats({ width: video.videoWidth, height: video.videoHeight, dropped, total, buffer, fps });
    }, 1000);
    return () => window.clearInterval(id);
  }, [videoRef, on]);
  return stats;
}

function sourceLine(stream: { sourceVideo?: string; sourceWidth?: number; sourceHeight?: number; scan?: string; sourceFps?: string }) {
  return joinFacts([stream.sourceVideo, sizeText(stream.sourceWidth, stream.sourceHeight), scanWord(stream.scan), stream.sourceFps]);
}

function outputLine(stream: { encoder?: string; outputWidth?: number; outputHeight?: number; outputFps?: string; bitrate?: string; decode?: string; video?: string }, picture: PictureStats) {
  const width = picture.width || stream.outputWidth;
  const height = picture.height || stream.outputHeight;
  const fps = stream.outputFps || (picture.fps > 1 ? picture.fps.toFixed(2) : "");
  const decode = stream.decode === "gpu" ? "GPU" : stream.decode === "cpu" ? "CPU" : stream.video === "copy" ? "Direct" : "";
  return joinFacts([sizeText(width, height), fps, stream.encoder, bitrateText(stream.bitrate), decode]);
}

function sizeText(width?: number, height?: number) {
  return width && height ? `${width}×${height}` : "";
}

function scanWord(scan?: string) {
  if (scan === "progressive") return "Progressive";
  if (scan === "interlaced") return "Interlaced";
  if (scan === "film") return "Film";
  return "";
}

function bitrateText(rate?: string) {
  if (!rate) return "";
  if (rate.endsWith("M")) return `${rate.slice(0, -1)} Mb/s`;
  if (rate.endsWith("k")) return `${rate.slice(0, -1)} kb/s`;
  return rate;
}

function joinFacts(parts: Array<string | undefined>) {
  const line = parts.filter(Boolean).join(" · ");
  return line || "Waiting";
}

function bufferedAhead(video: HTMLVideoElement) {
  const t = video.currentTime;
  let ahead = 0;
  for (let i = 0; i < video.buffered.length; i++) {
    const start = video.buffered.start(i);
    const end = video.buffered.end(i);
    if (start <= t + 0.05 && end >= t) {
      ahead = Math.max(ahead, end - t);
    }
  }
  return Math.max(0, ahead);
}

function syncLine(sync: SyncStatus) {
  if (sync.state === "off") return "Off";
  const word = sync.state.charAt(0).toUpperCase() + sync.state.slice(1);
  return `${word} · ${Math.round(sync.drift)} ms`;
}

function formatBehind(seconds: number) {
  const s = Math.round(seconds);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  return `${m}m ${s % 60}s`;
}
