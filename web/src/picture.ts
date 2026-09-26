export type Zoom = "fit" | "fill" | "zoom";
export type SkipMode = "auto" | "button" | "manual";
export type PictureMode = "broadcast" | "smooth" | "film";

export type BufferProfile = "phone" | "desktop" | "tv" | "tile";

/** Forward buffer stays past the 10s live latency. Tiles and phones keep less behind the playhead. */
export function liveHlsConfig(profile: BufferProfile = "desktop") {
  const buffers = {
    phone: { liveSyncDurationCount: 3, maxBufferLength: 16, maxMaxBufferLength: 24, backBufferLength: 30 },
    desktop: { liveSyncDurationCount: 3, maxBufferLength: 24, maxMaxBufferLength: 40, backBufferLength: 90 },
    tv: { liveSyncDurationCount: 4, maxBufferLength: 30, maxMaxBufferLength: 60, backBufferLength: 120 },
    tile: { liveSyncDurationCount: 3, maxBufferLength: 16, maxMaxBufferLength: 24, backBufferLength: 20 },
  }[profile];
  return {
    ...buffers,
    // Parts of the open segment are playable. liveSyncDuration is not also set:
    // hls.js rejects both, and the count keeps a long-running stream off the edge.
    lowLatencyMode: true,
    liveMaxLatencyDurationCount: 100000,
    maxLiveSyncPlaybackRate: 1,
    maxBufferHole: 0.5,
    stretchShortVideoTrack: true,
    capLevelToPlayerSize: profile === "phone" || profile === "tile",
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
