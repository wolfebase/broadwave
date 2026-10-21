import { lazy, Suspense, useEffect, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { guideUiMessages } from "../../i18n/messages/guideUi.ts";
import { bind, formatDateTime } from "../../i18n/runtime.ts";
import { useData } from "../../app/data";
import { usePlayer } from "../../app/player";
import { focusRing } from "../../app/remote";
import { navigate } from "../../app/router";
import { useLayout } from "../../app/layout";
import {
  airingAt,
  categoryLabel,
  categoryOf,
  channelWatchName,
  emptyGuideLabel,
  guideCellId,
  guideCellName,
  guideOrigin,
  guideRowScroll,
  guideShowsGrid,
  guideViewBox,
  isRecording,
  keptRow,
  nextAfter,
  reorderChannels,
  primeTime,
  progress,
  recordingKeys,
  spanLabel,
  timeLabel,
  type Category,
} from "../../lib/guide";
import { guideEnterAction, guidePageRow, isInfoKey, slotUnderNow } from "../../lib/guideKeys";
import { scrollBehavior } from "../../lib/motion";
import { markLogoTone } from "../../lib/logoTone";
import { canonicalKey } from "../../lib/tvKey";
import type { Airing, Channel } from "../../types";
import { SearchIcon, StarIcon } from "../../ui/icons";
import { Atsc3Tag, ChannelBadge, Chip, Empty, Progress, RecDot } from "../../ui/primitives";
import { useScoreMap } from "../sports/scores";
import { LiveFrame } from "../../ui/LiveFrame";
import { dayJumpTarget, readGuideFilter, saveGuideFilter, type GuideFilter } from "./prefs";
import "./guide.css";

const t = bind(guideUiMessages);

// The sheet opens after a cell is chosen, so it stays out of the first script.
const ProgramSheet = lazy(() => import("./ProgramSheet").then((m) => ({ default: m.ProgramSheet })));

type Filter = GuideFilter;
const MIN = 60_000;
const ORDER_KEY = "broadwave-guide-order";

function loadOrder(): number[] {
  try {
    const raw = JSON.parse(localStorage.getItem(ORDER_KEY) || "[]") as unknown;
    return Array.isArray(raw) ? raw.filter((id): id is number => typeof id === "number") : [];
  } catch {
    return [];
  }
}

function sameDay(a: number, b: number) {
  return new Date(a).toDateString() === new Date(b).toDateString();
}

function dayWord(midnight: number, now: number) {
  const day = new Date(midnight);
  if (sameDay(midnight, now)) return t("guideUi.today");
  const tomorrow = new Date(now);
  tomorrow.setDate(tomorrow.getDate() + 1);
  if (sameDay(midnight, tomorrow.getTime())) return t("guideUi.tomorrow");
  return formatDateTime(day, { weekday: "short" });
}

export function Guide() {
  const { channels, devices, index, now, planned, recordings, virtuals, favorite, editChannel, ready } = useData();
  const player = usePlayer();
  const layout = useLayout();
  const [filter, setFilterRaw] = useState<Filter>(readGuideFilter);
  const [query, setQueryRaw] = useState("");
  const [when, setWhen] = useState<"now" | "tonight">("now");
  const [sheet, setSheet] = useState<{ channel: Channel; airing?: Airing } | null>(null);
  const [focus, setFocus] = useState<{ row: number; at: number; id: number }>({ row: 0, at: now, id: 0 });
  const scrollRef = useRef<HTMLDivElement>(null);
  const [view, setView] = useState({ top: 0, left: 0, height: 800, width: 1200 });
  const [order, setOrder] = useState<number[]>(loadOrder);
  const [landscape, setLandscape] = useState(() => window.matchMedia("(orientation: landscape) and (max-height: 520px)").matches);
  const scores = useScoreMap();
  // OK held on a TV opens the details instead of tuning.
  const enterHeld = useRef<{ timer: number; fired: boolean } | null>(null);
  useEffect(() => () => window.clearTimeout(enterHeld.current?.timer), []);

  const tv = layout === "tv";
  const grid = guideShowsGrid(layout, landscape);
  // Channels with not one listing: a playlist without a guide, or a listings fetch that failed.
  const noListings = ready && channels.length > 0 && index.size === 0;
  const antenna = devices.some((d) => d.tunerCount > 0);
  const rowH = tv ? 96 : 68;
  const pxPerMin = tv ? 9.6 : 6.4;
  const channelW = tv ? 280 : 220;
  const headH = 48;
  const origin = guideOrigin(now);
  const hours = useMemo(() => {
    let latest = origin + 24 * 60 * MIN;
    for (const list of index.values()) {
      const last = list[list.length - 1];
      if (last) latest = Math.max(latest, Date.parse(last.end));
    }
    return Math.min(14 * 24, Math.max(6, Math.ceil((latest - origin) / (60 * MIN))));
  }, [index, origin]);
  const end = origin + hours * 60 * MIN;
  const width = hours * 60 * pxPerMin;
  const days = useMemo(() => {
    const start = new Date(origin);
    start.setHours(0, 0, 0, 0);
    const out: number[] = [];
    for (let t = start.getTime(); t < end && out.length < 3; t += 24 * 60 * MIN) out.push(t);
    return out;
  }, [origin, end]);
  const keys = useMemo(() => recordingKeys(planned, recordings), [planned, recordings]);

  const ordered = useMemo(() => {
    // The sort is stable, so unranked rows keep the server's channel-number order,
    // as the mini-guide does. An id tiebreak put channels from a later scan last.
    const rank = new Map(order.map((id, i) => [id, i]));
    return [...channels].sort((a, b) => (rank.get(a.id) ?? 10_000) - (rank.get(b.id) ?? 10_000));
  }, [channels, order]);

  const rows = useMemo(() => {
    const q = query.trim().toLowerCase();
    const windowEnd = now + 4 * 60 * MIN;
    return ordered.filter((c) => {
      if (filter === "favorites" && !c.favorite) return false;
      const list = index.get(c.id) ?? [];
      if (filter === "recording") {
        if (!list.some((a) => Date.parse(a.end) > now && isRecording(keys, a, now))) return false;
      } else if (filter !== "all" && filter !== "favorites") {
        if (!list.some((a) => Date.parse(a.end) > now && Date.parse(a.start) < windowEnd && categoryOf(a) === filter)) return false;
      }
      if (!q) return true;
      if (c.displayNumber.startsWith(q) || c.displayName.toLowerCase().includes(q)) return true;
      return list.some((a) => a.title.toLowerCase().includes(q) || (a.subtitle ?? "").toLowerCase().includes(q));
    });
  }, [ordered, index, filter, query, now, keys]);

  const kept = keptRow(rows, focus);
  if (kept.row !== focus.row || kept.id !== focus.id) setFocus({ ...focus, row: kept.row, id: kept.id });

  const counts = useMemo(() => {
    const windowEnd = now + 4 * 60 * MIN;
    const out: Partial<Record<Category, number>> = {};
    for (const c of channels) {
      const seen = new Set<Category>();
      for (const a of index.get(c.id) ?? []) {
        if (Date.parse(a.end) > now && Date.parse(a.start) < windowEnd) seen.add(categoryOf(a));
      }
      for (const cat of seen) out[cat] = (out[cat] ?? 0) + 1;
    }
    return out;
  }, [channels, index, now]);

  useEffect(() => {
    const media = window.matchMedia("(orientation: landscape) and (max-height: 520px)");
    const apply = () => setLandscape(media.matches);
    apply();
    media.addEventListener("change", apply);
    return () => media.removeEventListener("change", apply);
  }, []);

  useLayoutEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    let alive = true;
    const measure = () => {
      const box = guideViewBox(el, alive);
      if (box) setView(box);
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    el.addEventListener("scroll", measure, { passive: true });
    return () => {
      alive = false;
      ro.disconnect();
      el.removeEventListener("scroll", measure);
    };
  }, [layout, grid]);

  useEffect(() => {
    if (!grid) return;
    scrollToTime(now - 30 * MIN, "auto");
    // The scroller exists only while the grid is mounted. Layout still matters:
    // desktop and tv share the grid but use different pixels per minute.
  }, [layout, grid]); // eslint-disable-line react-hooks/exhaustive-deps

  // On a TV, back from the player the keys pick up on the channel that is playing.
  const placed = useRef(false);
  useEffect(() => {
    if (placed.current || rows.length === 0) return;
    placed.current = true;
    const playing = player.channel?.id;
    const row = tv && playing ? rows.findIndex((c) => c.id === playing) : -1;
    if (row > 0) move(row, now);
  }, [rows]); // eslint-disable-line react-hooks/exhaustive-deps

  function scrollToTime(t: number, behavior: ScrollBehavior = "smooth") {
    const el = scrollRef.current;
    if (!el) return;
    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    el.scrollTo({ left: Math.max(0, ((t - origin) / MIN) * pxPerMin), behavior: scrollBehavior(reduced, behavior) });
  }

  // Now keeps half an hour of what just aired on the left and moves the cursor
  // to the show on now. The jump is instant: the cursor effect would stop a smooth one short.
  function jumpToNow() {
    scrollToTime(now - 30 * MIN, "auto");
    move(focus.row, now);
  }

  function tonight() {
    scrollToTime(primeTime(now) - 30 * MIN);
  }

  function jumpToDay(midnight: number) {
    const today = new Date(now);
    today.setHours(0, 0, 0, 0);
    if (midnight === today.getTime()) {
      scrollToTime(now - 30 * MIN);
      return;
    }
    const lastEnds: number[] = [];
    for (const list of index.values()) {
      const last = list[list.length - 1];
      if (last) lastEnds.push(Date.parse(last.end));
    }
    const visible = ((scrollRef.current?.clientWidth ?? 1200) - channelW) * (MIN / pxPerMin);
    scrollToTime(dayJumpTarget(midnight, lastEnds, visible));
  }

  function reorder(fromId: number, toId: number) {
    const next = reorderChannels(
      ordered.map((c) => c.id),
      fromId,
      toId,
    );
    if (!next) return;
    localStorage.setItem(ORDER_KEY, JSON.stringify(next));
    setOrder(next);
  }

  // The sheet came from the grid's keys, so closing it gives the grid its keys back.
  const sheetFromGrid = useRef(false);
  function open(channel: Channel, airing?: Airing) {
    setSheet({ channel, airing });
  }

  function watch(channel: Channel) {
    setSheet(null);
    player.open(channel);
  }

  function move(row: number, at: number) {
    const r = Math.max(0, Math.min(rows.length - 1, row));
    const t = Math.max(origin, Math.min(end - MIN, at));
    setFocus({ row: r, at: t, id: rows[r]?.id ?? focus.id });
    const el = scrollRef.current;
    // A scroller that has not been laid out reports 0. Writing scrollLeft from that
    // parks the cursor off to the side, and nothing later puts it back.
    if (!el || el.clientWidth < 80 || el.clientHeight < 80) return;
    const top = guideRowScroll(el.scrollTop, r, rowH, el.clientHeight, headH);
    if (top !== el.scrollTop) el.scrollTop = top;
    const x = ((t - origin) / MIN) * pxPerMin;
    if (x < el.scrollLeft + 40) el.scrollLeft = Math.max(0, x - 80);
    else if (x > el.scrollLeft + el.clientWidth - channelW - 120) el.scrollLeft = x - el.clientWidth + channelW + 240;
  }

  // A new filter or search starts at the top of what it found.
  function toTop() {
    const el = scrollRef.current;
    if (el) el.scrollTop = 0;
    if (!grid) document.querySelector("main.content")?.scrollTo({ top: 0 });
    setFocus((f) => ({ row: 0, at: f.at, id: 0 }));
  }

  function setFilter(next: Filter) {
    setFilterRaw(next);
    saveGuideFilter(next);
    toTop();
  }

  function setQuery(next: string) {
    setQueryRaw(next);
    toTop();
  }

  function clearFilters() {
    setFilterRaw("all");
    saveGuideFilter("all");
    setQueryRaw("");
    toTop();
  }

  function closeSheet() {
    const fromGrid = Boolean(document.activeElement?.closest(".guide-canvas") || sheetFromGrid.current);
    sheetFromGrid.current = false;
    setSheet(null);
    if (!grid || !fromGrid) return;
    // The dialog hands focus back to the cell first. The grid is what holds the keys.
    window.requestAnimationFrame(() => focusRing(document.querySelector<HTMLElement>(".guide-canvas")));
  }

  function onKeyUp(e: KeyboardEvent) {
    const held = enterHeld.current;
    if (canonicalKey(e) !== "Enter" || !held) return;
    window.clearTimeout(held.timer);
    enterHeld.current = null;
    if (held.fired) return;
    const row = rows[focus.row];
    if (row) watch(row);
  }

  function onKey(e: KeyboardEvent) {
    const target = e.target;
    if (!(target instanceof HTMLElement) || !target.closest(".guide-canvas")) return;
    const key = canonicalKey(e);
    // A channel name reached with Tab is its own button. The grid's cursor may sit on another row.
    if ((key === "Enter" || key === " ") && target.closest(".guide-channel")) return;
    const row = rows[focus.row];
    if (!row) {
      move(focus.row, focus.at);
      return;
    }
    const cur = airingAt(index, row.id, focus.at);
    const handled = () => e.preventDefault();
    switch (key) {
      case "ArrowDown":
        handled();
        move(focus.row + 1, focus.at);
        break;
      case "ArrowUp":
        // The grid eats every arrow. Up from the first row has to reach Now,
        // or a remote cannot jump back to the current show.
        if (tv && focus.row <= 0) {
          const jump = document.querySelector<HTMLElement>(".guide-jump button");
          if (jump) {
            handled();
            focusRing(jump);
            return;
          }
        }
        handled();
        move(focus.row - 1, focus.at);
        break;
      case "ArrowRight": {
        handled();
        const next = cur ? Date.parse(cur.end) : focus.at + 30 * MIN;
        move(focus.row, next + 1);
        break;
      }
      case "ArrowLeft": {
        handled();
        const prev = cur ? Date.parse(cur.start) - 1 : focus.at - 30 * MIN;
        move(focus.row, prev);
        break;
      }
      case "PageDown":
      case "PageUp": {
        handled();
        const el = scrollRef.current;
        move(guidePageRow(focus.row, key === "PageDown" ? 1 : -1, el?.clientHeight ?? 600, rowH, headH, rows.length), focus.at);
        break;
      }
      case "Home":
        handled();
        if (e.ctrlKey || e.metaKey) move(0, now);
        else {
          scrollToTime(now - 30 * MIN, "auto");
          move(focus.row, now);
        }
        break;
      case "End": {
        handled();
        if (e.ctrlKey || e.metaKey) {
          move(rows.length - 1, focus.at);
          break;
        }
        const list = index.get(row.id) ?? [];
        const last = list[list.length - 1];
        move(focus.row, last ? Date.parse(last.start) : focus.at);
        break;
      }
      case "Enter": {
        handled();
        const airingNow = !!cur && Date.parse(cur.start) <= now && Date.parse(cur.end) > now;
        if (guideEnterAction(tv, airingNow, false) === "watch") {
          // Tune on release. Held past half a second, OK opens the details instead.
          if (e.repeat || enterHeld.current) break;
          const slot = { timer: 0, fired: false };
          slot.timer = window.setTimeout(() => {
            slot.fired = true;
            sheetFromGrid.current = true;
            open(row, cur);
          }, 500);
          enterHeld.current = slot;
          break;
        }
        sheetFromGrid.current = true;
        open(row, cur);
        break;
      }
      case " ":
        handled();
        watch(row);
        break;
      case "f":
        handled();
        void favorite(row);
        break;
      case "h":
        handled();
        void editChannel(row, { hidden: true });
        break;
      default:
        if (isInfoKey(key)) {
          handled();
          sheetFromGrid.current = true;
          open(row, cur);
        }
    }
  }

  const firstRow = Math.max(0, Math.floor(view.top / rowH) - 3);
  const lastRow = Math.min(rows.length, Math.ceil((view.top + view.height) / rowH) + 3);
  const leftT = origin + ((view.left - 200) / pxPerMin) * MIN;
  const rightT = origin + ((view.left + view.width) / pxPerMin) * MIN;
  const nowX = ((now - origin) / MIN) * pxPerMin;
  const slots = Array.from({ length: hours * 2 }, (_, i) => origin + i * 30 * MIN).filter((t) => t > leftT - 60 * MIN && t < rightT + 60 * MIN);
  // The now line sits at channelW + nowX on the canvas, and the channel column covers the scroller's first channelW.
  const nowInView = nowX - view.left > 8 && nowX - view.left < view.width - channelW - 24;
  const focusRow = rows[focus.row];
  const focusAiring = focusRow ? airingAt(index, focusRow.id, focus.at) : undefined;
  const activeId = focusRow ? guideCellId(focusRow.id, focusAiring?.id) : undefined;

  // A filter can move the kept row outside the mounted slice. Scroll before the
  // cell lookup, which returns when that row is not in the document yet.
  useLayoutEffect(() => {
    const el = scrollRef.current;
    if (!el || el.clientHeight < 80) return;
    const top = guideRowScroll(el.scrollTop, focus.row, rowH, el.clientHeight, headH);
    if (top !== el.scrollTop) el.scrollTop = top;
  }, [focus.row, rowH, headH]);

  // The keyboard cell is the active descendant. A gap has no program block, so that
  // cell can sit past the last listing, outside the window move() aimed at the time.
  useLayoutEffect(() => {
    const el = scrollRef.current;
    if (!el || el.clientWidth < 80 || el.clientHeight < 80) return;
    const cell = activeId ? document.getElementById(activeId) : null;
    if (!cell) return;
    const box = el.getBoundingClientRect();
    const cellBox = cell.getBoundingClientRect();
    if (cellBox.width < 1 || cellBox.height < 1) return;
    const pad = 12;
    const visibleLeft = box.left + channelW + pad;
    const visibleRight = box.right - pad;
    const visibleTop = box.top + headH + pad;
    const visibleBottom = box.bottom - pad;
    // A show can be wider than the screen. On screen means the cell meets the grid.
    const seen =
      cellBox.bottom > visibleTop && cellBox.top < visibleBottom && cellBox.right > visibleLeft && cellBox.left < visibleRight;
    if (seen) return;
    const slip = Math.min(cellBox.width, 80);
    let dx = 0;
    let dy = 0;
    if (cellBox.right <= visibleLeft) dx = cellBox.right - visibleLeft - slip;
    else if (cellBox.left >= visibleRight) dx = cellBox.left - visibleRight + slip;
    if (cellBox.bottom <= visibleTop) dy = cellBox.bottom - visibleTop - Math.min(cellBox.height, 40);
    else if (cellBox.top >= visibleBottom) dy = cellBox.top - visibleBottom + Math.min(cellBox.height, 40);
    if (dx !== 0) el.scrollLeft += dx;
    if (dy !== 0) el.scrollTop += dy;
  }, [activeId, focus.row, focus.at, layout, view.width, view.height, channelW, headH]);

  if (!grid) {
    return (
      <div className="guide-page phone">
        <h1 className="sr-only">{t("guideUi.guide")}</h1>
        <GuideControls
          filter={filter}
          setFilter={setFilter}
          query={query}
          setQuery={setQuery}
          counts={counts}
          pressed={when}
          onNow={() => setWhen("now")}
          onTonight={() => setWhen("tonight")}
          compact
        />
        {noListings ? <NoListings antenna={antenna} /> : null}
        {rows.length === 0 ? <NoMatch filter={filter} query={query} onClear={clearFilters} /> : null}
        <ul className="onnow-list">
          {rows.map((c) => {
            const at = when === "tonight" ? primeTime(now) : now;
            const a = airingAt(index, c.id, at);
            const n = nextAfter(index, c.id, a ? Date.parse(a.end) : at);
            const rec = a ? isRecording(keys, a, now) : null;
            return (
              <li key={c.id}>
                <button type="button" className="onnow-row" data-cat={categoryOf(a)} onClick={() => open(c, a)}>
                  <ChannelBadge channel={c} size="sm" />
                  <span className="onnow-body">
                    <span className="onnow-title">
                      {rec ? <RecDot scheduled={rec === "scheduled"} /> : null}
                      {a?.title ?? emptyGuideLabel(index.get(c.id) ?? [], at)}
                    </span>
                    <Progress value={progress(a, now)} category={categoryOf(a)} />
                    {n ? (
                      <span className="onnow-next">
                        {timeLabel(n.start)} {n.title}
                      </span>
                    ) : null}
                  </span>
                </button>
              </li>
            );
          })}
        </ul>
        {sheet ? (
          <Suspense fallback={null}>
            <ProgramSheet {...sheet} onClose={closeSheet} onWatch={watch} />
          </Suspense>
        ) : null}
      </div>
    );
  }

  return (
    <div className="guide-page">
      <h1 className="sr-only">{t("guideUi.guide")}</h1>
      <a
        className="skip-link"
        href="#guide-grid"
        onClick={(event) => {
          event.preventDefault();
          focusRing(document.querySelector<HTMLElement>(".guide-canvas"));
        }}
      >
        {t("guideUi.skipToGrid")}
      </a>
      <GuideControls
        filter={filter}
        setFilter={setFilter}
        query={query}
        setQuery={setQuery}
        counts={counts}
        days={days}
        now={now}
        onNow={jumpToNow}
        onTonight={tonight}
        onDay={jumpToDay}
      />
      {noListings ? <NoListings antenna={antenna} /> : null}
      {rows.length === 0 ? <NoMatch filter={filter} query={query} onClear={clearFilters} /> : null}
      <div
        ref={scrollRef}
        className="guide-scroll"
        hidden={rows.length === 0}
        onKeyDown={onKey}
        onKeyUp={onKeyUp}
        style={{ ["--row" as string]: `${rowH}px`, ["--channel" as string]: `${channelW}px`, ["--head" as string]: `${headH}px` }}
      >
        <div
          className="guide-canvas"
          id="guide-grid"
          role="grid"
          aria-label={t("guideUi.tvGuide")}
          aria-rowcount={rows.length}
          aria-activedescendant={activeId}
          tabIndex={0}
          style={{ width: channelW + width, height: headH + rows.length * rowH }}
        >
          <div className="guide-times glass-flat" aria-hidden="true" style={{ width: channelW + width }}>
            <div className="guide-corner">{formatDateTime(new Date(Math.max(leftT, origin)), { weekday: "short", month: "short", day: "numeric" })}</div>
            {slots.map((t) => {
              const x = ((t - origin) / MIN) * pxPerMin;
              return (
                <div key={t} className={slotUnderNow(x, nowX, tv ? 60 : 40, tv ? 110 : 64) ? "guide-slot under-now" : "guide-slot"} style={{ left: channelW + x, width: 30 * pxPerMin }}>
                  <span className="guide-slot-label">{timeLabel(t)}</span>
                </div>
              );
            })}
            <div className="now-flag" style={{ left: channelW + nowX }}>
              {timeLabel(now)}
            </div>
          </div>
          <div className="now-line" style={{ left: channelW + nowX, top: headH, height: rows.length * rowH }} aria-hidden="true" />
          {rows.slice(firstRow, lastRow).map((c, i) => {
            const r = firstRow + i;
            const programs = index.get(c.id) ?? [];
            const list = programs.filter((a) => Date.parse(a.end) > leftT && Date.parse(a.start) < rightT);
            const gap = r === focus.row && !focusAiring;
            const gapLabel = emptyGuideLabel(programs, focus.at);
            const trailId = !gap && list.length > 0 && activeId === guideCellId(c.id) ? activeId : undefined;
            return (
              <div key={c.id} className="guide-row" role="row" aria-rowindex={r + 1} style={{ top: headH + r * rowH, width: channelW + width }}>
                <div role="rowheader" className="guide-rowhead">
                <button
                  type="button"
                  className="guide-channel"
                  // The canvas is the remote stop. A name in the arrow order traps Down in this column.
                  tabIndex={layout === "tv" ? -1 : 0}
                  draggable
                  onDragStart={(event) => {
                    event.dataTransfer.setData("text/plain", String(c.id));
                    event.dataTransfer.effectAllowed = "move";
                  }}
                  onDragOver={(event) => event.preventDefault()}
                  onDrop={(event) => {
                    event.preventDefault();
                    reorder(Number(event.dataTransfer.getData("text/plain")), c.id);
                  }}
                  onContextMenu={(event) => {
                    event.preventDefault();
                    void editChannel(c, { hidden: true });
                  }}
                  onClick={() => watch(c)}
                  aria-label={channelWatchName(c.displayNumber, c.displayName, { atsc3: c.standard === "atsc3", favorite: c.favorite })}
                  title={t("guideUi.reorderHint")}
                >
                  <span className="gc-num">{c.displayNumber}</span>
                  <span className="gc-body">
                    <span className="gc-name">
                      {c.artUrl ? <img className="gc-logo" alt="" src={`/media/art/channel/${c.id}?w=72`} onLoad={(event) => markLogoTone(event.currentTarget)} /> : null}
                      <span className="gc-text" title={c.displayName}>
                        {c.displayName}
                      </span>
                    </span>
                    {c.standard === "atsc3" ? (
                      <span className="gc-meta">
                        <Atsc3Tag />
                      </span>
                    ) : null}
                  </span>
                  <span className="gc-fav" aria-hidden="true">
                    {c.favorite ? <StarIcon filled className="gc-star" /> : null}
                  </span>
                </button>
                </div>
                {list.length === 0 ? (
                  // An empty stretch opens the channel's sheet, where Record a time is.
                  <div role="gridcell" id={activeId === guideCellId(c.id) ? activeId : undefined} className="guide-cell empty" style={{ left: channelW + view.left + 4, width: Math.max(200, view.width - channelW - 8) }} onClick={() => open(c)}>
                    <LiveFrame id={c.id} className="cell-frame" />
                    <span className="cell-title">{emptyGuideLabel(programs, leftT)}</span>
                  </div>
                ) : null}
                {gap && list.length > 0 ? (
                  <div
                    role="gridcell"
                    id={guideCellId(c.id)}
                    className="guide-cell empty focused"
                    aria-label={`${gapLabel}, ${c.displayName}`}
                    onClick={() => open(c)}
                    style={{ left: channelW + ((Math.max(origin, focus.at) - origin) / MIN) * pxPerMin, width: Math.max(160, 24 * pxPerMin) }}
                  >
                    <span className="cell-title">{gapLabel}</span>
                  </div>
                ) : null}
                {list.length > 0 && !gap && Date.parse(programs[programs.length - 1].end) < end - 60_000
                  ? (() => {
                      // The note runs to the end of the grid and starts at the visible edge, as a title does.
                      // With no program in view, the empty cell above already says it.
                      const left = channelW + ((Math.max(origin, Date.parse(programs[programs.length - 1].end)) - origin) / MIN) * pxPerMin + 4;
                      const w = Math.max(280, channelW + width - left - 4);
                      const inset = Math.min(Math.max(0, view.left + channelW - left), w - 280);
                      return (
                        <div role="gridcell" id={trailId} className="guide-cell empty" style={{ left, width: w, paddingLeft: 14 + inset }} onClick={() => open(c)}>
                          <span className="cell-title">{emptyGuideLabel(programs, end)}</span>
                        </div>
                      );
                    })()
                  : null}
                {list.map((a) => {
                  const s = Math.max(origin, Date.parse(a.start));
                  const e2 = Math.min(end, Date.parse(a.end));
                  const left = channelW + ((s - origin) / MIN) * pxPerMin;
                  const w = Math.max(24, ((e2 - s) / MIN) * pxPerMin - 4);
                  const cat = categoryOf(a);
                  const onNow = Date.parse(a.start) <= now && Date.parse(a.end) > now;
                  const past = Date.parse(a.end) <= now;
                  const rec = isRecording(keys, a, now);
                  const focused = r === focus.row && Date.parse(a.start) <= focus.at && Date.parse(a.end) > focus.at;
                  const dim = filter !== "all" && filter !== "favorites" && filter !== "recording" && cat !== filter;
                  const hidden = view.left + channelW - left;
                  // The title starts at the visible edge. Capping the inset let it slide back under the channel name,
                  // but past the cell's end the padding widened the cell over what follows. Only the last sliver is capped.
                  const inset = hidden > 0 ? Math.min(hidden, Math.max(0, w - 26 - (a.imageUrl && w > 220 ? 64 : 0))) : 0;
                  return (
                    <button
                      key={a.id}
                      type="button"
                      role="gridcell"
                      id={guideCellId(c.id, a.id)}
                      tabIndex={-1}
                      className={`guide-cell${onNow ? " now" : ""}${past ? " past" : ""}${focused ? " focused" : ""}${dim ? " dim" : ""}${a.imageUrl && w > 220 ? " has-thumb" : ""}`}
                      data-cat={cat}
                      style={{ left, width: w, paddingLeft: 14 + inset, ["--p" as string]: onNow ? progress(a, now) : 0 }}
                      onClick={() => {
                        setFocus({ row: r, at: Math.max(now, Date.parse(a.start)), id: c.id });
                        open(c, a);
                      }}
                      aria-label={guideCellName(a.title, spanLabel(a), c.displayName, {
                        recording: rec,
                        fresh: a.new,
                        score: a.gameId ? scores.get(a.gameId) : undefined,
                      })}
                    >
                      <span className="cell-title">
                        {rec ? <RecDot scheduled={rec === "scheduled"} /> : null}
                        {a.title}
                        {a.gameId && scores.get(a.gameId) ? <span className="cell-score">{scores.get(a.gameId)}</span> : null}
                        {a.new ? <span className="cell-new">{t("guideUi.new")}</span> : null}
                      </span>
                      <span className="cell-sub">{a.subtitle || (w > 160 ? spanLabel(a) : timeLabel(a.start))}</span>
                      {a.imageUrl && w > 220 ? <img className="cell-thumb" alt="" loading="lazy" src={`/media/art/airing/${a.id}?w=96`} /> : null}
                      {onNow && !a.imageUrl && w > 220 ? <LiveFrame id={c.id} className="cell-thumb" /> : null}
                    </button>
                  );
                })}
              </div>
            );
          })}
        </div>
      </div>
      {virtuals.length > 0 && filter === "all" && !query ? (
        <div className="library-channels">
          <span className="lc-label">{t("guideUi.libraryChannels")}</span>
          {virtuals.map((v) => (
            <button key={v.id} type="button" className="chip" onClick={() => navigate(`/watch?virtual=${v.id}`)}>
              <span className="gc-num">{v.number}</span> {v.name}
            </button>
          ))}
        </div>
      ) : null}
      {nowInView || rows.length === 0 ? null : (
        <button
          type="button"
          className="guide-now-float pill-btn"
          onClick={jumpToNow}
          onKeyDown={(event) => {
            if (!tv || canonicalKey(event) !== "ArrowDown") return;
            event.preventDefault();
            focusRing(document.querySelector<HTMLElement>(".guide-canvas"));
          }}
        >
          {t("guideUi.now")}
        </button>
      )}
      {sheet ? (
        <Suspense fallback={null}>
          <ProgramSheet {...sheet} onClose={closeSheet} onWatch={watch} />
        </Suspense>
      ) : null}
    </div>
  );
}

const filterWords: Record<Filter, () => string> = {
  all: () => t("guideUi.nothingHint"),
  favorites: () => t("guideUi.noFavorites"),
  recording: () => t("guideUi.noRecording"),
  sports: () => t("guideUi.noCategory", { category: categoryLabel.sports }),
  news: () => t("guideUi.noCategory", { category: categoryLabel.news }),
  movies: () => t("guideUi.noCategory", { category: categoryLabel.movies }),
  kids: () => t("guideUi.noCategory", { category: categoryLabel.kids }),
  series: () => t("guideUi.noCategory", { category: categoryLabel.series }),
  other: () => t("guideUi.nothingHint"),
};

/** No channel has a listing. Channels still play; the guide needs a listings source. */
function NoListings({ antenna }: { antenna: boolean }) {
  return (
    <div className="banner-update guide-nolistings" role="status">
      <p>
        <strong>{t("guideUi.noListingsTitle")}</strong> {antenna ? t("guideUi.noListingsAntenna") : t("guideUi.noListingsHint")}
      </p>
      <button type="button" className="btn small" onClick={() => navigate("/settings/guide")}>
        {t("guideUi.guideSettings")}
      </button>
    </div>
  );
}

/** A filter or search with no rows says why and offers the way back. */
function NoMatch({ filter, query, onClear }: { filter: Filter; query: string; onClear: () => void }) {
  const q = query.trim();
  return (
    <div className="guide-empty" role="status">
      <Empty
        title={q ? t("guideUi.noMatchQuery", { query: q }) : t("guideUi.nothingMatches")}
        action={
          <div className="guide-empty-actions">
            <button type="button" className="btn primary" onClick={onClear}>
              {t("guideUi.clear")}
            </button>
            {q.length >= 2 ? (
              <button type="button" className="btn" onClick={() => navigate(`/search?q=${encodeURIComponent(q)}`)}>
                <SearchIcon /> {t("guideUi.searchEverything")}
              </button>
            ) : null}
          </div>
        }
      >
        {q ? t("guideUi.queryHint") : filterWords[filter]()}
      </Empty>
    </div>
  );
}

function GuideControls({
  filter,
  setFilter,
  query,
  setQuery,
  counts,
  days,
  now,
  onNow,
  onTonight,
  onDay,
  compact,
  pressed,
}: {
  filter: Filter;
  setFilter: (f: Filter) => void;
  query: string;
  setQuery: (q: string) => void;
  counts: Partial<Record<Category, number>>;
  days?: number[];
  now?: number;
  onNow: () => void;
  onTonight: () => void;
  onDay?: (midnight: number) => void;
  compact?: boolean;
  pressed?: "now" | "tonight";
}) {
  const tvLayout = useLayout() === "tv";
  const cats: Category[] = ["sports", "news", "movies", "kids"];
  // Down from Now returns to the grid. The canvas is one big target, so spatial
  // nav would rather stop on the chips or the floating Now.
  const enterGrid = (event: KeyboardEvent) => {
    if (!tvLayout || canonicalKey(event) !== "ArrowDown") return;
    event.preventDefault();
    focusRing(document.querySelector<HTMLElement>(".guide-canvas"));
  };
  return (
    <div className="guide-controls">
      <div className="guide-jump">
        <button type="button" className="pill-btn" aria-pressed={pressed ? pressed === "now" : undefined} onClick={onNow} onKeyDown={enterGrid}>
          {t("guideUi.now")}
        </button>
        <button type="button" className="pill-btn" aria-pressed={pressed ? pressed === "tonight" : undefined} onClick={onTonight}>
          {t("guideUi.tonight")}
        </button>
        {!compact && days && now != null
          ? days
              .filter((day) => !sameDay(day, now))
              .map((day) => (
                <button key={day} type="button" className="pill-btn" onClick={() => onDay?.(day)}>
                  {dayWord(day, now)}
                </button>
              ))
          : null}
      </div>
      <div className="guide-chips" role="group" aria-label={t("guideUi.filter")}>
        <Chip on={filter === "all"} onClick={() => setFilter("all")}>
          {t("guideUi.all")}
        </Chip>
        <Chip on={filter === "favorites"} onClick={() => setFilter("favorites")}>
          {t("guideUi.favorites")}
        </Chip>
        {cats.map((c) => (
          <Chip key={c} on={filter === c} onClick={() => setFilter(c)} count={counts[c]}>
            <span className="chip-swatch" data-cat={c} aria-hidden="true" />
            {categoryLabel[c]}
          </Chip>
        ))}
        <Chip on={filter === "recording"} onClick={() => setFilter("recording")}>
          {t("guideUi.recording")}
        </Chip>
      </div>
      <label className="guide-search">
        <SearchIcon />
        <input
          type="search"
          placeholder={t("guideUi.searchPlaceholder")}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && query.trim().length >= 2) navigate(`/search?q=${encodeURIComponent(query.trim())}`);
          }}
          aria-label={t("guideUi.searchLabel")}
        />
      </label>
    </div>
  );
}
