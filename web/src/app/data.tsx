import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import {
  addPass,
  discover,
  getAirings,
  getChannels,
  getDevices,
  getPasses,
  getRecordings,
  getSchedule,
  getServer,
  getSettings,
  getStorage,
  getVirtuals,
  patchChannel,
  putSettings,
  startRecording,
  stopRecording,
} from "../api";
import { events } from "../lib/events";
import { indexAirings, sortChannels, type AiringIndex } from "../lib/guide";
import { hasSnapshotFlag, loadSnapshot, saveSnapshot } from "../lib/snapshot";
import type { Airing, Channel, ChannelPatch, Device, Pass, PlannedAiring, Recording, ServerInfo, Settings, StorageInfo, VirtualChannel } from "../types";

const defaults: Settings = {
  layout: "auto",
  recordingsPath: "",
  profile: "transparent",
  audio: "stereo",
  watermarkGB: "10",
  pictureMode: "broadcast",
  autoplay: "1",
  hdhrEmulate: "0",
};

type Data = {
  ready: boolean;
  /** True once a cached snapshot or the first network window is on screen. */
  settled: boolean;
  /** Full-screen boot. Only the very first visit, before any snapshot exists. */
  booting: boolean;
  error: string;
  now: number;
  channels: Channel[];
  allChannels: Channel[];
  devices: Device[];
  airings: Airing[];
  index: AiringIndex;
  recordings: Recording[];
  passes: Pass[];
  planned: PlannedAiring[];
  virtuals: VirtualChannel[];
  settings: Settings;
  storage: StorageInfo | null;
  refresh: (what?: ("channels" | "recordings" | "passes" | "airings" | "virtuals" | "devices")[]) => Promise<void>;
  favorite: (channel: Channel) => Promise<void>;
  editChannel: (channel: Channel, patch: ChannelPatch) => Promise<void>;
  saveSettings: (values: Partial<Settings>) => Promise<void>;
  record: (channel: Channel, title: string) => Promise<void>;
  stopRecord: (id: number) => Promise<void>;
  recordSeries: (title: string, channel: Channel) => Promise<void>;
  rediscover: (ip?: string) => Promise<void>;
  setError: (message: string) => void;
  /** Devices that showed up after the house was already known. One line each. */
  notices: string[];
  dismissNotice: () => void;
  /** Set when a newer release is available. */
  update?: ServerInfo["update"];
};

const Ctx = createContext<Data | null>(null);

export function useData(): Data {
  const v = useContext(Ctx);
  if (!v) throw new Error("useData outside DataProvider");
  return v;
}

function guideWindow(now = Date.now()) {
  return {
    from: new Date(now - 30 * 60_000).toISOString(),
    to: new Date(now + 4 * 60 * 60_000).toISOString(),
    restTo: new Date(now + 14 * 24 * 60 * 60_000).toISOString(),
  };
}

function mergeAirings(current: Airing[], more: Airing[]): Airing[] {
  const seen = new Set(current.map((airing) => airing.id));
  const out = current.slice();
  for (const airing of more) {
    if (!seen.has(airing.id)) out.push(airing);
  }
  return out;
}

export function DataProvider({ children }: { children: ReactNode }) {
  const [ready, setReady] = useState(false);
  const [settled, setSettled] = useState(false);
  const [booting, setBooting] = useState(() => !hasSnapshotFlag());
  const [error, setError] = useState("");
  const [notices, setNotices] = useState<string[]>([]);
  const dismissNotice = useCallback(() => {
    setNotices((cur) => cur.slice(1));
  }, []);
  const [now, setNow] = useState(() => Date.now());
  const [channels, setChannels] = useState<Channel[]>([]);
  const [allChannels, setAllChannels] = useState<Channel[]>([]);
  const [devices, setDevices] = useState<Device[]>([]);
  const [airings, setAirings] = useState<Airing[]>([]);
  const [recordings, setRecordings] = useState<Recording[]>([]);
  const [passes, setPasses] = useState<Pass[]>([]);
  const [planned, setPlanned] = useState<PlannedAiring[]>([]);
  const [virtuals, setVirtuals] = useState<VirtualChannel[]>([]);
  const [settings, setSettings] = useState<Settings>(defaults);
  const [storage, setStorage] = useState<StorageInfo | null>(null);
  const [update, setUpdate] = useState<ServerInfo["update"]>();
  const loading = useRef(false);

  const refresh = useCallback<Data["refresh"]>(async (what) => {
    const all = !what;
    const want = new Set(what ?? []);
    const jobs: Promise<unknown>[] = [];
    if (all || want.has("channels")) {
      jobs.push(
        Promise.all([getChannels(true), getChannels(false)]).then(([g, a]) => {
          setChannels(sortChannels(g.channels));
          setAllChannels(sortChannels(a.channels));
        }),
      );
    }
    if (all || want.has("devices")) jobs.push(getDevices().then((r) => setDevices(r.devices)));
    if (all || want.has("airings")) jobs.push(getAirings().then((r) => setAirings(r.airings)));
    if (all || want.has("recordings")) jobs.push(getRecordings().then((r) => setRecordings(r.recordings)));
    if (all || want.has("passes")) {
      jobs.push(getPasses().then((r) => setPasses(r.passes)));
      jobs.push(getSchedule().then((r) => setPlanned(r.items)).catch(() => undefined));
    }
    if (all || want.has("virtuals")) jobs.push(getVirtuals().then((r) => setVirtuals(r.virtuals)));
    if (all) {
      jobs.push(getSettings().then(setSettings));
      jobs.push(getServer().then((info) => setUpdate(info.update)).catch(() => undefined));
      jobs.push(getStorage().then(setStorage).catch(() => undefined));
    }
    await Promise.all(jobs);
  }, []);

  useEffect(() => {
    if (loading.current) return;
    loading.current = true;
    void (async () => {
      try {
        const snap = await loadSnapshot();
        if (snap) {
          setChannels(sortChannels(snap.channels));
          setAllChannels(sortChannels(snap.allChannels));
          setAirings(snap.airings);
          setRecordings(snap.recordings);
          setSettled(true);
          setBooting(false);
          setReady(true);
        }
        const [nextSettings, info] = await Promise.all([getSettings(), getServer().catch(() => null)]);
        setSettings(nextSettings);
        if (info) setUpdate(info.update);
        if (nextSettings.needsSetup === "1") {
          setReady(true);
          setSettled(true);
          setBooting(false);
          return;
        }
        const span = guideWindow();
        const [guideChannels, everyChannel, windowed, recs] = await Promise.all([
          getChannels(true),
          getChannels(false),
          getAirings({ from: span.from, to: span.to }),
          getRecordings(),
        ]);
        const channelsNow = sortChannels(guideChannels.channels);
        const allNow = sortChannels(everyChannel.channels);
        setChannels(channelsNow);
        setAllChannels(allNow);
        setAirings(windowed.airings);
        setRecordings(recs.recordings);
        setSettled(true);
        setReady(true);
        setBooting(false);
        void saveSnapshot({
          channels: channelsNow,
          allChannels: allNow,
          airings: windowed.airings,
          recordings: recs.recordings,
          savedAt: Date.now(),
        });
        const idle = window.requestIdleCallback ?? ((cb: IdleRequestCallback) => window.setTimeout(() => cb({ didTimeout: false, timeRemaining: () => 0 } as IdleDeadline), 400));
        idle(() => {
          void (async () => {
            const rest = await getAirings({ from: span.to, to: span.restTo }).catch(() => null);
            if (rest) {
              setAirings((current) => {
                const merged = mergeAirings(current, rest.airings);
                void saveSnapshot({
                  channels: channelsNow,
                  allChannels: allNow,
                  airings: merged,
                  recordings: recs.recordings,
                  savedAt: Date.now(),
                });
                return merged;
              });
            }
            await refresh(["devices", "passes", "virtuals"]);
            void getStorage().then(setStorage).catch(() => undefined);
            const found = await getDevices().catch(() => null);
            if (found && found.devices.length === 0) {
              const again = await discover().catch(() => null);
              if (again) setDevices(again.devices);
              await refresh(["channels"]);
            }
          })();
        });
      } catch (err) {
        setError(err instanceof Error ? err.message : "The server could not be reached.");
        setReady(true);
        setSettled(true);
        setBooting(false);
      }
    })();
  }, [refresh]);

  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 15_000);
    return () => window.clearInterval(id);
  }, []);

  useEffect(() => {
    const bus = events();
    const offActivity = bus.on("activity", (data) => {
      const kind = (data as { kind?: string }).kind;
      if (kind === "recording") void refresh(["recordings", "passes"]);
      if (kind === "guide") void refresh(["airings"]);
      if (kind === "source") {
        const message = (data as { message?: string }).message;
        if (message) setError(message);
      }
      if (kind === "home") {
        const message = (data as { message?: string }).message?.trim();
        if (message) setNotices((cur) => [...cur, message]);
      }
    });
    const offLive = bus.on("live.changed", () => void refresh(["recordings"]));
    const offFound = bus.on("sources.found", () => void refresh(["devices", "channels"]));
    return () => {
      offActivity();
      offLive();
      offFound();
    };
  }, [refresh]);

  const value = useMemo<Data>(
    () => ({
      ready,
      settled,
      booting,
      error,
      now,
      channels,
      allChannels,
      devices,
      airings,
      index: indexAirings(airings),
      recordings,
      passes,
      planned,
      virtuals,
      settings,
      storage,
      refresh,
      setError,
      favorite: async (channel) => {
        await patchChannel(channel.id, { favorite: !channel.favorite });
        await refresh(["channels"]);
      },
      editChannel: async (channel, patch) => {
        await patchChannel(channel.id, patch);
        await refresh(["channels"]);
      },
      saveSettings: async (values) => {
        setSettings(await putSettings(values));
        setStorage(await getStorage().catch(() => null));
        if ("checkUpdates" in values) {
          const info = await getServer().catch(() => null);
          if (info) setUpdate(info.update);
        }
      },
      record: async (channel, title) => {
        try {
          await startRecording(channel.id, 0, title);
          setError("");
        } catch (err) {
          setError(err instanceof Error ? err.message : "Recording did not start.");
        }
        await refresh(["recordings"]);
      },
      stopRecord: async (id) => {
        await stopRecording(id);
        await refresh(["recordings"]);
      },
      recordSeries: async (title, channel) => {
        await addPass(title, channel.id);
        await refresh(["passes"]);
      },
      notices,
      dismissNotice,
      update,
      rediscover: async (ip) => {
        try {
          const res = await discover(ip);
          setDevices(res.devices);
          setError("");
        } catch (err) {
          setError(err instanceof Error ? err.message : "No tuner answered.");
        }
        await refresh(["channels"]);
      },
    }),
    [ready, settled, booting, error, now, channels, allChannels, devices, airings, recordings, passes, planned, virtuals, settings, storage, refresh, notices, dismissNotice, update],
  );

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}
