import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import type { Airing, Channel, Recording, VirtualChannel } from "./types";
import { copy } from "./strings";
import { SLOT_COUNT, SLOT_MINUTES, addMinutes, formatClock, halfHourFloor } from "./time";

type Filter = "all" | "favorites" | "hd";
type Focus = { row: number; col: number };
type GuideRow = { kind: "live"; channel: Channel } | { kind: "library"; virtual: VirtualChannel };

export function Guide({
  channels,
  virtuals,
  recordings,
  layout,
  airings,
  onFavorite,
  onWatch,
  onWatchVirtual,
  onRecord,
  onPass,
}: {
  channels: Channel[];
  virtuals: VirtualChannel[];
  recordings: Recording[];
  layout: "desktop" | "tv" | "phone";
  airings: Airing[];
  onFavorite: (channel: Channel) => Promise<Channel>;
  onWatch: (channel: Channel) => void;
  onWatchVirtual: (virtual: VirtualChannel) => void;
  onRecord: (channel: Channel, title: string) => void;
  onPass: (title: string, channel: Channel) => void;
}) {
  const [filter, setFilter] = useState<Filter>("all");
  const [query, setQuery] = useState("");
  const [hour, setHour] = useState<"now" | "tonight">("now");
  const [now, setNow] = useState(() => new Date());
  const [focus, setFocus] = useState<Focus>({ row: 0, col: 0 });
  const [sheet, setSheet] = useState<GuideRow | null>(null);
  const [sheetAt, setSheetAt] = useState(() => new Date());
  const [scrollTop, setScrollTop] = useState(0);
  const [viewH, setViewH] = useState(640);
  const gridRef = useRef<HTMLDivElement>(null);
  const sheetRef = useRef<HTMLDivElement>(null);
  const typed = useRef("");
  const typedTimer = useRef(0);

  const rowH = layout === "tv" ? 84 : 56;
  const slotW = layout === "tv" ? 210 : 168;
  const channelW = layout === "tv" ? 300 : 240;
  const headH = 44;

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    const live = channels.filter((channel) => {
      if (filter === "favorites" && !channel.favorite) return false;
      if (filter === "hd" && !channel.hd) return false;
      return matchesLive(channel, airings, q);
    });
    const library =
      filter === "all" || q !== ""
        ? [...virtuals]
            .sort((a, b) => numberValue(a.number) - numberValue(b.number))
            .filter((virtual) => matchesLibrary(virtual, recordings, q))
            .map((virtual): GuideRow => ({ kind: "library", virtual }))
        : [];
    return [...live.map((channel): GuideRow => ({ kind: "live", channel })), ...library];
  }, [channels, virtuals, recordings, airings, filter, query]);

  useEffect(() => {
    const id = window.setInterval(() => setNow(new Date()), 30_000);
    return () => window.clearInterval(id);
  }, []);

  useEffect(() => {
    setFocus((prev) => ({
      row: Math.min(prev.row, Math.max(0, filtered.length - 1)),
      col: prev.col,
    }));
  }, [filtered.length]);

  useEffect(() => {
    const el = gridRef.current;
    if (!el || layout === "phone") return;
    const onScroll = () => setScrollTop(el.scrollTop);
    el.addEventListener("scroll", onScroll, { passive: true });
    const observer = new ResizeObserver(() => setViewH(el.clientHeight));
    observer.observe(el);
    setViewH(el.clientHeight);
    return () => {
      el.removeEventListener("scroll", onScroll);
      observer.disconnect();
    };
  }, [layout, filtered.length]);

  useEffect(() => {
    if (sheet) sheetRef.current?.focus();
  }, [sheet]);

  const start = halfHourFloor(now);
  const tonight = new Date(now);
  tonight.setHours(19, 0, 0, 0);

  function scrollGuide(next: "now" | "tonight") {
    setHour(next);
    const el = gridRef.current;
    if (!el) return;
    const target = next === "now" ? start : tonight;
    const delta = target.getTime() - start.getTime();
    const span = SLOT_COUNT * SLOT_MINUTES * 60_000;
    el.scrollLeft = delta > 0 && delta < span ? (delta / (SLOT_MINUTES * 60_000)) * slotW : 0;
  }
  const nowX = ((now.getTime() - start.getTime()) / (SLOT_MINUTES * 60_000)) * slotW;
  const active = filtered[focus.row];

  function openRow(row: GuideRow, at?: Date) {
    setSheetAt(at ?? (hour === "tonight" ? tonight : new Date()));
    setSheet(row);
  }

  async function toggleFavorite(channel: Channel) {
    const next = await onFavorite(channel);
    setSheet((current) => (current && current.kind === "live" && current.channel.id === next.id ? { kind: "live", channel: next } : current));
  }

  function move(next: Focus) {
    const row = Math.max(0, Math.min(filtered.length - 1, next.row));
    const col = Math.max(0, Math.min(SLOT_COUNT - 1, next.col));
    setFocus({ row, col });
    const el = gridRef.current;
    if (!el) return;
    const top = row * rowH;
    const bottom = top + rowH;
    const visibleBottom = el.scrollTop + el.clientHeight - headH;
    if (top < el.scrollTop) el.scrollTop = top;
    else if (bottom > visibleBottom) el.scrollTop = bottom - el.clientHeight + headH;
    const left = col * slotW;
    const right = left + slotW;
    const viewLeft = el.scrollLeft;
    const viewRight = el.scrollLeft + el.clientWidth - channelW;
    if (left < viewLeft) el.scrollLeft = left;
    else if (right > viewRight) el.scrollLeft = right - (el.clientWidth - channelW);
  }

  function jumpToNumber(digits: string) {
    const exact = filtered.findIndex((row) => rowNumber(row) === digits);
    const hit = exact >= 0 ? exact : filtered.findIndex((row) => rowNumber(row).startsWith(digits));
    if (hit >= 0) move({ row: hit, col: focus.col });
  }

  function onKeyDown(event: KeyboardEvent) {
    if (sheet) {
      if (event.key === "Escape") {
        event.preventDefault();
        setSheet(null);
        gridRef.current?.focus();
      }
      return;
    }
    if (/^[0-9.]$/.test(event.key)) {
      event.preventDefault();
      typed.current += event.key;
      window.clearTimeout(typedTimer.current);
      typedTimer.current = window.setTimeout(() => {
        typed.current = "";
      }, 1200);
      jumpToNumber(typed.current);
      return;
    }
    if (!active) return;
    if (event.key === "ArrowDown") {
      event.preventDefault();
      move({ row: focus.row + 1, col: focus.col });
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      move({ row: focus.row - 1, col: focus.col });
    } else if (event.key === "ArrowRight") {
      event.preventDefault();
      move({ row: focus.row, col: focus.col + 1 });
    } else if (event.key === "ArrowLeft") {
      event.preventDefault();
      move({ row: focus.row, col: focus.col - 1 });
    } else if (event.key === "Enter") {
      event.preventDefault();
      openRow(active, hour === "tonight" ? tonight : addMinutes(start, focus.col * SLOT_MINUTES));
    } else if ((event.key === "f" || event.key === "F") && active.kind === "live") {
      event.preventDefault();
      void toggleFavorite(active.channel);
    } else if (event.key === "r" || event.key === "R") {
      event.preventDefault();
      openRow(active, hour === "tonight" ? tonight : addMinutes(start, focus.col * SLOT_MINUTES));
    }
  }

  const first = Math.max(0, Math.floor(scrollTop / rowH) - 4);
  const last = Math.min(filtered.length, Math.ceil((scrollTop + viewH) / rowH) + 6);
  const focusInWindow = focus.row >= first && focus.row < last;
  const startRow = focusInWindow ? first : Math.min(first, focus.row);
  const endRow = focusInWindow ? last : Math.max(last, focus.row + 1);

  return (
    <section className="page guide-page">
      <div className="page-head">
        <div>
          <h2>{copy.nav.guide}</h2>
          <p className="lede">{copy.guide.help}</p>
        </div>
        <div className="segmented" role="tablist" aria-label="Channel filter">
          {(["all", "favorites", "hd"] as const).map((key) => (
            <button
              key={key}
              type="button"
              role="tab"
              aria-selected={filter === key}
              className={filter === key ? "seg on" : "seg"}
              onClick={() => setFilter(key)}
            >
              {copy.guide.filters[key]}
            </button>
          ))}
        </div>
      </div>
      <div className="segmented" role="group" aria-label="Guide time">
        <button type="button" className={hour === "now" ? "seg on" : "seg"} onClick={() => scrollGuide("now")}>
          Now
        </button>
        <button type="button" className={hour === "tonight" ? "seg on" : "seg"} onClick={() => scrollGuide("tonight")}>
          Tonight
        </button>
      </div>
      <label className="lookup guide-search">
        Search
        <input
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder="Title, description, or channel"
          aria-label="Search titles, descriptions, and channels"
        />
      </label>

      {layout === "phone" ? (
        <PhoneList
          rows={filtered}
          airings={airings}
          when={hour === "tonight" ? tonight : null}
          recordings={recordings}
          focusRow={focus.row}
          onFocusRow={(row) => setFocus({ row, col: 0 })}
          onOpen={(row) => openRow(row)}
          onFavorite={(channel) => void toggleFavorite(channel)}
          onKeyDown={onKeyDown}
        />
      ) : filtered.length === 0 ? (
        <p className="empty">{copy.guide.empty}</p>
      ) : (
        <div
          className="guide-scroll"
          ref={gridRef}
          tabIndex={0}
          role="grid"
          aria-label={copy.nav.guide}
          aria-rowcount={filtered.length}
          aria-activedescendant={active ? `cell-${focus.row}-${focus.col}` : undefined}
          onKeyDown={onKeyDown}
          style={{ ["--channel" as string]: `${channelW}px`, ["--slot" as string]: `${slotW}px`, ["--row" as string]: `${rowH}px` }}
        >
          <div className="guide-head" role="row">
            <div className="corner">{copy.guide.channel}</div>
            <div className="times" style={{ width: SLOT_COUNT * slotW }}>
              {Array.from({ length: SLOT_COUNT }, (_, index) => {
                const at = addMinutes(start, index * SLOT_MINUTES);
                return (
                  <div key={index} className="time-label" style={{ left: index * slotW, width: slotW }}>
                    {formatClock(at)}
                  </div>
                );
              })}
              <div className="now-flag" style={{ left: nowX }}>
                {copy.guide.now}
              </div>
            </div>
          </div>
          <div className="guide-body" style={{ height: filtered.length * rowH, width: channelW + SLOT_COUNT * slotW }}>
            <div className="now-line" style={{ left: channelW + nowX }} />
            {filtered.slice(startRow, endRow).map((row, offset) => {
              const index = startRow + offset;
              return (
                <div key={rowKey(row)} className="guide-row" role="row" style={{ top: index * rowH, height: rowH }}>
                  <div className="channel-cell" role="rowheader">
                    <span className="ch-num">{rowNumber(row)}</span>
                    <span className="ch-copy">
                      <span className="ch-name">{rowName(row)}</span>
                      <span className="ch-tags">{row.kind === "live" ? liveTags(row.channel) : "Library"}</span>
                    </span>
                    {row.kind === "live" ? (
                      <button
                        type="button"
                        className={row.channel.favorite ? "star on" : "star"}
                        aria-pressed={row.channel.favorite}
                        aria-label={row.channel.favorite ? copy.guide.favoriteOn : copy.guide.favoriteOff}
                        onClick={(event) => {
                          event.stopPropagation();
                          void toggleFavorite(row.channel);
                        }}
                      >
                        <Star filled={row.channel.favorite} />
                      </button>
                    ) : null}
                  </div>
                  <div className="track">
                    {row.kind === "live"
                      ? airings
                          .filter((airing) => airing.channelId === row.channel.id)
                          .map((airing) => {
                            const box = airingBox(airing, start, slotW, SLOT_COUNT);
                            if (!box) return null;
                            return (
                              <button key={airing.id} type="button" className="program" style={{ left: box.left, width: box.width }} onClick={() => openRow(row, new Date(airing.start))}>
                                <strong>{airing.title}</strong>
                                {airing.subtitle ? <span>{airing.subtitle}</span> : null}
                              </button>
                            );
                          })
                      : libraryClock(row.virtual, recordings, start).map((airing) => {
                          const box = airingBox(airing, start, slotW, SLOT_COUNT);
                          if (!box) return null;
                          return (
                            <button key={airing.id} type="button" className="program library" style={{ left: box.left, width: box.width }} onClick={() => openRow(row, new Date(airing.start))}>
                              <strong>{airing.title}</strong>
                              <span>No tuner</span>
                            </button>
                          );
                        })}
                    {Array.from({ length: SLOT_COUNT }, (_, col) => {
                      const selected = focus.row === index && focus.col === col;
                      const at = addMinutes(start, col * SLOT_MINUTES);
                      const emptyLive = row.kind === "live" && !airings.some((airing) => airing.channelId === row.channel.id);
                      return (
                        <button
                          key={col}
                          id={`cell-${index}-${col}`}
                          type="button"
                          role="gridcell"
                          tabIndex={-1}
                          className={selected ? "slot selected" : "slot"}
                          aria-selected={selected}
                          aria-label={`${rowNumber(row)} ${rowName(row)} ${formatClock(at)}`}
                          onClick={() => {
                            setFocus({ row: index, col });
                            gridRef.current?.focus();
                          }}
                          onDoubleClick={() => openRow(row, addMinutes(start, col * SLOT_MINUTES))}
                        >
                          {col === 0 && emptyLive ? <span className="slot-note">{copy.guide.noListings}</span> : null}
                        </button>
                      );
                    })}
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {sheet ? (
        <div className="sheet-backdrop" onMouseDown={() => setSheet(null)}>
          <div
            className="sheet"
            role="dialog"
            aria-modal="true"
            aria-labelledby="sheet-title"
            tabIndex={-1}
            ref={sheetRef}
            onMouseDown={(event) => event.stopPropagation()}
            onKeyDown={onKeyDown}
          >
            {sheet.kind === "live" ? (
              <>
                <p className="kicker">
                  {sheet.channel.displayNumber}
                  {sheet.channel.hd ? " · HD" : ""}
                  {sheet.channel.videoCodec ? ` · ${labelCodec(sheet.channel.videoCodec)}` : ""}
                  {sheet.channel.audioCodec ? ` · ${labelCodec(sheet.channel.audioCodec)}` : ""}
                </p>
                <h3 id="sheet-title">{sheet.channel.displayName}</h3>
                <LiveDetails
                  channel={sheet.channel}
                  airing={airingAt(airings, sheet.channel.id, sheetAt)}
                  onWatch={() => onWatch(sheet.channel)}
                  onRecord={(title) => onRecord(sheet.channel, title)}
                  onPass={(title) => onPass(title, sheet.channel)}
                  onFavorite={() => void toggleFavorite(sheet.channel)}
                />
              </>
            ) : (
              <>
                <p className="kicker">{sheet.virtual.number} · Library</p>
                <h3 id="sheet-title">{sheet.virtual.name}</h3>
                <p>{recordingTitles(sheet.virtual, recordings) || "No recordings are on this channel yet."}</p>
                <div className="sheet-actions">
                  <button type="button" className="btn primary" onClick={() => onWatchVirtual(sheet.virtual)} disabled={sheet.virtual.recordings.length === 0}>
                    {copy.guide.watch}
                  </button>
                </div>
                <p className="hint">This channel plays recordings already on disk. It does not use an antenna tuner.</p>
              </>
            )}
          </div>
        </div>
      ) : null}
    </section>
  );
}

function PhoneList({
  rows,
  airings,
  when,
  recordings,
  focusRow,
  onFocusRow,
  onOpen,
  onFavorite,
  onKeyDown,
}: {
  rows: GuideRow[];
  airings: Airing[];
  when: Date | null;
  recordings: Recording[];
  focusRow: number;
  onFocusRow: (row: number) => void;
  onOpen: (row: GuideRow) => void;
  onFavorite: (channel: Channel) => void;
  onKeyDown: (event: KeyboardEvent) => void;
}) {
  if (rows.length === 0) return <p className="empty">{copy.guide.empty}</p>;
  return (
    <div className="phone-list" role="listbox" aria-label={copy.nav.guide} tabIndex={0} onKeyDown={onKeyDown}>
      {rows.map((row, index) => (
        <div key={rowKey(row)} className={index === focusRow ? "phone-row selected" : "phone-row"} role="option" aria-selected={index === focusRow}>
          <button
            type="button"
            className="phone-open"
            onClick={() => {
              onFocusRow(index);
              onOpen(row);
            }}
          >
            <span className="ch-num">{rowNumber(row)}</span>
            <span className="ch-copy">
              <span className="ch-name">{rowName(row)}</span>
              <span className="ch-tags">
                {row.kind === "live"
                  ? (when ? titleAt(airings, row.channel.id, when) : currentTitle(airings, row.channel.id)) || copy.guide.noListings
                  : recordingTitles(row.virtual, recordings) || "Recordings · no tuner"}
              </span>
            </span>
          </button>
          {row.kind === "live" ? (
            <button
              type="button"
              className={row.channel.favorite ? "star on" : "star"}
              aria-pressed={row.channel.favorite}
              aria-label={row.channel.favorite ? copy.guide.favoriteOn : copy.guide.favoriteOff}
              onClick={() => onFavorite(row.channel)}
            >
              <Star filled={row.channel.favorite} />
            </button>
          ) : null}
        </div>
      ))}
    </div>
  );
}

function Star({ filled }: { filled: boolean }) {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true" className="star-icon">
      <path
        d="M12 3.1 14.8 9l6.4.6-4.8 4.2 1.5 6.2L12 16.9 6.1 20l1.5-6.2L2.8 9.6 9.2 9z"
        fill={filled ? "currentColor" : "none"}
        stroke="currentColor"
        strokeWidth="1.4"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function rowKey(row: GuideRow) {
  return row.kind === "live" ? `live-${row.channel.id}` : `library-${row.virtual.id}`;
}

function rowNumber(row: GuideRow) {
  return row.kind === "live" ? row.channel.displayNumber : row.virtual.number;
}

function rowName(row: GuideRow) {
  return row.kind === "live" ? row.channel.displayName : row.virtual.name;
}

function liveTags(channel: Channel) {
  return `${channel.hd ? "HD" : "SD"}${channel.videoCodec ? ` · ${labelCodec(channel.videoCodec)}` : ""}`;
}

function recordingTitles(virtual: VirtualChannel, recordings: Recording[]) {
  return virtual.recordings
    .map((id) => recordings.find((recording) => recording.id === id)?.title)
    .filter((title): title is string => Boolean(title))
    .join(" · ");
}

function matchesLive(channel: Channel, airings: Airing[], query: string) {
  if (!query) return true;
  if (channel.displayNumber.toLowerCase().includes(query) || channel.displayName.toLowerCase().includes(query)) return true;
  return airings.some((airing) => airing.channelId === channel.id && `${airing.title} ${airing.subtitle ?? ""} ${airing.description ?? ""} ${airing.category ?? ""}`.toLowerCase().includes(query));
}

function matchesLibrary(virtual: VirtualChannel, recordings: Recording[], query: string) {
  if (!query) return true;
  if (virtual.number.toLowerCase().includes(query) || virtual.name.toLowerCase().includes(query)) return true;
  return recordingTitles(virtual, recordings).toLowerCase().includes(query);
}

function numberValue(value: string) {
  const [major, minor] = value.split(".");
  return (Number(major) || 0) * 1000 + (Number(minor) || 0);
}

function LiveDetails({
  channel,
  airing,
  onWatch,
  onRecord,
  onPass,
  onFavorite,
}: {
  channel: Channel;
  airing?: Airing;
  onWatch: () => void;
  onRecord: (title: string) => void;
  onPass: (title: string) => void;
  onFavorite: () => void;
}) {
  const title = airing?.title || "";
  return (
    <>
      <p>{title || copy.guide.sheetListings}</p>
      {airing ? (
        <p className="hint">
          {formatClock(new Date(airing.start))}–{formatClock(new Date(airing.end))}
          {airing.category ? ` · ${airing.category}` : ""}
        </p>
      ) : null}
      {airing?.subtitle ? <p>{airing.subtitle}</p> : null}
      {airing?.description ? <p className="blurb">{airing.description}</p> : null}
      <div className="sheet-actions">
        <button type="button" className="btn primary" onClick={onWatch}>
          {copy.guide.watch}
        </button>
        <button type="button" className="btn" onClick={() => onRecord(title || channel.displayName)}>
          {copy.guide.record}
        </button>
        <button type="button" className="btn" onClick={() => onPass(title)} disabled={!title}>
          Series pass
        </button>
        <button type="button" className="btn" onClick={onFavorite}>
          {channel.favorite ? copy.guide.favoriteOn : copy.guide.favoriteOff}
        </button>
      </div>
      <p className="hint">{copy.guide.watchWhy}</p>
      <p className="hint">{copy.guide.recordWhy}</p>
    </>
  );
}

function airingAt(airings: Airing[], channelId: number, at: Date): Airing | undefined {
  const t = at.getTime();
  return airings.find((airing) => airing.channelId === channelId && new Date(airing.start).getTime() <= t && new Date(airing.end).getTime() > t);
}

function titleAt(airings: Airing[], channelId: number, at: Date): string {
  const t = at.getTime();
  const hit = airings.find((airing) => airing.channelId === channelId && new Date(airing.start).getTime() <= t && new Date(airing.end).getTime() > t);
  return hit?.title ?? "";
}

function currentTitle(airings: Airing[], channelId: number): string {
  const now = Date.now();
  const hit = airings.find((airing) => airing.channelId === channelId && new Date(airing.start).getTime() <= now && new Date(airing.end).getTime() > now);
  return hit?.title ?? "";
}

function airingBox(airing: Airing, windowStart: Date, slotW: number, slots: number) {
  const startMs = new Date(airing.start).getTime();
  const endMs = new Date(airing.end).getTime();
  const origin = windowStart.getTime();
  const span = slots * 30 * 60_000;
  const leftMs = Math.max(startMs, origin);
  const rightMs = Math.min(endMs, origin + span);
  if (rightMs <= leftMs) return null;
  return {
    left: ((leftMs - origin) / (30 * 60_000)) * slotW + 4,
    width: Math.max(48, ((rightMs - leftMs) / (30 * 60_000)) * slotW - 8),
  };
}

function libraryClock(virtual: VirtualChannel, recordings: Recording[], origin: Date): Airing[] {
  let items = virtual.recordings
    .map((id) => recordings.find((rec) => rec.id === id))
    .filter((rec): rec is Recording => Boolean(rec));
  if (virtual.orderMode === "release") {
    items = [...items].sort((a, b) => a.startedAt.localeCompare(b.startedAt));
  } else if (virtual.orderMode === "season" || virtual.orderMode === "show") {
    items = [...items].sort((a, b) => (a.subtitle || a.title).localeCompare(b.subtitle || b.title));
  }
  if (items.length === 0) {
    return [{ id: virtual.id, channelId: 0, title: virtual.name || "Recordings", start: origin.toISOString(), end: new Date(origin.getTime() + 30 * 60_000).toISOString() }];
  }
  const out: Airing[] = [];
  let cursor = origin.getTime();
  const horizon = origin.getTime() + SLOT_COUNT * SLOT_MINUTES * 60_000;
  for (let i = 0; cursor < horizon && i < 200; i++) {
    const rec = items[i % items.length];
    const seconds = rec.durationSec && rec.durationSec > 60 ? rec.durationSec : 30 * 60;
    const end = cursor + seconds * 1000;
    out.push({
      id: virtual.id * 1000 + i,
      channelId: 0,
      title: rec.subtitle || rec.title,
      start: new Date(cursor).toISOString(),
      end: new Date(end).toISOString(),
    });
    cursor = end;
  }
  return out;
}

function labelCodec(value: string): string {
  if (value === "MPEG2") return "MPEG-2";
  if (value === "H264") return "H.264";
  if (value === "AC3") return "Dolby Digital";
  return value;
}
