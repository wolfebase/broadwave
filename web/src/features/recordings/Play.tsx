import Hls from "hls.js";
import { useEffect, useRef, useState, useSyncExternalStore, type KeyboardEvent } from "react";
import { addMarker, deleteMarker, detectBreaks, playRecording, saveProgress } from "../../api";
import { fileHlsConfig, markerAt, readSkip, readZoom, saveSkip, saveZoom, type PictureMode, type SkipMode, type Zoom } from "../../picture";
import { copy } from "../../strings";
import { Stage } from "../player/Stage";
import type { Recording } from "../../types";
import { episodeTag } from "../library/model";
import { breakScans, idleBreakScan } from "./breaks";
import { DownloadLink } from "./DownloadLink";
import { introSkip, upNext } from "./ends";
import { planRecordingSeek } from "./seek";

type Marker = { id: number; start: number; end: number; confidence?: number };

// The server skips on its own from this confidence up and offers the skip
// below it. A marker someone set, or one from an older server, is sure.
const autoSkip = 0.7;
const sure = (marker: Marker) => (marker.confidence ?? 1) >= autoSkip;

export function Play({
  recording,
  pictureMode,
  autoplay,
  next,
  onNext,
  onBack,
}: {
  recording: Recording;
  pictureMode: PictureMode;
  autoplay: boolean;
  /** The episode Up next offers; onNext plays it. */
  next?: Recording;
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
  // Where this encode started. The playlist before that is gap, not the picture.
  const encodedStart = useRef(0);
  const relocate = useRef<(to: number) => void>(() => {});
  const seekToRef = useRef<(value: number) => void>(() => {});
  // The spot a reload is already fetching. Time updates must not start it again.
  const relocating = useRef<number | null>(null);
  const scan = useSyncExternalStore(
    breakScans.subscribe,
    () => breakScans.get(recording.id),
    () => idleBreakScan,
  );
  const [seenScan, setSeenScan] = useState(scan);
  if (scan !== seenScan) {
    setSeenScan(scan);
    if (scan.markers) setMarkers(scan.markers);
  }
  // Not now holds for the rest of this recording, its end included.
  const [dismissed, setDismissed] = useState(false);
  const [seenId, setSeenId] = useState(recording.id);
  if (seenId !== recording.id) {
    setSeenId(recording.id);
    setDismissed(false);
  }

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    viewerSought.current = false;
    let dead = false;
    let gen = 0;
    let hls: Hls | null = null;
    let detach: (() => void) | null = null;
    const track = document.createElement("track");
    track.kind = "captions";
    track.label = "Captions";
    track.src = `/media/file/${recording.id}/captions.vtt`;
    video.appendChild(track);

    const start = async (at: number | null) => {
      const mine = ++gen;
      detach?.();
      detach = null;
      hls?.destroy();
      hls = null;
      if (at != null) {
        whereRef.current = at;
        try {
          await saveProgress(recording.id, at);
        } catch {
          // Play still reads whatever progress was saved.
        }
        if (dead || mine !== gen) return;
      } else {
        relocating.current = null;
      }
      let resumeAt = 0;
      let placed = false;
      try {
        const next = await playRecording(recording.id, pictureMode);
        if (dead || mine !== gen) return;
        setError("");
        setMarkers(next.markers);
        setGrowing(next.growing);
        encodedStart.current = next.position;
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
            if (data.fatal && mine === gen) setError(`${data.type}: ${data.details}`);
          });
        } else {
          video.src = next.playlist;
          video.addEventListener("loadedmetadata", place);
        }
        video.addEventListener("progress", place);
        detach = () => {
          video.removeEventListener("progress", place);
          video.removeEventListener("loadedmetadata", place);
        };
        await video.play().catch(() => undefined);
        if (dead || mine !== gen) return;
        place();
      } catch (err) {
        if (mine === gen) relocating.current = null;
        if (!dead && mine === gen) setError(err instanceof Error ? err.message : "Playback did not start.");
      }
    };

    relocate.current = (to: number) => {
      void start(to);
    };
    void start(null);
    return () => {
      dead = true;
      gen += 1;
      relocate.current = () => {};
      detach?.();
      hls?.destroy();
      track.remove();
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
      if (hit && sure(hit)) seekToRef.current(hit.end);
    };
    const ended = () => {
      if (autoplay && !dismissed) onNext();
    };
    video.addEventListener("timeupdate", tick);
    video.addEventListener("ended", ended);
    return () => {
      video.removeEventListener("timeupdate", tick);
      video.removeEventListener("ended", ended);
      window.clearTimeout(saveTimer.current);
    };
  }, [markers, skipMode, recording.id, autoplay, dismissed, onNext]);

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
      seekTo(aheadBreak.current.end);
      aheadBreak.current = null;
      aheadAt.current = 0;
      return;
    }
    aheadBreak.current = inside;
    aheadAt.current = now;
    seekTo(video.currentTime + 30);
  }

  async function markHere() {
    const video = videoRef.current;
    if (!video) return;
    const start = video.currentTime;
    const created = await addMarker(recording.id, start, Math.min(video.duration || start + 3, start + 3));
    setMarkers((prev) => [...prev, { id: created.id, start: created.start, end: created.end }]);
  }

  function scanCommercials() {
    if (!breakScans.begin(recording.id)) return;
    void detectBreaks(recording.id)
      .then((found) => breakScans.finish(recording.id, found.markers, copy.library.foundBreaks(found.markers.length)))
      .catch(() => breakScans.fail(recording.id, copy.library.findFailed));
  }

  async function removeMarker(id: number) {
    await deleteMarker(id);
    setMarkers((prev) => prev.filter((marker) => marker.id !== id));
  }

  function seekTo(value: number) {
    const video = videoRef.current;
    if (!video) return;
    const end = video.seekable.length ? video.seekable.end(video.seekable.length - 1) : 0;
    const plan = planRecordingSeek(value, encodedStart.current, end);
    if (plan.action === "within") {
      relocating.current = null;
      video.currentTime = plan.to;
      whereRef.current = plan.to;
      return;
    }
    if (relocating.current != null && Math.abs(relocating.current - plan.to) < 0.5) return;
    relocating.current = plan.to;
    relocate.current(plan.to);
  }
  seekToRef.current = seekTo;

  async function startOver() {
    sought();
    const video = videoRef.current;
    const end = video && video.seekable.length ? video.seekable.end(video.seekable.length - 1) : 0;
    if (planRecordingSeek(0, encodedStart.current, end).action === "reload") {
      relocate.current(0);
      return;
    }
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
    if (!video) return;
    seekTo(Math.max(0, video.currentTime - seconds));
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
  // The playlist grows while the recording transcodes, so the player's own
  // duration starts at a few seconds. The recording's length is the file,
  // including a show that is still being recorded.
  const total = Math.max(length, recording.durationSec || 0);
  const introEnd = inside ? null : introSkip(recording, where);
  const card = next && !dismissed && !growing ? upNext(where, total, recording.creditsStart, autoplay) : null;
  const left = card?.left;
  // Only a count the viewer saw plays the next one: a resume or a scrub that
  // lands past it leaves this one playing to its end.
  const counted = useRef(false);
  useEffect(() => {
    if (left == null) counted.current = false;
    else if (left > 0) counted.current = true;
    else if (counted.current) {
      counted.current = false;
      onNext();
    }
  }, [left, onNext]);
  const nextLabel = next ? [episodeTag(next), next.subtitle].filter(Boolean).join(" · ") || next.title : "";

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
        seekTo(value);
      }}
      markers={markers}
      onJump={(delta) => {
        if (delta > 0) ahead();
        else back(-delta);
      }}
      error={error}
      note={card ? `Up next: ${nextLabel}` : undefined}
      noteAction={
        card ? (
          <>
            {card.left !== null ? <p className="hint" aria-hidden="true">Playing in {card.left} s</p> : null}
            <div className="sheet-actions">
              <button type="button" className="btn" onClick={onNext}>Play now</button>
              <button type="button" className="btn" onClick={() => setDismissed(true)}>Not now</button>
            </div>
          </>
        ) : null
      }
      hold={Boolean(card)}
      tools={
        inside && (skipMode === "button" || (skipMode === "auto" && !sure(inside))) ? (
          <button type="button" className="text-btn on" onClick={() => { sought(); seekTo(inside.end); }}>
            Skip break
          </button>
        ) : introEnd !== null ? (
          <button
            type="button"
            className="text-btn on"
            onClick={() => {
              sought();
              seekTo(introEnd);
            }}
          >
            Skip intro
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
            <button
              type="button"
              className="btn"
              disabled={scan.running}
              aria-busy={scan.running ? true : undefined}
              onClick={scanCommercials}
            >
              {scan.running ? copy.library.findingCommercials : copy.library.findCommercials}
            </button>
          </div>
          {scan.note ? <p className="hint" role="status">{scan.note}</p> : null}
          {markers.length > 0 ? (
            <ul className="marker-list">
              {markers.map((marker) => (
                <li key={marker.id}>
                  <span>
                    {marker.start.toFixed(1)}s–{marker.end.toFixed(1)}s{sure(marker) ? "" : " · maybe"}
                  </span>
                  <button type="button" className="btn" onClick={() => void removeMarker(marker.id)}>Remove</button>
                </li>
              ))}
            </ul>
          ) : null}
          <p className="hint">
            {growing ? "This show is still recording. It keeps playing as the file grows." : "Breaks show as marks on the timeline. One marked maybe gets a Skip button instead of skipping on its own."}
          </p>
        </>
      }
    />
  );
}
