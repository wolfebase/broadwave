import { useEffect, useState } from "react";
import type { Settings, StorageInfo } from "../../types";
import { getEvents, getTuners } from "../../api";
import { copy } from "../../strings";
import { formatBytes } from "../../lib/format";
export function SettingsScreen({
  settings,
  storage,
  onChange,
}: {
  settings: Settings;
  storage: StorageInfo | null;
  onChange: (values: Partial<Settings>) => void;
}) {
  const [encoder, setEncoder] = useState("");
  const [tunerLine, setTunerLine] = useState("Checking tuners.");
  const [lastEvent, setLastEvent] = useState("");
  useEffect(() => {
    let stop = false;
    void getTuners()
      .then((res) => {
        if (stop) return;
        setEncoder(res.encoder || "");
        const busy = (res.tuners ?? []).filter((tuner) => tuner.guide || tuner.target).length;
        const total = (res.tuners ?? []).length;
        if (total === 0) setTunerLine("Tuner status is not available yet.");
        else if (busy === 0) setTunerLine(`All ${total} tuners are free.`);
        else setTunerLine(`${busy} of ${total} tuners are in use.`);
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
  }, []);
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
      <label className="field">
        {copy.settings.hideScores}
        <select value={settings.hideScores || "0"} onChange={(event) => onChange({ hideScores: event.target.value })}>
          <option value="0">Show</option>
          <option value="1">Hide</option>
        </select>
        <span className="hint">{copy.settings.hideScoresHint}</span>
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
      <label className="field">
        Offer this server as an HDHomeRun on port 8478
        <select value={settings.hdhrEmulate || "0"} onChange={(event) => onChange({ hdhrEmulate: event.target.value })}>
          <option value="0">Off</option>
          <option value="1">On</option>
        </select>
        <span className="hint">Other apps can add this machine on port 8478. Discovery stays quiet so the real tuner is unchanged. The change applies within a minute.</span>
      </label>
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
        <input
          value={settings.recordingsPath}
          onChange={(event) => onChange({ recordingsPath: event.target.value })}
          spellCheck={false}
        />
        <span className="hint">{copy.settings.recordingsHint}</span>
      </label>
      <ReserveField value={settings.watermarkGB || "10"} storage={storage} onSave={(watermarkGB) => onChange({ watermarkGB })} />
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
          void fetch("/api/v1/backup", { method: "POST", body: file }).then((res) => {
            if (!res.ok) throw new Error("Restore failed");
            window.location.reload();
          });
        }}
      >
        <label>
          Restore a catalog backup
          <input name="backup" type="file" accept=".db" />
        </label>
        <button type="submit" className="btn">Restore</button>
      </form>
      <p className="hint">{copy.settings.backupHint}</p>
      <article className="quiet-card wide">
        <h3>{copy.settings.passwordTitle}</h3>
        <p>{copy.settings.passwordBody}</p>
      </article>
    </section>
  );
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

function storageLine(storage: StorageInfo) {
  const reserve = storage.watermarkGB === 0 ? "The free-space reserve is off." : `New recordings stop under ${storage.watermarkGB} GB.`;
  return `${formatBytes(storage.freeBytes)} free of ${formatBytes(storage.totalBytes)}. ${reserve}`;
}
