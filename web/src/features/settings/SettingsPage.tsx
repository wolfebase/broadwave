import { useEffect, useRef, useState, type ComponentType, type ReactNode, type SVGProps } from "react";
import { getHome, getServer } from "../../api";
import { pagesMessages } from "../../i18n/messages/pagesCopy.ts";
import { settingsHubMessages } from "../../i18n/messages/settingsHub.ts";
import { settingsUiMessages } from "../../i18n/messages/settingsUi.ts";
import { bind } from "../../i18n/runtime.ts";
import { useData } from "../../app/data";
import { useLayout } from "../../app/layout";
import { focusRing } from "../../app/remote";
import { copy } from "../../strings";
import { gateFeature } from "../../lib/compat";
import { formatBytes } from "../../lib/format";
import { isTextField } from "../../lib/textField";
import { inAppDepth, navigate, useRoute } from "../../app/router";
import { readAccent, readTheme } from "../../theme/look";
import { readLiveDelay } from "../../lib/events";
import type { ServerInfo } from "../../types";
import { Appearance, accentName } from "./Appearance";
import { DevicesSection } from "./Devices";
import { AboutSection, BackupsSection, GuideSection, PlaybackSection, RecordingSection } from "./Settings";
import { ShareLinks } from "./ShareLinks";
import { isLoopback, sharePageURL, tunerAddress } from "./tunerShare";
import { Sources } from "./Sources";
import { Group, SwitchRow } from "./rows";
import { sectionFromPath, sectionIds, sectionPath, type SectionId } from "./sections";
import { AboutIcon, AntennaIcon, AppearanceIcon, BackupIcon, ChevronLeft, ChevronRight, DevicesIcon, DiskIcon, GuideDataIcon, LinkIcon, PlaybackIcon } from "./icons";
import "../manage.css";
import "./settings.css";

const t = bind(pagesMessages);
const h = bind(settingsHubMessages);
const ui = bind(settingsUiMessages);

const icons: Record<SectionId, ComponentType<SVGProps<SVGSVGElement>>> = {
  appearance: AppearanceIcon,
  tuners: AntennaIcon,
  playback: PlaybackIcon,
  guide: GuideDataIcon,
  recording: DiskIcon,
  devices: DevicesIcon,
  integrations: LinkIcon,
  backups: BackupIcon,
  about: AboutIcon,
};

// Set when a row of the Settings list opened the section, so Back returns to the list.
let fromList = false;

function open(id: SectionId, viaList: boolean) {
  fromList = viaList;
  navigate(sectionPath(id));
}

function backToList() {
  if (fromList && inAppDepth() > 0) {
    fromList = false;
    window.history.back();
    return;
  }
  navigate("/settings", true);
}

function linkClick(event: React.MouseEvent<HTMLAnchorElement>, go: () => void) {
  if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) return;
  event.preventDefault();
  go();
}

export function SettingsPage() {
  const data = useData();
  const { settings, storage, devices, allChannels, server: known } = data;
  const { path } = useRoute();
  const layout = useLayout();
  const [fetched, setFetched] = useState<ServerInfo | null>(null);
  const server = fetched ?? known;
  useEffect(() => {
    void getServer().then(setFetched).catch(() => undefined);
  }, []);
  const picked = sectionFromPath(path, typeof window === "undefined" ? "" : window.location.hash);
  // A desktop or tablet window shows the list beside the section. Phone and TV go one level at a time.
  const split = layout === "desktop";
  const id: SectionId | null = picked ?? (split ? "appearance" : null);
  const detailRef = useRef<HTMLElement>(null);
  const listRef = useRef<HTMLUListElement>(null);
  const lastId = useRef<SectionId | null>(null);

  // Old addresses (/sources, /settings#sources, /about) land on their section's own address.
  useEffect(() => {
    if (!picked) return;
    const want = sectionPath(picked);
    // Through the router, so the next render reads the new path and not a hash that is gone.
    if (path !== want) navigate(want, true);
  }, [picked, path]);

  // On a TV, focus starts on the section's first control, or on the row a viewer came back from.
  useEffect(() => {
    if (layout !== "tv") return;
    const frame = window.requestAnimationFrame(() => {
      if (id) {
        const first = detailRef.current?.querySelector<HTMLElement>(".settings-body button:not([disabled]), .settings-body a[href], .settings-body input:not([disabled]), .settings-body select");
        focusRing(first ?? detailRef.current?.querySelector<HTMLElement>(".settings-back"));
      } else {
        const back = lastId.current ? listRef.current?.querySelector<HTMLElement>(`[data-section="${lastId.current}"]`) : null;
        focusRing(back ?? listRef.current?.querySelector<HTMLElement>("a"));
      }
    });
    lastId.current = id;
    return () => window.cancelAnimationFrame(frame);
  }, [id, layout]);

  // A section opened from a link has nothing behind it in this tab. Back goes to the list, not out of the app.
  useEffect(() => {
    if (split || !picked) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.defaultPrevented || (event.key !== "Escape" && event.key !== "Backspace")) return;
      if (isTextField(event.target instanceof HTMLElement ? event.target : null)) return;
      if (document.querySelector("[role='dialog']")) return;
      if (inAppDepth() > 0 && fromList) return;
      event.preventDefault();
      backToList();
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [split, picked]);

  const titles: Record<SectionId, string> = {
    appearance: h("section.appearance"),
    tuners: h("section.tuners"),
    playback: h("section.playback"),
    guide: h("section.guide"),
    recording: h("section.recording"),
    devices: h("section.devices"),
    integrations: h("section.integrations"),
    backups: h("section.backups"),
    about: h("section.about"),
  };
  const offline = devices.filter((device) => device.offline).length;
  const summaries: Record<SectionId, string> = {
    appearance: h("sum.appearance", { theme: themeWord(), accent: accentName(readAccent()) }),
    tuners: [h("sum.tuners", devices.length), offline ? h("sum.tunersOffline", offline) : "", h("sum.channels", allChannels.filter((c) => c.present && !c.sameAs).length)].filter(Boolean).join(" · "),
    playback: h("sum.playback", { motion: motionWord(settings.pictureMode), delay: delayWord() }),
    guide: settings.sdUser ? h("sum.guideSd") : settings.guideUrl ? h("sum.guideXmltv") : h("sum.guideTuner"),
    recording: storage ? h("sum.recording", { free: formatBytes(storage.freeBytes) }) : h("sum.recordingUnknown"),
    devices: settings.deviceAuth === "1" ? h("sum.devicesOn") : h("sum.devicesOff"),
    integrations: settings.hdhrEmulate === "1" ? h("sum.integrationsOn") : h("sum.integrationsOff"),
    backups: h("sum.backups"),
    about: server?.version ? h("sum.about", { version: server.version }) : h("sum.aboutUnknown"),
  };

  const serverLine = server ? h("hub.server", { name: server.name, version: server.version }) : "";

  if (!id) {
    return (
      <div className="page-wrap settings-hub hub-list">
        <header className="page-header">
          <h1>{copy.settings.title}</h1>
          {serverLine ? <p className="page-sub">{serverLine}</p> : null}
        </header>
        <nav aria-label={h("hub.nav")}>
          <ul ref={listRef} className="settings-menu">
            {sectionIds.map((item) => {
              const Icon = icons[item];
              return (
                <li key={item}>
                  <a href={sectionPath(item)} data-section={item} className="settings-menu-row" onClick={(event) => linkClick(event, () => open(item, true))}>
                    <span className={`settings-menu-icon ${item}`} aria-hidden="true">
                      <Icon />
                    </span>
                    <span className="settings-menu-text">
                      <span className="settings-menu-title">{titles[item]}</span>
                      <span className="settings-menu-sum">{summaries[item]}</span>
                    </span>
                    <ChevronRight className="settings-menu-chev" />
                  </a>
                </li>
              );
            })}
          </ul>
        </nav>
      </div>
    );
  }

  const lede = id === "about" ? serverLine : h(`lede.${id}` as "lede.appearance");
  const body = <SectionBody id={id} server={server} />;

  // One tree for both shapes, so the section's controls survive a window crossing between them.
  return (
    <div className={split ? "page-wrap settings-hub hub-split" : "page-wrap settings-hub hub-detail"}>
      {split ? (
        <header className="page-header">
          <h1>{copy.settings.title}</h1>
          {serverLine ? <p className="page-sub">{serverLine}</p> : null}
        </header>
      ) : null}
      <div className="settings-split">
        {split ? (
          <nav className="settings-nav" aria-label={h("hub.nav")}>
            <ul>
              {sectionIds.map((item) => {
                const Icon = icons[item];
                return (
                  <li key={item}>
                    <a href={sectionPath(item)} aria-current={item === id ? "page" : undefined} className={item === id ? "settings-nav-link on" : "settings-nav-link"} onClick={(event) => linkClick(event, () => open(item, false))}>
                      <span className={`settings-menu-icon ${item}`} aria-hidden="true">
                        <Icon />
                      </span>
                      <span>{titles[item]}</span>
                    </a>
                  </li>
                );
              })}
            </ul>
          </nav>
        ) : null}
        <section ref={detailRef} className="settings-detail" aria-labelledby="settings-section-title">
          <header className="settings-detail-head">
            {split ? null : (
              <button type="button" className="settings-back" aria-label={h("hub.backAria")} onClick={backToList}>
                <ChevronLeft />
                <span>{h("hub.back")}</span>
              </button>
            )}
            {split ? <h2 id="settings-section-title">{titles[id]}</h2> : <h1 id="settings-section-title">{titles[id]}</h1>}
            {lede ? <p className="settings-lede">{lede}</p> : null}
          </header>
          <div className="settings-body" key={id}>
            {body}
          </div>
        </section>
      </div>
    </div>
  );
}

function SectionBody({ id, server }: { id: SectionId; server: ServerInfo | null }): ReactNode {
  const { settings, storage, saveSettings, devices, channels, allChannels, error, rediscover, forgetDevice, editChannel, refresh } = useData();
  const [busy, setBusy] = useState(false);
  const change = (values: Parameters<typeof saveSettings>[0]) => void saveSettings(values);
  switch (id) {
    case "appearance":
      return <Appearance settings={settings} onChange={change} />;
    case "tuners":
      return (
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
          onRemove={(deviceId) => {
            setBusy(true);
            void forgetDevice(deviceId).finally(() => setBusy(false));
          }}
          onRefresh={() => refresh(["devices", "channels"])}
        />
      );
    case "playback":
      return <PlaybackSection settings={settings} onChange={change} />;
    case "guide":
      return <GuideSection settings={settings} onChange={change} />;
    case "recording":
      return <RecordingSection settings={settings} storage={storage} onChange={change} />;
    case "devices":
      return <DevicesSection settings={settings} onChange={saveSettings} />;
    case "integrations":
      return <Integrations server={server} channels={channels} allChannels={allChannels} />;
    case "backups":
      return <BackupsSection />;
    case "about":
      return <AboutSection server={server} settings={settings} onChange={change} />;
  }
}

function Integrations({ server, channels, allChannels }: { server: ServerInfo | null; channels: { id: number }[]; allChannels: { id: number; displayName: string }[] }) {
  const { settings, saveSettings } = useData();
  const hdhrNote = gateFeature(server, "hdhrEmulation");
  // Opened at localhost, the addresses come from the server's home network address.
  const [lanTuner, setLanTuner] = useState("");
  useEffect(() => {
    if (!isLoopback(window.location.hostname)) return;
    let stop = false;
    void getHome()
      .then((res) => {
        if (!stop) setLanTuner(res.tunerAddress);
      })
      .catch(() => undefined);
    return () => {
      stop = true;
    };
  }, []);
  const pageURL = sharePageURL(window.location.href, lanTuner);
  const address = tunerAddress(pageURL);
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
    <>
      <Group>
        {hdhrNote ? (
          <p className="set-note" role="status">
            {hdhrNote}
          </p>
        ) : (
          <SwitchRow
            label={h("share.sharing")}
            hint={h("share.sharingHint", { address })}
            checked={settings.hdhrEmulate === "1"}
            onChange={(on) => void saveSettings({ hdhrEmulate: on ? "1" : "0" })}
          />
        )}
        <div className="set-row stack">
          <ShareLinks pageURL={pageURL} sharing={settings.hdhrEmulate === "1"} hdhr={!hdhrNote} />
        </div>
      </Group>
      <Group title={t("pages.mosaics")}>
        {mosaics.map((key, i) =>
          key ? (
            <div key={key} className="set-row">
              <div className="set-row-text">
                <span className="set-row-label">{t("pages.mosaicLine", { slot: i + 1, name: mosaicName(key) })}</span>
                {listed(key) ? null : <span className="set-row-hint">{t("pages.mosaicHidden")}</span>}
              </div>
              <button type="button" className="btn small ghost" aria-label={t("pages.removeMosaic", { name: mosaicName(key) })} onClick={() => void saveSettings({ exportMosaics: removeMosaic(mosaics, key) })}>
                {t("pages.remove")}
              </button>
            </div>
          ) : null,
        )}
        {mosaics.some(Boolean) ? null : <p className="set-note">{t("pages.mosaicEmpty")}</p>}
      </Group>
    </>
  );
}

function themeWord() {
  const theme = readTheme();
  return theme === "system" ? h("sum.themeSystem") : theme === "light" ? h("sum.themeLight") : h("sum.themeDark");
}

function motionWord(mode: string | undefined) {
  if (mode === "smooth") return ui("settings.smooth");
  if (mode === "film") return ui("settings.film");
  return ui("settings.broadcast");
}

function delayWord() {
  const delay = readLiveDelay();
  return delay === "lowest" ? ui("settings.lowest") : delay === "stable" ? ui("settings.stable") : ui("settings.balanced");
}
