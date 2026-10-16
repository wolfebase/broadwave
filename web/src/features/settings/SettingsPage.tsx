import { useEffect, useState } from "react";
import { getServer } from "../../api";
import { pagesMessages } from "../../i18n/messages/pagesCopy.ts";
import { bind } from "../../i18n/runtime.ts";
import { useData } from "../../app/data";
import { copy } from "../../strings";
import { gateFeature } from "../../lib/compat";
import { navigate } from "../../app/router";
import type { ServerInfo } from "../../types";
import { HomeList } from "../setup/HomeList";
import { Appearance } from "./Appearance";
import { PairedDevices } from "./PairedDevices";
import { RemoteAccess } from "./RemoteAccess";
import { SettingsScreen } from "./Settings";
import { ShareLinks } from "./ShareLinks";
import { tunerAddress } from "./tunerShare";
import { Sources } from "./Sources";

const t = bind(pagesMessages);

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
