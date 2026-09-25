import Hls from "hls.js";
import { useEffect, useRef, useState, type RefObject } from "react";
import { stopWatch, watchChannel, type ApiFailure } from "../../api";
import { SyncEngine, type SyncStatus } from "../../lib/sync";
import type { Caps, Channel, Prefs, WatchSession } from "../../types";
import { liveHlsConfig, type BufferProfile } from "../../picture";
import { rememberChannel } from "../../recent";

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
  },
) {
  const syncRef = useRef<SyncEngine | null>(null);
  const hlsRef = useRef<Hls | null>(null);
  const audibleRef = useRef(audible);
  const [session, setSession] = useState<WatchSession | null>(null);
  const [error, setError] = useState("");
  const [needsConfirm, setNeedsConfirm] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const confirmLive = useRef(false);
  const [syncStatus, setSyncStatus] = useState<SyncStatus>({ state: "off", drift: 0, members: 0 });

  useEffect(() => {
    const video = videoRef.current;
    if (!video || !channelId) return;
    let dead = false;
    let hls: Hls | null = null;
    let joined = "";
    const id = channelId;
    if (remember) rememberChannel(remember);
    const started = performance.now();
    let primed = false;
    let stallAt = 0;
    const onPlaying = () => {
      if (!primed) {
        primed = true;
        video.dataset.ttff = String(Math.round(performance.now() - started));
        return;
      }
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
    };
    video.addEventListener("playing", onPlaying);
    video.addEventListener("waiting", onWaiting);
    void (async () => {
      try {
        const allow = confirmLive.current;
        confirmLive.current = false;
        setNeedsConfirm(false);
        const next = await watchChannel(id, webCaps(), { quality, audio, picture, track, even }, "", allow);
        joined = next.rendition;
        if (dead) {
          await stopWatch(id, joined);
          return;
        }
        setError("");
        setSession(next);
        if (Hls.isSupported()) {
          hls = new Hls(liveHlsConfig(profile));
          hlsRef.current = hls;
          (video as HTMLVideoElement & { hls?: Hls }).hls = hls;
          hls.loadSource(next.playlist);
          hls.attachMedia(video);
          hls.on(Hls.Events.ERROR, (_e, data) => {
            video.dataset.hlsError = `${data.type}:${data.details}${data.fatal ? ":fatal" : ""}`;
            if (data.fatal) setError("The picture stopped. Trying again usually fixes it.");
          });
        } else {
          video.src = next.playlist;
        }
        video.muted = !audibleRef.current;
        await video.play().catch(async () => {
          video.muted = true;
          await video.play().catch(() => undefined);
        });
      } catch (err) {
        if (dead) return;
        const failed = err as ApiFailure;
        if (failed.status === 409 && failed.code === "recording_soon") {
          setNeedsConfirm(true);
          setError(failed.message);
          return;
        }
        setNeedsConfirm(false);
        setError(err instanceof Error ? err.message : "This channel did not start.");
      }
    })();
    const beacon = () => navigator.sendBeacon?.(`/api/v1/watch/${id}/stop`, new Blob([JSON.stringify({ rendition: joined })], { type: "application/json" }));
    window.addEventListener("pagehide", beacon);
    return () => {
      dead = true;
      window.removeEventListener("pagehide", beacon);
      syncRef.current?.stop();
      syncRef.current = null;
      hls?.destroy();
      hlsRef.current = null;
      video.removeEventListener("playing", onPlaying);
      video.removeEventListener("waiting", onWaiting);
      void stopWatch(id, joined);
    };
    // remember is the channel record; its identity changes on every guide poll.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [channelId, quality, audio, track, even, picture, profile, attempt]);

  useEffect(() => {
    const video = videoRef.current;
    if (!video || !session || !sync || !room || session.channelId !== channelId) return;
    const engine = new SyncEngine(video, hlsRef.current, room, channelId, setSyncStatus);
    syncRef.current = engine;
    engine.start();
    return () => {
      engine.stop();
      if (syncRef.current === engine) syncRef.current = null;
    };
  }, [session, sync, room, channelId, videoRef]);

  useEffect(() => {
    audibleRef.current = audible;
    const video = videoRef.current;
    if (!video) return;
    video.muted = !audible;
    if (audible) void video.play().catch(() => undefined);
  }, [audible, videoRef]);

  return {
    session: session?.channelId === channelId ? session : null,
    error,
    needsConfirm,
    confirm: () => {
      confirmLive.current = true;
      setNeedsConfirm(false);
      setAttempt((n) => n + 1);
    },
    syncStatus,
    command: (action: "play" | "pause" | "seek" | "live", mediaTime?: number) => syncRef.current?.command(action, mediaTime),
    mediaNow: () => syncRef.current?.mediaNow() ?? null,
  };
}
