import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { createVirtual, deleteRecording, getServer, setKeep, setWatched, stopRecording } from "../api";
import { pagesMessages } from "../i18n/messages/pagesCopy.ts";
import { bind } from "../i18n/runtime.ts";
import { useData } from "../app/data";
import { copy } from "../strings";
import { gateFeature } from "../lib/compat";
import { focusRing } from "../app/remote";
import { navigate, useRoute } from "../app/router";
import type { ServerInfo } from "../types";
import { Library } from "./library/Library";
import { StoragePlanView } from "./library/StoragePlanView";
import { nextEpisode } from "./library/model";
import { Play } from "./recordings/Play";
import { VirtualPlay } from "./recordings/Virtual";
import { Schedule } from "./schedule/Schedule";
import { HomeList } from "./setup/HomeList";
import { Appearance } from "./settings/Appearance";
import { PairedDevices } from "./settings/PairedDevices";
import { RemoteAccess } from "./settings/RemoteAccess";
import { SettingsScreen } from "./settings/Settings";
import { ShareLinks } from "./settings/ShareLinks";
import { tunerAddress } from "./settings/tunerShare";
import { Sources } from "./settings/Sources";

const t = bind(pagesMessages);

export function RecordingsPage() {
  const { recordings, virtuals, refresh } = useData();
  const { params } = useRoute();
  const show = params.get("show")?.trim() ?? "";
  const storage = params.get("view") === "storage";
  const libraryRef = useRef<HTMLButtonElement>(null);
  const storageRef = useRef<HTMLButtonElement>(null);
  const openView = (next: "library" | "storage") => {
    const query = new URLSearchParams(params);
    if (next === "storage") query.set("view", "storage");
    else query.delete("view");
    const search = query.toString();
    navigate(search ? `/recordings?${search}` : "/recordings");
    focusRing(next === "storage" ? storageRef.current : libraryRef.current);
  };
  const listed =
    show === ""
      ? recordings
      : recordings.filter((rec) => rec.title.trim().toLocaleLowerCase() === show.toLocaleLowerCase());
  const [note, setNote] = useState("");
  return (
    <div className="page-wrap">
      <header className="page-header">
        <h1>{t("pages.recordings")}</h1>
        <div className="guide-chips">
          <div className="segmented" role="group" aria-label={t("pages.recordings")}>
            <button ref={libraryRef} type="button" className={storage ? "seg" : "seg on"} aria-pressed={!storage} onClick={() => openView("library")}>
              {t("pages.library")}
            </button>
            <button ref={storageRef} type="button" className={storage ? "seg on" : "seg"} aria-pressed={storage} onClick={() => openView("storage")}>
              {t("pages.storage")}
            </button>
          </div>
        </div>
      </header>
      {storage ? (
        <StoragePlanView />
      ) : (
        <Library
          key={show}
          recordings={listed}
          show={show}
          note={note}
          onMany={async (action, recs) => {
            const results = await Promise.allSettled(recs.map((r) => (action === "delete" ? deleteRecording(r.id) : setWatched(r.id, action === "watched"))));
            const failed = results.filter((r) => r.status === "rejected") as PromiseRejectedResult[];
            const done = recs.length - failed.length;
            const said = action === "delete" ? t("pages.deletedMany", done) : action === "watched" ? t("pages.markedWatched", done) : t("pages.markedUnwatched", done);
            setNote(failed.length === 0 ? said : `${done > 0 ? `${said} ` : ""}${t("pages.couldNotChange", { count: failed.length, reason: String(failed[0].reason?.message ?? failed[0].reason) })}`);
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
            void deleteRecording(r.id).then(
              () => {
                setNote(t("pages.deleted", { title: r.title }));
                return refresh(["recordings"]);
              },
              (err: unknown) => setNote(err instanceof Error ? err.message : String(err)),
            )
          }
          onStop={(r) =>
            void stopRecording(r.id).then(() => {
              setNote(t("pages.stopped", { title: r.title }));
              return refresh(["recordings"]);
            })
          }
          onVirtual={(r) => {
            const used = new Set(virtuals.map((v) => v.number));
            let n = 900;
            while (used.has(String(n))) n++;
            void createVirtual(String(n), t("pages.virtualName", { title: r.title }), [r.id]).then(() => {
              setNote(t("pages.virtualNote", { number: n, title: r.title }));
              return refresh(["virtuals"]);
            });
          }}
        />
      )}
    </div>
  );
}

export function SchedulePage() {
  const { recordings, passes, refresh } = useData();
  return (
    <div className="page-wrap">
      <header className="page-header">
        <h1>{copy.schedule.title}</h1>
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
  const hdhrNote = gateFeature(server, "hdhrEmulation");
  const address = tunerAddress(window.location.href);
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
        <h1>{copy.settings.title}</h1>
        {server ? (
          <p className="page-sub">
            {t("pages.serverVersion", { name: server.name, version: server.version })}
            {server.encoder ? ` · ${t("pages.encoding", { encoder: server.encoder.replace("h264_", "").toUpperCase() })}` : ""}
          </p>
        ) : null}
      </header>
      <div className="setup-row" style={{ marginBottom: 8 }}>
        <button type="button" className="btn" onClick={() => navigate("/diagnostics")}>
          {t("pages.diagnostics")}
        </button>
        <button type="button" className="btn" onClick={() => navigate("/setup")}>
          {t("pages.setupAgain")}
        </button>
        <button type="button" className="btn" onClick={() => navigate("/about")} aria-label={t("pages.aboutAria")}>
          {t("pages.about")}
        </button>
      </div>
      <Appearance />
      <section className="settings-section">
        <HomeList heading="h2" />
      </section>
      <section id="sources" className="settings-section">
        <h2>{t("pages.tuners")}</h2>
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
        <h2>{t("pages.playback")}</h2>
        <SettingsScreen settings={settings} storage={storage} tunerCount={server?.tunerCount} onChange={(v) => void saveSettings(v)} />
      </section>
      <section className="settings-section">
        <PairedDevices deviceAuth={settings.deviceAuth ?? "0"} onChange={saveSettings} />
      </section>
      <section className="settings-section">
        <RemoteAccess onChange={saveSettings} />
      </section>
      <section className="settings-section">
        <h2>{t("pages.share")}</h2>
        <p className="dim">{t("pages.shareLead")}</p>
        {hdhrNote ? (
          <p className="hint" role="status">{hdhrNote}</p>
        ) : (
          <label className="switch-row">
            <input type="checkbox" checked={settings.hdhrEmulate === "1"} onChange={(e) => void saveSettings({ hdhrEmulate: e.target.checked ? "1" : "0" })} />
            <span>{t("pages.hdhr", { address })}</span>
          </label>
        )}
        {hdhrNote ? null : <p className="hint">{t("pages.hdhrHint")}</p>}
        <ShareLinks pageURL={window.location.href} sharing={settings.hdhrEmulate === "1"} hdhr={!hdhrNote} />
        <h3 className="section-title">{t("pages.mosaics")}</h3>
        {mosaics.some(Boolean) ? (
          <ul className="share-mosaics">
            {mosaics.map((key, i) =>
              key ? (
                <li key={key}>
                  <span>
                    {t("pages.mosaicLine", { slot: i + 1, name: mosaicName(key) })}
                    {listed(key) ? null : <span className="hint">{t("pages.mosaicHidden")}</span>}
                  </span>
                  <button type="button" className="btn small" aria-label={t("pages.removeMosaic", { name: mosaicName(key) })} onClick={() => void saveSettings({ exportMosaics: removeMosaic(mosaics, key) })}>
                    {t("pages.remove")}
                  </button>
                </li>
              ) : null,
            )}
          </ul>
        ) : (
          <p className="hint">{t("pages.mosaicEmpty")}</p>
        )}
      </section>
    </div>
  );
}
