import { useEffect, useRef, useState, type FormEvent } from "react";
import { addPass, search } from "../../api";
import { useData } from "../../app/data";
import { useLayout } from "../../app/layout";
import { usePlayer } from "../../app/player";
import { focusRing } from "../../app/remote";
import { navigate, useRoute } from "../../app/router";
import { actionName } from "../../lib/actionName";
import { cappedCss } from "../../lib/art";
import { dayLabel, spanLabel } from "../../lib/guide";
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

  const typing = useRef(0);
  const field = useRef<HTMLInputElement>(null);
  const layout = useLayout();

  const active = initial.trim().length >= 2;

  useEffect(() => () => window.clearTimeout(typing.current), []);

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
        setAirings(res.airings);
        const playable = res.recordings.filter((rec) => !rec.missing);
        setRecordings(playable);
        setNote(res.airings.length === 0 && playable.length === 0 ? "Nothing matches." : "");
      })
      .catch((err: unknown) => {
        if (!stop) setNote(err instanceof Error ? err.message : "Search failed.");
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
          ref={field}
          type="search"
          value={draft}
          placeholder="Shows, people, recordings"
          aria-label="Search shows, people, and recordings"
          onChange={(event) => type(event.target.value)}
        />
      </form>
      {active && note ? (
        <p className="hint" role={note === "Nothing matches." || note.startsWith("Recording every ") ? "status" : "alert"}>
          {note}
        </p>
      ) : null}
      {active && airings.length > 0 ? (
        <section>
          <h2 className="section-title">Guide</h2>
          <ul className="search-list">
            {airings.map((airing) => (
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
                  aria-label={actionName("Record every airing", airing.title)}
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
      {active && recordings.length > 0 ? (
        <section>
          <h2 className="section-title">Recordings</h2>
          <ul className="search-list">
            {recordings.map((rec) => (
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
