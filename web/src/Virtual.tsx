import Hls from "hls.js";
import { useEffect, useRef, useState } from "react";
import { playVirtual } from "./api";
import { fileHlsConfig, type PictureMode } from "./picture";
import { Stage } from "./Stage";

type Marker = { id: number; start: number; end: number };

export function VirtualPlay({ id, pictureMode, onBack }: { id: number; pictureMode: PictureMode; onBack: () => void }) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const [index, setIndex] = useState(0);
  const [error, setError] = useState("");
  const [title, setTitle] = useState("");
  const [channel, setChannel] = useState("");
  const [count, setCount] = useState(0);
  const [markers, setMarkers] = useState<Marker[]>([]);
  const [skip, setSkip] = useState(true);
  const [where, setWhere] = useState(0);
  const [length, setLength] = useState(0);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    let dead = false;
    let hls: Hls | null = null;
    setError("");
    void (async () => {
      try {
        const next = await playVirtual(id, index, pictureMode);
        if (dead) return;
        if (next.usesTuner) {
          setError("This library channel tried to use an antenna tuner.");
          return;
        }
        setTitle(next.recording.title);
        setChannel(`${next.number} ${next.name}`);
        setCount(next.count);
        setMarkers(next.markers);
        if (Hls.isSupported()) {
          hls = new Hls(fileHlsConfig(-1));
          hls.loadSource(next.playlist);
          hls.attachMedia(video);
          hls.on(Hls.Events.ERROR, (_event, data) => {
            if (data.fatal) setError(`${data.type}: ${data.details}`);
          });
        } else {
          video.src = next.playlist;
        }
        await video.play().catch(() => undefined);
      } catch (err) {
        if (!dead) setError(err instanceof Error ? err.message : "Playback did not start.");
      }
    })();
    return () => {
      dead = true;
      hls?.destroy();
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
      const hit = markers.find((marker) => t >= marker.start && t < marker.end - 0.25);
      if (hit) video.currentTime = hit.end;
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
  }, [markers, skip, count]);

  return (
    <Stage
      videoRef={videoRef}
      videoClass="stage-video"
      eyebrow={channel || "Library channel"}
      title={title || "Recording"}
      onBack={onBack}
      backLabel="Guide"
      position={where}
      duration={length}
      onSeek={(value) => {
        const video = videoRef.current;
        if (video) video.currentTime = value;
      }}
      markers={markers}
      onJump={(delta) => {
        const video = videoRef.current;
        if (video) video.currentTime = Math.max(0, video.currentTime + delta);
      }}
      error={error}
      tools={
        count > 1 ? (
          <button type="button" className="text-btn" onClick={() => setIndex((current) => (current + 1) % count)}>
            Next
          </button>
        ) : null
      }
      more={
        <>
          <button type="button" className={skip ? "btn primary" : "btn"} onClick={() => setSkip((value) => !value)}>
            {skip ? "Skipping breaks" : "Playing through breaks"}
          </button>
          <p className="hint">
            {count > 1 ? `Recording ${index + 1} of ${count}. ` : ""}
            This channel plays files already on disk. It does not use an antenna tuner.
          </p>
        </>
      }
    />
  );
}
