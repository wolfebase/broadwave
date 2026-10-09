import { useEffect, useState } from "react";
import type { CatalogBackup, Settings, StorageInfo, StorageShow } from "../../types";
import { getEvents, getStorageShows, getTuners, listBackups, restoreBackup } from "../../api";
import { navigate } from "../../app/router";
import { authHeaders } from "../../lib/deviceToken";
import { gateFeature } from "../../lib/compat";
import { copy } from "../../strings";
import { formatBytes } from "../../lib/format";
import { readLiveDelay, saveLiveDelay, type LiveDelay } from "../../lib/events";
import { tunerLine as describeTuners } from "./deviceCard";
export function SettingsScreen({
  settings,
  storage,
  features,
  tunerCount,
  onChange,
}: {
  settings: Settings;
  storage: StorageInfo | null;
  /** Omitted until GET /api/v1/server has answered. */
  features?: string[];
  /** Every tuner the server knows, answering or not. */
  tunerCount?: number;
  onChange: (values: Partial<Settings>) => void;
}) {
  const [encoder, setEncoder] = useState("");
  const [delay, setDelay] = useState<LiveDelay>(readLiveDelay);
  const [tunerLine, setTunerLine] = useState("Checking tuners.");
  const [lastEvent, setLastEvent] = useState("");
  useEffect(() => {
    let stop = false;
    void getTuners()
      .then((res) => {
        if (stop) return;
        setEncoder(res.encoder || "");
        const busy = (res.tuners ?? []).filter((tuner) => tuner.guide || tuner.target).length;
        setTunerLine(describeTuners(busy, (res.tuners ?? []).length, tunerCount));
      })
      .catch(() => {
        if (!stop) setTunerLine("Tuner status is not available yet.");
      });
    void getEvents()
      .then((res) => {
        if (!stop && res.events[0]) setLastEvent(res.events[0].message);
      })
      .catch(() => undefined);
    return () => {
      stop = true;
    };
  }, [tunerCount]);
  const hdhrNote = gateFeature(features ? { features } : null, "hdhrEmulation");
  return (
    <section className="page">
      <div className="page-head">
        <h2>{copy.settings.title}</h2>
      </div>
      <h3 className="section-title">Picture</h3>
      <article className="quiet-card wide">
        <h3>This server</h3>
        <p>{encoder ? `Picture encoder ${encoder}.` : "Picture encoder is still being detected."}</p>
        <p>{tunerLine}</p>
        {storage ? <p>{storageLine(storage)}</p> : null}
        {lastEvent ? <p>Latest activity: {lastEvent}</p> : null}
      </article>
      <label className="field">
        {copy.settings.profile}
        <input value="Home" readOnly />
      </label>
      <div className="field">
        <span>Picture</span>
        <div className="segmented" role="group" aria-label="Picture motion">
          {(["broadcast", "smooth", "film"] as const).map((item) => (
            <button
              key={item}
              type="button"
              className={settings.pictureMode === item ? "seg on" : "seg"}
              onClick={() => onChange({ pictureMode: item })}
            >
              {item === "broadcast" ? "Broadcast" : item === "smooth" ? "Smooth" : "Film"}
            </button>
          ))}
        </div>
        <span className="hint">Broadcast rebuilds interlaced channels at 60 frames a second. Smooth adds motion compensation when this server can hold it. Film is for movies.</span>
      </div>
      <div className="field">
        <span>{copy.settings.liveDelay}</span>
        <div className="segmented" role="group" aria-label={copy.settings.liveDelay}>
          {(["lowest", "balanced", "stable"] as const).map((item) => (
            <button
              key={item}
              type="button"
              className={delay === item ? "seg on" : "seg"}
              aria-pressed={delay === item}
              onClick={() => {
                setDelay(item);
                saveLiveDelay(item);
              }}
            >
              {item === "lowest" ? "Lowest" : item === "balanced" ? "Balanced" : "Stable"}
            </button>
          ))}
        </div>
        <span className="hint">{copy.settings.liveDelayHint}</span>
      </div>
      <h3 className="section-title">Guide</h3>
      <label className="field">
        {copy.settings.guideAccount}
        <input
          value={settings.sdUser ?? ""}
          autoComplete="username"
          spellCheck={false}
          onChange={(event) => onChange({ sdUser: event.target.value })}
        />
        <span className="hint">{copy.settings.guideAccountHint}</span>
      </label>
      <label className="field">
        Password
        <input
          type="password"
          autoComplete="current-password"
          placeholder={settings.sdPasswordSet === "1" ? "Saved" : ""}
          onBlur={(event) => {
            const value = event.target.value;
            if (value) onChange({ sdPassword: value });
            event.target.value = "";
          }}
        />
      </label>
      <label className="field">
        {copy.settings.guideLineup}
        <input
          value={settings.sdLineup ?? ""}
          spellCheck={false}
          onChange={(event) => onChange({ sdLineup: event.target.value })}
        />
      </label>
      <label className="field">
        {copy.settings.guideAddress}
        <input
          value={settings.guideUrl ?? ""}
          spellCheck={false}
          placeholder="https://"
          onChange={(event) => onChange({ guideUrl: event.target.value })}
        />
        <span className="hint">{copy.settings.guideAddressHint}</span>
      </label>
      <label className="field">
        {copy.settings.movieArt}
        <input
          type="password"
          autoComplete="off"
          spellCheck={false}
          placeholder={settings.tmdbKeySet === "1" ? "Saved" : ""}
          onBlur={(event) => {
            const value = event.target.value;
            if (value) onChange({ tmdbKey: value });
            event.target.value = "";
          }}
        />
        <span className="hint">{copy.settings.movieArtHint}</span>
      </label>
      <h3 className="section-title">Sports</h3>
      <p className="hint">{settings.sportsdbKeySet === "1" ? copy.settings.scoresFromSportsDB : copy.settings.scoresFrom}</p>
      <label className="field">
        {copy.settings.liveScores}
        <select value={settings.liveScores || "1"} onChange={(event) => onChange({ liveScores: event.target.value })}>
          <option value="1">On</option>
          <option value="0">Off</option>
        </select>
        <span className="hint">{copy.settings.liveScoresHint}</span>
      </label>
      <label className="field">
        {copy.settings.sportsdbKey}
        <input
          type="password"
          autoComplete="off"
          spellCheck={false}
          placeholder={settings.sportsdbKeySet === "1" ? "Saved" : ""}
          onBlur={(event) => {
            const value = event.target.value;
            if (value) onChange({ sportsdbKey: value });
            event.target.value = "";
          }}
        />
        <span className="hint">{copy.settings.sportsdbKeyHint}</span>
      </label>
      <label className="field">
        {copy.settings.hideScores}
        <select value={settings.hideScores || "0"} onChange={(event) => onChange({ hideScores: event.target.value })}>
          <option value="0">Show</option>
          <option value="1">Hide</option>
        </select>
        <span className="hint">{copy.settings.hideScoresHint}</span>
      </label>
      <label className="field">
        Game alerts
        <select
          value={settings.gameAlerts === "teams" || settings.gameAlerts === "off" ? settings.gameAlerts : "all"}
          onChange={(event) => onChange({ gameAlerts: event.target.value as Settings["gameAlerts"] })}
        >
          <option value="all">Your teams and close games</option>
          <option value="teams">Your teams only</option>
          <option value="off">Off</option>
        </select>
        <span className="hint">
          A note when a team you follow starts playing, or a game on your channels comes down to the last minutes. A game you&apos;re recording and haven&apos;t watched gets no score and no close-game note.
        </span>
      </label>
      <h3 className="section-title">DVR</h3>
      <label className="field">
        Play the next episode
        <select value={settings.autoplay || "1"} onChange={(event) => onChange({ autoplay: event.target.value })}>
          <option value="1">On</option>
          <option value="0">Off</option>
        </select>
      </label>
      <h3 className="section-title">Sources</h3>
      {hdhrNote ? (
        <p className="hint" role="status">{hdhrNote}</p>
      ) : (
        <label className="field">
          Offer this server as an HDHomeRun on port 8478
          <select value={settings.hdhrEmulate || "0"} onChange={(event) => onChange({ hdhrEmulate: event.target.value })}>
            <option value="0">Off</option>
            <option value="1">On</option>
          </select>
          <span className="hint">Other apps can add this machine on port 8478. Discovery stays quiet so the real tuner is unchanged. The change applies within a minute.</span>
        </label>
      )}
      <h3 className="section-title">Storage</h3>
      <label className="field">
        {copy.settings.layout}
        <select
          value={settings.layout}
          onChange={(event) => onChange({ layout: event.target.value as Settings["layout"] })}
        >
          <option value="auto">Auto</option>
          <option value="desktop">Desktop</option>
          <option value="tv">TV</option>
          <option value="phone">Phone</option>
        </select>
        <span className="hint">{copy.settings.layoutHint}</span>
      </label>
      <label className="field">
        {copy.settings.recordings}
        <input value={storage?.path ?? ""} readOnly spellCheck={false} />
        <span className="hint">{copy.settings.recordingsHint}</span>
      </label>
      <ShowSpace />
      <label className="field">
        Recording folders
        <select
          value={settings.folderLayout === "flat" ? "flat" : "shows"}
          onChange={(event) => onChange({ folderLayout: event.target.value === "flat" ? "flat" : "shows" })}
        >
          <option value="shows">By show, for Plex and Jellyfin</option>
          <option value="flat">All in one folder</option>
        </select>
        <span className="hint">
          By show puts new recordings in TV/Show/Season 01 and Movies/Title (Year). Point a TV library at TV and a movie library at Movies.
        </span>
      </label>
      <label className="field">
        {copy.settings.nfo}
        <select
          value={settings.writeNfo === "1" ? "1" : "0"}
          onChange={(event) => onChange({ writeNfo: event.target.value === "1" ? "1" : "0" })}
        >
          <option value="0">Off</option>
          <option value="1">On</option>
        </select>
        <span className="hint">{copy.settings.nfoHint}</span>
      </label>
      <ReserveField value={settings.watermarkGB || "10"} storage={storage} onSave={(watermarkGB) => onChange({ watermarkGB })} />
      <label className="field">
        When space runs low
        <select value={settings.makeRoom === "1" ? "1" : "0"} onChange={(event) => onChange({ makeRoom: event.target.value === "1" ? "1" : "0" })}>
          <option value="0">Skip new recordings</option>
          <option value="1">Delete the oldest watched</option>
        </select>
        <span className="hint">Delete makes room before a new recording is skipped, oldest watched first. Unwatched and kept recordings stay.</span>
      </label>
      <label className="field">
        Delete watched recordings
        <select value={watchedDays(settings.deleteWatchedDays)} onChange={(event) => onChange({ deleteWatchedDays: event.target.value })}>
          {watchedChoices(settings.deleteWatchedDays).map((days) => (
            <option key={days} value={days}>
              {days === "0" ? "Never" : days === "1" ? "After 1 day" : `After ${days} days`}
            </option>
          ))}
        </select>
        <span className="hint">Counts from when a recording was played to its end or marked watched. Recordings you keep forever stay.</span>
      </label>
      <label className="field">
        {copy.settings.buffer}
        <select
          value={settings.bufferMinutes ?? "60"}
          onChange={(event) => onChange({ bufferMinutes: event.target.value as Settings["bufferMinutes"] })}
        >
          <option value="0">Off</option>
          <option value="30">30 minutes</option>
          <option value="60">1 hour</option>
          <option value="120">2 hours</option>
          <option value="240">4 hours</option>
        </select>
        <span className="hint">{copy.settings.bufferHint}</span>
      </label>
      <h3 className="section-title">{copy.settings.backups}</h3>
      <BackupList />
      <a className="btn" href="/api/v1/backup">
        {copy.settings.backup}
      </a>
      <form
        className="lookup"
        onSubmit={(event) => {
          event.preventDefault();
          const input = event.currentTarget.elements.namedItem("backup") as HTMLInputElement;
          const file = input.files?.[0];
          if (!file) return;
          void fetch("/api/v1/backup", { method: "POST", body: file, headers: authHeaders() }).then((res) => {
            if (!res.ok) throw new Error("Restore failed");
            window.location.reload();
          });
        }}
      >
        <label>
          Restore a catalog backup
          <input name="backup" type="file" accept=".db" />
        </label>
        <button type="submit" className="btn">{copy.settings.restore}</button>
      </form>
      <p className="hint">{copy.settings.backupHint}</p>
      <h3 className="section-title">Updates</h3>
      <label className="field">
        {copy.settings.updates}
        <select
          value={settings.checkUpdates === "0" ? "0" : "1"}
          onChange={(event) => onChange({ checkUpdates: event.target.value })}
        >
          <option value="1">On</option>
          <option value="0">Off</option>
        </select>
        <span className="hint">{copy.settings.updatesHint}</span>
      </label>
      <h3 className="section-title">Support</h3>
      <a className="btn" href="/api/v1/support">
        {copy.settings.support}
      </a>
      <p className="hint">{copy.settings.supportHint}</p>
      <article className="quiet-card wide">
        <h3>{copy.settings.passwordTitle}</h3>
        <p>{copy.settings.passwordBody}</p>
      </article>
    </section>
  );
}

function ShowSpace() {
  const [rows, setRows] = useState<StorageShow[] | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    let stop = false;
    void getStorageShows()
      .then((res) => {
        if (!stop) setRows(res.shows ?? []);
      })
      .catch(() => {
        if (!stop) setError(copy.settings.byShowFailed);
      });
    return () => {
      stop = true;
    };
  }, []);
  return (
    <section aria-label={copy.settings.byShow}>
      <h3 className="section-title">{copy.settings.byShow}</h3>
      {error ? (
        <p className="error" role="alert">
          {error}
        </p>
      ) : null}
      {rows && rows.length === 0 ? <p className="hint">{copy.settings.byShowEmpty}</p> : null}
      {rows && rows.length > 0 ? (
        <ul className="source-list">
          {rows.map((row) => {
            const href = `/recordings?show=${encodeURIComponent(row.title)}`;
            const count = row.count === 1 ? "1 recording" : `${row.count} recordings`;
            return (
              <li key={row.title} className="source-row">
                <a
                  href={href}
                  onClick={(event) => {
                    if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) return;
                    event.preventDefault();
                    navigate(href);
                  }}
                >
                  {row.title}
                </a>
                <span>{count}</span>
                <span className="hint">{formatBytes(row.bytes)}</span>
              </li>
            );
          })}
        </ul>
      ) : null}
    </section>
  );
}

function BackupList() {
  const [rows, setRows] = useState<CatalogBackup[] | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  useEffect(() => {
    let stop = false;
    void listBackups()
      .then((res) => {
        if (!stop) setRows(res.backups ?? []);
      })
      .catch(() => {
        if (!stop) setError(copy.settings.backupFailed);
      });
    return () => {
      stop = true;
    };
  }, []);
  async function restore(row: CatalogBackup) {
    if (!window.confirm(copy.settings.restoreConfirm)) return;
    setBusy(row.name);
    setError("");
    try {
      await restoreBackup(row.name);
      window.location.reload();
    } catch {
      setError(copy.settings.restoreFailed);
      setBusy("");
    }
  }
  return (
    <>
      {error ? (
        <p className="error" role="alert">
          {error}
        </p>
      ) : null}
      {rows && rows.length === 0 ? <p className="hint">{copy.settings.backupEmpty}</p> : null}
      {rows && rows.length > 0 ? (
        <ul className="source-list">
          {rows.map((row) => {
            const when = backupWhen(row.takenAt);
            const kind = backupKind(row.kind);
            return (
              <li key={row.name} className="source-row">
                <span>{kind}</span>
                <span>{when}</span>
                <span className="hint">{formatBytes(row.bytes)}</span>
                <a
                  className="btn small"
                  href={`/api/v1/backups/${encodeURIComponent(row.name)}`}
                  aria-label={`${copy.settings.backupDownload} ${kind} ${when}`}
                >
                  {copy.settings.backupDownload}
                </a>
                <button
                  type="button"
                  className="btn small"
                  disabled={busy !== ""}
                  aria-label={`${copy.settings.restore} ${kind} ${when}`}
                  onClick={() => void restore(row)}
                >
                  {copy.settings.restore}
                </button>
              </li>
            );
          })}
        </ul>
      ) : null}
    </>
  );
}

function backupKind(kind: string) {
  if (kind === "weekly") return copy.settings.backupWeekly;
  if (kind === "version") return copy.settings.backupUpdate;
  return copy.settings.backupNightly;
}

function backupWhen(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

function ReserveField({ value, storage, onSave }: { value: string; storage: StorageInfo | null; onSave: (value: string) => void }) {
  const [text, setText] = useState(value);
  const [seen, setSeen] = useState(value);
  if (value !== seen) {
    setSeen(value);
    setText(value);
  }
  return (
    <label className="field">
      {copy.settings.reserve}
      <input
        type="number"
        min={0}
        max={1000000}
        aria-label="Gigabytes to keep free"
        value={text}
        onChange={(event) => setText(event.target.value)}
        onBlur={() => {
          const next = Math.round(Number(text));
          if (!Number.isFinite(next) || next < 0 || next > 1000000) {
            setText(value);
            return;
          }
          setText(String(next));
          if (String(next) !== value) onSave(String(next));
        }}
      />
      <span className="hint">
        {storage ? `${storageLine(storage)} ` : ""}
        {copy.settings.reserveHint}
      </span>
    </label>
  );
}

function watchedDays(raw: string | undefined) {
  const n = Math.round(Number(raw ?? "0"));
  return Number.isFinite(n) && n > 0 ? String(n) : "0";
}

/** The usual choices, plus a value saved from elsewhere. */
function watchedChoices(raw: string | undefined) {
  const choices = ["0", "1", "3", "7", "14", "30", "60", "90"];
  const now = watchedDays(raw);
  return choices.includes(now) ? choices : [...choices, now].sort((a, b) => Number(a) - Number(b));
}

function storageLine(storage: StorageInfo) {
  const reserve = storage.watermarkGB === 0 ? "The free-space reserve is off." : `New recordings stop under ${storage.watermarkGB} GB.`;
  return `${formatBytes(storage.freeBytes)} free of ${formatBytes(storage.totalBytes)}. ${reserve}`;
}
