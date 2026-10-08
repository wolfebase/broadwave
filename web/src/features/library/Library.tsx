import { useEffect, useRef, useState } from "react";
import type { Recording } from "../../types";
import { copy } from "../../strings";
import { formatBytes, formatClockPoint } from "../../lib/format";
import { focusRing } from "../../app/remote";
import { navigate } from "../../app/router";
import { RecordingCard, Shelf } from "../../ui/shelf";
import { DownloadLink } from "../recordings/DownloadLink";
import { signalLine } from "./health";
import { MoveFile } from "./MoveFile";
import { RecordAgain, recordAgainOffered } from "./RecordAgain";
import { buildLibrary, continueWatching, episodeTag, filterRecordings, totalBytes, watched, type Kind, type Show, type Sort } from "./model";

export type Many = "watched" | "unwatched" | "delete";

// A show on the main page lists this many; its own page lists them all.
const preview = 4;

export function Library({
  recordings,
  show = "",
  note,
  onPlay,
  onVirtual,
  onDelete,
  onWatched,
  onKeep,
  onStop,
  onMany,
}: {
  recordings: Recording[];
  show?: string;
  note: string;
  onPlay: (recording: Recording) => void;
  onVirtual: (recording: Recording) => void;
  onDelete: (recording: Recording) => void;
  onWatched: (recording: Recording, watched: boolean) => void;
  onKeep: (recording: Recording, keep: boolean) => void;
  onStop: (recording: Recording) => void;
  onMany: (action: Many, recordings: Recording[]) => Promise<void>;
}) {
  const [armed, setArmed] = useState<number | null>(null);
  const [libraryFilter, setLibraryFilter] = useState<"all" | "unwatched">("all");
  const [kind, setKind] = useState<Kind>("all");
  const [sort, setSort] = useState<Sort>(show ? "oldest" : "newest");
  const [selecting, setSelecting] = useState(false);
  const [picked, setPicked] = useState<Set<number>>(new Set());
  const [armedMany, setArmedMany] = useState(false);
  const [busy, setBusy] = useState(false);
  const rootRef = useRef<HTMLElement>(null);
  const armedWas = useRef<number | null>(null);
  const shown = filterRecordings(recordings, show ? "all" : kind, libraryFilter === "unwatched");
  const { shows, movies } = buildLibrary(shown, sort);
  const resume = show ? [] : continueWatching(recordings);
  // Only what the filters show, so a delete never reaches a recording nobody saw.
  const choosable = shown.filter((rec) => rec.status !== "recording");
  const chosen = choosable.filter((rec) => picked.has(rec.id));
  useEffect(() => {
    const previous = armedWas.current;
    armedWas.current = armed;
    // The confirm button replaces Delete. When the file is gone, that button is gone too.
    if (previous == null || recordings.some((rec) => rec.id === previous)) return;
    focusRing(rootRef.current?.querySelector<HTMLElement>("button, a[href]"));
  }, [recordings, armed]);
  const pick = (recs: Recording[], on: boolean) =>
    setPicked((was) => {
      const next = new Set(was);
      for (const rec of recs) {
        if (rec.status === "recording") continue;
        if (on) next.add(rec.id);
        else next.delete(rec.id);
      }
      return next;
    });
  const stopSelecting = () => {
    setSelecting(false);
    setPicked(new Set());
    setArmedMany(false);
  };
  const runMany = (action: Many) => {
    if (chosen.length === 0 || busy) return;
    setBusy(true);
    void onMany(action, chosen).finally(() => {
      setBusy(false);
      stopSelecting();
      focusRing(rootRef.current?.querySelector<HTMLElement>(".library-tools button"));
    });
  };
  const row = (rec: Recording) => (
    <LibraryRow
      key={rec.id}
      rec={rec}
      armed={armed}
      setArmed={setArmed}
      selecting={selecting}
      picked={picked.has(rec.id)}
      onPick={(on) => pick([rec], on)}
      onPlay={onPlay}
      onVirtual={onVirtual}
      onDelete={onDelete}
      onWatched={onWatched}
      onKeep={onKeep}
      onStop={onStop}
    />
  );
  const headline = show ? (shows[0]?.title ?? movies[0]?.title ?? show) : copy.library.title;
  return (
    <section className="page" ref={rootRef}>
      <div className="page-head">
        {show ? (
          <button type="button" className="text-btn" onClick={() => navigate("/recordings")}>
            All recordings
          </button>
        ) : null}
        <h2>{headline}</h2>
        {recordings.length > 0 ? <p className="ch-tags">{summary(recordings)}</p> : null}
      </div>
      {note ? (
        <p className="lede" role="status">
          {note}
        </p>
      ) : null}
      {resume.length > 0 ? (
        <Shelf title="Continue watching">
          {resume.map((rec) => (
            <RecordingCard key={rec.id} rec={rec} />
          ))}
        </Shelf>
      ) : null}
      <div className="library-tools sheet-actions">
        <div className="segmented" role="tablist" aria-label="Library filter">
          <button type="button" role="tab" aria-selected={libraryFilter === "all"} className={libraryFilter === "all" ? "seg on" : "seg"} onClick={() => setLibraryFilter("all")}>
            All
          </button>
          <button type="button" role="tab" aria-selected={libraryFilter === "unwatched"} className={libraryFilter === "unwatched" ? "seg on" : "seg"} onClick={() => setLibraryFilter("unwatched")}>
            Unwatched
          </button>
        </div>
        {show ? null : (
          <label>
            Show
            <select value={kind} onChange={(e) => setKind(e.target.value as Kind)}>
              <option value="all">Everything</option>
              <option value="shows">Shows</option>
              <option value="movies">Movies</option>
              <option value="sports">Sports</option>
            </select>
          </label>
        )}
        <label>
          Sort
          <select value={sort} onChange={(e) => setSort(e.target.value as Sort)}>
            <option value="newest">Newest first</option>
            <option value="oldest">Oldest first</option>
            <option value="title">By name</option>
            <option value="largest">Largest first</option>
          </select>
        </label>
        {choosable.length > 0 || selecting ? (
          <button type="button" className="btn" aria-pressed={selecting} onClick={() => (selecting ? stopSelecting() : setSelecting(true))}>
            {selecting ? "Done" : "Select"}
          </button>
        ) : null}
      </div>
      {selecting ? (
        <div className="sheet-actions" role="toolbar" aria-label="Selected recordings" aria-busy={busy}>
          <span aria-live="polite">{chosen.length === 1 ? "1 selected" : `${chosen.length} selected`}</span>
          <button type="button" className="btn" disabled={chosen.length === choosable.length} onClick={() => pick(choosable, true)}>
            Select all
          </button>
          <button type="button" className="btn" disabled={chosen.length === 0 || busy} onClick={() => runMany("watched")}>
            Mark watched
          </button>
          <button type="button" className="btn" disabled={chosen.length === 0 || busy} onClick={() => runMany("unwatched")}>
            Mark unwatched
          </button>
          {armedMany ? (
            <>
              <button type="button" className="btn primary" disabled={chosen.length === 0 || busy} onClick={() => runMany("delete")}>
                {chosen.length === 1 ? "Delete 1 file" : `Delete ${chosen.length} files`}
              </button>
              <button type="button" className="btn" onClick={() => setArmedMany(false)}>
                Keep them
              </button>
            </>
          ) : (
            <button type="button" className="btn" disabled={chosen.length === 0 || busy} onClick={() => setArmedMany(true)}>
              Delete
            </button>
          )}
        </div>
      ) : null}
      {shows.length === 0 && movies.length === 0 ? (
        <article className="quiet-card wide">
          <p>{recordings.length === 0 ? copy.library.body : libraryFilter === "unwatched" ? "Everything here has been watched." : "Nothing here matches."}</p>
        </article>
      ) : (
        <div className="show-grid">
          {shows.map((item) =>
            show ? (
              <ShowPage key={item.title} show={item} row={row} />
            ) : (
              <section key={item.title}>
                <h3 className="show-title">{item.title}</h3>
                {item.items.length > 1 ? <p className="ch-tags">{showLine(item)}</p> : null}
                {selecting && item.items.length > 1 ? (
                  <button type="button" className="text-btn" onClick={() => pick(item.items, !item.items.every((rec) => rec.status === "recording" || picked.has(rec.id)))}>
                    {item.items.every((rec) => rec.status === "recording" || picked.has(rec.id)) ? `Clear ${item.title}` : `Select all of ${item.title}`}
                  </button>
                ) : null}
                <ul className="source-list">{item.items.slice(0, preview).map(row)}</ul>
                {item.items.length > preview ? (
                  <button type="button" className="text-btn" onClick={() => navigate(`/recordings?show=${encodeURIComponent(item.title)}`)}>
                    All {item.items.length} of {item.title}
                  </button>
                ) : null}
              </section>
            ),
          )}
          {movies.length > 0 ? (
            <section>
              <h3 className="show-title">Movies</h3>
              <ul className="source-list">{movies.map(row)}</ul>
            </section>
          ) : null}
        </div>
      )}
    </section>
  );
}

function ShowPage({ show, row }: { show: Show; row: (rec: Recording) => React.ReactNode }) {
  const titled = show.seasons.some((s) => s.season > 0);
  return (
    <>
      <p className="ch-tags">{showLine(show)}</p>
      {show.seasons.map((season) => (
        <section key={season.season}>
          {titled ? <h3 className="show-title">{season.season > 0 ? `Season ${season.season}` : "Other episodes"}</h3> : null}
          <ul className="source-list">{season.items.map(row)}</ul>
        </section>
      ))}
    </>
  );
}

function summary(recordings: Recording[]) {
  const n = recordings.length;
  return `${n === 1 ? "1 recording" : `${n} recordings`} · ${formatBytes(totalBytes(recordings))}`;
}

function showLine(show: Show) {
  const n = show.items.length;
  const count = n === 1 ? "1 recording" : `${n} recordings`;
  const left = show.unwatched === 0 ? "all watched" : `${show.unwatched} unwatched`;
  return `${count} · ${left} · ${formatBytes(show.bytes)}`;
}

function LibraryRow({
  rec,
  armed,
  setArmed,
  selecting,
  picked,
  onPick,
  onPlay,
  onVirtual,
  onDelete,
  onWatched,
  onKeep,
  onStop,
}: {
  rec: Recording;
  armed: number | null;
  setArmed: (id: number | null) => void;
  selecting: boolean;
  picked: boolean;
  onPick: (on: boolean) => void;
  onPlay: (recording: Recording) => void;
  onVirtual: (recording: Recording) => void;
  onDelete: (recording: Recording) => void;
  onWatched: (recording: Recording, watched: boolean) => void;
  onKeep: (recording: Recording, keep: boolean) => void;
  onStop: (recording: Recording) => void;
}) {
  const seen = watched(rec);
  const line = signalLine(rec.health);
  const tag = episodeTag(rec);
  const name = rec.subtitle || rec.title;
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
          src={rec.missing ? `/media/art/channel/${rec.channelId}?w=320` : `/media/poster/${rec.id}`}
          data-fallback={rec.missing ? "1" : undefined}
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
        {selecting && rec.status !== "recording" ? (
          <label>
            <input type="checkbox" checked={picked} onChange={(e) => onPick(e.target.checked)} aria-label={`Select ${tag ? `${tag} ` : ""}${name}`} /> <strong>{name}</strong>
          </label>
        ) : (
          <strong>{name}</strong>
        )}
        {rec.missing ? (
          <span className="ch-tags">
            {rec.guideNumber} · {copy.library.gone}
          </span>
        ) : (
          <span className="ch-tags">
            {tag ? `${tag} · ` : ""}
            {rec.guideNumber}
            {statusLabel(rec.status) ? ` · ${statusLabel(rec.status)}` : ""}
            {" · "}
            {formatBytes(rec.bytes ?? 0)}
            {rec.durationSec ? ` · ${formatClockPoint(rec.durationSec)}` : ""}
            {(rec.position ?? 0) > 1 ? ` · resume ${formatClockPoint(rec.position ?? 0)}` : ""}
            {seen ? " · Watched" : ""}
            {rec.keep ? " · Kept forever" : ""}
            {rec.error ? ` · ${rec.error}` : ""}
          </span>
        )}
        {line && !rec.missing ? <span className="ch-tags">{line}</span> : null}
        {selecting ? null : rec.missing ? (
          <div className="sheet-actions">
            {armed === rec.id ? (
              <button ref={confirmRef} type="button" className="btn primary" onClick={() => onDelete(rec)}>
                {copy.library.removeGone}
              </button>
            ) : (
              <button type="button" className="btn" onClick={() => setArmed(rec.id)}>
                Delete
              </button>
            )}
          </div>
        ) : (
          <div className="sheet-actions">
            <button type="button" className="btn primary" onClick={() => onPlay(rec)}>
              Play
            </button>
            {rec.status !== "recording" ? (
              <button type="button" className="btn" onClick={() => onWatched(rec, !seen)}>
                {seen ? "Mark unwatched" : "Mark watched"}
              </button>
            ) : null}
            {rec.status !== "recording" ? (
              <button type="button" className="btn" aria-pressed={rec.keep === true} onClick={() => onKeep(rec, !rec.keep)}>
                Keep forever
              </button>
            ) : null}
            {rec.status !== "recording" ? (
              <button type="button" className="btn" onClick={() => onVirtual(rec)}>
                Make channel
              </button>
            ) : null}
            <DownloadLink id={rec.id} status={rec.status} />
            {rec.file && rec.status !== "recording" ? <MoveFile rec={rec} /> : null}
            {recordAgainOffered(rec) ? <RecordAgain rec={rec} /> : null}
            {rec.status === "recording" ? (
              <button type="button" className="btn" onClick={() => onStop(rec)}>
                Stop recording
              </button>
            ) : null}
            {rec.status === "recording" ? null : armed === rec.id ? (
              <button ref={confirmRef} type="button" className="btn primary" onClick={() => onDelete(rec)}>
                Delete this file
              </button>
            ) : (
              <button type="button" className="btn" onClick={() => setArmed(rec.id)}>
                Delete
              </button>
            )}
          </div>
        )}
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
