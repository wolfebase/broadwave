import Hls from "hls.js";
import { useEffect, useLayoutEffect, useRef, useState, type RefObject } from "react";
import { getDeviceHealth, getSignals, getTuners, stopWatch, watchChannel, type ApiFailure } from "../../api";
import { events } from "../../lib/events";
import { startOnRoom, SyncEngine, type SyncStatus } from "../../lib/sync";
import type { Caps, Channel, Prefs, WatchSession } from "../../types";
import { liveHlsConfig, type BufferProfile } from "../../picture";
import { rememberChannel } from "../../recent";
import { applySound } from "./extras";
import { followCaptions, watchTimeline } from "./liveCaptions";
import { awayBeforeSeekMs, resumePlan } from "./resume";
import {
  aTunerAnswers,
  aTunerIsFree,
  classifySnap,
  holdPictureMessage,
  pictureRetryDelay,
  pictureRetryEveryMs,
  pictureStopped,
  recoveryReady,
  startAttempts,
  startRetryMs,
  viewerFailure,
  type Recovery,
  type RecoverySnap,
} from "./outage";

const restartAskLastMs = 3000;

function webCaps(): Caps {
  const mse = typeof MediaSource !== "undefined" ? MediaSource : undefined;
  const audio = ["aac"];
  if (mse?.isTypeSupported('audio/mp4; codecs="ac-3"')) audio.push("ac3");
  if (mse?.isTypeSupported('audio/mp4; codecs="ec-3"')) audio.push("eac3");
  const conn = (navigator as Navigator & { connection?: { type?: string; saveData?: boolean } }).connection;
  return { platform: "web", video: ["h264"], audio, network: conn?.type === "cellular" || conn?.saveData ? "cellular" : "lan" };
}

export function useLiveStream(
  videoRef: RefObject<HTMLVideoElement | null>,
  {
    channelId,
    quality,
    audio,
    track,
    even,
    picture,
    room,
    sync,
    profile,
    audible,
    remember,
    after,
    captions,
  }: {
    channelId: number;
    quality?: Prefs["quality"];
    audio?: Prefs["audio"];
    track?: Prefs["track"];
    even?: boolean;
    picture?: Prefs["picture"];
    room: string | null;
    sync: boolean;
    profile: BufferProfile;
    audible: boolean;
    remember?: Channel | null;
    // Another player asks first. A watch that is already playing keeps going;
    // a start waits until this turns false.
    after?: boolean;
    captions?: boolean;
  },
) {
  const syncRef = useRef<SyncEngine | null>(null);
  const hlsRef = useRef<Hls | null>(null);
  const audibleRef = useRef(audible);
  const [session, setSession] = useState<WatchSession | null>(null);
  const [error, setError] = useState("");
  const [recovery, setRecovery] = useState<Recovery>("");
  const [pictureStopAt, setPictureStopAt] = useState(0);
  const pictureStopAtRef = useRef(0);
  const quietRetry = useRef<number | null>(null);
  // A quiet watch still starting or holding for its first picture.
  const quietPending = useRef(false);
  const [needsConfirm, setNeedsConfirm] = useState(false);
  const [attempt, setAttempt] = useState(0);
  // Starts asked again on their own for this channel. A picture, a channel
  // change, or the viewer's Try again starts the count over.
  const autoTries = useRef({ channel: 0, n: 0 });
  const confirmLive = useRef(false);
  const retrying = useRef(false);
  // This player holds a watch. After a restart, a player without one asks
  // last, so the pictures that were playing get their room on the server first.
  const watching = useRef(false);
  const syncing = useRef(sync);
  const roomRef = useRef(room);
  useEffect(() => {
    syncing.current = sync;
    roomRef.current = room;
  }, [sync, room]);
  // Layout effects run before the watch effect, so a start in the same render sees the gate.
  const afterRef = useRef(!!after);
  useLayoutEffect(() => {
    afterRef.current = !!after;
  }, [after]);
  const held = useRef(false);
  // A player with no watch when the server came back asks after the ones that
  // had a picture, so theirs is not the one a full picture budget turns away.
  const askLast = useRef(false);
  // True from a start until its watch answers or gives up.
  const [asking, setAsking] = useState(true);
  // A local rewind has to land before the engine's next tick puts the playhead back.
  const holdSync = useRef(false);
  const [syncStatus, setSyncStatus] = useState<SyncStatus>({ state: "off", drift: 0, members: 0 });

  useEffect(() => {
    // A channel change, a viewer retry, or leaving this watch ends the quiet
    // clock. An automatic retry names its channel before bumping attempt; the
    // name stays until that watch answers, so a rerun of this effect keeps it.
    const quiet = quietRetry.current != null && quietRetry.current === channelId;
    if (!quiet && pictureStopAtRef.current) {
      pictureStopAtRef.current = 0;
      setPictureStopAt(0);
    }
    const video = videoRef.current;
    if (!video || !channelId) return;
    let dead = false;
    let hls: Hls | null = null;
    const id = channelId;
    watching.current = false;
    if (afterRef.current) {
      held.current = true;
      retrying.current = false;
      return;
    }
    askLast.current = false;
    let joined = "";
    // The stop names the server process that counted this viewer. After a
    // restart the new process ignores it instead of taking someone else's.
    let boot = "";
    // The watch request can outlive this effect (Strict Mode runs it twice,
    // and leaving the page races the response). Both paths must release that
    // one viewer, and neither may release a viewer the request has not added.
    // Aborting is how a channel change drops a watch that has not answered:
    // the server releases that viewer. stopWatch runs only once this request
    // has a rendition.
    const ctrl = new AbortController();
    let released = false;
    const release = () => {
      if (released || !joined) return;
      released = true;
      void stopWatch(id, joined, boot);
    };
    if (remember) rememberChannel(remember);
    const started = performance.now();
    let primed = false;
    let stallAt = 0;
    let stuck = 0;
    delete video.dataset.ttff;
    delete video.dataset.moving;
    delete video.dataset.stalls;
    delete video.dataset.stallMs;
    const onPlaying = () => {
      if (!primed) {
        primed = true;
        video.dataset.ttff = String(Math.round(performance.now() - started));
        return;
      }
      window.clearTimeout(stuck);
      if (stallAt) {
        const soFar = Number(video.dataset.stallMs || 0);
        video.dataset.stallMs = String(Math.round(soFar + performance.now() - stallAt));
        stallAt = 0;
      }
    };
    const onWaiting = () => {
      if (!primed) return;
      stallAt = performance.now();
      video.dataset.stalls = String(Number(video.dataset.stalls || 0) + 1);
      window.clearTimeout(stuck);
      stuck = window.setTimeout(() => void noteOutage(false), stuckMs);
    };
    // moving is the first time the picture advances and stays in play.
    // The first playing event can be the frame sync then holds.
    let attached = false;
    let mark = Number.NaN;
    const onTime = () => {
      if (video.paused || !primed) return;
      // The message stays through a quiet retry until this watch's picture
      // actually advances. The previous picture's clock does not count.
      if (quiet && attached && pictureStopAtRef.current) {
        if (Number.isNaN(mark)) mark = video.currentTime;
        else if (video.currentTime > mark + 0.2) {
          pictureStopAtRef.current = 0;
          setPictureStopAt(0);
          setError("");
          setRecovery("");
        }
      }
      if (video.dataset.moving) return;
      if (video.currentTime > 0.2) video.dataset.moving = String(Math.round(performance.now() - started));
    };
    // hls.js rides through short stalls and single failed loads on its own, and
    // naming an outage stops the picture. A long stall is named only when the
    // server or every tuner is gone; a fatal error always is. Coming back from
    // a frozen tab is not an outage: the playlist is old, and the next one fixes it.
    let surfaced = false;
    let resumeQuiet = false;
    let outageGen = 0;
    let recovered = false;
    let heldFatal = false;
    let quietTimer = 0;
    let playlist = "";
    let startTimer = 0;
    const rememberOutage = (message: string, kind: Recovery) => {
      quietPending.current = false;
      setNeedsConfirm(false);
      setError(message);
      setRecovery(kind);
      if (message === pictureStopped && kind === "") {
        if (!pictureStopAtRef.current) {
          pictureStopAtRef.current = performance.now();
          setPictureStopAt(pictureStopAtRef.current);
        }
        return;
      }
      if (pictureStopAtRef.current) {
        pictureStopAtRef.current = 0;
        setPictureStopAt(0);
      }
    };
    const noteOutage = async (fatal: boolean) => {
      if (dead || surfaced || resumeQuiet) return;
      const gen = outageGen;
      const mapped = await classifyPlayback(id, playlist);
      if (dead || surfaced || resumeQuiet || gen !== outageGen) return;
      if (!fatal && !mapped.recovery) return;
      surfaced = true;
      rememberOutage(mapped.message, mapped.recovery);
      if (mapped.recovery) hls?.destroy();
    };
    video.addEventListener("playing", onPlaying);
    video.addEventListener("waiting", onWaiting);
    video.addEventListener("timeupdate", onTime);
    void (async () => {
      retrying.current = true;
      setAsking(true);
      try {
        const allow = confirmLive.current;
        confirmLive.current = false;
        setNeedsConfirm(false);
        const next = await watchChannel(id, webCaps(), { quality, audio, picture, track, even }, "", allow, ctrl.signal);
        joined = next.rendition;
        boot = next.boot ?? "";
        watching.current = true;
        autoTries.current = { channel: id, n: 0 };
        if (dead) {
          release();
          return;
        }
        quietRetry.current = null;
        // A quiet retry keeps the message up until the new picture moves, and
        // keeps the clock running in case it never does.
        if (quiet) {
          if (!pictureStopAtRef.current) {
            pictureStopAtRef.current = performance.now();
            setPictureStopAt(pictureStopAtRef.current);
          }
        } else {
          if (pictureStopAtRef.current) {
            pictureStopAtRef.current = 0;
            setPictureStopAt(0);
          }
          setError("");
          setRecovery("");
        }
        setSession(next);
        playlist = next.playlist;
        if (Hls.isSupported()) {
          const onRoom = syncing.current && !holdSync.current ? roomRef.current : null;
          hls = new Hls({ ...liveHlsConfig(profile), autoStartLoad: !onRoom });
          hlsRef.current = hls;
          (video as HTMLVideoElement & { hls?: Hls }).hls = hls;
          if (onRoom) startOnRoom(hls, video, onRoom);
          watchTimeline(hls, Hls.Events);
          hls.loadSource(next.playlist);
          hls.attachMedia(video);
          attached = true;
          mark = Number.NaN;
          hls.on(Hls.Events.ERROR, (_e, data) => {
            video.dataset.hlsError = `${data.type}:${data.details}${data.fatal ? ":fatal" : ""}`;
            if (!data.fatal) return;
            if (resumeQuiet) {
              // One media error from the stale buffer is expected; anything
              // else is named when the quiet window ends.
              if (data.type === Hls.ErrorTypes.MEDIA_ERROR && !recovered) {
                recovered = true;
                hls?.recoverMediaError();
              } else heldFatal = true;
              return;
            }
            void noteOutage(true);
          });
        } else {
          video.src = next.playlist;
          attached = true;
          mark = Number.NaN;
        }
        if (audibleRef.current) applySound(video);
        else video.muted = true;
        await video.play().catch(async () => {
          video.muted = true;
          await video.play().catch(() => undefined);
        });
      } catch (err) {
        if (dead) return;
        quietRetry.current = null;
        quietPending.current = false;
        const failed = err as ApiFailure;
        if (failed.status === 409 && failed.code === "recording_soon") {
          if (pictureStopAtRef.current) {
            pictureStopAtRef.current = 0;
            setPictureStopAt(0);
          }
          setNeedsConfirm(true);
          setRecovery("");
          setError(failed.message);
          return;
        }
        const tries = autoTries.current.channel === id ? autoTries.current.n + 1 : 1;
        if (tries < startAttempts(failed.code)) {
          autoTries.current = { channel: id, n: tries };
          startTimer = window.setTimeout(() => setAttempt((n) => n + 1), startRetryMs);
          return;
        }
        autoTries.current = { channel: id, n: 0 };
        const mapped = viewerFailure(err);
        if (quiet && holdPictureMessage(mapped)) {
          rememberOutage(pictureStopped, "");
          return;
        }
        rememberOutage(mapped.message, mapped.recovery);
      } finally {
        if (!dead) {
          retrying.current = false;
          if (!startTimer) setAsking(false);
        }
      }
    })();
    // pagehide also fires when the browser keeps the page for Back. That page
    // stops fetching, so the tuner has to go too. pageshow starts a new watch.
    const beacon = () => {
      if (released || !joined) return;
      const body = new Blob([JSON.stringify({ rendition: joined, boot })], { type: "application/json" });
      if (navigator.sendBeacon?.(`/api/v1/watch/${id}/stop`, body)) released = true;
    };
    const onShow = (event: PageTransitionEvent) => {
      if (!event.persisted || dead) return;
      if (retrying.current) return;
      retrying.current = true;
      quietRetry.current = id;
      setAttempt((n) => n + 1);
    };
    // A frozen or hidden tab does not move the playhead. The room does. On
    // return, seek to the edge once the playlist knows it. The sync engine
    // then trims onto the room.
    let leftAt = 0;
    let edgeAtLeave = 0;
    let poll = 0;
    let kicked = false;
    const stopPoll = () => {
      window.clearInterval(poll);
      poll = 0;
    };
    const comeBack = () => {
      if (dead || document.visibilityState === "hidden") return;
      const away = leftAt ? performance.now() - leftAt : 0;
      leftAt = 0;
      if (away < awayBeforeSeekMs) {
        if (video.paused && syncing.current) void video.play().catch(() => undefined);
        return;
      }
      if (!syncing.current && video.paused) return;
      resumeQuiet = true;
      recovered = false;
      heldFatal = false;
      window.clearTimeout(quietTimer);
      outageGen++;
      surfaced = false;
      window.clearTimeout(stuck);
      stallAt = 0;
      setNeedsConfirm(false);
      setError("");
      setRecovery("");
      if (pictureStopAtRef.current) {
        pictureStopAtRef.current = 0;
        setPictureStopAt(0);
      }
      const began = performance.now();
      kicked = false;
      stopPoll();
      const step = () => {
        if (dead) return;
        const rawDrift = video.dataset.syncDrift;
        const plan = resumePlan({
          current: video.currentTime,
          liveSync: hls?.liveSyncPosition ?? null,
          awayMs: away,
          held: !syncing.current && video.paused,
          waitedMs: performance.now() - began,
          edgeAtLeave,
          driftMs: rawDrift ? Number(rawDrift) : null,
        });
        if (plan.action === "wait") {
          if (!kicked && hls) {
            kicked = true;
            hls.startLoad();
          }
          return;
        }
        stopPoll();
        if (plan.action === "seek") video.currentTime = plan.to;
        if (plan.action !== "ignore") void video.play().catch(() => undefined);
        quietTimer = window.setTimeout(() => {
          resumeQuiet = false;
          if (heldFatal) void noteOutage(true);
        }, 8000);
      };
      poll = window.setInterval(step, 200);
      step();
    };
    const markLeft = () => {
      if (leftAt) return;
      leftAt = performance.now();
      edgeAtLeave = hls?.liveSyncPosition ?? video.currentTime;
    };
    const onVis = () => {
      if (document.visibilityState === "hidden") markLeft();
      else comeBack();
    };
    document.addEventListener("visibilitychange", onVis);
    document.addEventListener("freeze", markLeft);
    document.addEventListener("resume", comeBack);
    // A frozen page does not run timers, so the next tick sees the whole gap
    // even when the resume event never arrives.
    let lastTick = performance.now();
    let edgeAtTick = video.currentTime;
    const gapTimer = window.setInterval(() => {
      const now = performance.now();
      const jumped = now - lastTick;
      const edgeThen = edgeAtTick;
      lastTick = now;
      edgeAtTick = hls?.liveSyncPosition ?? video.currentTime;
      if (jumped < awayBeforeSeekMs || leftAt || poll) return;
      leftAt = now - jumped;
      edgeAtLeave = edgeThen;
      comeBack();
    }, 1000);
    window.addEventListener("pagehide", beacon);
    window.addEventListener("pageshow", onShow);
    return () => {
      dead = true;
      ctrl.abort();
      window.clearTimeout(stuck);
      stopPoll();
      window.clearTimeout(quietTimer);
      window.clearTimeout(startTimer);
      window.clearInterval(gapTimer);
      document.removeEventListener("visibilitychange", onVis);
      document.removeEventListener("freeze", markLeft);
      document.removeEventListener("resume", comeBack);
      window.removeEventListener("pagehide", beacon);
      window.removeEventListener("pageshow", onShow);
      syncRef.current?.stop();
      syncRef.current = null;
      hls?.destroy();
      hlsRef.current = null;
      video.removeEventListener("playing", onPlaying);
      video.removeEventListener("waiting", onWaiting);
      video.removeEventListener("timeupdate", onTime);
      release();
    };
    // remember is the channel record; its identity changes on every guide poll.
    // profile is only the buffer size. The mini player asks for a smaller one,
    // and rebuilding the watch to apply it freezes the picture. A watch that
    // is already playing keeps going; the next watch uses the profile it starts with.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [channelId, quality, audio, track, even, picture, attempt]);

  useEffect(() => {
    const video = videoRef.current;
    if (!sync) holdSync.current = false;
    if (!video || !session || !sync || holdSync.current || !room || session.channelId !== channelId) return;
    const engine = new SyncEngine(video, hlsRef.current, room, channelId, setSyncStatus);
    syncRef.current = engine;
    engine.start();
    return () => {
      engine.stop();
      if (syncRef.current === engine) syncRef.current = null;
    };
  }, [session, sync, room, channelId, videoRef]);

  useEffect(() => {
    if (after || !held.current) return;
    held.current = false;
    if (!askLast.current) {
      setAttempt((n) => n + 1);
      return;
    }
    const later = window.setTimeout(() => {
      if (!watching.current && !retrying.current) setAttempt((n) => n + 1);
    }, restartAskLastMs);
    return () => window.clearTimeout(later);
  }, [after]);

  // A restarted server has lost this watch. Start it again now instead of
  // waiting for the picture to run dry and the stall clock to name it.
  useEffect(() => {
    if (!channelId) return;
    let later = 0;
    const again = () => {
      if (retrying.current) return;
      retrying.current = true;
      quietRetry.current = channelId;
      setAttempt((n) => n + 1);
    };
    const off = events().on("restarted", () => {
      window.clearTimeout(later);
      if (watching.current) again();
      else {
        askLast.current = true;
        later = window.setTimeout(again, restartAskLastMs);
      }
    });
    return () => {
      off();
      window.clearTimeout(later);
    };
  }, [channelId]);

  useEffect(() => {
    if (!recovery) return;
    let dead = false;
    let ticking = false;
    const seen = { key: "" };
    const lastAsk = recovery === "server" || recovery === "restart";
    let readyAt = 0;
    const tick = async () => {
      if (dead || retrying.current || ticking) return;
      ticking = true;
      try {
        const snap = await readRecoverySnap(channelId, recovery === "signal");
        if (dead || retrying.current) return;
        const key = `${snap.health}:${snap.freeTuner}:${snap.tunerAnswers}:${snap.online}:${snap.signalLost}`;
        if (!recoveryReady(recovery, snap)) {
          seen.key = key;
          readyAt = 0;
          return;
        }
        if (key === seen.key) return;
        if (lastAsk && !watching.current) {
          askLast.current = true;
          readyAt ||= performance.now();
          if (performance.now() - readyAt < restartAskLastMs) return;
        }
        seen.key = key;
        retrying.current = true;
        if (recovery === "restart") quietRetry.current = channelId;
        setAttempt((n) => n + 1);
      } finally {
        ticking = false;
      }
    };
    const first = window.setTimeout(() => void tick(), 400);
    const id = window.setInterval(() => void tick(), 1000);
    return () => {
      dead = true;
      window.clearTimeout(first);
      window.clearInterval(id);
    };
  }, [recovery, channelId]);

  // The picture message is the outage nothing else can see change. Start a
  // watch on the clock, and stop when the viewer leaves or the channel changes.
  useEffect(() => {
    const started = pictureStopAtRef.current;
    if (!started || started !== pictureStopAt) return;
    let dead = false;
    let timer = 0;
    let fired = 0;
    let skipped = false;
    quietPending.current = false;
    const arm = () => {
      if (dead || pictureStopAtRef.current !== started) return;
      // A timer can fire a hair early; counting fires keeps it from firing twice.
      const elapsed = Math.max(performance.now() - started, fired * pictureRetryEveryMs);
      const wait = pictureRetryDelay(pictureStopped, "", elapsed);
      if (wait == null) return;
      timer = window.setTimeout(() => {
        if (dead || pictureStopAtRef.current !== started) return;
        fired += 1;
        // A fresh tune can hold its first picture longer than one turn. The
        // watch still starting gets one more turn instead of starting over.
        if (quietPending.current && !skipped) {
          skipped = true;
        } else if (!retrying.current) {
          skipped = false;
          quietPending.current = true;
          quietRetry.current = channelId;
          setAttempt((n) => n + 1);
        }
        arm();
      }, wait);
    };
    arm();
    return () => {
      dead = true;
      window.clearTimeout(timer);
    };
  }, [pictureStopAt, channelId]);

  // A new watch makes a new hls.js, so captions follow the session.
  const mainPlaylist = session?.channelId === channelId ? session.mainPlaylist : undefined;
  useEffect(() => {
    const video = videoRef.current;
    const hls = hlsRef.current;
    if (!captions || !mainPlaylist || !video || !hls) return;
    return followCaptions(hls, Hls.Events, video, new URL("captions.m3u8", new URL(mainPlaylist, window.location.href)).toString());
  }, [captions, mainPlaylist, session, videoRef]);

  useEffect(() => {
    audibleRef.current = audible;
    const video = videoRef.current;
    if (!video) return;
    if (audible) applySound(video);
    else video.muted = true;
    if (audible) void video.play().catch(() => undefined);
  }, [audible, videoRef]);

  return {
    session: session?.channelId === channelId ? session : null,
    // Captions need hls.js; a native player gets none yet.
    canCaption: !!mainPlaylist && Hls.isSupported(),
    error,
    needsConfirm,
    asking,
    confirm: () => {
      confirmLive.current = true;
      setNeedsConfirm(false);
      setAttempt((n) => n + 1);
    },
    retry: () => {
      quietRetry.current = null;
      autoTries.current = { channel: 0, n: 0 };
      setAttempt((n) => n + 1);
    },
    syncStatus,
    // Stop lining up with the room before a local seek. The flag stays until sync
    // is turned off, so a render in between does not start the engine again.
    releaseSync: () => {
      holdSync.current = true;
      syncRef.current?.stop();
      syncRef.current = null;
    },
    command: (action: "play" | "pause" | "seek" | "live", mediaTime?: number) => syncRef.current?.command(action, mediaTime),
    mediaNow: () => syncRef.current?.mediaNow() ?? null,
  };
}

const stuckMs = 8000;

async function classifyPlayback(channelId: number, playlist: string): Promise<{ message: string; recovery: Recovery }> {
  const snap = await readRecoverySnap(channelId, false);
  if (snap.health && playlist) {
    try {
      snap.watchGone = (await fetch(playlist, { cache: "no-store" })).status === 404;
    } catch {
      snap.watchGone = false;
    }
  }
  return classifySnap(snap);
}

async function readRecoverySnap(channelId: number, assumeLost: boolean): Promise<RecoverySnap> {
  const online = typeof navigator === "undefined" ? true : navigator.onLine;
  let health: boolean;
  try {
    health = (await fetch("/api/v1/health")).ok;
  } catch {
    health = false;
  }
  if (!health) return { health: false, freeTuner: false, tunerAnswers: false, online, signalLost: false };
  let freeTuner: boolean;
  let tunerAnswers: boolean;
  try {
    const body = await getTuners();
    freeTuner = aTunerIsFree(body.tuners ?? []);
  } catch {
    freeTuner = false;
  }
  try {
    const body = await getDeviceHealth();
    tunerAnswers = aTunerAnswers(body.devices ?? []);
  } catch {
    tunerAnswers = false;
  }
  let signalLost: boolean;
  try {
    const body = await getSignals();
    const row = (body.channels ?? []).find((item) => item.channelId === channelId);
    signalLost = !!row?.live && row.verdict === "Lost";
  } catch {
    signalLost = assumeLost;
  }
  return { health, freeTuner, tunerAnswers, online, signalLost };
}
