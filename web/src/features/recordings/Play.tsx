import Hls from "hls.js";
import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import { addMarker, deleteMarker, detectBreaks, playRecording, saveProgress } from "../../api";
import { fileHlsConfig, markerAt, readSkip, readZoom, saveSkip, saveZoom, type PictureMode, type SkipMode, type Zoom } from "../../picture";
import { Stage } from "../player/Stage";
import type { Recording } from "../../types";
import { DownloadLink } from "./DownloadLink";

type Marker = { id: number; start: number; end: number };

export function Play({
  recording,
  pictureMode,
  autoplay,
  onNext,
  onBack,
}: {
  recording: Recording;
  pictureMode: PictureMode;
  autoplay: boolean;
  onNext: () => void;
  onBack: () => void;
}) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const [error, setError] = useState("");
  const [markers, setMarkers] = useState<Marker[]>([]);
  const [skipMode, setSkipMode] = useState<SkipMode>(readSkip);
  const [zoom, setZoom] = useState<Zoom>(readZoom);
  const [where, setWhere] = useState(0);
  const [length, setLength] = useState(0);
  const aheadAt = useRef(0);
  const aheadBreak = useRef<Marker | null>(null);
  const [growing, setGrowing] = useState(recording.status === "recording");
  const whereRef = useRef(0);
  const saveTimer = useRef(0);
  // A resume seek that lands after the viewer has already moved would undo Start over.
  const viewerSought = useRef(false);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    viewerSought.current = false;
    let dead = false;
    let hls: Hls | null = null;
    let resumeAt = 0;
    let placed = false;
    void (async () => {
      try {
        const next = await playRecording(recording.id, pictureMode);
        if (dead) return;
        setMarkers(next.markers);
        setGrowing(next.growing);
        resumeAt = next.position;
        const started = performance.now();
        const place = () => {
          if (viewerSought.current || placed || resumeAt < 2) {
            placed = true;
            return;
          }
          if (performance.now() - started > 8000) {
            placed = true;
            return;
          }
          const end = video.seekable.length > 0 ? video.seekable.end(video.seekable.length - 1) : 0;
          if (end + 0.25 >= resumeAt) {
            video.currentTime = resumeAt;
            placed = true;
          }
        };
        if (Hls.isSupported()) {
          hls = new Hls(fileHlsConfig(resumeAt));
          hls.loadSource(next.playlist);
          hls.attachMedia(video);
          hls.on(Hls.Events.MANIFEST_PARSED, place);
          hls.on(Hls.Events.ERROR, (_e, data) => {
            if (data.fatal) setError(`${data.type}: ${data.details}`);
          });
        } else {
          video.src = next.playlist;
          video.addEventListener("loadedmetadata", place);
        }
        video.addEventListener("progress", place);
        const track = document.createElement("track");
        track.kind = "captions";
        track.label = "Captions";
        track.src = `/media/file/${recording.id}/captions.vtt`;
        video.appendChild(track);
        await video.play().catch(() => undefined);
        place();
      } catch (err) {
        if (!dead) setError(err instanceof Error ? err.message : "Playback did not start.");
      }
    })();
    return () => {
      dead = true;
      hls?.destroy();
      if (whereRef.current > 1) void saveProgress(recording.id, whereRef.current);
    };
  }, [recording.id, pictureMode]);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    const tick = () => {
      const t = video.currentTime;
      whereRef.current = t;
      setWhere(t);
      if (Number.isFinite(video.duration)) setLength(video.duration);
      if (t > 1) {
        window.clearTimeout(saveTimer.current);
        saveTimer.current = window.setTimeout(() => void saveProgress(recording.id, t), 4000);
      }
      if (skipMode !== "auto") return;
      const hit = markerAt(markers, t);
      if (hit) video.currentTime = hit.end;
    };
    const ended = () => {
      if (autoplay) onNext();
    };
    video.addEventListener("timeupdate", tick);
    video.addEventListener("ended", ended);
    return () => {
      video.removeEventListener("timeupdate", tick);
      video.removeEventListener("ended", ended);
      window.clearTimeout(saveTimer.current);
    };
  }, [markers, skipMode, recording.id, autoplay, onNext]);

  function chooseZoom(next: Zoom) {
    setZoom(next);
    saveZoom(next);
  }

  function chooseSkip(next: SkipMode) {
    setSkipMode(next);
    saveSkip(next);
  }

  function sought() {
    viewerSought.current = true;
  }

  function ahead() {
    sought();
    const video = videoRef.current;
    if (!video) return;
    const now = Date.now();
    const inside = markerAt(markers, video.currentTime) ?? null;
    if (aheadBreak.current && now - aheadAt.current < 1200) {
      video.currentTime = aheadBreak.current.end;
      aheadBreak.current = null;
      aheadAt.current = 0;
      return;
    }
    aheadBreak.current = inside;
    aheadAt.current = now;
    const end = video.seekable.length ? video.seekable.end(video.seekable.length - 1) : video.duration || video.currentTime + 30;
    video.currentTime = Math.min(end, video.currentTime + 30);
  }

  async function markHere() {
    const video = videoRef.current;
    if (!video) return;
    const start = video.currentTime;
    const created = await addMarker(recording.id, start, Math.min(video.duration || start + 3, start + 3));
    setMarkers((prev) => [...prev, { id: created.id, start: created.start, end: created.end }]);
  }

  async function scan() {
    const found = await detectBreaks(recording.id);
    setMarkers(found.markers);
  }

  async function removeMarker(id: number) {
    await deleteMarker(id);
    setMarkers((prev) => prev.filter((marker) => marker.id !== id));
  }

  async function startOver() {
    sought();
    const video = videoRef.current;
    if (video) {
      video.pause();
      video.currentTime = 0;
    }
    whereRef.current = 0;
    await saveProgress(recording.id, 0);
  }

  function togglePlay() {
    const video = videoRef.current;
    if (!video) return;
    if (video.paused) void video.play().catch(() => undefined);
    else video.pause();
  }

  function back(seconds: number) {
    sought();
    const video = videoRef.current;
    if (video) video.currentTime = Math.max(0, video.currentTime - seconds);
  }

  function onKey(event: KeyboardEvent) {
    if (event.target instanceof HTMLInputElement || event.target instanceof HTMLTextAreaElement) return;
    if (event.key === " " && event.target instanceof HTMLButtonElement) return;
    const actions: Record<string, () => void> = {
      " ": togglePlay,
      k: togglePlay,
      ArrowLeft: () => back(15),
      ArrowRight: ahead,
      Escape: onBack,
    };
    const fn = actions[event.key];
    if (!fn) return;
    event.preventDefault();
    fn();
  }

  const inside = markers.find((marker) => where >= marker.start && where < marker.end);
  // A finished recording plays from a playlist that grows while it transcodes,
  // so the player's own duration starts at a few seconds.
  const total = growing ? length : Math.max(length, recording.durationSec || 0);

  return (
    <Stage
      videoRef={videoRef}
      videoClass={zoom === "fit" ? "stage-video" : `stage-video ${zoom}`}
      eyebrow={recording.guideNumber}
      title={recording.subtitle ? `${recording.title} · ${recording.subtitle}` : recording.title}
      onBack={onBack}
      backLabel="Library"
      position={where}
      duration={total}
      onKeyDown={onKey}
      onSeek={(value) => {
        sought();
        const video = videoRef.current;
        if (!video) return;
        const end = video.seekable.length ? video.seekable.end(video.seekable.length - 1) : value;
        video.currentTime = Math.min(value, end);
      }}
      markers={markers}
      onJump={(delta) => {
        if (delta > 0) ahead();
        else back(-delta);
      }}
      error={error}
      tools={
        inside && skipMode === "button" ? (
          <button type="button" className="text-btn on" onClick={() => { if (videoRef.current) videoRef.current.currentTime = inside.end; }}>
            Skip break
          </button>
        ) : null
      }
      more={
        <>
          <div className="segmented" role="group" aria-label="Picture size">
            {(["fit", "fill", "zoom"] as const).map((item) => (
              <button key={item} type="button" className={zoom === item ? "seg on" : "seg"} onClick={() => chooseZoom(item)}>
                {item === "fit" ? "Fit" : item === "fill" ? "Fill" : "Zoom"}
              </button>
            ))}
          </div>
          <div className="segmented" role="group" aria-label="Break skipping">
            {(["auto", "button", "manual"] as const).map((item) => (
              <button key={item} type="button" className={skipMode === item ? "seg on" : "seg"} onClick={() => chooseSkip(item)}>
                {item === "auto" ? "Skip auto" : item === "button" ? "Skip button" : "Manual"}
              </button>
            ))}
          </div>
          <div className="sheet-actions">
            <button type="button" className="btn" onClick={() => void startOver()}>Start over</button>
            <DownloadLink id={recording.id} status={recording.status} />
            <button type="button" className="btn" onClick={() => void markHere()}>Mark 3 seconds</button>
            <button type="button" className="btn" onClick={() => void scan()}>Find black frames</button>
          </div>
          {markers.length > 0 ? (
            <ul className="marker-list">
              {markers.map((marker) => (
                <li key={marker.id}>
                  <span>{marker.start.toFixed(1)}s–{marker.end.toFixed(1)}s</span>
                  <button type="button" className="btn" onClick={() => void removeMarker(marker.id)}>Remove</button>
                </li>
              ))}
            </ul>
          ) : null}
          <p className="hint">
            {growing ? "This show is still recording. Playback starts at the beginning and keeps going as the file grows." : "Breaks show as marks on the timeline."}
          </p>
        </>
      }
    />
  );
}
