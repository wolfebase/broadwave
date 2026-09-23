import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { useData } from "../../app/data";
import { navigate } from "../../app/router";
import { airingAt, categoryOf, minutesLeft, progress } from "../../lib/guide";
import type { SyncStatus } from "../../lib/sync";
import { readZoom, saveZoom, type PictureMode, type Zoom } from "../../picture";
import type { Channel } from "../../types";
import { InfoIcon, ListIcon, RecordIcon, SideBySideIcon, SyncIcon } from "../../ui/icons";
import { Progress } from "../../ui/primitives";
import { Stage } from "./Stage";
import { useLiveStream } from "./useLiveStream";

type Quality = "auto" | "original" | "high" | "medium" | "saver";
type Sound = "auto" | "surround" | "stereo";
type Options = { quality: Quality; audio: Sound; sync: boolean; shared: boolean };

function readOptions(): Options {
  try {
    return { quality: "auto", audio: "auto", sync: true, shared: false, ...JSON.parse(localStorage.getItem("ota-live") || "{}") };
  } catch {
    return { quality: "auto", audio: "auto", sync: true, shared: false };
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
  const videoRef = useRef<HTMLVideoElement>(null);
  const rootRef = useRef<HTMLElement>(null);
  const [opts, setOpts] = useState<Options>(readOptions);
  const [picture, setPicture] = useState<PictureMode>(settings.pictureMode || "broadcast");
  const room = opts.shared ? `group:ch${channel.id}` : `channel:${channel.id}`;
  const stream = useLiveStream(videoRef, {
    channelId: channel.id,
    quality: opts.quality,
    audio: opts.audio,
    picture,
    room,
    sync: opts.sync,
    small: false,
    audible: true,
    remember: channel,
  });
  const session = stream.session;
  const error = stream.error;
  const sync: SyncStatus = stream.syncStatus;
  const [behind, setBehind] = useState(0);
  const [span, setSpan] = useState({ at: 0, len: 1 });
  const [panel, setPanel] = useState<"none" | "guide" | "info" | "sync">("none");
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
      m: () => navigate(`/multiview?ch=${channel.id}&layout=${localStorage.getItem("waveguide-mv-layout") || "2up"}&focus=${channel.id}&add=1`),
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
            onClick={() => navigate(`/multiview?ch=${channel.id}&layout=${localStorage.getItem("waveguide-mv-layout") || "2up"}&focus=${channel.id}&add=1`)}
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
              <dt>Picture</dt>
              <dd>{session.stream.video === "copy" ? `Original ${session.stream.sourceVideo ?? ""}` : `${session.stream.video}p ${session.stream.mode ?? ""} from ${session.stream.sourceVideo ?? "broadcast"}`}</dd>
              <dt>Sound</dt>
              <dd>{session.stream.audio === "copy" ? `Original ${session.stream.sourceAudio ?? ""}` : session.stream.audio === "aac6" ? "5.1 AAC" : "Stereo AAC"}</dd>
              {session.stream.encoder ? (
                <>
                  <dt>Encoder</dt>
                  <dd>{session.stream.encoder}</dd>
                </>
              ) : null}
              <dt>Tuner</dt>
              <dd>{session.shared ? `Shared · ${session.viewers} watching` : "This screen only"}</dd>
              <dt>Behind live</dt>
              <dd>{formatBehind(behind)}</dd>
              <dt>Sync</dt>
              <dd>{sync.state === "off" ? "Off" : `${sync.state} · ${Math.round(sync.drift)} ms`}</dd>
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

function formatBehind(seconds: number) {
  const s = Math.round(seconds);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  return `${m}m ${s % 60}s`;
}
