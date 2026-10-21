import { useEffect, useId, useRef, useState } from "react";
import type { CatalogBackup, DirectStatus, ServerInfo, Settings, StorageInfo, StorageShow } from "../../types";
import { getDirect, getStorageShows, importRecordings, listBackups, restoreBackup } from "../../api";
import { useData } from "../../app/data";
import { navigate } from "../../app/router";
import { authHeaders } from "../../lib/deviceToken";
import { settingsUiMessages } from "../../i18n/messages/settingsUi.ts";
import { settingsHubMessages } from "../../i18n/messages/settingsHub.ts";
import { bind, formatDateTime } from "../../i18n/runtime.ts";
import { copy } from "../../strings";
import { formatBytes } from "../../lib/format";
import { blenderCreditText } from "../../legal";
import { readLiveDelay, saveLiveDelay, type LiveDelay } from "../../lib/events";
import { settingTextCommit, settingTextDraft, shownSettingText } from "./field";
import { Confirm } from "./Confirm";
import { ChoiceRow, Group, Row, SelectRow, SwitchRow } from "./rows";

const t = bind(settingsUiMessages);
const h = bind(settingsHubMessages);

type Change = (values: Partial<Settings>) => void;

const onOff = (value: string | undefined, fallback: "0" | "1") => (value === "0" || value === "1" ? value : fallback) === "1";

export function PlaybackSection({ settings, onChange }: { settings: Settings; onChange: Change }) {
  const [delay, setDelay] = useState<LiveDelay>(readLiveDelay);
  return (
    <>
      <Group title={h("playback.picture")}>
        <ChoiceRow
          label={h("playback.motion")}
          hint={t("settings.pictureHint")}
          value={settings.pictureMode ?? "broadcast"}
          options={[
            { value: "broadcast", label: t("settings.broadcast") },
            { value: "smooth", label: t("settings.smooth") },
            { value: "film", label: t("settings.film") },
          ]}
          onChange={(pictureMode) => onChange({ pictureMode })}
        />
        <SwitchRow label={copy.settings.adaptive} hint={h("playback.adaptiveHint")} checked={settings.adaptive === "1"} onChange={(on) => onChange({ adaptive: on ? "1" : "0" })} />
      </Group>
      <Group title={h("playback.live")}>
        <ChoiceRow
          label={h("playback.delay")}
          hint={copy.settings.liveDelayHint}
          value={delay}
          options={[
            { value: "lowest", label: t("settings.lowest") },
            { value: "balanced", label: t("settings.balanced") },
            { value: "stable", label: t("settings.stable") },
          ]}
          onChange={(item) => {
            setDelay(item);
            saveLiveDelay(item);
          }}
        />
        <SelectRow
          label={h("playback.bufferLabel")}
          hint={h("playback.bufferHint")}
          value={settings.bufferMinutes ?? "60"}
          options={[
            { value: "0", label: t("settings.off") },
            { value: "30", label: t("settings.minutes30") },
            { value: "60", label: t("settings.hour1") },
            { value: "120", label: t("settings.hours2") },
            { value: "240", label: t("settings.hours4") },
          ]}
          onChange={(bufferMinutes) => onChange({ bufferMinutes: bufferMinutes as Settings["bufferMinutes"] })}
        />
      </Group>
      <Group title={h("playback.recordings")}>
        <SwitchRow label={t("settings.autoplay")} hint={h("playback.autoplayHint")} checked={onOff(settings.autoplay, "1")} onChange={(on) => onChange({ autoplay: on ? "1" : "0" })} />
      </Group>
    </>
  );
}

export function GuideSection({ settings, onChange }: { settings: Settings; onChange: Change }) {
  return (
    <>
      <Group title={h("guide.listings")} lede={h("guide.listingsLede")}>
        <TextRow label={h("guide.sdUser")} hint={copy.settings.guideAccountHint} value={settings.sdUser ?? ""} autoComplete="username" onSave={(sdUser) => onChange({ sdUser })} />
        <SecretRow label={h("guide.sdPassword")} saved={settings.sdPasswordSet === "1"} autoComplete="current-password" onSave={(sdPassword) => onChange({ sdPassword })} />
        <TextRow label={copy.settings.guideLineup} value={settings.sdLineup ?? ""} onSave={(sdLineup) => onChange({ sdLineup })} />
        <TextRow label={copy.settings.guideAddress} hint={copy.settings.guideAddressHint} value={settings.guideUrl ?? ""} placeholder={t("settings.guidePlaceholder")} onSave={(guideUrl) => onChange({ guideUrl })} />
      </Group>
      <Group title={h("guide.art")}>
        <SecretRow label={h("guide.tmdb")} hint={copy.settings.movieArtHint} saved={settings.tmdbKeySet === "1"} onSave={(tmdbKey) => onChange({ tmdbKey })} />
      </Group>
      <Group title={h("guide.sports")} lede={settings.sportsdbKeySet === "1" ? copy.settings.scoresFromSportsDB : copy.settings.scoresFrom}>
        <SwitchRow label={copy.settings.liveScores} hint={copy.settings.liveScoresHint} checked={onOff(settings.liveScores, "1")} onChange={(on) => onChange({ liveScores: on ? "1" : "0" })} />
        <SwitchRow label={copy.settings.hideScores} hint={copy.settings.hideScoresHint} checked={onOff(settings.hideScores, "0")} onChange={(on) => onChange({ hideScores: on ? "1" : "0" })} />
        <ChoiceRow
          label={t("settings.gameAlerts")}
          hint={t("settings.alertsHint")}
          stack
          value={settings.gameAlerts === "teams" || settings.gameAlerts === "off" ? settings.gameAlerts : "all"}
          options={[
            { value: "all", label: t("settings.alertsAll") },
            { value: "teams", label: t("settings.alertsTeams") },
            { value: "off", label: t("settings.off") },
          ]}
          onChange={(gameAlerts) => onChange({ gameAlerts: gameAlerts as Settings["gameAlerts"] })}
        />
        <SecretRow label={copy.settings.sportsdbKey} hint={copy.settings.sportsdbKeyHint} saved={settings.sportsdbKeySet === "1"} onSave={(sportsdbKey) => onChange({ sportsdbKey })} />
      </Group>
    </>
  );
}

export function RecordingSection({ settings, storage, onChange }: { settings: Settings; storage: StorageInfo | null; onChange: Change }) {
  return (
    <>
      <Group title={h("rec.space")}>
        {storage ? <SpaceMeter storage={storage} /> : null}
        <ReserveRow value={settings.watermarkGB || "10"} onSave={(watermarkGB) => onChange({ watermarkGB })} />
      </Group>
      <Group title={h("rec.cleanup")}>
        <ChoiceRow
          label={t("settings.lowSpace")}
          hint={t("settings.lowSpaceHint")}
          stack
          value={settings.makeRoom === "1" ? "1" : "0"}
          options={[
            { value: "0", label: t("settings.skipNew") },
            { value: "1", label: t("settings.deleteOldest") },
          ]}
          onChange={(makeRoom) => onChange({ makeRoom })}
        />
        <SelectRow
          label={t("settings.deleteWatched")}
          hint={t("settings.deleteWatchedHint")}
          value={watchedDays(settings.deleteWatchedDays)}
          options={watchedChoices(settings.deleteWatchedDays).map((days) => ({ value: days, label: t("settings.afterDays", Number(days)) }))}
          onChange={(deleteWatchedDays) => onChange({ deleteWatchedDays })}
        />
        <ChoiceRow
          label={t("settings.favorites")}
          hint={t("settings.favoritesHint")}
          stack
          value={settings.protectFavorites === "1" ? "1" : "0"}
          options={[
            { value: "0", label: t("settings.favoritesCleanUp") },
            { value: "1", label: t("settings.favoritesLeave") },
          ]}
          onChange={(protectFavorites) => onChange({ protectFavorites })}
        />
      </Group>
      <Group title={h("rec.files")}>
        <Row label={h("rec.folder")} hint={copy.settings.recordingsHint} stack>
          <code className="set-path">{storage?.path || "—"}</code>
        </Row>
        <ChoiceRow
          label={t("settings.folders")}
          hint={t("settings.foldersHint")}
          stack
          value={settings.folderLayout === "flat" ? "flat" : "shows"}
          options={[
            { value: "shows", label: t("settings.foldersByShow") },
            { value: "flat", label: t("settings.foldersFlat") },
          ]}
          onChange={(folderLayout) => onChange({ folderLayout })}
        />
        <SwitchRow label={copy.settings.nfo} hint={copy.settings.nfoHint} checked={settings.writeNfo === "1"} onChange={(on) => onChange({ writeNfo: on ? "1" : "0" })} />
        <FindRecordingsRow />
      </Group>
      <ShowSpace />
    </>
  );
}

/** Adds recordings that are in the folder but not in the list, and says how many. */
function FindRecordingsRow() {
  const { refresh } = useData();
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState("");
  async function find() {
    setBusy(true);
    setNote("");
    try {
      const res = await importRecordings();
      setNote(h("rec.found", res.added));
      if (res.added > 0) await refresh(["recordings"]);
    } catch (err) {
      setNote(err instanceof Error && err.message ? err.message : h("rec.findFailed"));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Row label={h("rec.find")} hint={h("rec.findHint")}>
      <button type="button" className="btn small" disabled={busy} onClick={() => void find()}>
        {busy ? h("rec.finding") : h("rec.findButton")}
      </button>
      {note ? (
        <span className="set-row-hint" role="status">
          {note}
        </span>
      ) : null}
    </Row>
  );
}

function SpaceMeter({ storage }: { storage: StorageInfo }) {
  const used = storage.totalBytes > 0 ? Math.min(1, Math.max(0, 1 - storage.freeBytes / storage.totalBytes)) : 0;
  const reserve = storage.totalBytes > 0 ? Math.min(1, (storage.watermarkGB * 1e9) / storage.totalBytes) : 0;
  return (
    <div className="set-row stack">
      <div className="set-row-text">
        <span className="set-row-label">{h("rec.free", { free: formatBytes(storage.freeBytes), total: formatBytes(storage.totalBytes) })}</span>
        <span className="set-row-hint">{storage.watermarkGB === 0 ? t("settings.reserveOff") : t("settings.reserveStop", { gb: storage.watermarkGB })}</span>
      </div>
      <div className="set-meter" role="meter" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(used * 100)} aria-label={h("rec.space")}>
        <span className="set-meter-fill" style={{ width: `${used * 100}%` }} />
        {reserve > 0 ? <span className="set-meter-reserve" style={{ width: `${reserve * 100}%` }} /> : null}
      </div>
    </div>
  );
}

function ShowSpace() {
  const [rows, setRows] = useState<StorageShow[] | null>(null);
  const [error, setError] = useState("");
  const [all, setAll] = useState(false);
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
  const limit = 8;
  const shown = rows ? (all ? rows : rows.slice(0, limit)) : [];
  return (
    <Group title={h("rec.byShow")}>
      {error ? (
        <p className="set-note error" role="alert">
          {error}
        </p>
      ) : null}
      {rows && rows.length === 0 ? <p className="set-note">{copy.settings.byShowEmpty}</p> : null}
      {shown.map((row) => {
        const href = `/recordings?show=${encodeURIComponent(row.title)}`;
        return (
          <div key={row.title} className="set-row">
            <div className="set-row-text">
              <a
                className="set-row-label"
                href={href}
                onClick={(event) => {
                  if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) return;
                  event.preventDefault();
                  navigate(href);
                }}
              >
                {row.title}
              </a>
              <span className="set-row-hint">{t("settings.recordingsCount", row.count)}</span>
            </div>
            <span className="set-row-value">{formatBytes(row.bytes)}</span>
          </div>
        );
      })}
      {rows && rows.length > limit ? (
        <div className="set-row more">
          <button type="button" className="btn small ghost" aria-expanded={all} onClick={() => setAll((v) => !v)}>
            {all ? h("backups.fewer") : h("rec.byShowMore", rows.length - limit)}
          </button>
        </div>
      ) : null}
    </Group>
  );
}

export function BackupsSection() {
  const [rows, setRows] = useState<CatalogBackup[] | null>(null);
  const [error, setError] = useState("");
  const [all, setAll] = useState(false);
  const [asking, setAsking] = useState<CatalogBackup | null>(null);
  const [file, setFile] = useState<File | null>(null);
  const [busy, setBusy] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);
  const fileId = useId();
  useEffect(() => {
    let stop = false;
    void listBackups()
      .then((res) => {
        if (!stop) setRows([...(res.backups ?? [])].sort((a, b) => Date.parse(b.takenAt) - Date.parse(a.takenAt)));
      })
      .catch(() => {
        if (!stop) setError(copy.settings.backupFailed);
      });
    return () => {
      stop = true;
    };
  }, []);
  async function restore(row: CatalogBackup) {
    setBusy(true);
    setError("");
    try {
      await restoreBackup(row.name);
      window.location.reload();
    } catch {
      setError(copy.settings.restoreFailed);
      setBusy(false);
      setAsking(null);
    }
  }
  async function restoreFile(chosen: File) {
    setBusy(true);
    setError("");
    try {
      const res = await fetch("/api/v1/backup", { method: "POST", body: chosen, headers: authHeaders() });
      if (!res.ok) throw new Error(t("settings.restoreError"));
      window.location.reload();
    } catch {
      setError(copy.settings.restoreFailed);
      setBusy(false);
      setFile(null);
      if (fileRef.current) fileRef.current.value = "";
    }
  }
  const limit = 4;
  const shown = rows ? (all ? rows : rows.slice(0, limit)) : [];
  return (
    <>
      <Group>
        <Row label={h("backups.now")} hint={h("backups.nowHint")}>
          <a className="btn primary" href="/api/v1/backup" download>
            {copy.settings.backup}
          </a>
        </Row>
      </Group>
      <Group title={h("backups.recent")} lede={copy.settings.backupHint}>
        {error ? (
          <p className="set-note error" role="alert">
            {error}
          </p>
        ) : null}
        {rows && rows.length === 0 ? <p className="set-note">{copy.settings.backupEmpty}</p> : null}
        {shown.length > 0 ? (
          <ul className="set-list backup-list">
            {shown.map((row) => {
              const when = backupWhen(row.takenAt);
              const kind = backupKind(row.kind);
              return (
                <li key={row.name} className="set-row backup-row">
                  <div className="set-row-text">
                    <span className="set-row-label">{when}</span>
                    <span className="set-row-hint">
                      {kind} · {formatBytes(row.bytes)}
                    </span>
                  </div>
                  <div className="set-row-control backup-actions">
                    <a className="btn small" href={`/api/v1/backups/${encodeURIComponent(row.name)}`} download aria-label={h("backups.downloadAria", { kind, when })}>
                      {h("backups.download")}
                    </a>
                    <button type="button" className="btn small ghost" disabled={busy} aria-label={h("backups.restoreAria", { kind, when })} onClick={() => setAsking(row)}>
                      {h("backups.restore")}
                    </button>
                  </div>
                </li>
              );
            })}
          </ul>
        ) : null}
        {rows && rows.length > limit ? (
          <div className="set-row more">
            <button type="button" className="btn small ghost" aria-expanded={all} onClick={() => setAll((v) => !v)}>
              {all ? h("backups.fewer") : h("backups.older", rows.length - limit)}
            </button>
          </div>
        ) : null}
      </Group>
      <Group>
        <Row label={h("backups.fromFile")} hint={h("backups.fromFileHint")} labelFor={fileId}>
          <label className="btn small ghost set-file">
            {h("backups.chooseFile")}
            <input
              ref={fileRef}
              id={fileId}
              type="file"
              accept=".db"
              className="sr-only"
              onChange={(event) => {
                const chosen = event.target.files?.[0];
                if (chosen) setFile(chosen);
              }}
            />
          </label>
        </Row>
      </Group>
      {asking ? (
        <Confirm
          title={h("backups.restoreTitle")}
          body={<p>{h("backups.restoreBody", { when: backupWhen(asking.takenAt) })}</p>}
          confirm={busy ? h("backups.restoring") : h("backups.restoreConfirm")}
          cancel={h("backups.cancel")}
          busy={busy}
          onConfirm={() => void restore(asking)}
          onCancel={() => setAsking(null)}
        />
      ) : null}
      {file ? (
        <Confirm
          title={h("backups.fileTitle", { file: file.name })}
          body={<p>{h("backups.fileBody")}</p>}
          confirm={busy ? h("backups.restoring") : h("backups.restoreConfirm")}
          cancel={h("backups.cancel")}
          busy={busy}
          onConfirm={() => void restoreFile(file)}
          onCancel={() => {
            setFile(null);
            if (fileRef.current) fileRef.current.value = "";
          }}
        />
      ) : null}
    </>
  );
}

const docsURL = "https://github.com/wolfebase/broadwave/tree/main/docs/guide";
const sourceURL = "https://github.com/wolfebase/broadwave";

export function AboutSection({ server, settings, onChange }: { server: ServerInfo | null; settings: Settings; onChange: Change }) {
  const notes = server?.update?.notesUrl && /^https:\/\/github\.com\/wolfebase\/broadwave\//.test(server.update.notesUrl) ? server.update.notesUrl : "";
  const encoder = server?.encoder ? server.encoder.replace(/^h264_/, "").toUpperCase() : "";
  return (
    <>
      <Group title={h("about.server")}>
        <Row label={h("about.name")}>
          <span className="set-row-value">{server?.name ?? "—"}</span>
        </Row>
        <Row label={h("about.version")} hint={server?.update?.message}>
          <span className="set-row-value">{server?.version ?? "—"}</span>
        </Row>
        <Row label={h("about.encoder")}>
          <span className="set-row-value">{encoder || h("about.encoderSoftware")}</span>
        </Row>
        <SecureNameRow />
        <SwitchRow label={copy.settings.updates} hint={copy.settings.updatesHint} checked={settings.checkUpdates !== "0"} onChange={(on) => onChange({ checkUpdates: on ? "1" : "0" })} />
      </Group>
      <Group title={h("about.help")}>
        <Row label={h("tuners.diagnostics")} hint={h("about.diagnosticsHint")}>
          <button type="button" className="btn small" onClick={() => navigate("/diagnostics")}>
            {h("tuners.diagnostics")}
          </button>
        </Row>
        <Row label={h("about.support")} hint={h("about.supportHint")}>
          <a className="btn small" href="/api/v1/support" download>
            {copy.settings.backupDownload}
          </a>
        </Row>
        <Row label={h("about.setup")} hint={h("about.setupHint")}>
          <button type="button" className="btn small" onClick={() => navigate("/setup")}>
            {h("tuners.setupAgain")}
          </button>
        </Row>
        <div className="set-row links">
          <a href={docsURL} target="_blank" rel="noreferrer">
            {h("about.docs")}
          </a>
          {notes ? (
            <a href={notes} target="_blank" rel="noreferrer">
              {h("about.notes")}
            </a>
          ) : null}
          <a href={sourceURL} target="_blank" rel="noreferrer">
            {h("about.source")}
          </a>
        </div>
      </Group>
      <Group title={h("about.licenses")}>
        <p className="set-note">{h("about.license")}</p>
        <p className="set-note">{blenderCreditText()}</p>
        <p className="set-note">
          <strong>{copy.settings.passwordTitle}.</strong> {copy.settings.passwordBody}
        </p>
      </Group>
    </>
  );
}

/** The secure name's state, and why it is off when the server turned it off itself. */
function SecureNameRow() {
  const [status, setStatus] = useState<DirectStatus | null>(null);
  useEffect(() => {
    let live = true;
    getDirect()
      .then((next) => {
        if (live) setStatus(next);
      })
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, []);
  if (!status) return null;
  const { value, hint } = secureNameText(status);
  return (
    <Row label={h("about.secure")} hint={hint}>
      <span className="set-row-value">{value}</span>
    </Row>
  );
}

function secureNameText(status: DirectStatus): { value: string; hint?: string } {
  switch (status.state) {
    case "ready":
      return { value: h("about.secureReady"), hint: [status.url ? status.url.replace(/^https:\/\//, "") : "", h("about.secureNote")].filter(Boolean).join(". ") };
    case "starting":
      return { value: h("about.secureStarting"), hint: h("about.secureNote") };
    case "error":
      return { value: h("about.secureError"), hint: status.lastError };
    case "unavailable":
      return { value: h("about.secureOff"), hint: status.reason };
    default:
      return { value: h("about.secureOff") };
  }
}

function backupKind(kind: string) {
  if (kind === "weekly") return copy.settings.backupWeekly;
  if (kind === "version") return copy.settings.backupUpdate;
  return copy.settings.backupNightly;
}

function backupWhen(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return formatDateTime(date, { weekday: "short", month: "short", day: "numeric", hour: "numeric", minute: "2-digit" });
}

/** A text setting saved when the field is left. A save that echoes old text does not erase what was typed. */
export function TextRow({
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
  const id = useId();
  const [typed, setTyped] = useState<string | null>(null);
  const draft = settingTextDraft(value, typed);
  if (draft !== typed) setTyped(draft);
  return (
    <Row label={label} hint={hint} labelFor={id} stack>
      <input
        id={id}
        className="set-input"
        value={shownSettingText(value, draft)}
        autoComplete={autoComplete ?? "off"}
        spellCheck={false}
        placeholder={placeholder}
        onChange={(event) => setTyped(event.target.value)}
        onBlur={(event) => {
          const commit = settingTextCommit(value, event.target.value);
          if (commit != null) onSave(commit);
        }}
      />
    </Row>
  );
}

/** A key or password. It is never shown again; a saved one says Saved. */
function SecretRow({ label, hint, saved, autoComplete, onSave }: { label: string; hint?: string; saved: boolean; autoComplete?: string; onSave: (value: string) => void }) {
  const id = useId();
  return (
    <Row label={label} hint={hint} labelFor={id} stack>
      <input
        id={id}
        className="set-input"
        type="password"
        autoComplete={autoComplete ?? "off"}
        spellCheck={false}
        placeholder={saved ? t("settings.saved") : ""}
        onBlur={(event) => {
          const value = event.target.value;
          if (value) onSave(value);
          event.target.value = "";
        }}
      />
    </Row>
  );
}

function ReserveRow({ value, onSave }: { value: string; onSave: (value: string) => void }) {
  const id = useId();
  const [typed, setTyped] = useState<string | null>(null);
  const draft = settingTextDraft(value, typed);
  if (draft !== typed) setTyped(draft);
  const text = shownSettingText(value, draft);
  return (
    <Row label={copy.settings.reserve} hint={copy.settings.reserveHint} labelFor={id}>
      <span className="set-unit">
        <input
          id={id}
          className="set-input num"
          type="number"
          inputMode="numeric"
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
        <span aria-hidden="true">{h("rec.reserveUnit")}</span>
      </span>
    </Row>
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
