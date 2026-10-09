import { useCallback, useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent } from "react";
import { planMultiview } from "../../api";
import { useData } from "../../app/data";
import { useLayout } from "../../app/layout";
import { usePlayer } from "../../app/player";
import { focusRing } from "../../app/remote";
import { inAppDepth, navigate, useRoute } from "../../app/router";
import { authHeaders } from "../../lib/deviceToken";
import { airingAt } from "../../lib/guide";
import { events } from "../../lib/events";
import { addMosaic, mosaicKey, mosaicShareMax, sharedMosaics } from "../../lib/mosaic";
import { copy } from "../../strings";
import type { Channel, MultiviewPlan } from "../../types";
import { CloseIcon, VolumeIcon } from "../../ui/icons";
import { LiveFrame } from "../../ui/LiveFrame";
import { pageFitsTiles, StartGate } from "../player/quietStart";
import { useLiveStream } from "../player/useLiveStream";
import { refreshScores, scoreLine, useScoreMap, type ScoreGame } from "../sports/scores";
import { clearBroadcast, fromStillOn, resolveClear, standInIds } from "./clear";
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
  const { channels, allChannels, ready, index, now, record, settings, saveSettings } = useData();
  const [shareNote, setShareNote] = useState<{ key: string; text: string } | null>(null);
  const { params } = useRoute();
  const layoutMode = useLayout();
  const player = usePlayer();
  const ids = parseIds(params.get("ch"));
  const fromIds = parseIds(params.get("from"));
  const fromKey = fromIds.join(",");
  const layout: MvLayout = layoutFromParam(params.get("layout")) ?? savedLayout();
  const focus = Number(params.get("focus")) || ids[0] || 0;
  const standIns = new Set(standInIds(fromIds, allChannels));
  const guide = params.get("add") === "1";
  // The ring and the sound are separate. Arrows move the ring; Enter moves the sound.
  // A new sound tile (Enter, or the page opening) puts the ring back on that tile.
  const [cursor, setCursor] = useState({ focus, id: focus });
  if (cursor.focus !== focus) setCursor({ focus, id: focus });
  const mark = cursor.focus === focus ? cursor.id : focus;
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
  // A link may name the hidden half of a 1.0/3.0 pair, or an encrypted 3.0 id.
  // Play the half that is on the guide. The address bar follows, so the tile is
  // that channel and an encrypted stand-in keeps its note.
  useEffect(() => {
    if (!ready || allChannels.length === 0) return;
    const asked = parseIds(chKey);
    const next = resolveClear(asked, channels, allChannels);
    if (next.ids.join(",") === asked.join(",")) return;
    const q = new URLSearchParams(window.location.search);
    q.set("ch", next.ids.join(","));
    if (next.from.length) q.set("from", next.from.join(","));
    else q.delete("from");
    const focusNow = Number(q.get("focus")) || asked[0] || 0;
    if (focusNow && !next.ids.includes(focusNow)) {
      const mapped = clearBroadcast(focusNow, channels, allChannels);
      const focusNext = mapped && next.ids.includes(mapped.id) ? mapped.id : (next.ids[0] ?? 0);
      if (focusNext) q.set("focus", String(focusNext));
      else q.delete("focus");
    }
    navigate(`/multiview?${q}`, true);
  }, [ready, chKey, channels, allChannels]);
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
  const shared = sharedMosaics(settings.exportMosaics);
  const shareable = ordered.length >= 2 && ordered.length <= 4;
  const shareFull = !shared.includes("") && shared.length >= mosaicShareMax;
  const shareKey = shareable ? mosaicKey(ordered.some((c) => c.id === focus) ? focus : ordered[0].id, ordered.map((c) => c.id)) : "";
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
      const kept = fromStillOn(parseIds(fromKey), next, allChannels);
      if (kept.length) q.set("from", kept.join(","));
      navigate(`/multiview?${q}`);
    });
    return () => window.clearTimeout(timer);
  }, [plan, chKey, layout, focus, fromKey, allChannels]);

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
    const kept = fromStillOn(parseIds(fromKey), parseIds(chKey), allChannels);
    if (kept.length) q.set("from", kept.join(","));
    navigate(`/multiview?${q}`, true);
  }, [autoChannel, focus, chKey, layout, guide, fromKey, allChannels]);

  function go(next: { ch?: number[]; layout?: MvLayout; focus?: number; add?: boolean }, replace = true) {
    const q = new URLSearchParams();
    const ch = next.ch ?? ids;
    q.set("ch", ch.join(","));
    q.set("layout", next.layout ?? layout);
    q.set("focus", String(next.focus ?? focus));
    if (next.add ?? guide) q.set("add", "1");
    const kept = fromStillOn(fromIds, ch, allChannels);
    if (kept.length) q.set("from", kept.join(","));
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
      // A button already inside the grid keeps the keys. The section itself does not.
      if (active instanceof HTMLElement && active !== document.body && root.contains(active) && !active.classList.contains("mv")) return;
      if (guide) {
        const buttons = [...root.querySelectorAll<HTMLButtonElement>(".mv-guide button:not([disabled])")];
        const other = buttons.find((button) => button.getAttribute("aria-selected") !== "true");
        focusRing(other ?? buttons[0]);
        return;
      }
      const tile = root.querySelector<HTMLElement>(`.mv-tile[data-channel="${mark}"]`);
      focusRing(tile ?? root);
    });
    return () => window.cancelAnimationFrame(frame);
  }, [layoutMode, guide, chKey, mark]);

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
    const target = event.target;
    const control = target instanceof Element && target.closest("button, a[href], input, select, textarea");
    if (k === "Escape" || k === "Backspace") {
      event.preventDefault();
      if (menu) return setMenu(false);
      if (guide) return go({ add: false });
      leaveGrid();
      return;
    }
    // Buttons, links, and fields keep their own keys. Enter activates them.
    if (control && (k === "Enter" || k.startsWith("Arrow"))) return;
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
    const i = Math.max(0, ordered.findIndex((c) => c.id === mark));
    if (k === "Enter") {
      event.preventDefault();
      if (mark && mark !== focus) focusManual(mark);
      else if (layout === "1+2" || layout === "1+3" || layout === "pip") focusManual(focus);
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
    const next = ordered[dest];
    if (!next || next.id === mark) return;
    setCursor({ focus, id: next.id });
    focusRing(document.querySelector<HTMLElement>(`.mv-tile[data-channel="${next.id}"]`));
  }

  const focused = ordered.find((c) => c.id === focus) ?? ordered[0];
  const [slots, setSlots] = useState<number | null>(null);
  const [pictures, setPictures] = useState<ReadonlySet<number>>(() => new Set());
  useEffect(() => {
    let dead = false;
    fetch("/api/v1/diagnostics", { headers: authHeaders() })
      .then((res) => res.json() as Promise<{ encoder?: { tiles?: number } }>)
      .then((body) => {
        const count = body.encoder?.tiles;
        if (!dead && typeof count === "number") setSlots(count);
      })
      .catch(() => undefined);
    return () => {
      dead = true;
    };
  }, []);
  const notePicture = useCallback((id: number, on: boolean) => {
    setPictures((prev) => {
      if (prev.has(id) === on) return prev;
      const next = new Set(prev);
      if (on) next.add(id);
      else next.delete(id);
      return next;
    });
  }, []);
  // A kept page starts every tile together only when the last budget covered
  // them. Otherwise the sound tile asks first again.
  const fits = pageFitsTiles(slots, ordered.length, pictures.size);
  // The sound tile asks for its picture first, on open and after a restart.
  // With fewer pictures than tiles, a quiet tile that asked first kept the
  // picture the viewer is listening to.
  const [gate] = useState(() => new StartGate());
  useEffect(() => {
    const offRestart = events().on("restarted", () => gate.restarted());
    const offConnection = events().on("connection", (up) => {
      if (!up) gate.dropped();
    });
    const offHello = events().on("hello", () => gate.back());
    return () => {
      offRestart();
      offConnection();
      offHello();
      gate.stop();
    };
  }, [gate]);
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
    <section className="mv" tabIndex={0} onKeyDown={onKey} aria-label={layoutLabel(layout)} data-fits={fits ? "1" : "0"} data-slots={slots ?? ""} data-pictures={pictures.size} data-tiles={ordered.length}>
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
        <button
          type="button"
          className="btn"
          title={shareFull ? `Other apps can list ${mosaicShareMax}. Remove one in Settings.` : "Plex, Jellyfin, and Channels see these channels as one"}
          disabled={!shareable || shared.includes(shareKey) || shareFull}
          onClick={() => {
            const added = addMosaic(shared, shareKey);
            void saveSettings({ exportMosaics: added.slots.join(",") }).then(
              () => setShareNote({ key: shareKey, text: `Other apps list this as channel 990.${added.slot + 1}. Settings has the list.` }),
              () => setShareNote({ key: shareKey, text: "That did not save. Try again." }),
            );
          }}
        >
          {shared.includes(shareKey) ? "In other apps" : "Add to other apps"}
        </button>
      </header>
      {shareNote?.key === shareKey ? <p className="mv-note" role="status">{shareNote.text}</p> : null}
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
            pointed={channel.id === mark}
            gate={gate}
            layout={layout}
            room={room}
            menu={menu && channel.id === (focused?.id ?? 0)}
            onFocus={() => focusManual(channel.id)}
            onHeard={heard}
            onRemove={() => remove(channel.id)}
            onFailed={markFailed}
            onPicture={notePicture}
            fits={fits}
            standIn={standIns.has(channel.id)}
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
  pointed,
  layout,
  room,
  menu,
  onFocus,
  onHeard,
  onRemove,
  onRecord,
  onFailed,
  onPicture,
  fits,
  gate,
  standIn,
}: {
  channel: Channel;
  title: string;
  score?: string;
  focused: boolean;
  pointed: boolean;
  layout: MvLayout;
  room: string;
  menu: boolean;
  onFocus: () => void;
  onHeard: () => void;
  onRemove: () => void;
  onRecord: () => void;
  onFailed: (id: number, failed: boolean) => void;
  onPicture: (id: number, on: boolean) => void;
  fits: boolean;
  gate: StartGate;
  standIn: boolean;
}) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const showedRef = useRef(false);
  const [showedPicture, setShowedPicture] = useState(false);
  const layoutMode = useLayout();
  const equal = layout === "2up" || layout === "quad";
  // Equal tiles all get the same picture, so moving the sound only unmutes one.
  // A focus-dependent picture restarted both tiles on every swap.
  const big = focused && !equal;
  // Layout effects run before the watch effect, so a start in this render has its rank.
  const rank = focused ? 0 : showedPicture ? 1 : 2;
  useLayoutEffect(() => gate.rank(channel.id, rank), [gate, channel.id, rank]);
  useEffect(() => () => gate.leave(channel.id), [gate, channel.id]);
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
    gate,
    fits,
  });
  useEffect(() => {
    if (stream.asking) return;
    onPicture(channel.id, stream.session != null);
  }, [stream.asking, stream.session, channel.id, onPicture]);
  useEffect(() => () => onPicture(channel.id, false), [channel.id, onPicture]);
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
    <div className={focused ? "mv-tile focused" : "mv-tile"} role="group" tabIndex={pointed ? 0 : -1} data-channel={channel.id} aria-label={`${channel.displayNumber} ${channel.displayName}${focused ? ", sound on" : ""}${standIn ? `. ${copy.player.encrypted}` : ""}`}>
      <video ref={videoRef} className="mv-video" autoPlay playsInline data-channel={channel.id} />
      <div className="mv-meta">
        <span>{channel.displayNumber}</span>
        <span className="mv-title">{title}{score ? ` · ${score}` : ""}</span>
        {standIn ? <p className="mv-note" role="status">{copy.player.encrypted}</p> : null}
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
