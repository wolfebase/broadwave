import { useCallback, useEffect, useRef, useState, type KeyboardEvent } from "react";
import { planMultiview } from "../../api";
import { useData } from "../../app/data";
import { useLayout } from "../../app/layout";
import { usePlayer } from "../../app/player";
import { navigate, useRoute } from "../../app/router";
import { airingAt } from "../../lib/guide";
import { events } from "../../lib/events";
import type { Channel, MultiviewPlan } from "../../types";
import { CloseIcon, VolumeIcon } from "../../ui/icons";
import { LiveFrame } from "../../ui/LiveFrame";
import { useLiveStream } from "../player/useLiveStream";
import { refreshScores, scoreLine, useScoreMap, type ScoreGame } from "../sports/scores";
import { layoutChoices, layoutFromParam, layoutLabel, rememberAuto, rememberLayout, roomId, saveSet, savedAuto, savedLayout, slotsFor, type MvLayout } from "./storage";
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
  const chKey = params.get("ch") ?? "";
  const waiting = ids.length > 1 && planFor !== chKey;
  const known = ids.map((id) => channels.find((c) => c.id === id)).filter((c): c is Channel => !!c);
  const blocked = new Set(plan?.blocked.map((b) => b.channelId) ?? []);
  const costs = new Map(plan?.offers?.map((offer) => [offer.channelId, offer]) ?? []);
  const warning = [...new Set((plan?.stops ?? []).map((stop) => stop.reason).filter(Boolean))].join(" ");
  const visible = waiting ? [] : known.filter((c) => !blocked.has(c.id)).slice(0, slotsFor(layout));
  const ordered = layout === "2up" || layout === "quad" ? visible : [visible.find((c) => c.id === focus) ?? visible[0], ...visible.filter((c) => c.id !== focus)].filter((c): c is Channel => !!c);
  const notice = warning || plan?.blocked[0]?.reason || plan?.note || "";
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

  function onKey(event: KeyboardEvent) {
    const k = event.key;
    const i = Math.max(0, ordered.findIndex((c) => c.id === focus));
    if (k === "Escape") {
      event.preventDefault();
      if (menu) return setMenu(false);
      if (guide) return go({ add: false });
      const current = ordered.find((c) => c.id === focus) ?? ordered[0];
      if (current) player.open(current);
      else navigate("/guide");
      return;
    }
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
      events().command(room, video?.paused ? "play" : "pause");
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
}) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const layoutMode = useLayout();
  const equal = layout === "2up" || layout === "quad";
  const stream = useLiveStream(videoRef, {
    channelId: channel.id,
    quality: focused ? "focus" : layout === "quad" || layout === "pip" ? "360" : "tile",
    audio: focused ? (equal ? "stereo" : "auto") : equal ? "stereo" : "none",
    picture: "broadcast",
    room,
    sync: true,
    profile: focused ? (layoutMode === "tv" ? "tv" : "desktop") : "tile",
    audible: focused,
  });
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
          ) : null}
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
