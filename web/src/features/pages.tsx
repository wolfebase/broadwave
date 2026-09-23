import { useEffect, useState } from "react";
import { createVirtual, deleteRecording, getServer, setWatched } from "../api";
import { useData } from "../app/data";
import { navigate, useRoute } from "../app/router";
import type { ServerInfo } from "../types";
import { Library } from "./library/Library";
import { Play } from "./recordings/Play";
import { VirtualPlay } from "./recordings/Virtual";
import { Schedule } from "./schedule/Schedule";
import { SettingsScreen } from "./settings/Settings";
import { Sources } from "./settings/Sources";

export function RecordingsPage() {
  const { recordings, virtuals, refresh } = useData();
  const [note, setNote] = useState("");
  return (
    <div className="page-wrap">
      <header className="page-header">
        <h1>Recordings</h1>
      </header>
      <Library
        recordings={recordings}
        note={note}
        onPlay={(r) => navigate(`/play?recording=${r.id}`)}
        onWatched={(r, flag) => void setWatched(r.id, flag).then(() => refresh(["recordings"]))}
        onDelete={(r) =>
          void deleteRecording(r.id).then(() => {
            setNote(`Deleted ${r.title}.`);
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
  if (!rec) return null;
  return (
    <div className="play-page">
      <Play
        recording={rec}
        pictureMode={settings.pictureMode}
        autoplay={settings.autoplay !== "0"}
        onNext={() => {
          const later = recordings.filter((r) => r.title === rec.title && r.id !== rec.id && r.status !== "recording").sort((a, b) => a.startedAt.localeCompare(b.startedAt));
          const next = later.find((r) => r.startedAt > rec.startedAt) ?? later[0];
          if (next) navigate(`/play?recording=${next.id}`, true);
        }}
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
      <VirtualPlay id={Number(params.get("virtual") || 0)} pictureMode={settings.pictureMode} onBack={() => navigate("/guide")} />
    </div>
  );
}

export function SettingsPage() {
  const { settings, storage, saveSettings, devices, allChannels, error, rediscover, editChannel } = useData();
  const [busy, setBusy] = useState(false);
  const [server, setServer] = useState<ServerInfo | null>(null);
  useEffect(() => {
    void getServer().then(setServer).catch(() => undefined);
  }, []);
  const origin = window.location.origin;
  const host = window.location.hostname;
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
        />
      </section>
      <section className="settings-section">
        <h2>Playback and recording</h2>
        <SettingsScreen settings={settings} storage={storage} onChange={(v) => void saveSettings(v)} />
      </section>
      <section className="settings-section">
        <h2>Share with other apps</h2>
        <p className="dim">Plex, Jellyfin, and Channels can watch through this server. They share its tuners, so they never fight over one.</p>
        <label className="switch-row">
          <input type="checkbox" checked={settings.hdhrEmulate === "1"} onChange={(e) => void saveSettings({ hdhrEmulate: e.target.checked ? "1" : "0" })} />
          <span>Act as an HDHomeRun at {host}:8478</span>
        </label>
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
      </section>
    </div>
  );
}
