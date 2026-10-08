import { useCallback, useEffect, useMemo, useState } from "react";
import { createVirtual, deleteRecording, getServer, setKeep, setWatched, stopRecording } from "../api";
import { useData } from "../app/data";
import { gateFeature } from "../lib/compat";
import { navigate, useRoute } from "../app/router";
import type { ServerInfo } from "../types";
import { Library } from "./library/Library";
import { nextEpisode } from "./library/model";
import { Play } from "./recordings/Play";
import { VirtualPlay } from "./recordings/Virtual";
import { Schedule } from "./schedule/Schedule";
import { HomeList } from "./setup/HomeList";
import { SettingsScreen } from "./settings/Settings";
import { Sources } from "./settings/Sources";

export function RecordingsPage() {
  const { recordings, virtuals, refresh } = useData();
  const { params } = useRoute();
  const show = params.get("show")?.trim() ?? "";
  const listed =
    show === ""
      ? recordings
      : recordings.filter((rec) => rec.title.trim().toLocaleLowerCase() === show.toLocaleLowerCase());
  const [note, setNote] = useState("");
  return (
    <div className="page-wrap">
      <header className="page-header">
        <h1>Recordings</h1>
      </header>
      <Library
        key={show}
        recordings={listed}
        show={show}
        note={note}
        onMany={async (action, recs) => {
          const results = await Promise.allSettled(recs.map((r) => (action === "delete" ? deleteRecording(r.id) : setWatched(r.id, action === "watched"))));
          const failed = results.filter((r) => r.status === "rejected") as PromiseRejectedResult[];
          const done = recs.length - failed.length;
          const what = done === 1 ? "1 recording" : `${done} recordings`;
          const said = action === "delete" ? `Deleted ${what}.` : `Marked ${what} ${action}.`;
          setNote(failed.length === 0 ? said : `${done > 0 ? `${said} ` : ""}${failed.length} could not be changed: ${String(failed[0].reason?.message ?? failed[0].reason)}`);
          await refresh(["recordings"]);
        }}
        onPlay={(r) => navigate(`/play?recording=${r.id}`)}
        onWatched={(r, flag) => void setWatched(r.id, flag).then(() => refresh(["recordings"]))}
        onKeep={(r, keep) =>
          void setKeep(r.id, keep).then(
            () => refresh(["recordings"]),
            (err: unknown) => setNote(err instanceof Error ? err.message : String(err)),
          )
        }
        onDelete={(r) =>
          void deleteRecording(r.id).then(() => {
            setNote(`Deleted ${r.title}.`);
            return refresh(["recordings"]);
          })
        }
        onStop={(r) =>
          void stopRecording(r.id).then(() => {
            setNote(`Stopped ${r.title}. What it recorded is kept.`);
            return refresh(["recordings"]);
          })
        }
        onVirtual={(r) => {
          const used = new Set(virtuals.map((v) => v.number));
          let n = 900;
          while (used.has(String(n))) n++;
          void createVirtual(String(n), `${r.title} channel`, [r.id]).then(() => {
            setNote(`Channel ${n} now plays ${r.title} around the clock, without a tuner.`);
            return refresh(["virtuals"]);
          });
        }}
      />
    </div>
  );
}

export function SchedulePage() {
  const { recordings, passes, refresh } = useData();
  return (
    <div className="page-wrap">
      <header className="page-header">
        <h1>Schedule</h1>
      </header>
      <Schedule recordings={recordings} passes={passes} onStop={() => void refresh(["recordings"])} onPasses={() => void refresh(["passes"])} />
    </div>
  );
}

export function PlayPage() {
  const { params } = useRoute();
  const { recordings, settings } = useData();
  const id = Number(params.get("recording") || 0);
  const rec = recordings.find((r) => r.id === id);
  const next = useMemo(() => (rec ? nextEpisode(recordings, rec) : undefined), [recordings, rec]);
  const onNext = useCallback(() => {
    if (next) navigate(`/play?recording=${next.id}`, true);
  }, [next]);
  if (!rec) return null;
  return (
    <div className="play-page">
      <Play
        recording={rec}
        pictureMode={settings.pictureMode ?? "broadcast"}
        autoplay={settings.autoplay !== "0"}
        next={next}
        onNext={onNext}
        onBack={() => window.history.back()}
      />
    </div>
  );
}

export function VirtualPage() {
  const { params } = useRoute();
  const { settings } = useData();
  return (
    <div className="play-page">
      <VirtualPlay id={Number(params.get("virtual") || 0)} pictureMode={settings.pictureMode ?? "broadcast"} onBack={() => navigate("/guide")} />
    </div>
  );
}

export function SettingsPage() {
  const { settings, storage, saveSettings, devices, channels, allChannels, error, rediscover, forgetDevice, editChannel, server: known } = useData();
  const [busy, setBusy] = useState(false);
  const [fetched, setFetched] = useState<ServerInfo | null>(null);
  const server = fetched ?? known;
  useEffect(() => {
    void getServer().then(setFetched).catch(() => undefined);
  }, []);
  const origin = window.location.origin;
  const host = window.location.hostname;
  const hdhrNote = gateFeature(server, "hdhrEmulation");
  const mosaics = (settings.exportMosaics ?? "").split(",");
  const mosaicName = (key: string) =>
    key
      .split("-")
      .map((id) => allChannels.find((c) => c.id === Number(id))?.displayName ?? id)
      .join(" + ");
  const listed = (key: string) => key.split("-").every((id) => channels.some((c) => c.id === Number(id)));
  const removeMosaic = (list: string[], key: string) => {
    const next = list.map((k) => (k === key ? "" : k));
    while (next.length && next[next.length - 1] === "") next.pop();
    return next.join(",");
  };
  return (
    <div className="page-wrap settings-page">
      <header className="page-header">
        <h1>Settings</h1>
        {server ? (
          <p className="page-sub">
            {server.name} · version {server.version}
            {server.encoder ? ` · ${server.encoder.replace("h264_", "").toUpperCase()} encoding` : ""}
          </p>
        ) : null}
      </header>
      <div className="setup-row" style={{ marginBottom: 8 }}>
        <button type="button" className="btn" onClick={() => navigate("/diagnostics")}>
          Diagnostics
        </button>
        <button type="button" className="btn" onClick={() => navigate("/setup")}>
          Run setup again
        </button>
        <button type="button" className="btn" onClick={() => navigate("/about")} aria-label="About Broadwave">
          About
        </button>
      </div>
      <section className="settings-section">
        <HomeList heading="h2" />
      </section>
      <section id="sources" className="settings-section">
        <h2>Tuners and channels</h2>
        <Sources
          devices={devices}
          channels={allChannels}
          busy={busy}
          error={error}
          onDiscover={() => {
            setBusy(true);
            void rediscover().finally(() => setBusy(false));
          }}
          onLookup={(ip) => {
            setBusy(true);
            void rediscover(ip).finally(() => setBusy(false));
          }}
          onPatch={(c, patch) => void editChannel(c, patch)}
          onRemove={(id) => {
            setBusy(true);
            void forgetDevice(id).finally(() => setBusy(false));
          }}
        />
      </section>
      <section className="settings-section">
        <h2>Playback and recording</h2>
        <SettingsScreen settings={settings} storage={storage} features={server?.features} tunerCount={server?.tunerCount} onChange={(v) => void saveSettings(v)} />
      </section>
      <section className="settings-section">
        <h2>Share with other apps</h2>
        <p className="dim">Plex, Jellyfin, and Channels can watch through this server. They share its tuners, so they never fight over one.</p>
        {hdhrNote ? (
          <p className="hint" role="status">{hdhrNote}</p>
        ) : (
          <label className="switch-row">
            <input type="checkbox" checked={settings.hdhrEmulate === "1"} onChange={(e) => void saveSettings({ hdhrEmulate: e.target.checked ? "1" : "0" })} />
            <span>Act as an HDHomeRun at {host}:8478</span>
          </label>
        )}
        <dl className="share-urls">
          <dt>M3U playlist</dt>
          <dd>
            <code>{origin}/export/lineup.m3u</code>
          </dd>
          <dt>XMLTV guide</dt>
          <dd>
            <code>{origin}/export/guide.xml</code>
          </dd>
        </dl>
        <h3 className="section-title">Multiview channels</h3>
        {mosaics.some(Boolean) ? (
          <ul className="share-mosaics">
            {mosaics.map((key, i) =>
              key ? (
                <li key={key}>
                  <span>
                    990.{i + 1} · Multiview: {mosaicName(key)}
                    {listed(key) ? null : <span className="hint"> Not listed: a channel in it is hidden or gone.</span>}
                  </span>
                  <button type="button" className="btn small" aria-label={`Remove Multiview: ${mosaicName(key)}`} onClick={() => void saveSettings({ exportMosaics: removeMosaic(mosaics, key) })}>
                    Remove
                  </button>
                </li>
              ) : null,
            )}
          </ul>
        ) : (
          <p className="hint">Other apps can show 2 to 4 channels at once as one channel. Open a multiview and choose Add to other apps.</p>
        )}
      </section>
    </div>
  );
}
