import Hls from "hls.js";
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { playVirtual } from "../../api";
import { recordingsMessages } from "../../i18n/messages/recordings.ts";
import { bind } from "../../i18n/runtime.ts";
import { mediaURL } from "../../lib/deviceToken";
import { fileHlsConfig, type PictureMode } from "../../picture";
import { markersForPlayback, releaseFileVideo, seekableSkip, takeFileFatal } from "./filePlay";
import { Stage } from "../player/Stage";

const t = bind(recordingsMessages);

type Marker = { id: number; recordingId?: number; start: number; end: number };

export function VirtualPlay({ id, pictureMode, onBack }: { id: number; pictureMode: PictureMode; onBack: () => void }) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const [index, setIndex] = useState(0);
  const [error, setError] = useState("");
  const [title, setTitle] = useState("");
  const [channel, setChannel] = useState("");
  const [count, setCount] = useState(0);
  const [markers, setMarkers] = useState<Marker[]>([]);
  const [fileId, setFileId] = useState<number | null>(null);
  const [seenIndex, setSeenIndex] = useState(index);
  const [seenChannel, setSeenChannel] = useState(id);
  let activeId = fileId;
  if (seenChannel !== id || seenIndex !== index) {
    setSeenChannel(id);
    setSeenIndex(index);
    setFileId(null);
    activeId = null;
  }
  const shown = useMemo(() => markersForPlayback(activeId, markers), [activeId, markers]);
  const [skip, setSkip] = useState(true);
  const [where, setWhere] = useState(0);
  const [length, setLength] = useState(0);
  const shownRef = useRef(shown);
  useLayoutEffect(() => {
    shownRef.current = shown;
  }, [shown]);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    let dead = false;
    let hls: Hls | null = null;
    const tried = { network: false, media: false };
    let gaveUp = false;
    setError("");
    void (async () => {
      try {
        const next = await playVirtual(id, index, pictureMode);
        if (dead) return;
        if (next.usesTuner) {
          setError(t("recordings.tunerUsed"));
          return;
        }
        setTitle(next.recording.title);
        setChannel(`${next.number} ${next.name}`);
        setCount(next.count);
        setFileId(next.recording.id);
        setMarkers(next.markers);
        if (Hls.isSupported()) {
          hls = new Hls(fileHlsConfig(-1));
          hls.loadSource(next.playlist);
          hls.attachMedia(video);
          hls.on(Hls.Events.ERROR, (_event, data) => {
            if (!data.fatal || dead || gaveUp) return;
            const action = takeFileFatal(data.type, tried);
            if (action === "startLoad") {
              hls?.startLoad();
              return;
            }
            if (action === "recoverMedia") {
              hls?.recoverMediaError();
              return;
            }
            gaveUp = true;
            setError(`${data.type}: ${data.details}`);
          });
        } else {
          video.src = mediaURL(next.playlist);
        }
        await video.play().catch(() => undefined);
      } catch (err) {
        if (!dead) setError(err instanceof Error ? err.message : t("recordings.playbackFailed"));
      }
    })();
    return () => {
      dead = true;
      hls?.destroy();
      releaseFileVideo(video);
    };
  }, [id, index, pictureMode]);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    const tick = () => {
      setWhere(video.currentTime);
      if (Number.isFinite(video.duration)) setLength(video.duration);
      if (!skip) return;
      const t = video.currentTime;
      const hit = shownRef.current.find((marker) => t >= marker.start && t < marker.end - 0.25);
      if (!hit) return;
      const end = video.seekable.length ? video.seekable.end(video.seekable.length - 1) : Number.NEGATIVE_INFINITY;
      const to = seekableSkip(hit.end, end);
      if (to != null) video.currentTime = to;
    };
    const ended = () => {
      setIndex((current) => (current + 1 < count ? current + 1 : current));
    };
    video.addEventListener("timeupdate", tick);
    video.addEventListener("ended", ended);
    return () => {
      video.removeEventListener("timeupdate", tick);
      video.removeEventListener("ended", ended);
    };
  }, [skip, count]);

  return (
    <Stage
      videoRef={videoRef}
      videoClass="stage-video"
      eyebrow={channel || t("recordings.libraryChannel")}
      title={title || t("recordings.recording")}
      onBack={onBack}
      backLabel={t("recordings.guide")}
      position={where}
      duration={length}
      onSeek={(value) => {
        const video = videoRef.current;
        if (video) video.currentTime = value;
      }}
      markers={shown}
      onJump={(delta) => {
        const video = videoRef.current;
        if (video) video.currentTime = Math.max(0, video.currentTime + delta);
      }}
      error={error}
      tools={
        count > 1 ? (
          <button type="button" className="text-btn" onClick={() => setIndex((current) => (current + 1) % count)}>
            {t("recordings.next")}
          </button>
        ) : null
      }
      more={
        <>
          <button type="button" className={skip ? "btn primary" : "btn"} onClick={() => setSkip((value) => !value)}>
            {skip ? t("recordings.skippingBreaks") : t("recordings.playingBreaks")}
          </button>
          <p className="hint">
            {count > 1 ? `${t("recordings.virtualIndex", { index: index + 1, count })} ` : ""}
            {t("recordings.virtualDisk")}
          </p>
        </>
      }
    />
  );
}
