import { useEffect, useState } from "react";
import type { CatalogBackup, Settings, StorageInfo, StorageShow } from "../../types";
import { getEvents, getStorageShows, getTuners, listBackups, restoreBackup } from "../../api";
import { navigate } from "../../app/router";
import { authHeaders } from "../../lib/deviceToken";
import { settingsUiMessages } from "../../i18n/messages/settingsUi.ts";
import { bind, formatDateTime } from "../../i18n/runtime.ts";
import { copy } from "../../strings";
import { formatBytes } from "../../lib/format";
import { browserName, readLiveDelay, saveLiveDelay, type LiveDelay } from "../../lib/events";
import { saveScreenName } from "./screenName";
import { tunerLine as describeTuners } from "./deviceCard";
import { settingTextCommit, settingTextDraft, shownSettingText } from "./field";

const t = bind(settingsUiMessages);

export function SettingsScreen({
  settings,
  storage,
  tunerCount,
  onChange,
}: {
  settings: Settings;
  storage: StorageInfo | null;
  /** Every tuner the server knows, answering or not. */
  tunerCount?: number;
  onChange: (values: Partial<Settings>) => void;
}) {
  const [screenName, setScreenName] = useState(browserName);
  const [encoder, setEncoder] = useState("");
  const [delay, setDelay] = useState<LiveDelay>(readLiveDelay);
  const [tunerFacts, setTunerFacts] = useState<{ busy: number; answering: number } | "down" | null>(null);
  const [lastEvent, setLastEvent] = useState("");
  useEffect(() => {
    let stop = false;
    void getTuners()
      .then((res) => {
        if (stop) return;
        setEncoder(res.encoder || "");
        const busy = (res.tuners ?? []).filter((tuner) => tuner.guide || tuner.target).length;
        setTunerFacts({ busy, answering: (res.tuners ?? []).length });
      })
      .catch(() => {
        if (!stop) setTunerFacts("down");
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
  const tunerLine =
    tunerFacts == null ? t("settings.checkingTuners") : tunerFacts === "down" ? t("settings.tunersUnavailable") : describeTuners(tunerFacts.busy, tunerFacts.answering, tunerCount);
  return (
    <section className="page">
      <div className="page-head">
        <h2>{copy.settings.title}</h2>
      </div>
      <h3 className="section-title">{t("settings.thisScreen")}</h3>
      <label className="field">
        {copy.settings.screenName}
        <input
          value={screenName}
          maxLength={40}
          spellCheck={false}
          autoComplete="off"
          onChange={(event) => setScreenName(event.target.value)}
          onBlur={() => {
            if (screenName !== browserName()) setScreenName(saveScreenName(screenName));
          }}
          onKeyDown={(event) => {
            if (event.key !== "Enter") return;
            event.preventDefault();
            setScreenName(saveScreenName(screenName));
          }}
        />
        <span className="hint">{copy.settings.screenNameHint}</span>
      </label>
      <h3 className="section-title">{t("settings.picture")}</h3>
      <article className="quiet-card wide">
        <h3>{t("settings.thisServer")}</h3>
        <p>{encoder ? t("settings.encoder", { name: encoder }) : t("settings.encoderPending")}</p>
        <p>{tunerLine}</p>
        {storage ? <p>{storageLine(storage)}</p> : null}
        {lastEvent ? <p>{t("settings.latest", { event: lastEvent })}</p> : null}
      </article>
      <label className="field">
        {copy.settings.profile}
        <input value={t("settings.homeProfile")} readOnly />
      </label>
      <div className="field">
        <span>{t("settings.picture")}</span>
        <div className="segmented" role="group" aria-label={t("settings.pictureMotion")}>
          {(["broadcast", "smooth", "film"] as const).map((item) => (
            <button
              key={item}
              type="button"
              className={settings.pictureMode === item ? "seg on" : "seg"}
              aria-pressed={settings.pictureMode === item}
              onClick={() => onChange({ pictureMode: item })}
            >
              {item === "broadcast" ? t("settings.broadcast") : item === "smooth" ? t("settings.smooth") : t("settings.film")}
            </button>
          ))}
        </div>
        <span className="hint">{t("settings.pictureHint")}</span>
      </div>
      <label className="field">
        {copy.settings.adaptive}
        <select
          value={settings.adaptive === "1" ? "1" : "0"}
          onChange={(event) => onChange({ adaptive: event.target.value === "1" ? "1" : "0" })}
        >
          <option value="0">{t("settings.off")}</option>
          <option value="1">{t("settings.on")}</option>
        </select>
        <span className="hint">{copy.settings.adaptiveHint}</span>
      </label>
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
              {item === "lowest" ? t("settings.lowest") : item === "balanced" ? t("settings.balanced") : t("settings.stable")}
            </button>
          ))}
        </div>
        <span className="hint">{copy.settings.liveDelayHint}</span>
      </div>
      <h3 className="section-title">{t("settings.guide")}</h3>
      <GuideText
        label={copy.settings.guideAccount}
        value={settings.sdUser ?? ""}
        hint={copy.settings.guideAccountHint}
        autoComplete="username"
        onSave={(sdUser) => onChange({ sdUser })}
      />
      <label className="field">
        {t("settings.password")}
        <input
          type="password"
          autoComplete="current-password"
          placeholder={settings.sdPasswordSet === "1" ? t("settings.saved") : ""}
          onBlur={(event) => {
            const value = event.target.value;
            if (value) onChange({ sdPassword: value });
            event.target.value = "";
          }}
        />
      </label>
      <GuideText
        label={copy.settings.guideLineup}
        value={settings.sdLineup ?? ""}
        onSave={(sdLineup) => onChange({ sdLineup })}
      />
      <GuideText
        label={copy.settings.guideAddress}
        value={settings.guideUrl ?? ""}
        hint={copy.settings.guideAddressHint}
        placeholder={t("settings.guidePlaceholder")}
        onSave={(guideUrl) => onChange({ guideUrl })}
      />
      <label className="field">
        {copy.settings.movieArt}
        <input
          type="password"
          autoComplete="off"
          spellCheck={false}
          placeholder={settings.tmdbKeySet === "1" ? t("settings.saved") : ""}
          onBlur={(event) => {
            const value = event.target.value;
            if (value) onChange({ tmdbKey: value });
            event.target.value = "";
          }}
        />
        <span className="hint">{copy.settings.movieArtHint}</span>
      </label>
      <h3 className="section-title">{t("settings.sports")}</h3>
      <p className="hint">{settings.sportsdbKeySet === "1" ? copy.settings.scoresFromSportsDB : copy.settings.scoresFrom}</p>
      <label className="field">
        {copy.settings.liveScores}
        <select value={settings.liveScores || "1"} onChange={(event) => onChange({ liveScores: event.target.value })}>
          <option value="1">{t("settings.on")}</option>
          <option value="0">{t("settings.off")}</option>
        </select>
        <span className="hint">{copy.settings.liveScoresHint}</span>
      </label>
      <label className="field">
        {copy.settings.sportsdbKey}
        <input
          type="password"
          autoComplete="off"
          spellCheck={false}
          placeholder={settings.sportsdbKeySet === "1" ? t("settings.saved") : ""}
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
          <option value="0">{t("settings.show")}</option>
          <option value="1">{t("settings.hide")}</option>
        </select>
        <span className="hint">{copy.settings.hideScoresHint}</span>
      </label>
      <label className="field">
        {t("settings.gameAlerts")}
        <select
          value={settings.gameAlerts === "teams" || settings.gameAlerts === "off" ? settings.gameAlerts : "all"}
          onChange={(event) => onChange({ gameAlerts: event.target.value as Settings["gameAlerts"] })}
        >
          <option value="all">{t("settings.alertsAll")}</option>
          <option value="teams">{t("settings.alertsTeams")}</option>
          <option value="off">{t("settings.off")}</option>
        </select>
        <span className="hint">{t("settings.alertsHint")}</span>
      </label>
      <h3 className="section-title">{t("settings.dvr")}</h3>
      <label className="field">
        {t("settings.autoplay")}
        <select value={settings.autoplay || "1"} onChange={(event) => onChange({ autoplay: event.target.value })}>
          <option value="1">{t("settings.on")}</option>
          <option value="0">{t("settings.off")}</option>
        </select>
      </label>
      <h3 className="section-title">{t("settings.storage")}</h3>
      <label className="field">
        {copy.settings.layout}
        <select
          value={settings.layout}
          onChange={(event) => onChange({ layout: event.target.value as Settings["layout"] })}
        >
          <option value="auto">{t("settings.layoutAuto")}</option>
          <option value="desktop">{t("settings.layoutDesktop")}</option>
          <option value="tv">{t("settings.layoutTv")}</option>
          <option value="phone">{t("settings.layoutPhone")}</option>
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
        {t("settings.folders")}
        <select
          value={settings.folderLayout === "flat" ? "flat" : "shows"}
          onChange={(event) => onChange({ folderLayout: event.target.value === "flat" ? "flat" : "shows" })}
        >
          <option value="shows">{t("settings.foldersByShow")}</option>
          <option value="flat">{t("settings.foldersFlat")}</option>
        </select>
        <span className="hint">{t("settings.foldersHint")}</span>
      </label>
      <label className="field">
        {copy.settings.nfo}
        <select
          value={settings.writeNfo === "1" ? "1" : "0"}
          onChange={(event) => onChange({ writeNfo: event.target.value === "1" ? "1" : "0" })}
        >
          <option value="0">{t("settings.off")}</option>
          <option value="1">{t("settings.on")}</option>
        </select>
        <span className="hint">{copy.settings.nfoHint}</span>
      </label>
      <ReserveField value={settings.watermarkGB || "10"} storage={storage} onSave={(watermarkGB) => onChange({ watermarkGB })} />
      <label className="field">
        {t("settings.lowSpace")}
        <select value={settings.makeRoom === "1" ? "1" : "0"} onChange={(event) => onChange({ makeRoom: event.target.value === "1" ? "1" : "0" })}>
          <option value="0">{t("settings.skipNew")}</option>
          <option value="1">{t("settings.deleteOldest")}</option>
        </select>
        <span className="hint">{t("settings.lowSpaceHint")}</span>
      </label>
      <label className="field">
        {t("settings.deleteWatched")}
        <select value={watchedDays(settings.deleteWatchedDays)} onChange={(event) => onChange({ deleteWatchedDays: event.target.value })}>
          {watchedChoices(settings.deleteWatchedDays).map((days) => (
            <option key={days} value={days}>
              {t("settings.afterDays", Number(days))}
            </option>
          ))}
        </select>
        <span className="hint">{t("settings.deleteWatchedHint")}</span>
      </label>
      <label className="field">
        {t("settings.favorites")}
        <select
          value={settings.protectFavorites === "1" ? "1" : "0"}
          onChange={(event) => onChange({ protectFavorites: event.target.value === "1" ? "1" : "0" })}
        >
          <option value="0">{t("settings.favoritesCleanUp")}</option>
          <option value="1">{t("settings.favoritesLeave")}</option>
        </select>
        <span className="hint">{t("settings.favoritesHint")}</span>
      </label>
      <label className="field">
        {copy.settings.buffer}
        <select
          value={settings.bufferMinutes ?? "60"}
          onChange={(event) => onChange({ bufferMinutes: event.target.value as Settings["bufferMinutes"] })}
        >
          <option value="0">{t("settings.off")}</option>
          <option value="30">{t("settings.minutes30")}</option>
          <option value="60">{t("settings.hour1")}</option>
          <option value="120">{t("settings.hours2")}</option>
          <option value="240">{t("settings.hours4")}</option>
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
            if (!res.ok) throw new Error(t("settings.restoreError"));
            window.location.reload();
          });
        }}
      >
        <label>
          {t("settings.restoreCatalog")}
          <input name="backup" type="file" accept=".db" />
        </label>
        <button type="submit" className="btn">{copy.settings.restore}</button>
      </form>
      <p className="hint">{copy.settings.backupHint}</p>
      <h3 className="section-title">{t("settings.updates")}</h3>
      <label className="field">
        {copy.settings.updates}
        <select
          value={settings.checkUpdates === "0" ? "0" : "1"}
          onChange={(event) => onChange({ checkUpdates: event.target.value })}
        >
          <option value="1">{t("settings.on")}</option>
          <option value="0">{t("settings.off")}</option>
        </select>
        <span className="hint">{copy.settings.updatesHint}</span>
      </label>
      <h3 className="section-title">{t("settings.support")}</h3>
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
            const count = t("settings.recordingsCount", row.count);
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
  return formatDateTime(date);
}

function GuideText({
  label,
  value,
  hint,
  autoComplete,
  placeholder,
  onSave,
}: {
  label: string;
  value: string;
  hint?: string;
  autoComplete?: string;
  placeholder?: string;
  onSave: (value: string) => void;
}) {
  const [typed, setTyped] = useState<string | null>(null);
  const draft = settingTextDraft(value, typed);
  if (draft !== typed) setTyped(draft);
  return (
    <label className="field">
      {label}
      <input
        value={shownSettingText(value, draft)}
        autoComplete={autoComplete}
        spellCheck={false}
        placeholder={placeholder}
        onChange={(event) => setTyped(event.target.value)}
        onBlur={(event) => {
          const commit = settingTextCommit(value, event.target.value);
          if (commit != null) onSave(commit);
        }}
      />
      {hint ? <span className="hint">{hint}</span> : null}
    </label>
  );
}

function ReserveField({ value, storage, onSave }: { value: string; storage: StorageInfo | null; onSave: (value: string) => void }) {
  const [typed, setTyped] = useState<string | null>(null);
  const draft = settingTextDraft(value, typed);
  if (draft !== typed) setTyped(draft);
  const text = shownSettingText(value, draft);
  return (
    <label className="field">
      {copy.settings.reserve}
      <input
        type="number"
        min={0}
        max={1000000}
        aria-label={t("settings.reserveAria")}
        value={text}
        onChange={(event) => setTyped(event.target.value)}
        onBlur={() => {
          const next = Math.round(Number(text));
          if (!Number.isFinite(next) || next < 0 || next > 1000000) {
            setTyped(null);
            return;
          }
          const rounded = String(next);
          setTyped(rounded);
          if (rounded !== value) onSave(rounded);
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
  const reserve = storage.watermarkGB === 0 ? t("settings.reserveOff") : t("settings.reserveStop", { gb: storage.watermarkGB });
  return `${t("settings.storageFree", { free: formatBytes(storage.freeBytes), total: formatBytes(storage.totalBytes) })} ${reserve}`;
}
