import { useEffect, useRef, useState } from "react";
import type { Recording } from "../../types";
import { libraryMessages } from "../../i18n/messages/library.ts";
import { bind } from "../../i18n/runtime.ts";
import { copy } from "../../strings";
import { formatBytes, formatClockPoint } from "../../lib/format";
import { focusRing } from "../../app/remote";
import { navigate } from "../../app/router";
import { dismissesLayer, pageCovered } from "../../lib/dialogFocus";
import { RecordingCard, Shelf } from "../../ui/shelf";
import { DownloadLink } from "../recordings/DownloadLink";
import { MoreIcon } from "../../ui/icons";
import { signalLine } from "./health";
import { MoveFile } from "./MoveFile";
import { RecordAgain, recordAgainOffered } from "./RecordAgain";
import { actionName } from "../../lib/actionName";
import "./library.css";
import { buildLibrary, continueWatching, episodeTag, filterRecordings, recordingSubject, totalBytes, watched, type Kind, type Show, type Sort } from "./model";

const t = bind(libraryMessages);

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
  const manyRef = useRef<HTMLButtonElement>(null);
  const restoreDelete = useRef<number | "many" | null>(null);
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
  const confirming = (armed != null && !selecting) || armedMany;
  // Escape cancels the confirm. Back would otherwise leave the library.
  useEffect(() => {
    if (confirming) {
      const onKey = (event: KeyboardEvent) => {
        if (event.metaKey || event.ctrlKey || event.altKey) return;
        if (!dismissesLayer(event.key, event.target, event.keyCode) || pageCovered()) return;
        const target = event.target;
        if (target instanceof HTMLElement && target.closest("form")) return;
        event.preventDefault();
        event.stopPropagation();
        if (armed != null && !selecting) {
          restoreDelete.current = armed;
          setArmed(null);
        } else {
          restoreDelete.current = "many";
          setArmedMany(false);
        }
      };
      window.addEventListener("keydown", onKey, true);
      return () => window.removeEventListener("keydown", onKey, true);
    }
    const back = restoreDelete.current;
    if (back == null) return;
    restoreDelete.current = null;
    const el =
      back === "many"
        ? rootRef.current?.querySelector<HTMLElement>("[data-delete-many]")
        : rootRef.current?.querySelector<HTMLElement>(`[data-delete="${back}"]`);
    focusRing(el);
  }, [armed, armedMany, confirming, selecting]);
  useEffect(() => {
    if (armedMany) focusRing(manyRef.current);
  }, [armedMany]);
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
  const headline = show ? (shows[0]?.title ?? movies[0]?.title ?? show) : "";
  const chip = (on: boolean, label: string, onClick: () => void) => (
    <button type="button" className={on ? "chip on" : "chip"} aria-pressed={on} onClick={onClick}>
      {label}
    </button>
  );
  // The page already says Recordings. A show's own page names the show.
  return (
    <section className="page library" ref={rootRef}>
      {show ? (
        <div className="page-head">
          <button type="button" className="text-btn" onClick={() => navigate("/recordings")}>
            {t("library.allRecordings")}
          </button>
          <h2>{headline}</h2>
        </div>
      ) : null}
      {recordings.length > 0 && !show ? <p className="ch-tags library-sum">{summary(recordings)}</p> : null}
      {note ? (
        <p className="lede" role="status">
          {note}
        </p>
      ) : null}
      {resume.length > 0 ? (
        <Shelf title={t("library.continueWatching")}>
          {resume.map((rec) => (
            <RecordingCard key={rec.id} rec={rec} />
          ))}
        </Shelf>
      ) : null}
      {recordings.length === 0 ? null : (
      <div className="library-tools">
        <div className="library-chips" role="group" aria-label={t("library.filter")}>
          {chip(libraryFilter === "all" && kind === "all", t("library.all"), () => {
            setLibraryFilter("all");
            setKind("all");
          })}
          {chip(libraryFilter === "unwatched", t("library.unwatchedTab"), () => setLibraryFilter(libraryFilter === "unwatched" ? "all" : "unwatched"))}
          {show
            ? null
            : (["shows", "movies", "sports"] as const).map((item) => (
                <span key={item} className="library-chip">
                  {chip(kind === item, t(`library.${item}`), () => setKind(kind === item ? "all" : item))}
                </span>
              ))}
        </div>
        <label className="library-sort">
          {t("library.sort")}
          <select value={sort} onChange={(e) => setSort(e.target.value as Sort)}>
            <option value="newest">{t("library.newest")}</option>
            <option value="oldest">{t("library.oldest")}</option>
            <option value="title">{t("library.byName")}</option>
            <option value="largest">{t("library.largest")}</option>
          </select>
        </label>
        {choosable.length > 0 || selecting ? (
          <button type="button" className="btn" aria-pressed={selecting} onClick={() => (selecting ? stopSelecting() : setSelecting(true))}>
            {selecting ? t("library.done") : t("library.select")}
          </button>
        ) : null}
      </div>
      )}
      {selecting ? (
        <div className="sheet-actions" role="toolbar" aria-label={t("library.selected")} aria-busy={busy}>
          <span aria-live="polite">{t("library.selectedCount", { count: chosen.length })}</span>
          <button type="button" className="btn" disabled={chosen.length === choosable.length} onClick={() => pick(choosable, true)}>
            {t("library.selectAll")}
          </button>
          <button type="button" className="btn" disabled={chosen.length === 0 || busy} onClick={() => runMany("watched")}>
            {t("library.markWatched")}
          </button>
          <button type="button" className="btn" disabled={chosen.length === 0 || busy} onClick={() => runMany("unwatched")}>
            {t("library.markUnwatched")}
          </button>
          {armedMany ? (
            <>
              <button ref={manyRef} type="button" className="btn primary" disabled={chosen.length === 0 || busy} onClick={() => runMany("delete")}>
                {t("library.deleteFiles", chosen.length)}
              </button>
              <button
                type="button"
                className="btn"
                onClick={() => {
                  restoreDelete.current = "many";
                  setArmedMany(false);
                }}
              >
                {t("library.keepThem")}
              </button>
            </>
          ) : (
            <button type="button" className="btn" data-delete-many disabled={chosen.length === 0 || busy} onClick={() => setArmedMany(true)}>
              {t("library.delete")}
            </button>
          )}
        </div>
      ) : null}
      {recordings.length === 0 ? (
        <article className="quiet-card wide library-empty">
          <h2>{t("library.emptyTitle")}</h2>
          <p>{copy.library.body}</p>
          <button type="button" className="btn primary" onClick={() => navigate("/guide")}>
            {t("library.openGuide")}
          </button>
        </article>
      ) : shows.length === 0 && movies.length === 0 ? (
        <article className="quiet-card wide">
          <p>{libraryFilter === "unwatched" ? t("library.allWatched") : t("library.nothingMatches")}</p>
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
                    {item.items.every((rec) => rec.status === "recording" || picked.has(rec.id)) ? t("library.clearShow", { title: item.title }) : t("library.selectShow", { title: item.title })}
                  </button>
                ) : null}
                <ul className="source-list">{item.items.slice(0, preview).map(row)}</ul>
                {item.items.length > preview ? (
                  <button type="button" className="text-btn" onClick={() => navigate(`/recordings?show=${encodeURIComponent(item.title)}`)}>
                    {t("library.allOf", { count: item.items.length, title: item.title })}
                  </button>
                ) : null}
              </section>
            ),
          )}
          {movies.length > 0 ? (
            <section>
              <h3 className="show-title">{t("library.movies")}</h3>
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
          {titled ? <h3 className="show-title">{season.season > 0 ? t("library.season", { number: season.season }) : t("library.otherEpisodes")}</h3> : null}
          <ul className="source-list">{season.items.map(row)}</ul>
        </section>
      ))}
    </>
  );
}

function summary(recordings: Recording[]) {
  return `${t("library.count", recordings.length)} · ${formatBytes(totalBytes(recordings))}`;
}

function showLine(show: Show) {
  return `${t("library.count", show.items.length)} · ${t("library.unwatched", show.unwatched)} · ${formatBytes(show.bytes)}`;
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
  const subject = recordingSubject(rec);
  const named = (action: string) => actionName(action, subject);
  const confirmRef = useRef<HTMLButtonElement>(null);
  const stopRef = useRef<HTMLButtonElement>(null);
  const [stopping, setStopping] = useState(false);
  const live = rec.status === "recording";
  const [more, setMore] = useState(false);
  const moreRef = useRef<HTMLButtonElement>(null);
  const groupRef = useRef<HTMLDivElement>(null);
  const moreId = `rec-more-${rec.id}`;
  // Escape (or Back on a remote) inside the extra actions folds them away, not the page.
  useEffect(() => {
    if (!more) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.metaKey || event.ctrlKey || event.altKey) return;
      if (!dismissesLayer(event.key, event.target, event.keyCode) || pageCovered()) return;
      const target = event.target;
      if (!(target instanceof Node) || !(groupRef.current?.contains(target) || moreRef.current?.contains(target))) return;
      if (target instanceof HTMLElement && target.closest("form")) return;
      event.preventDefault();
      event.stopPropagation();
      setMore(false);
      focusRing(moreRef.current);
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [more]);
  useEffect(() => {
    if (armed === rec.id) focusRing(confirmRef.current);
  }, [armed, rec.id]);
  useEffect(() => {
    if (stopping) focusRing(stopRef.current);
  }, [stopping]);
  const title = (
    <strong>
      {name}
      {live ? <span className="live-chip">{t("library.statusRecording")}</span> : null}
    </strong>
  );
  const cut = cutLine(rec);
  return (
    <li className={live ? "media-card live" : "media-card"}>
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
            <input type="checkbox" checked={picked} onChange={(e) => onPick(e.target.checked)} aria-label={t("library.selectNamed", { name: `${tag ? `${tag} ` : ""}${name}` })} /> {title}
          </label>
        ) : (
          title
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
            {(rec.position ?? 0) > 1 ? ` · ${t("library.resumeAt", { time: formatClockPoint(rec.position ?? 0) })}` : ""}
            {seen ? ` · ${t("library.watched")}` : ""}
            {rec.keep ? ` · ${t("library.keptForever")}` : ""}
            {rec.error ? ` · ${rec.error}` : ""}
          </span>
        )}
        {cut && !rec.missing ? <span className="ch-tags cut">{cut}</span> : null}
        {line && !rec.missing ? <span className="ch-tags">{line}</span> : null}
        {selecting ? null : rec.missing ? (
          <div className="sheet-actions">
            {recordAgainOffered(rec) ? <RecordAgain rec={rec} /> : null}
            {armed === rec.id ? (
              <button ref={confirmRef} type="button" className="btn primary" aria-label={named(copy.library.removeGone)} onClick={() => onDelete(rec)}>
                {copy.library.removeGone}
              </button>
            ) : (
              <button type="button" className="btn" data-delete={rec.id} aria-label={named(t("library.delete"))} onClick={() => setArmed(rec.id)}>
                {t("library.delete")}
              </button>
            )}
          </div>
        ) : (
          <div className="sheet-actions">
            <button type="button" className="btn primary" aria-label={named(t("library.play"))} onClick={() => onPlay(rec)}>
              {t("library.play")}
            </button>
            {recordAgainOffered(rec) ? <RecordAgain rec={rec} /> : null}
            {live ? (
              <span className="stop-zone">
                {stopping ? (
                  <>
                    <span className="stop-ask">{t("library.stopAsk")}</span>
                    <button type="button" className="btn danger" aria-label={named(t("library.stopNow"))} onClick={() => onStop(rec)}>
                      {t("library.stopNow")}
                    </button>
                    <button ref={stopRef} type="button" className="btn" onClick={() => setStopping(false)}>
                      {t("library.keepRecording")}
                    </button>
                  </>
                ) : (
                  <button type="button" className="btn ghost" aria-label={named(t("library.stopRecording"))} onClick={() => setStopping(true)}>
                    {t("library.stopRecording")}
                  </button>
                )}
              </span>
            ) : null}
            {live ? null : (
              <button
                ref={moreRef}
                type="button"
                className="btn more-btn"
                aria-expanded={more}
                aria-controls={moreId}
                aria-label={named(t("library.more"))}
                title={t("library.more")}
                onClick={() => setMore((on) => !on)}
              >
                <MoreIcon />
              </button>
            )}
            {rec.status === "recording" ? null : armed === rec.id ? (
              <button ref={confirmRef} type="button" className="btn primary" aria-label={named(t("library.deleteFile"))} onClick={() => onDelete(rec)}>
                {t("library.deleteFile")}
              </button>
            ) : (
              <button type="button" className="btn" data-delete={rec.id} aria-label={named(t("library.delete"))} onClick={() => setArmed(rec.id)}>
                {t("library.delete")}
              </button>
            )}
          </div>
        )}
        {selecting || rec.missing || live ? null : (
          <div id={moreId} ref={groupRef} className="sheet-actions more-actions" role="group" aria-label={named(t("library.more"))} hidden={!more}>
            <button type="button" className="btn" aria-label={named(seen ? t("library.markUnwatched") : t("library.markWatched"))} onClick={() => onWatched(rec, !seen)}>
              {seen ? t("library.markUnwatched") : t("library.markWatched")}
            </button>
            <button type="button" className="btn" aria-pressed={rec.keep === true} aria-label={named(t("library.keepForever"))} onClick={() => onKeep(rec, !rec.keep)}>
              {t("library.keepForever")}
            </button>
            <button type="button" className="btn" aria-label={named(t("library.makeChannel"))} aria-describedby={`${moreId}-hint`} onClick={() => onVirtual(rec)}>
              {t("library.makeChannel")}
            </button>
            <DownloadLink id={rec.id} status={rec.status} name={subject} />
            {rec.file ? <MoveFile rec={rec} /> : null}
            <span id={`${moreId}-hint`} className="hint more-hint">
              {t("library.makeChannelHint")}
            </span>
          </div>
        )}
      </div>
    </li>
  );
}

/** A piece of an airing says so, plainly. */
function cutLine(rec: Recording): string {
  if (rec.status === "interrupted") return t("library.cutShort");
  if (rec.partial) return t("library.partOf");
  return "";
}

/** Nothing for a finished recording; the others say what happened. */
function statusLabel(status: Recording["status"]) {
  switch (status) {
    case "failed":
      return t("library.statusFailed");
    case "stopped":
      return t("library.statusStopped");
    case "imported":
      return t("library.statusImported");
    default:
      return "";
  }
}
