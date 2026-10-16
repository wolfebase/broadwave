import { useEffect, useRef, useState, type KeyboardEvent, type PointerEvent as ReactPointer, type ReactNode, type RefObject } from "react";
import { focusRing } from "../../app/remote";
import { playerUiMessages } from "../../i18n/messages/playerUi.ts";
import { bind } from "../../i18n/runtime.ts";
import { dismissesLayer } from "../../lib/dialogFocus";
import { canonicalKey } from "../../lib/tvKey";
import { optionArrow, optionRows } from "../../lib/optionArrow";
import { BackIcon, CloseIcon, ExpandIcon, PauseIcon, PipIcon, PlayIcon, VolumeIcon } from "../../ui/icons";
import { playerControls, playerTabTarget } from "./focusCycle";
import { shownOutage } from "./outage";
import { tvPictureEnter } from "./keys";
import { swallowWakeClick } from "./startOver";
import { classifyTouch, seeksOnTap, swipeStep, tapZone, touchEcho, type TapZone } from "./gestures";
import "./player.css";

const t = bind(playerUiMessages);

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
  announce,
  onTogglePlay,
  loading,
  onSound,
  hold,
  moreAside,
  onSwipe,
  timeText,
  valueText,
  below,
  flash,
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
  /** A live region that stays available after the chrome fades. */
  announce?: ReactNode;
  onTogglePlay?: () => void;
  /** Shown over the picture until the first frame. The chrome stays up meanwhile. */
  loading?: ReactNode;
  /** The viewer changed volume or mute. Live TV remembers it. */
  onSound?: () => void;
  /** Keeps the chrome and the note up, for a note that asks something. */
  hold?: boolean;
  /** A panel opened from Options covers it. The sheet steps aside and comes back when the panel closes. */
  moreAside?: boolean;
  /** A vertical swipe on the picture: 1 is up (channel up), -1 is down. */
  onSwipe?: (dir: 1 | -1) => void;
  /** Replaces the clock beside the transport, such as live's program time. */
  timeText?: ReactNode;
  /** What the scrubber reads out instead of its clock. */
  valueText?: string;
  /** A phone held upright stacks the picture, the controls, and this under them. */
  below?: ReactNode;
  /** A short note over the picture, such as the channel a swipe moved to. */
  flash?: ReactNode;
}) {
  const [paused, setPaused] = useState(false);
  const [timedIdle, setTimedIdle] = useState(false);
  const [open, setOpen] = useState(false);
  const [muted, setMuted] = useState(false);
  const ownRoot = useRef<HTMLElement>(null);
  const root = rootRef ?? ownRoot;
  const optionsRef = useRef<HTMLButtonElement>(null);
  // A tap that brings the chrome back. The click that follows must not pause
  // or press the control that fades in under the finger.
  const wake = useRef(0);
  const wakeSwallow = useRef<(() => void) | null>(null);
  useEffect(() => () => wakeSwallow.current?.(), []);
  // Touch on the picture: a tap shows or hides the chrome, a double tap on an
  // edge seeks, a swipe changes channel. It never pauses.
  const touch = useRef<{ id: number; x: number; y: number; at: number; idle: boolean } | null>(null);
  const lastTap = useRef<{ at: number; zone: TapZone; idle: boolean } | null>(null);
  const lastTouch = useRef(-1e9);
  const [ripple, setRipple] = useState<{ zone: TapZone; n: number } | null>(null);
  useEffect(() => {
    if (!ripple) return;
    const timer = window.setTimeout(() => setRipple(null), 650);
    return () => window.clearTimeout(timer);
  }, [ripple]);
  const portrait = usePortrait(mode === "full" && below != null);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    const sync = () => {
      setPaused(video.paused);
      setMuted(video.muted);
    };
    sync();
    video.addEventListener("play", sync);
    video.addEventListener("pause", sync);
    video.addEventListener("volumechange", sync);
    return () => {
      video.removeEventListener("play", sync);
      video.removeEventListener("pause", sync);
      video.removeEventListener("volumechange", sync);
    };
  }, [videoRef]);

  // Upright, the controls sit under the picture and never cover it, so they stay.
  const showChrome = paused || open || mode === "mini" || portrait || Boolean(error) || Boolean(loading) || Boolean(hold);
  const idle = !showChrome && timedIdle;
  const [seenShow, setSeenShow] = useState(showChrome);
  if (seenShow !== showChrome) {
    setSeenShow(showChrome);
    if (!showChrome) setTimedIdle(false);
  }

  useEffect(() => {
    if (showChrome) return;
    // The bar goes inert when it fades, which would drop keyboard focus to the
    // page; hand it back to the stage, which still takes the keys. Remote
    // focus fades too, or a TV would keep the bar over the picture.
    const dock = ".stage-top, .stage-dock, .player-note, .stage-fab";
    const fade = () => {
      const active = document.activeElement;
      if (active instanceof Element && active.closest(dock)) root.current?.focus({ preventScroll: true });
      setTimedIdle(true);
    };
    let timer = window.setTimeout(fade, 3200);
    const poke = (event: Event) => {
      // A touch on the picture is a tap or a swipe, handled where it lands.
      if (event.type === "touchstart" && event.target instanceof Element && event.target.closest(".stage-screen")) return;
      // A tap is followed by the mouse events browsers make up for it.
      if (event.type === "mousemove" && touchEcho(lastTouch.current, performance.now())) return;
      if (swallowWakeClick(idle, event.type)) {
        wake.current = performance.now();
        armWakeSwallow(wakeSwallow);
      }
      // The bar is opacity 0 while idle, so a Tab would land on a control the
      // viewer cannot see. Show it and move into it. A guide or help panel that
      // is already focused keeps its own stops.
      const active = document.activeElement;
      const inPanel = active instanceof Element && Boolean(active.closest(".info-panel, .mini-guide, .help-panel"));
      if (event instanceof KeyboardEvent && event.key === "Tab" && idle && !event.defaultPrevented && !inPanel) {
        event.preventDefault();
        const shift = event.shiftKey;
        // The bar drops inert in the render this key starts. Focus once that
        // commit is on the page; one frame later if it has not landed yet.
        const enter = () => {
          const list = playerControls(root.current);
          if (list.length === 0) return false;
          focusRing(shift ? list[list.length - 1] : list[0]);
          return true;
        };
        window.requestAnimationFrame(() => {
          if (!enter()) window.requestAnimationFrame(enter);
        });
      }
      // Tab inside a panel that stayed up keeps that panel. Any other input shows the bar.
      if (!(event instanceof KeyboardEvent && event.key === "Tab" && inPanel)) setTimedIdle(false);
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

  function hideChrome() {
    const active = document.activeElement;
    if (active instanceof Element && active.closest(".stage-top, .stage-dock, .player-note, .stage-fab")) root.current?.focus({ preventScroll: true });
    setTimedIdle(true);
  }

  function onPointerDown(event: ReactPointer) {
    if (event.pointerType !== "touch" || mode !== "full") return;
    touch.current = { id: event.pointerId, x: event.clientX, y: event.clientY, at: performance.now(), idle };
  }

  function onPointerUp(event: ReactPointer) {
    const start = touch.current;
    if (event.pointerType !== "touch" || !start || start.id !== event.pointerId) return;
    touch.current = null;
    const at = performance.now();
    lastTouch.current = at;
    const kind = classifyTouch(event.clientX - start.x, event.clientY - start.y, at - start.at);
    const dir = swipeStep(kind);
    if (dir) {
      lastTap.current = null;
      onSwipe?.(dir);
      return;
    }
    if (kind !== "tap") return;
    const box = event.currentTarget.getBoundingClientRect();
    const zone = tapZone(event.clientX - box.left, box.width);
    if (seeksOnTap(lastTap.current, at, zone)) {
      // The first tap of the pair showed or hid the chrome. Put it back.
      const wasIdle = lastTap.current!.idle;
      lastTap.current = { at, zone, idle: wasIdle };
      if (wasIdle && !showChrome) setTimedIdle(true);
      else if (!wasIdle) setTimedIdle(false);
      onJump(zone === "left" ? -15 : 30);
      setRipple((r) => ({ zone, n: (r?.zone === zone ? r.n : 0) + 1 }));
      return;
    }
    lastTap.current = { at, zone, idle: start.idle };
    if (showChrome) return;
    if (start.idle) setTimedIdle(false);
    else hideChrome();
  }

  const span = Math.max(duration, 0.1);
  const at = Math.min(Math.max(position, 0), span);

  return (
    <section
      ref={root}
      className={mode === "mini" ? "stage mini" : `stage${idle ? " idle" : ""}${portrait ? " portrait" : ""}`}
      tabIndex={mode === "mini" ? -1 : 0}
      onKeyDown={
        mode === "mini"
          ? undefined
          : (event) => {
              // Tab cycles the bar and any open panel. Past the last control it
              // would leave for the navigation hidden under the picture.
              const key = canonicalKey(event);
              if (event.key === "Tab" && !event.altKey && !event.metaKey && !event.ctrlKey) {
                const list = playerControls(root.current);
                const index = list.findIndex((el) => el === document.activeElement);
                const target = playerTabTarget(list.length, index, event.shiftKey);
                if (target != null && list[target]) {
                  event.preventDefault();
                  focusRing(list[target]);
                }
              }
              // Arrows inside the open options change the setting. They do not seek.
              if (open && (key === "ArrowLeft" || key === "ArrowRight" || key === "ArrowUp" || key === "ArrowDown")) {
                const region = root.current?.querySelector("#player-options");
                const current = event.target;
                const slider = current instanceof HTMLInputElement && current.type === "range";
                const rows = region && current instanceof HTMLElement && region.contains(current) && !slider ? optionRows(region) : [];
                const row = rows.findIndex((items) => items.includes(current as HTMLElement));
                // A control outside the rows keeps its own arrows and the remote's.
                if (row >= 0) {
                  event.preventDefault();
                  event.stopPropagation();
                  const col = rows[row].indexOf(current as HTMLElement);
                  const next = optionArrow(rows.map((items) => items.length), row, col, key);
                  if (next) focusRing(rows[next.row]?.[next.col]);
                  return;
                }
              }
              // A TV remote has no Tab, and its arrows on the picture change channel
              // and seek. OK on the picture opens the bar and moves into it at Play.
              const typing = root.current?.querySelector(".typed-number") != null;
              if (tvPictureEnter(document.documentElement.dataset.layout === "tv", key, event.target === event.currentTarget, open || typing)) {
                event.preventDefault();
                event.stopPropagation();
                if (idle) setTimedIdle(false);
                const enter = () => {
                  const list = playerControls(root.current);
                  if (list.length === 0) return false;
                  focusRing(list.find((el) => el.classList.contains("transport-play")) ?? list[0]);
                  return true;
                };
                if (!idle && enter()) return;
                window.requestAnimationFrame(() => {
                  if (!enter()) window.requestAnimationFrame(enter);
                });
                return;
              }
              // Options is a layer over the picture. Close it before leaving the page.
              if (open && dismissesLayer(event.key, event.target, event.keyCode)) {
                const rootEl = root.current;
                const above = rootEl?.querySelector("[role='dialog'], .mini-guide");
                const typing = event.key === "Backspace" && rootEl?.querySelector(".typed-number");
                if (!above && !typing) {
                  event.preventDefault();
                  event.stopPropagation();
                  setOpen(false);
                  focusRing(optionsRef.current);
                  return;
                }
              }
              onKeyDown?.(event);
            }
      }
      aria-label={mode === "mini" ? t("player.nowPlaying", { title }) : t("player.player")}
    >
      <div className="stage-screen" onPointerDown={onPointerDown} onPointerUp={onPointerUp} onPointerCancel={() => (touch.current = null)}>
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
          if (touchEcho(lastTouch.current, performance.now())) return;
          if (performance.now() - wake.current < 700) {
            wake.current = 0;
            return;
          }
          toggle();
        }}
        onDoubleClick={() => {
          if (!touchEcho(lastTouch.current, performance.now())) fullscreen();
        }}
      />
      {loading && !error ? loading : null}
      {mode === "full" && paused && !loading ? (
        <button type="button" className="stage-fab" onClick={toggle} aria-label={t("player.play")}>
          <PlayIcon />
        </button>
      ) : null}
      {ripple ? (
        <span key={`${ripple.zone}${ripple.n}`} className={`seek-ripple ${ripple.zone}`} aria-hidden="true">
          {ripple.zone === "left" ? t("player.back15short") : t("player.forward30short")}
        </span>
      ) : null}
      {flash}
      </div>
      {announce}
      {!error && note && idle ? (
        <span className="sr-only" role="status" aria-live="polite">
          {shownOutage(note)}
        </span>
      ) : null}
      {mode === "mini" ? (
        <div className="mini-bar">
          <button type="button" className="mini-title" onClick={onExpand} aria-label={t("player.openPlayer")}>
            <span className="mini-eyebrow">{eyebrow}</span>
            <strong>{title}</strong>
          </button>
          <button type="button" className="glass-icon" onClick={toggle} aria-label={paused ? t("player.play") : t("player.pause")}>
            {paused ? <PlayIcon /> : <PauseIcon />}
          </button>
          <button type="button" className="glass-icon" onClick={onClose} aria-label={t("player.stopWatching")}>
            <CloseIcon />
          </button>
        </div>
      ) : null}
      {mode === "full" ? (
      <div className="stage-hud">
        <header className="stage-top" inert={idle ? true : undefined}>
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
                className={liveLabel === t("player.live") ? "live-pill on" : "live-pill"}
                aria-label={liveLabel === t("player.live") ? undefined : t("player.goLive", { label: liveLabel })}
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
            <p role="alert">{shownOutage(error)}</p>
            {errorAction}
          </div>
        ) : null}
        {!error && note ? (
          <div className="player-note" inert={idle ? true : undefined}>
            <p role="status">{shownOutage(note)}</p>
            {noteAction}
          </div>
        ) : null}
        <footer className="stage-dock glass" inert={idle ? true : undefined}>
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
              aria-label={t("player.position")}
              aria-valuetext={valueText ?? t("player.positionOf", { at: formatClock(at), span: formatClock(span) })}
              onChange={(event) => onSeek(Number(event.target.value))}
            />
          </div>
          <div className="transport">
            <button type="button" className="transport-play" onClick={toggle} aria-label={paused ? t("player.play") : t("player.pause")}>
              {paused ? <PlayIcon /> : <PauseIcon />}
            </button>
            <button type="button" className="text-btn" onClick={() => onJump(-15)} aria-label={t("player.back15")}>
              {t("player.back15short")}
            </button>
            <button type="button" className="text-btn" onClick={() => onJump(30)} aria-label={t("player.forward30")}>
              {t("player.forward30short")}
            </button>
            {timeText ?? (
              <span className="time-read">
                {formatClock(at)}
                <span>{t("player.timeSep")}{formatClock(span)}</span>
              </span>
            )}
            <span className="transport-gap" />
            {tools}
            <button
              type="button"
              className="glass-icon"
              onClick={() => {
                const v = videoRef.current;
                if (!v) return;
                v.muted = !v.muted;
                setMuted(v.muted);
                onSound?.();
              }}
              aria-label={muted ? t("player.unmute") : t("player.mute")}
            >
              <VolumeIcon muted={muted} />
            </button>
            {"pictureInPictureEnabled" in document ? (
              <button type="button" className="glass-icon" onClick={pip} aria-label={t("player.pip")}>
                <PipIcon />
              </button>
            ) : null}
            <button type="button" className="glass-icon" onClick={fullscreen} aria-label={t("player.fullScreen")}>
              <ExpandIcon />
            </button>
            {more ? (
              <button ref={optionsRef} type="button" className={open ? "text-btn on" : "text-btn"} data-options onClick={() => setOpen((value) => !value)} aria-expanded={open} aria-controls={open && !moreAside ? "player-options" : undefined}>
                {t("player.options")}
              </button>
            ) : null}
          </div>
          {open && more && !moreAside ? (
            <div id="player-options" className="stage-more" role="region" aria-label={t("player.options")}>
              {more}
            </div>
          ) : null}
        </footer>
        {portrait ? <div className="stage-below">{below}</div> : null}
      </div>
      ) : null}
    </section>
  );
}

const upright = typeof window !== "undefined" && typeof window.matchMedia === "function" ? window.matchMedia("(orientation: portrait) and (max-width: 760px)") : null;

/** A phone held upright, while the player has something to show under the picture. */
function usePortrait(wanted: boolean): boolean {
  const [on, setOn] = useState(() => upright?.matches ?? false);
  useEffect(() => {
    if (!upright) return;
    const sync = () => setOn(upright.matches);
    upright.addEventListener("change", sync);
    return () => upright.removeEventListener("change", sync);
  }, []);
  return wanted && on;
}

function formatClock(seconds: number) {
  const total = Math.max(0, Math.floor(seconds));
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  if (h > 0) return `${h}:${m.toString().padStart(2, "0")}:${s.toString().padStart(2, "0")}`;
  return `${m}:${s.toString().padStart(2, "0")}`;
}

/** The click after a wake tap never reaches the control that fades in. */
function armWakeSwallow(slot: { current: (() => void) | null }) {
  slot.current?.();
  let swallowTimer = 0;
  function cancel() {
    window.removeEventListener("click", listener, true);
    window.clearTimeout(swallowTimer);
    if (slot.current === cancel) slot.current = null;
  }
  function listener(click: Event) {
    click.preventDefault();
    click.stopPropagation();
    cancel();
  }
  swallowTimer = window.setTimeout(cancel, 700);
  slot.current = cancel;
  window.addEventListener("click", listener, true);
}
