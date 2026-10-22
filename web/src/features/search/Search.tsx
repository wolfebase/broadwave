import { useEffect, useRef, useState, type FormEvent } from "react";
import { searchMessages } from "../../i18n/messages/search.ts";
import { bind } from "../../i18n/runtime.ts";
import { search } from "../../api";
import { useData } from "../../app/data";
import { useLayout } from "../../app/layout";
import { usePlayer } from "../../app/player";
import { focusRing } from "../../app/remote";
import { navigate, useRoute } from "../../app/router";
import { actionName } from "../../lib/actionName";
import { cappedCss } from "../../lib/art";
import { categoryOf, dayLabel, minutesLeft, searchRows, spanLabel, timeLabel } from "../../lib/guide";
import { noteTone, searchFieldValue } from "./field";
import { groupByShow, seriesChannelId, type ShowGroup } from "./groups";
import type { Recording, SearchAiring, SearchChannel } from "../../types";
import { PlayIcon, RecordIcon, SearchIcon } from "../../ui/icons";
import { Atsc3Tag, Empty } from "../../ui/primitives";
import { ProgramSheet } from "../guide/ProgramSheet";
import "./search.css";

const t = bind(searchMessages);

function SearchArt({
  src,
  width,
  height,
  boxW,
  boxH,
  fallback,
}: {
  src: string;
  width?: number;
  height?: number;
  boxW: number;
  boxH: number;
  fallback?: string;
}) {
  const [gone, setGone] = useState(false);
  const [url, setUrl] = useState(src);
  if (gone) return null;
  const scale = window.devicePixelRatio || 1;
  const maxW = cappedCss(width ?? 0, boxW, scale);
  const maxH = cappedCss(height ?? 0, boxH, scale);
  return (
    <span className="search-thumb" style={{ width: boxW, height: boxH }}>
      <img
        alt=""
        loading="lazy"
        src={url}
        style={maxW > 0 || maxH > 0 ? { maxWidth: maxW || undefined, maxHeight: maxH || undefined } : undefined}
        onError={() => {
          if (fallback && url !== fallback) {
            setUrl(fallback);
            return;
          }
          setGone(true);
        }}
      />
    </span>
  );
}

export function SearchPage() {
  const { params } = useRoute();
  const { now, allChannels, passes, planned, recordSeries, recordOnce, record } = useData();
  const player = usePlayer();
  const [open, setOpen] = useState<SearchAiring | null>(null);
  const openChannel = open ? allChannels.find((channel) => channel.id === open.channelId) : undefined;
  const initial = params.get("q") ?? "";
  const [draft, setDraft] = useState(initial);
  const [shownQ, setShownQ] = useState(initial);
  const [airings, setAirings] = useState<SearchAiring[]>([]);
  const [recordings, setRecordings] = useState<Recording[]>([]);
  const [channelHits, setChannelHits] = useState<SearchChannel[]>([]);
  const [note, setNote] = useState("");
  // The query those rows belong to. A newer query shows nothing until its own request finishes.
  const [resultQuery, setResultQuery] = useState("");

  const typing = useRef(0);
  const field = useRef<HTMLInputElement>(null);
  const layout = useLayout();

  const q = initial.trim();
  // The address changed (Back, Forward, or another search). Show that query.
  if (shownQ !== initial) {
    setShownQ(initial);
    setDraft(searchFieldValue(initial, draft, true));
  }

  // Drop a pending replace so a half-typed word cannot overwrite Back or a new search.
  useEffect(() => {
    window.clearTimeout(typing.current);
    return () => window.clearTimeout(typing.current);
  }, [initial]);

  const active = q.length >= 2;
  const listed = searchRows<SearchAiring, Recording>(
    active && q === resultQuery ? { status: "done", airings, recordings } : { status: "pending" },
  );
  const groups = groupByShow(listed.airings, now, (id) => allChannels.some((c) => c.id === id));
  const channelOf = (id: number) => allChannels.find((c) => c.id === id);
  const shownChannels = active && q === resultQuery ? channelHits : [];

  function where(a: SearchAiring) {
    return `${a.guideNumber ?? ""} ${a.channelName ?? ""}`.trim();
  }

  /** One show: when it is on, and the ways to watch or record it. */
  // A plain function, not a component: a component made in render would remount
  // each tick and close an open list of airings.
  function showRow(g: ShowGroup<SearchAiring>) {
    const lead = g.onNow ?? g.next ?? g.airings[0];
    const leadChannel = channelOf(lead.channelId);
    const series = passes.some((p) => p.kind !== "once" && p.title.toLowerCase() === g.title.toLowerCase());
    const next = g.next;
    const nextChannel = next ? channelOf(next.channelId) : undefined;
    const onceSet = next
      ? passes.some((p) => p.kind === "once" && p.channelId === next.channelId && p.airingStart && Date.parse(p.airingStart) === Date.parse(next.start)) ||
        planned.some((p) => !p.skipped && p.airing.channelId === next.channelId && Date.parse(p.airing.start) === Date.parse(next.start))
      : false;
    const nowChannel = g.onNow ? channelOf(g.onNow.channelId) : undefined;
    const atsc3 = leadChannel?.standard === "atsc3";
    // A game is not a series. Its pass records every airing, as the guide says.
    const everyLabel = categoryOf(lead) === "sports" ? t("search.recordEvery") : t("search.recordSeries");
    return (
      <li key={g.key} className="sg">
        <button type="button" className="search-main" disabled={!leadChannel} onClick={() => setOpen(lead)}>
          {g.art?.imageUrl ? <SearchArt src={`/media/art/airing/${g.art.id}?w=160`} width={g.art.imageWidth} height={g.art.imageHeight} boxW={84} boxH={56} /> : null}
          <span className="search-copy">
            <strong>{g.title}</strong>
            <span className="search-meta">
              {atsc3 ? <Atsc3Tag /> : null}
              <span>
                {g.onNow
                  ? t("search.onNowAt", { channel: where(g.onNow), left: minutesLeft(g.onNow, now) })
                  : t("search.nextAt", { when: `${dayLabel(lead.start, now)} · ${timeLabel(lead.start)}`, channel: where(lead) })}
              </span>
            </span>
            {g.airings.length > 1 ? <span className="search-more">{g.channels > 1 ? t("search.airingsOn", { count: g.airings.length, channels: g.channels }) : t("search.airings", g.airings.length)}</span> : null}
          </span>
        </button>
        <div className="sg-actions">
          {g.onNow && nowChannel ? (
            <button type="button" className="btn primary" aria-label={actionName(t("search.watchNow"), g.title)} onClick={() => player.open(nowChannel)}>
              <PlayIcon /> {t("search.watchNow")}
            </button>
          ) : null}
          {leadChannel ? (
            <button type="button" className="btn" disabled={series} aria-label={actionName(series ? t("search.seriesSet") : everyLabel, g.title)} onClick={() => void recordSeries(g.title, seriesChannelId(g, leadChannel.id))}>
              {series ? t("search.seriesSet") : everyLabel}
            </button>
          ) : null}
          {!series && next && nextChannel ? (
            <button type="button" className="btn" disabled={onceSet} aria-label={actionName(onceSet ? t("search.onceSet") : t("search.recordOnce"), `${g.title}, ${dayLabel(next.start, now)} ${timeLabel(next.start)}`)} onClick={() => void recordOnce(next, nextChannel)}>
              <RecordIcon className="tally" /> {onceSet ? t("search.onceSet") : t("search.recordOnce")}
            </button>
          ) : !series && !next && g.onNow && nowChannel ? (
            <button type="button" className="btn" aria-label={actionName(t("search.recordNow"), g.title)} onClick={() => void record(nowChannel, g.title)}>
              <RecordIcon className="tally" /> {t("search.recordNow")}
            </button>
          ) : null}
        </div>
        {g.airings.length > 1 ? (
          <details className="sg-all">
            <summary>{t("search.allAirings")}</summary>
            <ul>
              {g.airings.map((a) => (
                <li key={a.id}>
                  <button type="button" className="text-btn" disabled={!channelOf(a.channelId)} onClick={() => setOpen(a)}>
                    {dayLabel(a.start, now)} · {spanLabel(a)} · {where(a)}
                  </button>
                </li>
              ))}
            </ul>
          </details>
        ) : null}
      </li>
    );
  }

  // A browser autofocus does not match :focus-visible, so a TV would open Search with no ring.
  useEffect(() => {
    if (initial) return;
    const input = field.current;
    if (!input) return;
    if (layout === "tv") focusRing(input);
    else input.focus();
  }, [initial, layout]);

  useEffect(() => {
    const q = initial.trim();
    if (q.length < 2) return;
    let stop = false;
    search(q)
      .then((res) => {
        if (stop) return;
        const playable = res.recordings.filter((rec) => !rec.missing);
        const next = searchRows({ status: "done", airings: res.airings, recordings: playable });
        // An older server sends no channels.
        const named = res.channels ?? [];
        setAirings(next.airings);
        setRecordings(next.recordings);
        setChannelHits(named);
        setNote(named.length === 0 && next.airings.length === 0 && next.recordings.length === 0 ? t("search.nothing") : "");
        setResultQuery(q);
      })
      .catch((err: unknown) => {
        if (stop) return;
        const next = searchRows<SearchAiring, Recording>({ status: "error" });
        setAirings(next.airings);
        setRecordings(next.recordings);
        setChannelHits([]);
        setNote(err instanceof Error ? err.message : t("search.failed"));
        setResultQuery(q);
      });
    return () => {
      stop = true;
    };
  }, [initial]);

  function submit(event: FormEvent) {
    event.preventDefault();
    window.clearTimeout(typing.current);
    navigate(`/search?q=${encodeURIComponent(draft.trim())}`);
  }

  // Results follow the typing after a short pause. The address is replaced, so
  // Back leaves Search instead of stepping through every partial word.
  function type(value: string) {
    setDraft(value);
    window.clearTimeout(typing.current);
    typing.current = window.setTimeout(() => navigate(`/search?q=${encodeURIComponent(value.trim())}`, true), 300);
  }

  return (
    <div className="page-wrap search-page">
      <header className="page-header">
        <h1>{t("search.title")}</h1>
      </header>
      <form className="search-form" onSubmit={submit}>
        <SearchIcon />
        <input
          ref={field}
          type="search"
          value={draft}
          placeholder={t("search.placeholder")}
          aria-label={t("search.label")}
          onChange={(event) => type(event.target.value)}
        />
      </form>
      {active && q === resultQuery && note && note !== t("search.nothing") ? (
        <p className="hint" role={noteTone(note)}>
          {note}
        </p>
      ) : null}
      {active && q === resultQuery && note === t("search.nothing") ? (
        <div role="status">
          <Empty
            title={t("search.nothingFor", { query: q })}
            action={
              <button type="button" className="btn" onClick={() => navigate("/guide")}>
                {t("search.openGuide")}
              </button>
            }
          >
            {t("search.nothingHint")}
          </Empty>
        </div>
      ) : null}
      {!active ? <p className="hint search-hint">{t("search.startHint")}</p> : null}
      {shownChannels.length > 0 ? (
        <section>
          <h2 className="section-title">{t("search.channels")}</h2>
          <ul className="search-list">
            {shownChannels.map((hit) => {
              const channel = channelOf(hit.id);
              const label = `${hit.number} ${hit.name}`;
              return (
                <li key={hit.id}>
                  <button type="button" className="search-rec search-channel" disabled={!channel} aria-label={actionName(t("search.watchNow"), label)} onClick={() => channel && player.open(channel)}>
                    {hit.logo ? (
                      <SearchArt src={`${hit.logo}?w=160`} boxW={84} boxH={48} />
                    ) : (
                      <span className="search-thumb search-chnum" style={{ width: 84, height: 48 }} aria-hidden="true">
                        {hit.number}
                      </span>
                    )}
                    <span>
                      <strong>{label}</strong>
                      <span className="search-meta">
                        {hit.standard === "atsc3" ? <Atsc3Tag /> : null}
                        <span>{hit.onNow ? t("search.channelOnNow", { title: hit.onNow.title, left: minutesLeft(hit.onNow, now) }) : (hit.network ?? "")}</span>
                      </span>
                    </span>
                  </button>
                </li>
              );
            })}
          </ul>
        </section>
      ) : null}
      {groups.length > 0 ? (
        <section>
          <h2 className="section-title">{t("search.guide")}</h2>
          <ul className="search-list search-groups">
            {groups.map((g) => showRow(g))}
          </ul>
        </section>
      ) : null}
      {listed.recordings.length > 0 ? (
        <section>
          <h2 className="section-title">{t("search.recordings")}</h2>
          <ul className="search-list">
            {listed.recordings.map((rec) => (
              <li key={rec.id}>
                <button type="button" className="search-rec" onClick={() => navigate(`/play?recording=${rec.id}`)}>
                  <SearchArt src={`/media/poster/${rec.id}`} boxW={84} boxH={48} fallback={`/media/art/channel/${rec.channelId}?w=320`} />
                  <span>
                    <strong>{rec.title}</strong>
                    <span>{rec.guideNumber}</span>
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </section>
      ) : null}
      {open && openChannel ? (
        <ProgramSheet
          channel={openChannel}
          airing={open}
          onClose={() => setOpen(null)}
          onWatch={(channel) => {
            setOpen(null);
            player.open(channel);
          }}
        />
      ) : null}
    </div>
  );
}
