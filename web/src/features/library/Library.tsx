import { useEffect, useRef, useState } from "react";
import type { Recording } from "../../types";
import { copy } from "../../strings";
import { formatBytes, formatClockPoint } from "../../lib/format";
import { focusRing } from "../../app/remote";
import { DownloadLink } from "../recordings/DownloadLink";
import { signalLine } from "./health";
export function Library({
  recordings,
  note,
  onPlay,
  onVirtual,
  onDelete,
  onWatched,
  onStop,
}: {
  recordings: Recording[];
  note: string;
  onPlay: (recording: Recording) => void;
  onVirtual: (recording: Recording) => void;
  onDelete: (recording: Recording) => void;
  onWatched: (recording: Recording, watched: boolean) => void;
  onStop: (recording: Recording) => void;
}) {
  const [armed, setArmed] = useState<number | null>(null);
  const [libraryFilter, setLibraryFilter] = useState<"all" | "unwatched">("all");
  const rootRef = useRef<HTMLElement>(null);
  const armedWas = useRef<number | null>(null);
  const shown = libraryFilter === "unwatched" ? recordings.filter((rec) => !watched(rec)) : recordings;
  const grouped = groupLibrary(shown);
  useEffect(() => {
    const previous = armedWas.current;
    armedWas.current = armed;
    // The confirm button replaces Delete. When the file is gone, that button is gone too.
    if (previous == null || recordings.some((rec) => rec.id === previous)) return;
    focusRing(rootRef.current?.querySelector<HTMLElement>("button, a[href]"));
  }, [recordings, armed]);
  return (
    <section className="page" ref={rootRef}>
      <div className="page-head">
        <h2>{copy.library.title}</h2>
      </div>
      {note ? <p className="lede">{note}</p> : null}
      <div className="segmented" role="tablist" aria-label="Library filter">
        <button type="button" role="tab" aria-selected={libraryFilter === "all"} className={libraryFilter === "all" ? "seg on" : "seg"} onClick={() => setLibraryFilter("all")}>
          All
        </button>
        <button type="button" role="tab" aria-selected={libraryFilter === "unwatched"} className={libraryFilter === "unwatched" ? "seg on" : "seg"} onClick={() => setLibraryFilter("unwatched")}>
          Unwatched
        </button>
      </div>
      {shown.length === 0 ? (
        <article className="quiet-card wide">
          <p>{recordings.length === 0 ? copy.library.body : "Everything in the library has been watched."}</p>
        </article>
      ) : (
        <div className="show-grid">
          {grouped.shows.map(([title, items]) => (
            <section key={title}>
              <h3 className="show-title">{title}</h3>
              <ul className="source-list">
                {items.map((rec) => (
                  <LibraryRow key={rec.id} rec={rec} armed={armed} setArmed={setArmed} onPlay={onPlay} onVirtual={onVirtual} onDelete={onDelete} onWatched={onWatched} onStop={onStop} />
                ))}
              </ul>
            </section>
          ))}
          {grouped.movies.length > 0 ? (
            <section>
              <h3 className="show-title">Movies</h3>
              <ul className="source-list">
                {grouped.movies.map((rec) => (
                  <LibraryRow key={rec.id} rec={rec} armed={armed} setArmed={setArmed} onPlay={onPlay} onVirtual={onVirtual} onDelete={onDelete} onWatched={onWatched} onStop={onStop} />
                ))}
              </ul>
            </section>
          ) : null}
        </div>
      )}
    </section>
  );
}

function LibraryRow({
  rec,
  armed,
  setArmed,
  onPlay,
  onVirtual,
  onDelete,
  onWatched,
  onStop,
}: {
  rec: Recording;
  armed: number | null;
  setArmed: (id: number | null) => void;
  onPlay: (recording: Recording) => void;
  onVirtual: (recording: Recording) => void;
  onDelete: (recording: Recording) => void;
  onWatched: (recording: Recording, watched: boolean) => void;
  onStop: (recording: Recording) => void;
}) {
  const seen = watched(rec);
  const line = signalLine(rec.health);
  const confirmRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (armed === rec.id) focusRing(confirmRef.current);
  }, [armed, rec.id]);
  return (
    <li className="media-card">
      <span className="poster-wrap">
        <img
          className="poster"
          alt=""
          src={`/media/poster/${rec.id}`}
          onError={(event) => {
            const img = event.currentTarget;
            if (img.dataset.fallback) {
              img.style.visibility = "hidden";
              return;
            }
            img.dataset.fallback = "1";
            img.src = `/media/art/channel/${rec.channelId}?w=320`;
          }}
        />
      </span>
      <div>
        <strong>{rec.subtitle || rec.title}</strong>
        <span className="ch-tags">
          {rec.guideNumber}
          {statusLabel(rec.status) ? ` · ${statusLabel(rec.status)}` : ""}
          {" · "}
          {formatBytes(rec.bytes ?? 0)}
          {rec.durationSec ? ` · ${formatClockPoint(rec.durationSec)}` : ""}
          {(rec.position ?? 0) > 1 ? ` · resume ${formatClockPoint(rec.position ?? 0)}` : ""}
          {seen ? " · Watched" : ""}
          {rec.error ? ` · ${rec.error}` : ""}
        </span>
        {line ? <span className="ch-tags">{line}</span> : null}
        <div className="sheet-actions">
      <button type="button" className="btn primary" onClick={() => onPlay(rec)}>Play</button>
      {rec.status !== "recording" ? <button type="button" className="btn" onClick={() => onWatched(rec, !seen)}>{seen ? "Mark unwatched" : "Mark watched"}</button> : null}
      {rec.status !== "recording" ? <button type="button" className="btn" onClick={() => onVirtual(rec)}>Make channel</button> : null}
      <DownloadLink id={rec.id} status={rec.status} />
      {rec.status === "recording" ? <button type="button" className="btn" onClick={() => onStop(rec)}>Stop recording</button> : null}
      {rec.status === "recording" ? null : armed === rec.id ? (
        <button ref={confirmRef} type="button" className="btn primary" onClick={() => onDelete(rec)}>Delete this file</button>
      ) : (
        <button type="button" className="btn" onClick={() => setArmed(rec.id)}>Delete</button>
      )}
        </div>
      </div>
    </li>
  );
}

/** Nothing for a finished recording; the others say what happened. */
function statusLabel(status: Recording["status"]) {
  switch (status) {
    case "recording":
      return "Recording";
    case "failed":
      return "Failed";
    case "stopped":
      return "Stopped early";
    case "imported":
      return "Imported";
    default:
      return "";
  }
}

function groupLibrary(recordings: Recording[]) {
  const movies: Recording[] = [];
  const map = new Map<string, Recording[]>();
  for (const rec of recordings) {
    if ((rec.category || "").toLowerCase().includes("movie")) {
      movies.push(rec);
      continue;
    }
    const list = map.get(rec.title) ?? [];
    list.push(rec);
    map.set(rec.title, list);
  }
  return { movies, shows: [...map.entries()] };
}

function watched(rec: Recording) {
  if (rec.watched === 1) return true;
  if (rec.watched === 2) return false;
  const position = rec.position ?? 0;
  const duration = rec.durationSec ?? 0;
  if (duration < 10 || position < 1) return false;
  return position >= duration - 15 || position / duration >= 0.9;
}
