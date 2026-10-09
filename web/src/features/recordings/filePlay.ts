// A file player (a recording, or one recording of a library channel) shares one
// <video> across files.

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
