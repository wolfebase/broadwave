import { useEffect, useState, type KeyboardEvent, type ReactNode, type RefObject } from "react";

export function Stage({
  videoRef,
  rootRef,
  videoClass,
  title,
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
  tools,
  more,
  children,
  onKeyDown,
}: {
  videoRef: RefObject<HTMLVideoElement | null>;
  rootRef?: RefObject<HTMLElement | null>;
  videoClass: string;
  title: string;
  eyebrow: string;
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
  tools?: ReactNode;
  more?: ReactNode;
  children?: ReactNode;
  onKeyDown?: (event: KeyboardEvent) => void;
}) {
  const [paused, setPaused] = useState(false);
  const [idle, setIdle] = useState(false);
  const [open, setOpen] = useState(false);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    const sync = () => setPaused(video.paused);
    video.addEventListener("play", sync);
    video.addEventListener("pause", sync);
    return () => {
      video.removeEventListener("play", sync);
      video.removeEventListener("pause", sync);
    };
  }, [videoRef]);

  useEffect(() => {
    if (paused || open) {
      setIdle(false);
      return;
    }
    let timer = window.setTimeout(() => setIdle(true), 2800);
    const poke = () => {
      setIdle(false);
      window.clearTimeout(timer);
      timer = window.setTimeout(() => setIdle(true), 2800);
    };
    window.addEventListener("mousemove", poke);
    window.addEventListener("keydown", poke);
    return () => {
      window.clearTimeout(timer);
      window.removeEventListener("mousemove", poke);
      window.removeEventListener("keydown", poke);
    };
  }, [paused, open]);

  function toggle() {
    const video = videoRef.current;
    if (!video) return;
    if (video.paused) void video.play();
    else video.pause();
  }

  const span = Math.max(duration, 0.1);
  const at = Math.min(Math.max(position, 0), span);

  return (
    <section ref={rootRef} className={idle ? "stage idle" : "stage"} tabIndex={0} onKeyDown={onKeyDown}>
      <video ref={videoRef} className={videoClass} autoPlay playsInline onClick={toggle} />
      {paused ? (
        <button type="button" className="stage-fab" onClick={toggle} aria-label="Play">
          <PlayIcon />
        </button>
      ) : null}
      <div className="stage-hud">
        <header className="stage-top">
          <button type="button" className="icon-btn" onClick={onBack} aria-label={backLabel}>
            <BackIcon />
          </button>
          <div className="stage-title">
            <p className="kicker">{eyebrow}</p>
            <h2>{title}</h2>
          </div>
          {liveLabel ? (
            <button type="button" className={liveLabel === "Live" ? "live-pill on" : "live-pill"} onClick={onLive}>
              {liveLabel}
            </button>
          ) : null}
        </header>
        {children}
        {error ? <p className="player-error">{error}</p> : null}
        <footer className="stage-dock">
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
              step={0.1}
              value={at}
              aria-label="Playback position"
              onChange={(event) => onSeek(Number(event.target.value))}
            />
          </div>
          <div className="transport">
            <button type="button" className="icon-btn" onClick={toggle} aria-label={paused ? "Play" : "Pause"}>
              {paused ? <PlayIcon /> : <PauseIcon />}
            </button>
            <button type="button" className="text-btn" onClick={() => onJump(-15)}>
              −15
            </button>
            <button type="button" className="text-btn" onClick={() => onJump(30)}>
              +30
            </button>
            <span className="time-read">
              {formatClock(at)}
              <span> / {formatClock(span)}</span>
            </span>
            <span className="transport-gap" />
            {tools}
            <button type="button" className={open ? "text-btn on" : "text-btn"} onClick={() => setOpen((value) => !value)} aria-expanded={open}>
              Options
            </button>
          </div>
          {open && more ? <div className="stage-more">{more}</div> : null}
        </footer>
      </div>
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

function PlayIcon() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <path d="M8 5.5v13l11-6.5z" fill="currentColor" />
    </svg>
  );
}

function PauseIcon() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <path d="M7 5h3.2v14H7zm6.8 0H17v14h-3.2z" fill="currentColor" />
    </svg>
  );
}

function BackIcon() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <path d="M14.5 5.5 8 12l6.5 6.5" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}
