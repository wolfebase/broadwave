import { useEffect, useRef, useState, type KeyboardEvent, type ReactNode, type RefObject } from "react";
import { BackIcon, CloseIcon, ExpandIcon, PauseIcon, PipIcon, PlayIcon, VolumeIcon } from "../../ui/icons";
import "./player.css";

export function Stage({
  videoRef,
  rootRef,
  videoClass,
  title,
  score,
  eyebrow,
  onBack,
  backLabel,
  liveLabel,
  onLive,
  position,
  duration,
  onSeek,
  markers,
  onJump,
  error,
  errorAction,
  note,
  noteAction,
  tools,
  more,
  children,
  onKeyDown,
  mode = "full",
  onExpand,
  onClose,
  badge,
  onTogglePlay,
  loading,
}: {
  videoRef: RefObject<HTMLVideoElement | null>;
  rootRef?: RefObject<HTMLElement | null>;
  videoClass: string;
  title: string;
  score?: string;
  eyebrow: ReactNode;
  onBack: () => void;
  backLabel: string;
  liveLabel?: string;
  onLive?: () => void;
  position: number;
  duration: number;
  onSeek: (seconds: number) => void;
  markers?: { id: number; start: number; end: number }[];
  onJump: (delta: number) => void;
  error?: string;
  errorAction?: ReactNode;
  note?: string;
  noteAction?: ReactNode;
  tools?: ReactNode;
  more?: ReactNode;
  children?: ReactNode;
  onKeyDown?: (event: KeyboardEvent) => void;
  mode?: "full" | "mini";
  onExpand?: () => void;
  onClose?: () => void;
  badge?: ReactNode;
  onTogglePlay?: () => void;
  /** Shown over the picture until the first frame. The chrome stays up meanwhile. */
  loading?: ReactNode;
}) {
  const [paused, setPaused] = useState(false);
  const [timedIdle, setTimedIdle] = useState(false);
  const [open, setOpen] = useState(false);
  const [muted, setMuted] = useState(false);
  const ownRoot = useRef<HTMLElement>(null);
  const root = rootRef ?? ownRoot;
  // A tap that brings the chrome back. The click that follows must not pause:
  // a paused picture keeps the chrome up.
  const wake = useRef(0);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    const sync = () => {
      setPaused(video.paused);
      setMuted(video.muted);
    };
    video.addEventListener("play", sync);
    video.addEventListener("pause", sync);
    video.addEventListener("volumechange", sync);
    return () => {
      video.removeEventListener("play", sync);
      video.removeEventListener("pause", sync);
      video.removeEventListener("volumechange", sync);
    };
  }, [videoRef]);

  const showChrome = paused || open || mode === "mini" || Boolean(error) || Boolean(loading);
  const idle = !showChrome && timedIdle;
  const [seenShow, setSeenShow] = useState(showChrome);
  if (seenShow !== showChrome) {
    setSeenShow(showChrome);
    if (!showChrome) setTimedIdle(false);
  }

  useEffect(() => {
    if (showChrome) return;
    // The chrome goes inert when it fades, which would drop keyboard focus to
    // the page; hand it back to the stage, which still takes the keys.
    const fade = () => {
      if (root.current?.querySelector(".stage-hud")?.contains(document.activeElement)) root.current.focus();
      setTimedIdle(true);
    };
    let timer = window.setTimeout(fade, 3200);
    const poke = (event: Event) => {
      if (event.type === "touchstart" && idle) wake.current = performance.now();
      // The chrome is opacity 0 while idle, so a Tab would land on a control
      // the viewer cannot see. Show it and move to the first control instead.
      if (event instanceof KeyboardEvent && event.key === "Tab" && idle) {
        event.preventDefault();
        setTimedIdle(false);
        window.requestAnimationFrame(() => {
          root.current?.querySelector<HTMLElement>(".stage-hud button, .stage-hud a[href], .stage-hud input, .stage-hud select, .stage-hud textarea")?.focus();
        });
      }
      setTimedIdle(false);
      window.clearTimeout(timer);
      timer = window.setTimeout(fade, 3200);
    };
    window.addEventListener("mousemove", poke);
    window.addEventListener("keydown", poke);
    window.addEventListener("touchstart", poke);
    return () => {
      window.clearTimeout(timer);
      window.removeEventListener("mousemove", poke);
      window.removeEventListener("keydown", poke);
      window.removeEventListener("touchstart", poke);
    };
  }, [showChrome, idle, root]);

  function toggle() {
    if (onTogglePlay) return onTogglePlay();
    const video = videoRef.current;
    if (!video) return;
    if (video.paused) void video.play();
    else video.pause();
  }

  function fullscreen() {
    const el = root.current;
    if (!el) return;
    if (document.fullscreenElement) void document.exitFullscreen();
    else void el.requestFullscreen?.().catch(() => undefined);
  }

  function pip() {
    const video = videoRef.current;
    if (!video) return;
    if (document.pictureInPictureElement) void document.exitPictureInPicture();
    else void video.requestPictureInPicture?.().catch(() => undefined);
  }

  const span = Math.max(duration, 0.1);
  const at = Math.min(Math.max(position, 0), span);

  return (
    <section
      ref={root}
      className={mode === "mini" ? "stage mini" : idle ? "stage idle" : "stage"}
      tabIndex={mode === "mini" ? -1 : 0}
      onKeyDown={mode === "mini" ? undefined : onKeyDown}
      aria-label={mode === "mini" ? `Now playing: ${title}` : "Player"}
    >
      <video
        ref={videoRef}
        className={videoClass}
        autoPlay
        playsInline
        onClick={() => {
          if (mode === "mini") {
            onExpand?.();
            return;
          }
          if (performance.now() - wake.current < 700) {
            wake.current = 0;
            return;
          }
          toggle();
        }}
        onDoubleClick={fullscreen}
      />
      {loading && !error ? loading : null}
      {mode === "mini" ? (
        <div className="mini-bar">
          <button type="button" className="mini-title" onClick={onExpand} aria-label="Open player">
            <span className="mini-eyebrow">{eyebrow}</span>
            <strong>{title}</strong>
          </button>
          <button type="button" className="glass-icon" onClick={toggle} aria-label={paused ? "Play" : "Pause"}>
            {paused ? <PlayIcon /> : <PauseIcon />}
          </button>
          <button type="button" className="glass-icon" onClick={onClose} aria-label="Stop watching">
            <CloseIcon />
          </button>
        </div>
      ) : null}
      {mode === "full" && paused && !loading ? (
        <button type="button" className="stage-fab" onClick={toggle} aria-label="Play">
          <PlayIcon />
        </button>
      ) : null}
      {mode === "full" ? (
      <div
        className="stage-hud"
        inert={idle ? true : undefined}
      >
        <header className="stage-top">
          <button type="button" className="glass-icon" onClick={onBack} aria-label={backLabel}>
            <BackIcon />
          </button>
          <div className="stage-title">
            <p className="stage-eyebrow">{eyebrow}</p>
            <h2>{title}</h2>
            {score ? <p className="stage-score">{score}</p> : null}
          </div>
          <div className="stage-top-right">
            {badge}
            {liveLabel ? (
              <button
                type="button"
                className={liveLabel === "Live" ? "live-pill on" : "live-pill"}
                aria-label={liveLabel === "Live" ? undefined : `Go to live, ${liveLabel}`}
                onClick={onLive}
              >
                <span className="live-dot-light" aria-hidden="true" />
                {liveLabel}
              </button>
            ) : null}
          </div>
        </header>
        {children}
        {error ? (
          <div className="player-error">
            <p role="alert">{error}</p>
            {errorAction}
          </div>
        ) : null}
        {!error && note ? (
          <div className="player-note">
            <p role="status">{note}</p>
            {noteAction}
          </div>
        ) : null}
        <footer className="stage-dock glass">
          <div className="scrub-wrap">
            <div className="scrub-marks" aria-hidden="true">
              {(markers ?? []).map((marker) => (
                <span
                  key={marker.id}
                  style={{
                    left: `${(marker.start / span) * 100}%`,
                    width: `${Math.max(0.4, ((marker.end - marker.start) / span) * 100)}%`,
                  }}
                />
              ))}
            </div>
            <input
              className="scrub"
              type="range"
              min={0}
              max={span}
              step={1}
              value={at}
              style={{ ["--at" as string]: `${(at / span) * 100}%` }}
              aria-label="Playback position"
              aria-valuetext={`${formatClock(at)} of ${formatClock(span)}`}
              onChange={(event) => onSeek(Number(event.target.value))}
            />
          </div>
          <div className="transport">
            <button type="button" className="transport-play" onClick={toggle} aria-label={paused ? "Play" : "Pause"}>
              {paused ? <PlayIcon /> : <PauseIcon />}
            </button>
            <button type="button" className="text-btn" onClick={() => onJump(-15)} aria-label="Back 15 seconds">
              −15
            </button>
            <button type="button" className="text-btn" onClick={() => onJump(30)} aria-label="Forward 30 seconds">
              +30
            </button>
            <span className="time-read">
              {formatClock(at)}
              <span> / {formatClock(span)}</span>
            </span>
            <span className="transport-gap" />
            {tools}
            <button
              type="button"
              className="glass-icon"
              onClick={() => {
                const v = videoRef.current;
                if (v) v.muted = !v.muted;
              }}
              aria-label={muted ? "Unmute" : "Mute"}
            >
              <VolumeIcon muted={muted} />
            </button>
            {"pictureInPictureEnabled" in document ? (
              <button type="button" className="glass-icon" onClick={pip} aria-label="Picture in picture">
                <PipIcon />
              </button>
            ) : null}
            <button type="button" className="glass-icon" onClick={fullscreen} aria-label="Full screen">
              <ExpandIcon />
            </button>
            {more ? (
              <button type="button" className={open ? "text-btn on" : "text-btn"} onClick={() => setOpen((value) => !value)} aria-expanded={open}>
                Options
              </button>
            ) : null}
          </div>
          {open && more ? <div className="stage-more">{more}</div> : null}
        </footer>
      </div>
      ) : null}
    </section>
  );
}

function formatClock(seconds: number) {
  const total = Math.max(0, Math.floor(seconds));
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  if (h > 0) return `${h}:${m.toString().padStart(2, "0")}:${s.toString().padStart(2, "0")}`;
  return `${m}:${s.toString().padStart(2, "0")}`;
}
