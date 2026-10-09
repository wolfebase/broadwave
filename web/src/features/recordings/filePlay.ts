// A file player (a recording, or one recording of a library channel) shares one
// <video> across files. These keep a later file from inheriting the previous
// file's resume seek, caption track, or playlist request.

export type FileFatal = "startLoad" | "recoverMedia" | "show";

/**
 * A fatal hls.js error on a file gets one recovery for that kind. A second of
 * the same kind, or any other kind, is shown. The caller keeps `tried` for the
 * life of that player instance.
 */
export function takeFileFatal(type: string, tried: { network: boolean; media: boolean }): FileFatal {
  if (type === "networkError" && !tried.network) {
    tried.network = true;
    return "startLoad";
  }
  if (type === "mediaError" && !tried.media) {
    tried.media = true;
    return "recoverMedia";
  }
  return "show";
}

export type FileListenerTarget = {
  addEventListener(type: string, listener: () => void): void;
  removeEventListener(type: string, listener: () => void): void;
};

export type FileTrack = { remove(): void };

/**
 * `progress` (and `loadedmetadata` for native playback) plus an optional caption
 * track. Stop removes every one of them, so the next file's element does not
 * still seek to this file's resume point.
 */
export function bindFilePlayback(video: FileListenerTarget, place: () => void, options: { native: boolean; track?: FileTrack | null }): () => void {
  video.addEventListener("progress", place);
  if (options.native) video.addEventListener("loadedmetadata", place);
  let removed = false;
  return () => {
    if (removed) return;
    removed = true;
    video.removeEventListener("progress", place);
    if (options.native) video.removeEventListener("loadedmetadata", place);
    options.track?.remove();
  };
}

export type FileVideo = {
  pause(): void;
  removeAttribute(name: string): void;
  load(): void;
};

/** Stops the element from pulling the playlist it was just playing. */
export function releaseFileVideo(video: FileVideo) {
  video.pause();
  video.removeAttribute("src");
  video.load();
}

/**
 * A playing file saves on a cadence. Resetting the wait on every timeupdate
 * would never fire while the picture is moving. `armed` is true while a save
 * is already waiting.
 */
export function progressSaveAction(armed: boolean, time: number): "arm" | "keep" | "idle" {
  if (!(time > 1)) return "idle";
  return armed ? "keep" : "arm";
}

/**
 * Positions at the start are not a resume point. A file still playing its
 * lead-in is not one either: saving it would wipe a place the seek has not reached.
 */
export function progressToStore(time: number, resumeAt = 0, sought = false): number | null {
  if (!(time > 1)) return null;
  if (!sought && resumeAt > time) return null;
  return time;
}

export type ResumeGate = { at: number; known: boolean };

/**
 * The playhead to store. Until the server's resume point is known, or while
 * the playhead is still behind it, only a seek the viewer made is stored.
 */
export function storedPlayhead(time: number, gate: ResumeGate, sought: boolean): number | null {
  if (!gate.known && !sought) return null;
  return progressToStore(time, gate.known ? gate.at : 0, sought);
}

/** A listener or save from an earlier file must not run against the one on screen. */
export function samePlayback(listenerGen: number, currentGen: number): boolean {
  return listenerGen === currentGen;
}

/** Auto-skip target. Null until the marker end is inside the seekable range. */
export function seekableSkip(markerEnd: number, seekableEnd: number): number | null {
  if (!Number.isFinite(markerEnd) || !Number.isFinite(seekableEnd)) return null;
  if (seekableEnd < markerEnd) return null;
  return markerEnd;
}

/** Markers stored for another recording. A marker with no recording id does not match. */
export function markersForPlayback<T extends { recordingId?: number }>(recordingId: number | null, markers: readonly T[]): T[] {
  if (recordingId == null) return [];
  return markers.filter((marker) => marker.recordingId === recordingId);
}

/**
 * Up next stays off while the file is still being written.
 * reported is the play response, null until it arrives.
 * A later status other than recording wins over a response that said true.
 */
export function playbackGrowing(status: string, reported: boolean | null): boolean {
  if (status !== "recording") return false;
  return reported !== false;
}
