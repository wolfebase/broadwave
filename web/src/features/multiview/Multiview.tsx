import { useCallback, useEffect, useRef, useState, type KeyboardEvent } from "react";
import { planMultiview } from "../../api";
import { useData } from "../../app/data";
import { useLayout } from "../../app/layout";
import { usePlayer } from "../../app/player";
import { focusRing } from "../../app/remote";
import { inAppDepth, navigate, useRoute } from "../../app/router";
import { airingAt } from "../../lib/guide";
import { events } from "../../lib/events";
import type { Channel, MultiviewPlan } from "../../types";
import { CloseIcon, VolumeIcon } from "../../ui/icons";
import { LiveFrame } from "../../ui/LiveFrame";
import { useLiveStream } from "../player/useLiveStream";
import { refreshScores, scoreLine, useScoreMap, type ScoreGame } from "../sports/scores";
import { channelsOnScreen, holdShown, layoutChoices, layoutFromParam, layoutLabel, rememberAuto, rememberLayout, roomId, saveSet, savedAuto, savedLayout, slotsFor, yieldsSound, type MvLayout } from "./storage";
import { pickFocus } from "./switcher";
import "./multiview.css";

function pressedAt() {
  return Date.now();
}

function linesFrom(games: ScoreGame[]) {
  const next = new Map<string, string>();
  for (const game of games) {
    const line = scoreLine(game);
    if (game.id && line) next.set(game.id, line);
  }
  return next;
}

function parseIds(raw: string | null) {
  return (raw ?? "")
    .split(",")
    .map((n) => Number(n))
    .filter((n) => Number.isFinite(n) && n > 0);
}

function scheduleStop(stops: { at: string; channelId: number }[], selected: number[], focus: number, drop: (next: number[], focus: number) => void) {
  const due = stops.filter((stop) => Number.isFinite(Date.parse(stop.at)));
  if (due.length === 0) return 0;
  const now = Date.now();
  const soonest = Math.min(...due.map((stop) => Date.parse(stop.at)));
  const wait = Math.max(0, soonest - now);
  return window.setTimeout(() => {
    const tick = Date.now();
    const gone = new Set(due.filter((stop) => Date.parse(stop.at) <= tick + 1000).map((stop) => stop.channelId));
    const next = selected.filter((id) => !gone.has(id));
    if (next.length === selected.length) return;
    drop(next, next.includes(focus) ? focus : (next[0] ?? 0));
  }, wait === 0 ? 4000 : wait);
}

export function Multiview() {
  const { channels, index, now, record } = useData();
  const { params } = useRoute();
  const layoutMode = useLayout();
  const player = usePlayer();
  const ids = parseIds(params.get("ch"));
  const layout: MvLayout = layoutFromParam(params.get("layout")) ?? savedLayout();
  const focus = Number(params.get("focus")) || ids[0] || 0;
  const guide = params.get("add") === "1";
  const [plan, setPlan] = useState<MultiviewPlan | null>(null);
  const [planFor, setPlanFor] = useState("");
  const [menu, setMenu] = useState(false);
  const [hint, setHint] = useState(() => localStorage.getItem("broadwave-mv-hint-seen") !== "1");
  const heard = useCallback(() => {
    setHint((on) => {
      if (!on) return false;
      localStorage.setItem("broadwave-mv-hint-seen", "1");
      return false;
    });
  }, []);
  const [room] = useState(roomId);
  const [failed, setFailed] = useState<ReadonlySet<number>>(() => new Set());
  // Channels this lineup has already shown. A plan refresh can call one blocked
  // for a moment; taking its tile down stops a picture that is fine.
  const [shown, setShown] = useState<{ key: string; ids: ReadonlySet<number> }>({ key: "", ids: new Set() });
  const markFailed = useCallback((id: number, on: boolean) => {
    setFailed((prev) => {
      if (prev.has(id) === on) return prev;
      const next = new Set(prev);
      if (on) next.add(id);
      else next.delete(id);
      return next;
    });
  }, []);
  const chKey = params.get("ch") ?? "";
  const waiting = ids.length > 1 && planFor !== chKey;
  const known = ids.map((id) => channels.find((c) => c.id === id)).filter((c): c is Channel => !!c);
  const blocked = new Set(plan?.blocked.map((b) => b.channelId) ?? []);
  const lineupKey = `${chKey}:${layout}`;
  const held = shown.key === lineupKey ? shown.ids : new Set<number>();
  if (!waiting) {
    const next = holdShown(held, known.map((channel) => channel.id), blocked);
    if (shown.key !== lineupKey || next !== held) setShown({ key: lineupKey, ids: next });
  }
  const costs = new Map(plan?.offers?.map((offer) => [offer.channelId, offer]) ?? []);
  const warning = [...new Set((plan?.stops ?? []).map((stop) => stop.reason).filter(Boolean))].join(" ");
  const onScreen = waiting ? [] : channelsOnScreen(known.map((c) => c.id), blocked, held, slotsFor(layout));
  const visible = onScreen.map((id) => known.find((c) => c.id === id)).filter((c): c is Channel => !!c);
  const ordered = layout === "2up" || layout === "quad" ? visible : [visible.find((c) => c.id === focus) ?? visible[0], ...visible.filter((c) => c.id !== focus)].filter((c): c is Channel => !!c);
  const dropped = (plan?.blocked ?? []).filter((item) => !held.has(item.channelId));
  const notice = warning || dropped[0]?.reason || plan?.note || "";
  const cachedScores = useScoreMap();
  const [auto, setAuto] = useState(() => savedAuto());
  const [holdUntil, setHoldUntil] = useState(0);
  const [board, setBoard] = useState<ScoreGame[]>([]);
  const [prior, setPrior] = useState<ScoreGame[]>([]);
  const holdRef = useRef(0);
  const boardRef = useRef<ScoreGame[]>([]);
  const scores = board.length > 0 ? linesFrom(board) : cachedScores;
  const live: { channelId: number; game: ScoreGame }[] = [];
  if (auto && holdUntil === 0) {
    const seen = new Set<string>();
    for (const channel of visible) {
      const gameId = airingAt(index, channel.id, now)?.gameId ?? "";
      if (!gameId || seen.has(gameId)) continue;
      const game = board.find((item) => item.id === gameId && item.state === "in");
      if (!game) continue;
      seen.add(gameId);
      live.push({ channelId: channel.id, game });
    }
  }
  const choice = live.length >= 2 ? pickFocus(now, 0, live.map((item) => item.game), prior) : null;
  const banner = choice?.banner ?? "";
  const autoChannel = choice ? (live.find((item) => item.game.id === choice.gameId)?.channelId ?? 0) : 0;

  useEffect(() => {
    rememberLayout(layout);
  }, [layout]);

  useEffect(() => {
    const list = parseIds(chKey);
    let dead = false;
    const load = () => {
      void planMultiview(list)
        .then((next) => {
          if (!dead) setPlan(next);
        })
        .catch(() => {
          if (!dead) setPlan(null);
        })
        .finally(() => {
          if (!dead) setPlanFor(chKey);
        });
    };
    load();
    const timer = window.setInterval(load, 20000);
    return () => {
      dead = true;
      window.clearInterval(timer);
    };
  }, [chKey]);

  useEffect(() => {
    const selected = parseIds(chKey);
    const timer = scheduleStop(plan?.stops ?? [], selected, focus, (next, nextFocus) => {
      const q = new URLSearchParams();
      q.set("ch", next.join(","));
      q.set("layout", layout);
      q.set("focus", String(nextFocus));
      navigate(`/multiview?${q}`);
    });
    return () => window.clearTimeout(timer);
  }, [plan, chKey, layout, focus]);

  useEffect(() => {
    if (!auto) return;
    let cancel = false;
    const pull = () => {
      void refreshScores().then((games) => {
        if (cancel) return;
        setPrior(boardRef.current);
        boardRef.current = games;
        setBoard(games);
      });
    };
    pull();
    const timer = window.setInterval(pull, 20000);
    return () => {
      cancel = true;
      window.clearInterval(timer);
    };
  }, [auto]);

  useEffect(() => {
    if (holdUntil === 0) return;
    const left = holdUntil + 120_000 - Date.now();
    const timer = window.setTimeout(() => {
      holdRef.current = 0;
      setHoldUntil(0);
    }, Math.max(0, left));
    return () => window.clearTimeout(timer);
  }, [holdUntil]);

  useEffect(() => {
    if (holdRef.current !== 0 && Date.now() - holdRef.current < 120_000) return;
    if (!autoChannel || autoChannel === focus) return;
    const q = new URLSearchParams();
    q.set("ch", chKey);
    q.set("layout", layout);
    q.set("focus", String(autoChannel));
    if (guide) q.set("add", "1");
    navigate(`/multiview?${q}`, true);
  }, [autoChannel, focus, chKey, layout, guide]);

  function go(next: { ch?: number[]; layout?: MvLayout; focus?: number; add?: boolean }, replace = true) {
    const q = new URLSearchParams();
    const ch = next.ch ?? ids;
    q.set("ch", ch.join(","));
    q.set("layout", next.layout ?? layout);
    q.set("focus", String(next.focus ?? focus));
    if (next.add ?? guide) q.set("add", "1");
    navigate(`/multiview?${q}`, replace);
  }

  function focusManual(id: number, extra?: { ch?: number[]; add?: boolean }) {
    const at = pressedAt();
    holdRef.current = at;
    setHoldUntil(at);
    go({ focus: id, ...extra });
  }

  function addChannel(id: number) {
    if (costs.get(id)?.cost === "none") return;
    if (ids.includes(id)) {
      focusManual(id, { add: false });
      return;
    }
    const cap = slotsFor(layout);
    let next = [...ids, id];
    if (next.length > cap) {
      const drop = [...ids].reverse().find((n) => n !== focus) ?? ids[ids.length - 1];
      next = [...ids.filter((n) => n !== drop), id];
    }
    focusManual(id, { ch: next, add: false });
  }

  function remove(id: number) {
    const next = ids.filter((n) => n !== id);
    focusManual(next[0] ?? 0, { ch: next, add: false });
    setMenu(false);
  }

  useEffect(() => {
    if (layoutMode !== "tv") return;
    const frame = window.requestAnimationFrame(() => {
      const root = document.querySelector<HTMLElement>(".mv");
      if (!root) return;
      const active = document.activeElement;
      if (active instanceof HTMLElement && active !== document.body && root.contains(active)) return;
      if (guide) {
        const buttons = [...root.querySelectorAll<HTMLButtonElement>(".mv-guide button:not([disabled])")];
        const other = buttons.find((button) => button.getAttribute("aria-selected") !== "true");
        focusRing(other ?? buttons[0]);
        return;
      }
      focusRing(root);
    });
    return () => window.cancelAnimationFrame(frame);
  }, [layoutMode, guide, chKey]);

  function leaveGrid() {
    if (inAppDepth() > 0) {
      window.history.back();
      return;
    }
    const current = ordered.find((c) => c.id === focus) ?? ordered[0];
    if (current) player.open(current);
    else navigate("/guide");
  }

  function onKey(event: KeyboardEvent) {
    const k = event.key;
    const i = Math.max(0, ordered.findIndex((c) => c.id === focus));
    const inBar = event.target instanceof Element && Boolean(event.target.closest(".mv-top, .mv-bottom, .mv-guide"));
    if (k === "Escape" || k === "Backspace") {
      event.preventDefault();
      if (menu) return setMenu(false);
      if (guide) return go({ add: false });
      leaveGrid();
      return;
    }
    // The add list and the bars are buttons. Arrows walk them; Enter activates them.
    if (inBar && (k === "Enter" || k.startsWith("Arrow"))) return;
    if (k === "g") {
      event.preventDefault();
      go({ add: !guide });
      return;
    }
    if (k === "o") {
      event.preventDefault();
      setMenu((v) => !v);
      return;
    }
    if (k === "Enter") {
      event.preventDefault();
      if (layout === "1+2" || layout === "1+3" || layout === "pip") focusManual(focus);
      return;
    }
    if (k === " ") {
      event.preventDefault();
      const video = document.querySelector<HTMLVideoElement>(".mv-tile.focused video");
      const action = video?.paused ? "play" : "pause";
      // Tiles do not share a room, so a pause has to name each one.
      for (const id of ordered.map((channel) => channel.id)) events().command(`${room}:${id}`, action);
      return;
    }
    const cols = layout === "2up" || layout === "quad" ? 2 : 1;
    const step = k === "ArrowRight" ? 1 : k === "ArrowLeft" ? -1 : k === "ArrowDown" ? cols : k === "ArrowUp" ? -cols : 0;
    if (!step) return;
    const dest = Math.max(0, Math.min(ordered.length - 1, i + step));
    event.preventDefault();
    if (ordered[dest]) focusManual(ordered[dest].id);
  }

  const focused = ordered.find((c) => c.id === focus) ?? ordered[0];
  // A tile that never started has no sound to give. One that already showed a
  // picture keeps it while that picture comes back.
  const soundTo = focused && failed.has(focused.id) ? (ordered.find((c) => !failed.has(c.id))?.id ?? 0) : 0;
  useEffect(() => {
    if (!soundTo) return;
    const q = new URLSearchParams(window.location.search);
    q.set("focus", String(soundTo));
    navigate(`/multiview?${q}`, true);
  }, [soundTo]);

  return (
    <section className="mv" tabIndex={0} onKeyDown={onKey} aria-label={layoutLabel(layout)}>
      <header className="mv-top">
        <button type="button" className="glass-icon" aria-label="Back to one channel" onClick={() => (focused ? player.open(focused) : navigate("/guide"))}>
          <CloseIcon />
        </button>
        <h1>{layoutLabel(layout)}</h1>
        <span className="mv-spacer" />
        <button
          type="button"
          className={auto ? "btn primary" : "btn"}
          aria-pressed={auto}
          title="Follow the game that matters"
          onClick={() => {
            const next = !auto;
            setAuto(next);
            rememberAuto(next);
          }}
        >
          Auto
        </button>
        <button
          type="button"
          className="btn"
          disabled={ordered.length < 2}
          onClick={() => {
            const name = ordered.map((c) => c.displayNumber).join(" and ");
            saveSet(name, ordered.map((c) => c.id), layout);
          }}
        >
          Save
        </button>
      </header>
      {banner ? <p className="mv-banner" role="status">{banner}</p> : null}
      {hint ? <p className="mv-note">Select a tile to hear it.</p> : null}
      {notice ? <p className="mv-note" role="status">{notice}</p> : null}
      <div className="mv-fit">
      <div className={`mv-grid${layoutMode === "phone" && layout === "2up" ? " stacked" : ""}`} data-layout={layout}>
        {waiting ? null : ordered.length === 0 ? <p className="mv-empty">Pick two channels.</p> : null}
        {ordered.map((channel) => (
          <Tile
            key={channel.id}
            channel={channel}
            title={airingAt(index, channel.id, now)?.title || channel.displayName}
            score={scores.get(airingAt(index, channel.id, now)?.gameId ?? "")}
            focused={channel.id === (focused?.id ?? 0)}
            layout={layout}
            room={room}
            menu={menu && channel.id === (focused?.id ?? 0)}
            onFocus={() => focusManual(channel.id)}
            onHeard={heard}
            onRemove={() => remove(channel.id)}
            onFailed={markFailed}
            onRecord={() => void record(channel, airingAt(index, channel.id, now)?.title || channel.displayName)}
          />
        ))}
      </div>
      </div>
      <div className="mv-bottom" role="toolbar" aria-label="Layout">
        {layoutChoices.map((item) => (
          <button key={item.id} type="button" className={layout === item.id ? "btn primary" : "btn"} aria-pressed={layout === item.id} onClick={() => go({ layout: item.id })}>
            {item.label}
          </button>
        ))}
        <button type="button" className={guide ? "btn primary" : "btn"} aria-pressed={guide} onClick={() => go({ add: !guide })}>
          Channels
        </button>
      </div>
      {guide ? (
        <div className="mv-guide" role="listbox" aria-label="Add a channel">
          {channels.map((c) => {
            const cost = costs.get(c.id);
            const full = cost?.cost === "none";
            return (
              <button key={c.id} type="button" role="option" aria-selected={ids.includes(c.id)} aria-disabled={full || undefined} disabled={full} className={ids.includes(c.id) ? "mv-ch on" : "mv-ch"} onClick={() => addChannel(c.id)}>
                <LiveFrame id={c.id} className="mv-frame" />
                {c.displayNumber}
                <small>{c.displayName}</small>
                {cost?.label ? <small className="mv-cost">{cost.label}</small> : null}
              </button>
            );
          })}
        </div>
      ) : null}
    </section>
  );
}

function Tile({
  channel,
  title,
  score,
  focused,
  layout,
  room,
  menu,
  onFocus,
  onHeard,
  onRemove,
  onRecord,
  onFailed,
}: {
  channel: Channel;
  title: string;
  score?: string;
  focused: boolean;
  layout: MvLayout;
  room: string;
  menu: boolean;
  onFocus: () => void;
  onHeard: () => void;
  onRemove: () => void;
  onRecord: () => void;
  onFailed: (id: number, failed: boolean) => void;
}) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const showedRef = useRef(false);
  const [showedPicture, setShowedPicture] = useState(false);
  const layoutMode = useLayout();
  const equal = layout === "2up" || layout === "quad";
  // Equal tiles all get the same picture, so moving the sound only unmutes one.
  // A focus-dependent picture restarted both tiles on every swap.
  const big = focused && !equal;
  const stream = useLiveStream(videoRef, {
    channelId: channel.id,
    quality: layout === "2up" || big ? "focus" : layout === "quad" || layout === "pip" ? "360" : "tile",
    audio: equal ? "stereo" : focused ? "auto" : "none",
    picture: "broadcast",
    // Each tile has its own room. One shared room steps every tile back when
    // any of them stalls, so a dead stream pauses the pictures that are fine.
    room: `${room}:${channel.id}`,
    sync: true,
    profile: big ? (layoutMode === "tv" ? "tv" : "desktop") : "tile",
    audible: focused,
  });
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    const mark = () => {
      if (showedRef.current || video.videoWidth <= 0 || video.currentTime <= 0.2) return;
      showedRef.current = true;
      setShowedPicture(true);
    };
    video.addEventListener("timeupdate", mark);
    video.addEventListener("playing", mark);
    return () => {
      video.removeEventListener("timeupdate", mark);
      video.removeEventListener("playing", mark);
    };
  }, []);
  const failed = yieldsSound(stream.error, showedPicture);
  useEffect(() => {
    onFailed(channel.id, failed);
  }, [channel.id, failed, onFailed]);
  useEffect(() => () => onFailed(channel.id, false), [channel.id, onFailed]);
  useEffect(() => {
    const video = videoRef.current;
    if (!focused || !video) return;
    const hear = () => {
      if (!video.muted && !video.paused) onHeard();
    };
    video.addEventListener("playing", hear);
    hear();
    return () => video.removeEventListener("playing", hear);
  }, [focused, onHeard]);
  return (
    <div className="mv-cell" onClick={onFocus}>
    <div className={focused ? "mv-tile focused" : "mv-tile"} role="group" aria-label={`${channel.displayNumber} ${channel.displayName}${focused ? ", sound on" : ""}`}>
      <video ref={videoRef} className="mv-video" autoPlay playsInline data-channel={channel.id} />
      <div className="mv-meta">
        <span>{channel.displayNumber}</span>
        <span className="mv-title">{title}{score ? ` · ${score}` : ""}</span>
        {focused ? (
          <span className="mv-audio">
            <VolumeIcon /> Sound
          </span>
        ) : null}
        {focused && stream.session?.stream.reason ? <span className="mv-detail">{stream.session.stream.reason}</span> : null}
      </div>
      {stream.error ? (
        <div className="mv-error" role="alert">
          <p>{stream.error}</p>
          {stream.needsConfirm ? (
            <button type="button" className="btn small" onClick={(event) => { event.stopPropagation(); stream.confirm(); }}>
              Watch anyway
            </button>
          ) : (
            <button type="button" className="btn small" onClick={(event) => { event.stopPropagation(); stream.retry(); }}>
              Try again
            </button>
          )}
          <button type="button" className="btn small" onClick={(event) => { event.stopPropagation(); onRemove(); }}>
            Remove
          </button>
        </div>
      ) : null}
      {menu ? (
        <div className="mv-menu" role="menu">
          <button type="button" className="btn" onClick={(e) => { e.stopPropagation(); onFocus(); }}>Make big</button>
          <button type="button" className="btn" onClick={(e) => { e.stopPropagation(); onRecord(); }}>Record</button>
          <button type="button" className="btn" onClick={(e) => { e.stopPropagation(); onRemove(); }}>Remove</button>
          <button
            type="button"
            className="btn"
            onClick={(e) => {
              e.stopPropagation();
              const el = e.currentTarget.closest(".mv-tile");
              void el?.requestFullscreen?.().catch(() => undefined);
            }}
          >
            Full screen
          </button>
        </div>
      ) : null}
    </div>
    </div>
  );
}
