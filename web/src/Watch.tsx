import Hls from "hls.js";
import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import { startRecording, stopRecording, stopWatch, watchChannel } from "./api";
import { liveHlsConfig, readZoom, saveZoom, type PictureMode, type Zoom } from "./picture";
import { Stage } from "./Stage";
import { rememberChannel } from "./recent";
import type { Airing, Channel, Recording, Settings, WatchSession } from "./types";

export function Watch({
  channel,
  channels,
  settings,
  recordings,
  airings,
  onChannel,
  onBack,
  onRecorded,
  onPicture,
}: {
  channel: Channel;
  channels: Channel[];
  settings: Settings;
  recordings: Recording[];
  airings: Airing[];
  onChannel: (channel: Channel) => void;
  onBack: () => void;
  onRecorded: () => void;
  onPicture: (mode: PictureMode) => void;
}) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const rootRef = useRef<HTMLElement>(null);
  const backRef = useRef(onBack);
  backRef.current = onBack;
  const [session, setSession] = useState<WatchSession | null>(null);
  const [error, setError] = useState("");
  const [profile, setProfile] = useState(settings.profile || "transparent");
  const [audio, setAudio] = useState(settings.audio || "stereo");
  const [picture, setPicture] = useState<PictureMode>(settings.pictureMode || "broadcast");
  const [clock, setClock] = useState("Live");
  const [bufferAt, setBufferAt] = useState(0);
  const [bufferSpan, setBufferSpan] = useState(0);
  const [guideOpen, setGuideOpen] = useState(false);
  const [guideRow, setGuideRow] = useState(0);
  const [zoom, setZoom] = useState<Zoom>(readZoom);
  const [sleepMinutes, setSleepMinutes] = useState(0);
  const [sleepUntil, setSleepUntil] = useState<number | null>(null);
  const [nowMs, setNowMs] = useState(() => Date.now());
  const typed = useRef("");
  const typedTimer = useRef(0);
  const active = recordings.find((rec) => rec.status === "recording" && rec.channelId === channel.id);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    let dead = false;
    let hls: Hls | null = null;
    const id = channel.id;
    setError("");
    setSession(null);
    void (async () => {
      try {
        const next = await watchChannel(id, profile, audio, picture);
        if (dead) {
          await stopWatch(id);
          return;
        }
        setSession(next);
        if (Hls.isSupported()) {
          hls = new Hls(liveHlsConfig());
          hls.loadSource(next.playlist);
          hls.attachMedia(video);
          hls.on(Hls.Events.ERROR, (_event, data) => {
            if (data.fatal) setError(`${data.type}: ${data.details}`);
          });
        } else {
          video.src = next.playlist;
        }
        await video.play().catch(() => undefined);
      } catch (err) {
        if (!dead) setError(err instanceof Error ? err.message : "The channel did not start.");
      }
    })();
    return () => {
      dead = true;
      hls?.destroy();
      void stopWatch(id);
    };
  }, [channel.id, profile, audio, picture]);

  useEffect(() => {
    const send = () => {
      navigator.sendBeacon?.(`/api/watch/${channel.id}/stop`);
    };
    window.addEventListener("pagehide", send);
    return () => window.removeEventListener("pagehide", send);
  }, [channel.id]);

  useEffect(() => {
    rememberChannel(channel);
    rootRef.current?.focus();
    const index = channels.findIndex((item) => item.id === channel.id);
    setGuideRow(index < 0 ? 0 : index);
  }, [channel, channels]);

  useEffect(() => {
    if (!sleepUntil) return;
    const id = window.setInterval(() => {
      const now = Date.now();
      setNowMs(now);
      if (now >= sleepUntil) backRef.current();
    }, 250);
    return () => window.clearInterval(id);
  }, [sleepUntil]);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    const tick = () => {
      const end = video.seekable.length ? video.seekable.end(video.seekable.length - 1) : video.currentTime;
      const start = video.seekable.length ? video.seekable.start(0) : 0;
      const gap = Math.max(0, end - video.currentTime);
      setBufferAt(Math.max(0, video.currentTime - start));
      setBufferSpan(Math.max(1, end - start));
      setClock(gap < 6 ? "Live" : `${Math.round(gap)}s behind`);
    };
    video.addEventListener("timeupdate", tick);
    return () => video.removeEventListener("timeupdate", tick);
  }, [channel.id]);

  function jump(delta: number) {
    const video = videoRef.current;
    if (!video) return;
    video.currentTime = Math.max(0, video.currentTime + delta);
  }

  function goLive() {
    const video = videoRef.current;
    if (!video || !video.seekable.length) return;
    video.currentTime = video.seekable.end(video.seekable.length - 1) - 1;
    void video.play();
  }

  function step(dir: number) {
    const index = channels.findIndex((item) => item.id === channel.id);
    const next = channels[index + dir];
    if (next) onChannel(next);
  }

  async function toggleRecord() {
    if (active) {
      await stopRecording(active.id);
    } else {
      await startRecording(channel.id, 0, onNow(airings, channel.id) || channel.displayName);
    }
    onRecorded();
  }

  function pick(next: Channel) {
    setGuideOpen(false);
    if (next.id !== channel.id) onChannel(next);
  }

  function jumpNumber(digits: string) {
    const exact = channels.find((item) => item.displayNumber === digits);
    const hit = exact ?? channels.find((item) => item.displayNumber.startsWith(digits));
    if (hit) pick(hit);
  }

  function onKey(event: KeyboardEvent) {
    if (event.key === "g" || event.key === "G") {
      event.preventDefault();
      setGuideOpen((open) => !open);
      return;
    }
    if (/^[0-9.]$/.test(event.key)) {
      event.preventDefault();
      typed.current += event.key;
      window.clearTimeout(typedTimer.current);
      typedTimer.current = window.setTimeout(() => {
        typed.current = "";
      }, 1200);
      jumpNumber(typed.current);
      return;
    }
    if (guideOpen) {
      if (event.key === "Escape") {
        event.preventDefault();
        setGuideOpen(false);
      } else if (event.key === "ArrowDown") {
        event.preventDefault();
        setGuideRow((row) => Math.min(channels.length - 1, row + 1));
      } else if (event.key === "ArrowUp") {
        event.preventDefault();
        setGuideRow((row) => Math.max(0, row - 1));
      } else if (event.key === "Enter") {
        event.preventDefault();
        const next = channels[guideRow];
        if (next) pick(next);
      }
      return;
    }
    if (event.key === "Escape") onBack();
    if (event.key === " ") {
      event.preventDefault();
      const video = videoRef.current;
      if (!video) return;
      if (video.paused) void video.play();
      else video.pause();
    }
    if (event.key === "ArrowLeft") jump(-15);
    if (event.key === "ArrowRight") jump(30);
    if (event.key === "ArrowUp") step(-1);
    if (event.key === "ArrowDown") step(1);
    if (event.key === "r" || event.key === "R") void toggleRecord();
  }

  const sleepLeft = sleepUntil ? Math.max(0, sleepUntil - nowMs) : 0;

  return (
    <Stage
      videoRef={videoRef}
      rootRef={rootRef}
      onKeyDown={onKey}
      videoClass={zoom === "fit" ? "stage-video" : `stage-video ${zoom}`}
      eyebrow={`${channel.displayNumber}${channel.hd ? " · HD" : ""}${session ? ` · ${session.encoder}` : ""}`}
      title={onNow(airings, channel.id) || channel.displayName}
      onBack={onBack}
      backLabel="Guide"
      liveLabel={clock}
      onLive={goLive}
      position={bufferAt}
      duration={bufferSpan}
      onSeek={(value) => {
        const video = videoRef.current;
        if (!video || !video.seekable.length) return;
        video.currentTime = video.seekable.start(0) + value;
      }}
      onJump={jump}
      error={error}
      tools={
        <>
          <button type="button" className={guideOpen ? "text-btn on" : "text-btn"} onClick={() => setGuideOpen((open) => !open)}>
            Channels
          </button>
          <button type="button" className="text-btn" onClick={() => step(-1)} aria-label="Channel down">
            Ch−
          </button>
          <button type="button" className="text-btn" onClick={() => step(1)} aria-label="Channel up">
            Ch+
          </button>
          <button type="button" className={active ? "text-btn on" : "text-btn"} onClick={() => void toggleRecord()}>
            {active ? "Stop" : "Record"}
          </button>
        </>
      }
      more={
        <>
          <div className="segmented" role="group" aria-label="Picture size">
            {(["fit", "fill", "zoom"] as const).map((item) => (
              <button key={item} type="button" className={zoom === item ? "seg on" : "seg"} onClick={() => { setZoom(item); saveZoom(item); }}>
                {item === "fit" ? "Fit" : item === "fill" ? "Fill" : "Zoom"}
              </button>
            ))}
          </div>
          <div className="segmented" role="group" aria-label="Picture motion">
            {(["broadcast", "smooth", "film"] as const).map((item) => (
              <button key={item} type="button" className={picture === item ? "seg on" : "seg"} onClick={() => { setPicture(item); onPicture(item); }}>
                {item === "broadcast" ? "Broadcast" : item === "smooth" ? "Smooth" : "Film"}
              </button>
            ))}
          </div>
          <div className="segmented">
            {(["transparent", "balanced", "saver"] as const).map((item) => (
              <button key={item} type="button" className={profile === item ? "seg on" : "seg"} onClick={() => setProfile(item)}>
                {item === "transparent" ? "Transparent" : item === "balanced" ? "Balanced" : "Saver"}
              </button>
            ))}
          </div>
          <div className="segmented">
            <button type="button" className={audio === "stereo" ? "seg on" : "seg"} onClick={() => setAudio("stereo")}>Stereo</button>
            <button type="button" className={audio === "surround" ? "seg on" : "seg"} onClick={() => setAudio("surround")}>5.1</button>
          </div>
          <div className="segmented" role="group" aria-label="Sleep">
            {([0, 30, 60, 90] as const).map((minutes) => (
              <button
                key={minutes}
                type="button"
                className={sleepMinutes === minutes ? "seg on" : "seg"}
                onClick={() => {
                  setSleepMinutes(minutes);
                  setSleepUntil(minutes === 0 ? null : Date.now() + minutes * 60_000);
                }}
              >
                {minutes === 0 ? "Sleep off" : sleepMinutes === minutes ? formatLeft(sleepLeft) : `${minutes}m`}
              </button>
            ))}
          </div>
          {session ? (
            <ul className="hints">
              {session.hints.map((hint) => (
                <li key={hint}>{hint}</li>
              ))}
            </ul>
          ) : (
            <p className="hint">Tuning the antenna. The first picture takes a couple of seconds.</p>
          )}
        </>
      }
    >
      {guideOpen ? (
        <div className="quick-guide" role="listbox" aria-label="Channels">
          {channels.map((item, index) => (
            <button
              key={item.id}
              type="button"
              role="option"
              aria-selected={index === guideRow}
              className={index === guideRow ? "quick-row on" : "quick-row"}
              onClick={() => pick(item)}
              onMouseEnter={() => setGuideRow(index)}
            >
              <span className="ch-num">{item.displayNumber}</span>
              <span className="ch-name">{item.displayName}</span>
              <span className="ch-tags">{onNow(airings, item.id) || "No listing"}</span>
            </button>
          ))}
        </div>
      ) : null}
    </Stage>
  );
}

function onNow(airings: Airing[], channelId: number) {
  const now = Date.now();
  const hit = airings.find((airing) => airing.channelId === channelId && new Date(airing.start).getTime() <= now && new Date(airing.end).getTime() > now);
  return hit?.title ?? "";
}

function formatLeft(ms: number) {
  const total = Math.max(0, Math.ceil(ms / 1000));
  const minutes = Math.floor(total / 60);
  const seconds = total % 60;
  return `${minutes}:${seconds.toString().padStart(2, "0")}`;
}
