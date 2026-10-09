import { useEffect, useRef, useState, type FormEvent } from "react";
import { addPass, search } from "../../api";
import { useData } from "../../app/data";
import { usePlayer } from "../../app/player";
import { navigate, useRoute } from "../../app/router";
import { cappedCss } from "../../lib/art";
import { dayLabel, searchRows, spanLabel } from "../../lib/guide";
import type { Recording, SearchAiring } from "../../types";
import { SearchIcon } from "../../ui/icons";
import { Atsc3Tag } from "../../ui/primitives";
import { ProgramSheet } from "../guide/ProgramSheet";
import "./search.css";

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
  const { refresh, now, allChannels } = useData();
  const player = usePlayer();
  const [open, setOpen] = useState<SearchAiring | null>(null);
  const openChannel = open ? allChannels.find((channel) => channel.id === open.channelId) : undefined;
  const initial = params.get("q") ?? "";
  const [draft, setDraft] = useState(initial);
  const [airings, setAirings] = useState<SearchAiring[]>([]);
  const [recordings, setRecordings] = useState<Recording[]>([]);
  const [note, setNote] = useState("");
  // The query those rows belong to. A newer query shows nothing until its own request finishes.
  const [resultQuery, setResultQuery] = useState("");

  const typing = useRef(0);

  const q = initial.trim();
  const active = q.length >= 2;
  const listed = searchRows<SearchAiring, Recording>(
    active && q === resultQuery ? { status: "done", airings, recordings } : { status: "pending" },
  );

  useEffect(() => () => window.clearTimeout(typing.current), []);

  useEffect(() => {
    const q = initial.trim();
    if (q.length < 2) return;
    let stop = false;
    search(q)
      .then((res) => {
        if (stop) return;
        const playable = res.recordings.filter((rec) => !rec.missing);
        const next = searchRows({ status: "done", airings: res.airings, recordings: playable });
        setAirings(next.airings);
        setRecordings(next.recordings);
        setNote(next.airings.length === 0 && next.recordings.length === 0 ? "Nothing matches." : "");
        setResultQuery(q);
      })
      .catch((err: unknown) => {
        if (stop) return;
        const next = searchRows<SearchAiring, Recording>({ status: "error" });
        setAirings(next.airings);
        setRecordings(next.recordings);
        setNote(err instanceof Error ? err.message : "Search failed.");
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
        <h1>Search</h1>
      </header>
      <form className="search-form" onSubmit={submit}>
        <SearchIcon />
        <input
          type="search"
          autoFocus={!initial}
          value={draft}
          placeholder="Shows, people, recordings"
          aria-label="Search shows, people, and recordings"
          onChange={(event) => type(event.target.value)}
        />
      </form>
      {active && q === resultQuery && note ? <p className="hint">{note}</p> : null}
      {listed.airings.length > 0 ? (
        <section>
          <h2 className="section-title">Guide</h2>
          <ul className="search-list">
            {listed.airings.map((airing) => (
              <li key={airing.id}>
                <button
                  type="button"
                  className="search-main"
                  disabled={!allChannels.some((channel) => channel.id === airing.channelId)}
                  onClick={() => setOpen(airing)}
                >
                  {airing.imageUrl ? (
                    <SearchArt
                      src={`/media/art/airing/${airing.id}?w=160`}
                      width={airing.imageWidth}
                      height={airing.imageHeight}
                      boxW={84}
                      boxH={56}
                    />
                  ) : null}
                  <span className="search-copy">
                    <strong>{airing.title}</strong>
                    <span className="search-meta">
                      {allChannels.some((channel) => channel.id === airing.channelId && channel.standard === "atsc3") ? <Atsc3Tag /> : null}
                      <span>
                        {airing.guideNumber} {airing.channelName} · {dayLabel(airing.start, now)} · {spanLabel(airing)}
                      </span>
                    </span>
                  </span>
                </button>
                <button
                  type="button"
                  className="btn"
                  onClick={() =>
                    void addPass(airing.title, airing.channelId).then(() => {
                      setNote(`Recording every ${airing.title}.`);
                      return refresh(["passes"]);
                    })
                  }
                >
                  Record every airing
                </button>
              </li>
            ))}
          </ul>
        </section>
      ) : null}
      {listed.recordings.length > 0 ? (
        <section>
          <h2 className="section-title">Recordings</h2>
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
