import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { actionError } from "./actionError";
import { shellMessages } from "../i18n/messages/shell";
import { bind } from "../i18n/runtime";
import {
  addPass,
  deletePass,
  discover,
  removeDevice,
  getAirings,
  getNow,
  getChannels,
  getDevices,
  getPasses,
  getRecordings,
  getSchedule,
  getClientMe,
  getPlayback,
  getServer,
  getSettings,
  getStorage,
  getVirtuals,
  patchChannel,
  putSettings,
  startRecording,
  stopRecording,
} from "../api";
import { isForbidden, isUnauthorized, onUnauthorized, settingsForWatcher, writeToken } from "../lib/deviceToken";
import { navigate } from "./router";
import { events } from "../lib/events";
import { beginSave, commitSave, followUpRead, LatestReads, type SaveSeq } from "../lib/latest";
import { guideSpan, indexAirings, keptGuideWindow, sortChannels, type AiringIndex } from "../lib/guide";
import { hasSnapshotFlag, loadSnapshot, saveSnapshot, type Snapshot } from "../lib/snapshot";
import { rememberServer, unreachableLine } from "./offline";
import { changedEvents, movedSince, readVersions, type DataVersions } from "../lib/dataVersions";
import type { Airing, Channel, ChannelPatch, Device, Pass, PlannedAiring, Recording, ServerInfo, Settings, StorageInfo, VirtualChannel } from "../types";

const shell = bind(shellMessages);

const defaults: Settings = {
  layout: "auto",
  profile: "transparent",
  audio: "stereo",
  watermarkGB: "10",
  bufferMinutes: "60",
  writeNfo: "0",
  folderLayout: "shows",
  gameAlerts: "all",
  pictureMode: "broadcast",
  adaptive: "0",
  autoplay: "1",
  hdhrEmulate: "0",
  deviceAuth: "0",
};

async function watcherSettings(): Promise<Settings> {
  const [playback, seat] = await Promise.all([getPlayback().catch(() => null), getClientMe().catch(() => null)]);
  return settingsForWatcher(defaults, playback, seat?.auth ?? "");
}

type Data = {
  ready: boolean;
  /** This visit's channel list has arrived. A saved copy can be missing a channel added since. */
  channelsReady: boolean;
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
  refresh: (what?: ("channels" | "recordings" | "passes" | "airings" | "virtuals" | "devices" | "settings")[]) => Promise<void>;
  favorite: (channel: Channel) => Promise<void>;
  editChannel: (channel: Channel, patch: ChannelPatch) => Promise<void>;
  saveSettings: (values: Partial<Settings>) => Promise<void>;
  /** True when the server asked this browser to pair. */
  needsPair: boolean;
  notePaired: () => void;
  record: (channel: Channel, title: string) => Promise<void>;
  stopRecord: (id: number) => Promise<void>;
  /** A series pass on this channel, or on any channel when channelId is 0. */
  recordSeries: (title: string, channelId: number) => Promise<void>;
  recordOnce: (airing: Airing, channel: Channel) => Promise<void>;
  removePass: (id: number) => Promise<void>;
  rediscover: (ip?: string) => Promise<void>;
  forgetDevice: (deviceId: string) => Promise<void>;
  setError: (message: string) => void;
  /** Devices that showed up after the house was already known. One line each. */
  notices: string[];
  dismissNotice: () => void;
  /** Set when a newer release is available. */
  update?: ServerInfo["update"];
  /** Latest GET /api/v1/server, once it has answered. */
  server: ServerInfo | null;
  /** This device may read the admin settings. */
  admin: boolean;
  /** When the lists on screen last came from the server or the saved copy (Unix ms). */
  freshAt: number;
};

const Ctx = createContext<Data | null>(null);

export function useData(): Data {
  const v = useContext(Ctx);
  if (!v) throw new Error("useData outside DataProvider");
  return v;
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
  const [channelsReady, setChannelsReady] = useState(false);
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
  const [needsPair, setNeedsPair] = useState(false);
  const [admin, setAdmin] = useState(false);
  useEffect(
    () =>
      onUnauthorized(() => {
        setNeedsPair(true);
        // An invite link is /pair?invite=CODE. Keep the code.
        if (window.location.pathname !== "/pair") navigate("/pair");
      }),
    [],
  );
  const [storage, setStorage] = useState<StorageInfo | null>(null);
  const [update, setUpdate] = useState<ServerInfo["update"]>();
  const [server, setServer] = useState<ServerInfo | null>(null);
  const loading = useRef(false);
  const [freshAt, setFreshAt] = useState(0);
  const loadFailed = useRef(false);
  // A slow read must not put a list back after a newer read has replaced it.
  const reads = useRef(new LatestReads());
  // One number per settings save. A later save makes an earlier response stale.
  const saveSeq = useRef<SaveSeq>({ n: 0 });

  const refresh = useCallback<Data["refresh"]>(async (what) => {
    const all = !what;
    const want = new Set(what ?? []);
    const keys: string[] = [];
    if (all || want.has("channels")) keys.push("channels");
    if (all || want.has("devices")) keys.push("devices");
    if (all || want.has("airings")) keys.push("airings");
    if (all || want.has("recordings")) keys.push("recordings");
    if (all || want.has("passes")) keys.push("passes", "planned");
    if (all || want.has("virtuals")) keys.push("virtuals");
    if (all || want.has("settings")) keys.push("settings");
    if (all) keys.push("server", "storage");
    const still = reads.current.start(keys);
    const jobs: Promise<unknown>[] = [];
    if (all || want.has("channels")) {
      jobs.push(
        Promise.all([getChannels(true), getChannels(false)]).then(([g, a]) => {
          if (!still("channels")) return;
          setChannels(sortChannels(g.channels, a.channels));
          setAllChannels(sortChannels(a.channels));
          setChannelsReady(true);
        }),
      );
    }
    if (all || want.has("devices")) {
      jobs.push(
        getDevices()
          .then((r) => still("devices") && setDevices(r.devices ?? []))
          .catch((err) => {
            if (!isForbidden(err) && !isUnauthorized(err)) throw err;
          }),
      );
    }
    if (all || want.has("airings")) jobs.push(getAirings({ ...keptGuideWindow(), fields: "grid" }).then((r) => still("airings") && setAirings(r.airings)));
    if (all || want.has("recordings")) jobs.push(getRecordings().then((r) => still("recordings") && setRecordings(r.recordings)));
    if (all || want.has("passes")) {
      jobs.push(getPasses().then((r) => still("passes") && setPasses(r.passes)));
      jobs.push(getSchedule().then((r) => still("planned") && setPlanned(r.items)).catch(() => undefined));
    }
    if (all || want.has("virtuals")) jobs.push(getVirtuals().then((r) => still("virtuals") && setVirtuals(r.virtuals)));
    if (all || want.has("settings")) {
      jobs.push(
        getSettings()
          .then((next) => {
            if (still("settings")) setSettings(next);
            setNeedsPair(false);
            setAdmin(true);
          })
          .catch(async (err) => {
            if (isUnauthorized(err) || isForbidden(err)) setAdmin(false);
            if (isUnauthorized(err)) setNeedsPair(true);
            else if (isForbidden(err)) setSettings(await watcherSettings());
            else throw err;
          }),
      );
    }
    if (all) {
      jobs.push(
        getServer()
          .then((info) => {
            if (!still("server")) return;
            setServer(info);
            setUpdate(info.update);
          })
          .catch(() => undefined),
      );
      jobs.push(getStorage().then((next) => still("storage") && setStorage(next)).catch(() => undefined));
    }
    await Promise.all(jobs);
  }, []);

  useEffect(() => {
    if (loading.current) return;
    loading.current = true;
    void (async () => {
      let snap: Snapshot | null = null;
      try {
        snap = await loadSnapshot();
        if (snap) {
          setChannels(sortChannels(snap.channels, snap.allChannels));
          setAllChannels(sortChannels(snap.allChannels));
          setAirings(snap.airings);
          setRecordings(snap.recordings);
          setFreshAt(snap.savedAt);
          setSettled(true);
          setBooting(false);
          setReady(true);
        }
        // Taken before the awaits, so a favorite or a reconnect during boot is not overwritten.
        const stillBoot = reads.current.start(["settings", "server", "channels", "airings", "recordings"]);
        let nextSettings: Settings;
        try {
          nextSettings = await getSettings();
          setNeedsPair(false);
          setAdmin(true);
        } catch (err) {
          setAdmin(false);
          if (isUnauthorized(err)) {
            setNeedsPair(true);
            setError("");
            setReady(true);
            setSettled(true);
            setBooting(false);
            return;
          }
          if (!isForbidden(err)) throw err;
          nextSettings = await watcherSettings();
        }
        const info = await getServer().catch(() => null);
        if (stillBoot("settings")) setSettings(nextSettings);
        if (info && stillBoot("server")) {
          rememberServer(info.name);
          setServer(info);
          setUpdate(info.update);
        }
        if (nextSettings.needsSetup === "1") {
          setReady(true);
          setChannelsReady(true);
          setSettled(true);
          setBooting(false);
          return;
        }
        const span = guideSpan();
        // A first visit has no saved copy. The channels and what is on now are
        // small answers, so they paint like a saved copy while the guide window
        // is still on its way.
        const listsP = Promise.all([getChannels(true), getChannels(false)]);
        const airingsAtBoot = reads.current.generation("airings");
        if (!snap) {
          void Promise.all([listsP, getNow().catch(() => null)])
            .then(([[g, a], nowNext]) => {
              const list = nowNext?.channels.flatMap((c) => [c.now, c.next].filter((x): x is Airing => Boolean(x))) ?? [];
              if (!list.length || reads.current.generation("airings") !== airingsAtBoot || !stillBoot("channels")) return;
              setChannels(sortChannels(g.channels, a.channels));
              setAllChannels(sortChannels(a.channels));
              setAirings((cur) => (cur.length ? cur : list));
              setSettled(true);
              setBooting(false);
              setReady(true);
            })
            .catch(() => undefined);
        }
        const [[guideChannels, everyChannel], windowed, recs] = await Promise.all([
          listsP,
          getAirings({ from: span.from, to: span.to, fields: "grid" }),
          getRecordings(),
        ]);
        const channelsNow = sortChannels(guideChannels.channels, everyChannel.channels);
        const allNow = sortChannels(everyChannel.channels);
        const wroteChannels = stillBoot("channels");
        const wroteAirings = stillBoot("airings");
        const wroteRecordings = stillBoot("recordings");
        if (wroteChannels) {
          setChannels(channelsNow);
          setAllChannels(allNow);
          setChannelsReady(true);
        }
        if (wroteAirings) setAirings(windowed.airings);
        if (wroteRecordings) setRecordings(recs.recordings);
        if (wroteChannels || wroteAirings || wroteRecordings) setFreshAt(Date.now());
        setSettled(true);
        setReady(true);
        setBooting(false);
        if (wroteChannels && wroteAirings && wroteRecordings) {
          void saveSnapshot({
            channels: channelsNow,
            allChannels: allNow,
            airings: windowed.airings,
            recordings: recs.recordings,
            savedAt: Date.now(),
          });
        }
        const idle = window.requestIdleCallback ?? ((cb: IdleRequestCallback) => window.setTimeout(() => cb({ didTimeout: false, timeRemaining: () => 0 } as IdleDeadline), 400));
        idle(() => {
          void (async () => {
            const airingsGen = reads.current.generation("airings");
            const rest = await getAirings({ from: span.to, to: span.restTo, fields: "grid" }).catch(() => null);
            if (rest && reads.current.generation("airings") === airingsGen) {
              setAirings((current) => {
                const merged = mergeAirings(current, rest.airings);
                if (wroteChannels && wroteRecordings) {
                  void saveSnapshot({
                    channels: channelsNow,
                    allChannels: allNow,
                    airings: merged,
                    recordings: recs.recordings,
                    savedAt: Date.now(),
                  });
                }
                return merged;
              });
            }
            await refresh(["devices", "passes", "virtuals"]);
            const devicesGen = reads.current.generation("devices");
            const storageGen = reads.current.generation("storage");
            void getStorage()
              .then((next) => {
                if (reads.current.generation("storage") === storageGen) setStorage(next);
              })
              .catch(() => undefined);
            const found = await getDevices().catch(() => null);
            if (found && reads.current.generation("devices") === devicesGen && found.devices.length === 0) {
              const again = await discover().catch(() => null);
              if (again && reads.current.generation("devices") === devicesGen) setDevices(again.devices ?? []);
              await refresh(["channels"]);
            }
          })();
        });
      } catch (err) {
        if (isUnauthorized(err)) {
          setNeedsPair(true);
          setError("");
          setReady(true);
          setSettled(true);
          setBooting(false);
          return;
        }
        setError(unreachableLine(err, Boolean(snap)) ?? (err instanceof Error ? err.message : shell("shell.unreachable")));
        // The saved guide's progress bars are a guess until the server answers.
        if (snap) document.documentElement.dataset.stale = "1";
        loadFailed.current = true;
        setReady(true);
        setChannelsReady(true);
        setSettled(true);
        setBooting(false);
      }
    })();
  }, [refresh]);

  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 15_000);
    return () => window.clearInterval(id);
  }, []);

  // The server's data counters. A server that sends them also sends a change
  // event per list, so only that list is read again.
  const versions = useRef<DataVersions | null>(null);
  useEffect(() => {
    const bus = events();
    const offActivity = bus.on("activity", (data) => {
      const kind = (data as { kind?: string }).kind;
      if (!versions.current && (kind === "recording" || kind === "delete")) void refresh(["recordings", "passes"]);
      if (!versions.current && kind === "guide") void refresh(["airings"]);
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
    const offChanged = Object.entries(changedEvents).map(([type, key]) => bus.on(type, () => void refresh([key])));
    let helloWait = 0;
    const settledBack = () => {
      setFreshAt(Date.now());
      if (loadFailed.current) {
        loadFailed.current = false;
        delete document.documentElement.dataset.stale;
        setError("");
      }
    };
    const offHello = bus.on("hello", (data) => {
      const next = readVersions(data);
      const prev = versions.current;
      versions.current = next;
      if (!next || !prev || !helloWait) return;
      window.clearTimeout(helloWait);
      helloWait = 0;
      // Devices carry no counter, and a tuner may have come or gone.
      void refresh([...movedSince(prev, next), "devices"])
        .then(settledBack)
        .catch(() => undefined);
    });
    const offFound = bus.on("sources.found", () => void refresh(["devices", "channels"]));
    // Events sent while this tab was away are gone. A server with data
    // counters says in its hello what moved; any other is read again in full.
    const back = () => {
      void refresh()
        .then(settledBack)
        .catch(() => undefined);
    };
    const offBack = bus.on("reconnected", () => {
      if (!versions.current || loadFailed.current) return back();
      window.clearTimeout(helloWait);
      helloWait = window.setTimeout(() => {
        helloWait = 0;
        back();
      }, 3000);
    });
    // A page opened while the server was down has not loaded anything yet.
    const offFirst = bus.on("connection", (up) => {
      if (up && loadFailed.current) back();
    });
    return () => {
      offBack();
      offFirst();
      offActivity();
      offLive();
      offFound();
      offHello();
      offChanged.forEach((off) => off());
      window.clearTimeout(helloWait);
    };
  }, [refresh]);

  const value = useMemo<Data>(
    () => ({
      ready,
      channelsReady,
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
      needsPair,
      notePaired: () => {
        setNeedsPair(false);
        void refresh();
      },
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
        const mine = beginSave(saveSeq.current);
        // Drops a GET that already started. commitSave drops one that starts during the PUT.
        reads.current.start(["settings", "storage", "server"]);
        const put = await putSettings(values);
        // The token is handed out once, so it is kept even when a newer save wins.
        if (put.deviceToken) writeToken(put.deviceToken);
        const stored = { ...put };
        delete stored.deviceToken;
        setNeedsPair(false);
        const committed = commitSave(saveSeq.current, mine, stored, reads.current, ["settings", "storage", "server"]);
        if (!committed) return;
        setSettings(committed.saved);
        // still() is after the await. A save that starts during the read must win.
        const disk = await getStorage().catch(() => null);
        const nextDisk = followUpRead(saveSeq.current, mine, committed.still("storage"), disk);
        if (nextDisk) setStorage(nextDisk);
        if ("checkUpdates" in values) {
          const info = await getServer().catch(() => null);
          const nextServer = followUpRead(saveSeq.current, mine, committed.still("server"), info);
          if (nextServer) {
            setServer(nextServer);
            setUpdate(nextServer.update);
          }
        }
      },
      record: async (channel, title) => {
        try {
          await startRecording(channel.id, 0, title);
          setError("");
        } catch (err) {
          setError(actionError(err, "shell.recordFailed"));
        }
        await refresh(["recordings"]);
      },
      stopRecord: async (id) => {
        await stopRecording(id);
        await refresh(["recordings"]);
      },
      recordSeries: async (title, channelId) => {
        await addPass(title, channelId);
        await refresh(["passes"]);
      },
      recordOnce: async (airing, channel) => {
        try {
          await addPass(airing.title, channel.id, airing.start);
          setError("");
        } catch (err) {
          setError(actionError(err, "shell.airingNotScheduled"));
        }
        await refresh(["passes"]);
      },
      removePass: async (id) => {
        try {
          await deletePass(id);
          setError("");
        } catch (err) {
          setError(actionError(err, "shell.passNotRemoved"));
        }
        await refresh(["passes"]);
      },
      notices,
      dismissNotice,
      update,
      server,
      admin,
      freshAt,
      rediscover: async (ip) => {
        reads.current.start(["devices"]);
        try {
          const res = await discover(ip);
          setDevices(res.devices ?? []);
          setError("");
        } catch (err) {
          setError(actionError(err, "shell.noTuner"));
        }
        await refresh(["channels"]);
      },
      forgetDevice: async (deviceId) => {
        reads.current.start(["devices"]);
        try {
          const res = await removeDevice(deviceId);
          setDevices(res.devices ?? []);
          setError("");
        } catch (err) {
          setError(actionError(err, "shell.deviceNotRemoved"));
        }
        await refresh(["channels", "passes"]);
      },
    }),
    [ready, channelsReady, settled, booting, error, now, channels, allChannels, devices, airings, recordings, passes, planned, virtuals, settings, needsPair, storage, refresh, notices, dismissNotice, update, server, admin, freshAt],
  );

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}
