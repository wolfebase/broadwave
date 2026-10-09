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

/** Positions at the start are not a resume point. Writing them would wipe one. */
export function progressToStore(time: number): number | null {
  return time > 1 ? time : null;
}

/** A listener or save from an earlier file must not run against the one on screen. */
export function samePlayback(listenerGen: number, currentGen: number): boolean {
  return listenerGen === currentGen;
}
