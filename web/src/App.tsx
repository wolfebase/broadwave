import { useCallback, useEffect, useState } from "react";
import { addPass, createVirtual, deleteRecording, discover, getAirings, getChannels, getDevices, getPasses, getRecordings, getSettings, getStorage, getVirtuals, patchChannel, putSettings, refreshGuide, setWatched, startRecording } from "./api";
import { Guide } from "./Guide";
import { Play } from "./Play";
import { Home, Library, Schedule, SettingsScreen, Sources } from "./screens";
import { copy } from "./strings";
import type { Airing, Channel, ChannelPatch, Device, Pass, Recording, Settings, StorageInfo, VirtualChannel } from "./types";
import { VirtualPlay } from "./Virtual";
import { Watch } from "./Watch";

type Screen = "home" | "guide" | "library" | "schedule" | "sources" | "settings";

const nav: { id: Screen; label: string; path: string }[] = [
  { id: "home", label: copy.nav.home, path: "/" },
  { id: "guide", label: copy.nav.guide, path: "/guide" },
  { id: "library", label: copy.nav.library, path: "/library" },
  { id: "schedule", label: copy.nav.schedule, path: "/schedule" },
  { id: "sources", label: copy.nav.sources, path: "/sources" },
  { id: "settings", label: copy.nav.settings, path: "/settings" },
];

function screenFromPath(path: string): Screen {
  const bare = path.split("?")[0];
  const hit = nav.find((item) => item.path !== "/" && bare.startsWith(item.path));
  return hit?.id ?? "home";
}

function nextVirtualNumber(virtuals: VirtualChannel[]) {
  const used = new Set(virtuals.map((item) => item.number));
  let number = 900;
  while (used.has(String(number))) number += 1;
  return String(number);
}

export function App() {
  const [path, setPath] = useState(() => window.location.pathname + window.location.search);
  const [guideChannels, setGuideChannels] = useState<Channel[]>([]);
  const [allChannels, setAllChannels] = useState<Channel[]>([]);
  const [devices, setDevices] = useState<Device[]>([]);
  const [settings, setSettings] = useState<Settings>({ layout: "auto", recordingsPath: "", profile: "transparent", audio: "stereo", watermarkGB: "10", pictureMode: "broadcast", autoplay: "1", hdhrEmulate: "0" });
  const [storage, setStorage] = useState<StorageInfo | null>(null);
  const [airings, setAirings] = useState<Airing[]>([]);
  const [recordings, setRecordings] = useState<Recording[]>([]);
  const [passes, setPasses] = useState<Pass[]>([]);
  const [virtuals, setVirtuals] = useState<VirtualChannel[]>([]);
  const [libraryNote, setLibraryNote] = useState("");
  const [ready, setReady] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [now, setNow] = useState(() => new Date());
  const [narrow, setNarrow] = useState(false);
  const [coarse, setCoarse] = useState(false);

  const screen = screenFromPath(path);
  const query = path.includes("?") ? path.slice(path.indexOf("?")) : "";
  const params = new URLSearchParams(query);
  const watchId = Number(params.get("channel") || "0");
  const virtualWatchId = Number(params.get("virtual") || "0");
  const watching = path.startsWith("/watch") && virtualWatchId === 0 ? guideChannels.find((item) => item.id === watchId) : undefined;
  const playId = Number(new URLSearchParams(window.location.search).get("recording") || "0");
  const playing = path.startsWith("/play") ? recordings.find((item) => item.id === playId) : undefined;

  useEffect(() => {
    const onPop = () => setPath(window.location.pathname + window.location.search);
    window.addEventListener("popstate", onPop);
    return () => window.removeEventListener("popstate", onPop);
  }, []);

  useEffect(() => {
    const id = window.setInterval(() => setNow(new Date()), 30_000);
    return () => window.clearInterval(id);
  }, []);

  useEffect(() => {
    const narrowQuery = window.matchMedia("(max-width: 760px)");
    const coarseQuery = window.matchMedia("(pointer: coarse) and (min-width: 1100px)");
    const apply = () => {
      setNarrow(narrowQuery.matches);
      setCoarse(coarseQuery.matches);
    };
    apply();
    narrowQuery.addEventListener("change", apply);
    coarseQuery.addEventListener("change", apply);
    return () => {
      narrowQuery.removeEventListener("change", apply);
      coarseQuery.removeEventListener("change", apply);
    };
  }, []);

  const reloadChannels = useCallback(async () => {
    const [guide, all] = await Promise.all([getChannels(true), getChannels(false)]);
    setGuideChannels(guide.channels);
    setAllChannels(all.channels);
  }, []);

  useEffect(() => {
    let cancel = false;
    (async () => {
      try {
        const [deviceRes, settingRes, storageRes] = await Promise.all([getDevices(), getSettings(), getStorage().catch(() => null)]);
        if (cancel) return;
        setSettings(settingRes);
        if (storageRes) setStorage(storageRes);
        let found = deviceRes.devices;
        if (found.length === 0) {
          const discovered = await discover();
          found = discovered.devices;
        }
        if (cancel) return;
        setDevices(found);
        await reloadChannels();
        try {
          await refreshGuide();
        } catch {
          // Listings can arrive a moment later from the server's own refresh.
        }
        const [listing, recorded, passList, virtualList] = await Promise.all([getAirings(), getRecordings(), getPasses(), getVirtuals()]);
        if (!cancel) {
          setAirings(listing.airings);
          setRecordings(recorded.recordings);
          setPasses(passList.passes);
          setVirtuals(virtualList.virtuals);
        }
      } catch (err) {
        if (!cancel) setError(err instanceof Error ? err.message : copy.status.failed);
      } finally {
        if (!cancel) setReady(true);
      }
    })();
    return () => {
      cancel = true;
    };
  }, [reloadChannels]);

  const queryLayout = new URLSearchParams(window.location.search).get("layout");
  const layout: "desktop" | "tv" | "phone" =
    queryLayout === "tv" || queryLayout === "phone" || queryLayout === "desktop"
      ? queryLayout
      : settings.layout !== "auto"
        ? settings.layout
        : narrow
          ? "phone"
          : coarse
            ? "tv"
            : "desktop";

  useEffect(() => {
    document.documentElement.dataset.layout = layout;
  }, [layout]);

  function go(next: string) {
    window.history.pushState({}, "", next);
    setPath(next);
  }

  async function runDiscover(ip?: string) {
    setBusy(true);
    setError("");
    try {
      const result = await discover(ip);
      setDevices(result.devices);
      await reloadChannels();
    } catch (err) {
      setError(err instanceof Error ? err.message : copy.status.failed);
    } finally {
      setBusy(false);
    }
  }

  async function favorite(channel: Channel): Promise<Channel> {
    const next = await patchChannel(channel.id, { favorite: !channel.favorite });
    await reloadChannels();
    return next;
  }

  async function edit(channel: Channel, patch: ChannelPatch) {
    await patchChannel(channel.id, patch);
    await reloadChannels();
  }

  async function saveSettings(values: Partial<Settings>) {
    const next = await putSettings(values);
    setSettings(next);
    const space = await getStorage().catch(() => null);
    if (space) setStorage(space);
  }

  return (
    <div className="shell">
      <nav className="rail" aria-label="Primary">
        <div className="brand">
          <span className="mark" aria-hidden="true">
            <svg viewBox="0 0 24 24">
              <path d="M12 3v10" />
              <path d="M7 8.5 12 13l5-4.5" />
              <path d="M4.5 14.5 12 21l7.5-6.5" />
            </svg>
          </span>
          <div>
            <strong>{copy.brand}</strong>
          </div>
        </div>
        {nav.map((item) => (
          <button
            key={item.id}
            type="button"
            className="nav-btn"
            aria-current={screen === item.id ? "page" : undefined}
            onClick={() => go(item.path)}
          >
            {item.label}
          </button>
        ))}
      </nav>
      <main className="main">
        {!ready ? <p className="status-line">{copy.status.looking}</p> : null}
        {ready && error && screen !== "sources" ? <p className="error page-error">{error}</p> : null}
        {ready && playing ? (
          <Play
            recording={playing}
            pictureMode={settings.pictureMode}
            autoplay={settings.autoplay !== "0"}
            onNext={() => {
              const later = recordings
                .filter((item) => item.title === playing.title && item.id !== playing.id && item.status !== "recording")
                .sort((a, b) => a.startedAt.localeCompare(b.startedAt));
              const next = later.find((item) => item.startedAt > playing.startedAt) ?? later[0];
              if (next) go(`/play?recording=${next.id}`);
            }}
            onBack={() => go("/library")}
          />
        ) : null}
        {ready && !playing && virtualWatchId > 0 ? <VirtualPlay id={virtualWatchId} pictureMode={settings.pictureMode} onBack={() => go("/guide")} /> : null}
        {ready && !playing && virtualWatchId === 0 && watching ? (
          <Watch
            channel={watching}
            channels={guideChannels}
            settings={settings}
            recordings={recordings}
            airings={airings}
            onChannel={(next) => go(`/watch?channel=${next.id}`)}
            onBack={() => go("/guide")}
            onRecorded={() => void getRecordings().then((res) => setRecordings(res.recordings))}
            onPicture={(pictureMode) => {
              setSettings((current) => ({ ...current, pictureMode }));
              void putSettings({ pictureMode });
            }}
          />
        ) : null}
        {ready && !playing && virtualWatchId === 0 && !watching && screen === "home" ? (
          <Home
            channels={guideChannels}
            airings={airings}
            recordings={recordings}
            storage={storage}
            now={now}
            onWatch={(channel) => go(`/watch?channel=${channel.id}`)}
            onResume={(recording) => go(`/play?recording=${recording.id}`)}
          />
        ) : null}
        {ready && !playing && virtualWatchId === 0 && !watching && screen === "guide" ? (
          <Guide
            channels={guideChannels}
            virtuals={virtuals}
            recordings={recordings}
            layout={layout}
            airings={airings}
            onFavorite={favorite}
            onWatch={(channel) => go(`/watch?channel=${channel.id}`)}
            onWatchVirtual={(virtual) => go(`/watch?virtual=${virtual.id}`)}
            onRecord={(channel, title) => {
              void startRecording(channel.id, 0, title)
                .then(() => getRecordings())
                .then((res) => {
                  setRecordings(res.recordings);
                  setError("");
                })
                .catch((err: unknown) => setError(err instanceof Error ? err.message : "Recording did not start."));
            }}
            onPass={(title, channel) => {
              if (!title) return;
              void addPass(title, channel.id).then(() => getPasses().then((res) => setPasses(res.passes)));
            }}
          />
        ) : null}
        {ready && !playing && virtualWatchId === 0 && !watching && screen === "library" ? (
          <Library
            recordings={recordings}
            note={libraryNote}
            onPlay={(recording) => go(`/play?recording=${recording.id}`)}
            onWatched={(recording, flag) => {
              void setWatched(recording.id, flag).then(() => getRecordings()).then((res) => setRecordings(res.recordings));
            }}
            onDelete={(recording) => {
              void deleteRecording(recording.id).then(() => getRecordings()).then((res) => {
                setRecordings(res.recordings);
                setLibraryNote(`${recording.title} was deleted. The file is gone from the recordings share.`);
              });
            }}
            onVirtual={(recording) => {
              const number = nextVirtualNumber(virtuals);
              void createVirtual(number, `${recording.title} channel`, [recording.id]).then(() => getVirtuals()).then((res) => {
                setVirtuals(res.virtuals);
                setLibraryNote(`Channel ${number} is on the guide. It plays this recording and does not use a tuner.`);
              });
            }}
          />
        ) : null}
        {ready && !playing && virtualWatchId === 0 && !watching && screen === "schedule" ? (
          <Schedule
            recordings={recordings}
            passes={passes}
            onStop={() => void getRecordings().then((res) => setRecordings(res.recordings))}
            onPasses={() => void getPasses().then((res) => setPasses(res.passes))}
          />
        ) : null}
        {ready && screen === "sources" ? (
          <Sources
            devices={devices}
            channels={allChannels}
            busy={busy}
            error={error}
            onDiscover={() => void runDiscover()}
            onLookup={(ip) => void runDiscover(ip)}
            onPatch={(channel, patch) => void edit(channel, patch)}
          />
        ) : null}
        {ready && screen === "settings" ? <SettingsScreen settings={settings} storage={storage} onChange={(values) => void saveSettings(values)} /> : null}
      </main>
    </div>
  );
}
