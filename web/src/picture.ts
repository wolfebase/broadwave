export type Zoom = "fit" | "fill" | "zoom";
export type SkipMode = "auto" | "button" | "manual";
export type PictureMode = "broadcast" | "smooth" | "film";

export function liveHlsConfig() {
  return {
    liveSyncDurationCount: 4,
    maxLiveSyncPlaybackRate: 1,
    maxBufferHole: 0.5,
    stretchShortVideoTrack: true,
  };
}

export function fileHlsConfig(start: number) {
  return {
    startPosition: start > 2 ? start : -1,
    maxBufferHole: 0.5,
    stretchShortVideoTrack: true,
  };
}

const zoomKey = "ota-zoom";
const skipKey = "ota-skip";

export function readZoom(): Zoom {
  const value = localStorage.getItem(zoomKey);
  return value === "fill" || value === "zoom" ? value : "fit";
}

export function saveZoom(value: Zoom) {
  localStorage.setItem(zoomKey, value);
}

export function readSkip(): SkipMode {
  const value = localStorage.getItem(skipKey);
  return value === "button" || value === "manual" ? value : "auto";
}

export function saveSkip(value: SkipMode) {
  localStorage.setItem(skipKey, value);
}

export function markerAt<T extends { start: number; end: number }>(markers: T[], time: number) {
  return markers.find((marker) => time >= marker.start && time < marker.end - 0.05);
}
