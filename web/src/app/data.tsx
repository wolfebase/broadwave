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
import type { Airing, Channel, ChannelPatch, Device, Pass, PlannedAiring, Recording, Settings, StorageInfo, VirtualChannel } from "../types";

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
};

const Ctx = createContext<Data | null>(null);

export function useData(): Data {
  const v = useContext(Ctx);
  if (!v) throw new Error("useData outside DataProvider");
  return v;
}

export function DataProvider({ children }: { children: ReactNode }) {
  const [ready, setReady] = useState(false);
  const [error, setError] = useState("");
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
      jobs.push(getStorage().then(setStorage).catch(() => undefined));
    }
    await Promise.all(jobs);
  }, []);

  useEffect(() => {
    if (loading.current) return;
    loading.current = true;
    void (async () => {
      try {
        const found = await getDevices();
        if (found.devices.length === 0) await discover().catch(() => undefined);
        await refresh();
      } catch (err) {
        setError(err instanceof Error ? err.message : "The server could not be reached.");
      } finally {
        setReady(true);
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
    });
    const offLive = bus.on("live.changed", () => void refresh(["recordings"]));
    return () => {
      offActivity();
      offLive();
    };
  }, [refresh]);

  const value = useMemo<Data>(
    () => ({
      ready,
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
    [ready, error, now, channels, allChannels, devices, airings, recordings, passes, planned, virtuals, settings, storage, refresh],
  );

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}
