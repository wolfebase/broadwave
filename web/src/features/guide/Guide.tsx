import { useEffect, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { useData } from "../../app/data";
import { usePlayer } from "../../app/player";
import { navigate } from "../../app/router";
import { useLayout } from "../../app/layout";
import {
  airingAt,
  categoryLabel,
  categoryOf,
  isRecording,
  nextAfter,
  progress,
  recordingKeys,
  spanLabel,
  timeLabel,
  type Category,
} from "../../lib/guide";
import type { Airing, Channel } from "../../types";
import { SearchIcon, StarIcon } from "../../ui/icons";
import { ChannelBadge, Chip, Empty, Progress, RecDot } from "../../ui/primitives";
import { ProgramSheet } from "./ProgramSheet";
import "./guide.css";

type Filter = "all" | "favorites" | Category | "recording";
const MIN = 60_000;
const ORDER_KEY = "waveguide-guide-order";

function floorHalfHour(t: number) {
  const d = new Date(t);
  d.setSeconds(0, 0);
  d.setMinutes(d.getMinutes() < 30 ? 0 : 30);
  return d.getTime();
}

function loadOrder(): number[] {
  try {
    const raw = JSON.parse(localStorage.getItem(ORDER_KEY) || "[]") as unknown;
    return Array.isArray(raw) ? raw.filter((id): id is number => typeof id === "number") : [];
  } catch {
    return [];
  }
}

function dayWord(midnight: number, now: number) {
  const day = new Date(midnight);
  const today = new Date(now);
  if (day.toDateString() === today.toDateString()) return "Today";
  const tomorrow = new Date(now);
  tomorrow.setDate(tomorrow.getDate() + 1);
  if (day.toDateString() === tomorrow.toDateString()) return "Tomorrow";
  return day.toLocaleDateString([], { weekday: "short" });
}

export function Guide() {
  const { channels, index, now, planned, recordings, virtuals, favorite, editChannel } = useData();
  const player = usePlayer();
  const layout = useLayout();
  const [filter, setFilter] = useState<Filter>("all");
  const [query, setQuery] = useState("");
  const [sheet, setSheet] = useState<{ channel: Channel; airing?: Airing } | null>(null);
  const [focus, setFocus] = useState<{ row: number; at: number }>({ row: 0, at: now });
  const scrollRef = useRef<HTMLDivElement>(null);
  const [view, setView] = useState({ top: 0, left: 0, height: 800, width: 1200 });
  const [order, setOrder] = useState<number[]>(loadOrder);
  const [landscape, setLandscape] = useState(() => window.matchMedia("(orientation: landscape) and (max-height: 520px)").matches);

  const tv = layout === "tv";
  const rowH = tv ? 96 : 68;
  const pxPerMin = tv ? 9.6 : 6.4;
  const channelW = tv ? 280 : 220;
  const headH = 48;
  const origin = useMemo(() => floorHalfHour(now) - 30 * MIN, [Math.floor(now / (30 * MIN))]); // eslint-disable-line react-hooks/exhaustive-deps
  const hours = useMemo(() => {
    let latest = origin + 24 * 60 * MIN;
    for (const list of index.values()) {
      const last = list[list.length - 1];
      if (last) latest = Math.max(latest, Date.parse(last.end));
    }
    return Math.min(48, Math.max(24, Math.ceil((latest - origin) / (60 * MIN))));
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
    const rank = new Map(order.map((id, i) => [id, i]));
    return [...channels].sort((a, b) => (rank.get(a.id) ?? 10_000) - (rank.get(b.id) ?? 10_000) || a.id - b.id);
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
    const measure = () => setView({ top: el.scrollTop, left: el.scrollLeft, height: el.clientHeight, width: el.clientWidth });
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    el.addEventListener("scroll", measure, { passive: true });
    return () => {
      ro.disconnect();
      el.removeEventListener("scroll", measure);
    };
  }, [layout]);

  useEffect(() => {
    scrollToTime(now - 30 * MIN, "auto");
    // Only on first mount and layout changes.
  }, [layout]); // eslint-disable-line react-hooks/exhaustive-deps

  function scrollToTime(t: number, behavior: ScrollBehavior = "smooth") {
    const el = scrollRef.current;
    if (!el) return;
    el.scrollTo({ left: Math.max(0, ((t - origin) / MIN) * pxPerMin), behavior });
  }

  function tonight() {
    const d = new Date(now);
    d.setHours(20, 0, 0, 0);
    if (d.getTime() < now) d.setDate(d.getDate() + 1);
    scrollToTime(d.getTime() - 30 * MIN);
  }

  function jumpToDay(midnight: number) {
    const today = new Date(now);
    today.setHours(0, 0, 0, 0);
    if (midnight === today.getTime()) {
      scrollToTime(now - 30 * MIN);
      return;
    }
    const prime = new Date(midnight);
    prime.setHours(20, 0, 0, 0);
    scrollToTime(prime.getTime() - 30 * MIN);
  }

  function reorder(fromId: number, toId: number) {
    const ids = rows.map((c) => c.id);
    const from = ids.indexOf(fromId);
    const to = ids.indexOf(toId);
    if (from < 0 || to < 0 || from === to) return;
    const [moved] = ids.splice(from, 1);
    ids.splice(to, 0, moved);
    const rest = channels.map((c) => c.id).filter((id) => !ids.includes(id));
    const next = [...ids, ...rest];
    localStorage.setItem(ORDER_KEY, JSON.stringify(next));
    setOrder(next);
  }

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
    setFocus({ row: r, at: t });
    const el = scrollRef.current;
    if (!el) return;
    const y = r * rowH;
    if (y < el.scrollTop) el.scrollTop = y;
    else if (y + rowH > el.scrollTop + el.clientHeight - headH) el.scrollTop = y + rowH - el.clientHeight + headH;
    const x = ((t - origin) / MIN) * pxPerMin;
    if (x < el.scrollLeft + 40) el.scrollLeft = Math.max(0, x - 80);
    else if (x > el.scrollLeft + el.clientWidth - channelW - 120) el.scrollLeft = x - el.clientWidth + channelW + 240;
  }

  function onKey(e: KeyboardEvent) {
    const row = rows[focus.row];
    if (!row) return;
    const cur = airingAt(index, row.id, focus.at);
    const handled = () => e.preventDefault();
    switch (e.key) {
      case "ArrowDown":
        handled();
        move(focus.row + 1, focus.at);
        break;
      case "ArrowUp":
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
      case "Enter":
        handled();
        open(row, cur);
        break;
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
    }
  }

  const firstRow = Math.max(0, Math.floor(view.top / rowH) - 3);
  const lastRow = Math.min(rows.length, Math.ceil((view.top + view.height) / rowH) + 3);
  const leftT = origin + ((view.left - 200) / pxPerMin) * MIN;
  const rightT = origin + ((view.left + view.width) / pxPerMin) * MIN;
  const nowX = ((now - origin) / MIN) * pxPerMin;
  const slots = Array.from({ length: hours * 2 }, (_, i) => origin + i * 30 * MIN).filter((t) => t > leftT - 60 * MIN && t < rightT + 60 * MIN);
  const nowInView = nowX - view.left > channelW - 8 && nowX - view.left < view.width - 24;

  if (layout === "phone" && !landscape) {
    return (
      <div className="guide-page phone">
        <GuideControls filter={filter} setFilter={setFilter} query={query} setQuery={setQuery} counts={counts} onNow={() => undefined} onTonight={() => undefined} compact />
        {rows.length === 0 ? <Empty title="Nothing matches">Try another filter or search.</Empty> : null}
        <ul className="onnow-list">
          {rows.map((c) => {
            const a = airingAt(index, c.id, now);
            const n = nextAfter(index, c.id, a ? Date.parse(a.end) : now);
            const rec = a ? isRecording(keys, a, now) : null;
            return (
              <li key={c.id}>
                <button type="button" className="onnow-row" data-cat={categoryOf(a)} onClick={() => open(c, a)}>
                  <ChannelBadge channel={c} size="sm" />
                  <span className="onnow-body">
                    <span className="onnow-title">
                      {rec ? <RecDot scheduled={rec === "scheduled"} /> : null}
                      {a?.title ?? "No listing"}
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
        {sheet ? <ProgramSheet {...sheet} onClose={() => setSheet(null)} onWatch={watch} /> : null}
      </div>
    );
  }

  return (
    <div className="guide-page">
      <GuideControls
        filter={filter}
        setFilter={setFilter}
        query={query}
        setQuery={setQuery}
        counts={counts}
        days={days}
        now={now}
        onNow={() => scrollToTime(now - 30 * MIN)}
        onTonight={tonight}
        onDay={jumpToDay}
      />
      <div
        ref={scrollRef}
        className="guide-scroll"
        role="grid"
        aria-label="TV guide"
        aria-rowcount={rows.length}
        tabIndex={0}
        onKeyDown={onKey}
        style={{ ["--row" as string]: `${rowH}px`, ["--channel" as string]: `${channelW}px`, ["--head" as string]: `${headH}px` }}
      >
        <div className="guide-canvas" style={{ width: channelW + width, height: headH + rows.length * rowH }}>
          <div className="guide-times glass-flat" style={{ width: channelW + width }}>
            <div className="guide-corner">{new Date(Math.max(leftT, origin)).toLocaleDateString([], { weekday: "short", month: "short", day: "numeric" })}</div>
            {slots.map((t) => (
              <div key={t} className="guide-slot" style={{ left: channelW + ((t - origin) / MIN) * pxPerMin, width: 30 * pxPerMin }}>
                {timeLabel(t)}
              </div>
            ))}
            <div className="now-flag" style={{ left: channelW + nowX }}>
              {timeLabel(now)}
            </div>
          </div>
          <div className="now-line" style={{ left: channelW + nowX, top: headH, height: rows.length * rowH }} aria-hidden="true" />
          {rows.slice(firstRow, lastRow).map((c, i) => {
            const r = firstRow + i;
            const list = (index.get(c.id) ?? []).filter((a) => Date.parse(a.end) > leftT && Date.parse(a.start) < rightT);
            return (
              <div key={c.id} className="guide-row" role="row" aria-rowindex={r + 1} style={{ top: headH + r * rowH, width: channelW + width }}>
                <button
                  type="button"
                  className="guide-channel"
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
                  aria-label={`Watch ${c.displayNumber} ${c.displayName}`}
                  title="Drag to reorder. Right-click to hide."
                >
                  <span className="gc-num">{c.displayNumber}</span>
                  <span className="gc-name">
                    {c.artUrl ? <img className="gc-logo" alt="" src={`/media/art/channel/${c.id}?w=72`} /> : null}
                    {c.displayName}
                  </span>
                  {c.favorite ? <StarIcon filled className="gc-star" /> : null}
                </button>
                {list.length === 0 ? (
                  <div className="guide-cell empty" style={{ left: channelW + view.left + 4, width: Math.max(200, view.width - channelW - 8) }}>
                    <span className="cell-title">No listings</span>
                  </div>
                ) : null}
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
                  const inset = hidden > 0 ? Math.min(hidden, Math.max(0, w - 120)) : 0;
                  return (
                    <button
                      key={a.id}
                      type="button"
                      role="gridcell"
                      tabIndex={-1}
                      className={`guide-cell${onNow ? " now" : ""}${past ? " past" : ""}${focused ? " focused" : ""}${dim ? " dim" : ""}${a.imageUrl && w > 220 ? " has-thumb" : ""}`}
                      data-cat={cat}
                      style={{ left, width: w, paddingLeft: 14 + inset, ["--p" as string]: onNow ? progress(a, now) : 0 }}
                      onClick={() => {
                        setFocus({ row: r, at: Math.max(now, Date.parse(a.start)) });
                        open(c, a);
                      }}
                      aria-label={`${a.title}, ${spanLabel(a)}, ${c.displayName}`}
                    >
                      <span className="cell-title">
                        {rec ? <RecDot scheduled={rec === "scheduled"} /> : null}
                        {a.title}
                        {a.new ? <span className="cell-new">New</span> : null}
                      </span>
                      <span className="cell-sub">{a.subtitle || (w > 160 ? spanLabel(a) : timeLabel(a.start))}</span>
                      {a.imageUrl && w > 220 ? <img className="cell-thumb" alt="" loading="lazy" src={`/media/art/airing/${a.id}?w=96`} /> : null}
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
          <span className="lc-label">Library channels</span>
          {virtuals.map((v) => (
            <button key={v.id} type="button" className="chip" onClick={() => navigate(`/watch?virtual=${v.id}`)}>
              <span className="gc-num">{v.number}</span> {v.name}
            </button>
          ))}
        </div>
      ) : null}
      {nowInView ? null : (
        <button type="button" className="guide-now-float pill-btn" onClick={() => scrollToTime(now - 30 * MIN)}>
          Now
        </button>
      )}
      {sheet ? <ProgramSheet {...sheet} onClose={() => setSheet(null)} onWatch={watch} /> : null}
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
}) {
  const cats: Category[] = ["sports", "news", "movies", "kids"];
  return (
    <div className="guide-controls">
      {!compact ? (
        <div className="guide-jump">
          <button type="button" className="pill-btn" onClick={onNow}>
            Now
          </button>
          <button type="button" className="pill-btn" onClick={onTonight}>
            Tonight
          </button>
          {days && now != null
            ? days
                .filter((day) => dayWord(day, now) !== "Today")
                .map((day) => (
                  <button key={day} type="button" className="pill-btn" onClick={() => onDay?.(day)}>
                    {dayWord(day, now)}
                  </button>
                ))
            : null}
        </div>
      ) : null}
      <div className="guide-chips" role="group" aria-label="Filter">
        <Chip on={filter === "all"} onClick={() => setFilter("all")}>
          All
        </Chip>
        <Chip on={filter === "favorites"} onClick={() => setFilter("favorites")}>
          Favorites
        </Chip>
        {cats.map((c) => (
          <Chip key={c} on={filter === c} onClick={() => setFilter(c)} count={counts[c]}>
            <span className="chip-swatch" data-cat={c} aria-hidden="true" />
            {categoryLabel[c]}
          </Chip>
        ))}
        <Chip on={filter === "recording"} onClick={() => setFilter("recording")}>
          Recording
        </Chip>
      </div>
      <label className="guide-search">
        <SearchIcon />
        <input
          type="search"
          placeholder="Search shows and channels"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && query.trim().length >= 2) navigate(`/search?q=${encodeURIComponent(query.trim())}`);
          }}
          aria-label="Search the guide"
        />
      </label>
    </div>
  );
}
