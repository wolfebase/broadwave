// hls.js on the receiver's video element. The token is the load message's, not this device's storage.

import Hls from "hls.js";
import { fragTime, type Frag } from "../lib/roomStart.ts";
import { liveHlsConfig } from "../picture.ts";
import { mediaTime } from "./protocol.ts";
import type { CastPlayer } from "./session.ts";

export function hlsCastPlayer(video: HTMLVideoElement): CastPlayer {
  let hls: Hls | null = null;
  let token = "";

  function fragments(): Frag[] {
    if (!hls) return [];
    const level = hls.levels[Math.max(0, hls.currentLevel ?? hls.loadLevel ?? 0)] ?? hls.levels[0];
    return (level?.details?.fragments ?? []).map((frag) => ({
      start: frag.start,
      duration: frag.duration,
      programDateTime: frag.programDateTime ?? null,
    }));
  }

  return {
    load(url, nextToken) {
      token = nextToken;
      hls?.destroy();
      hls = null;
      if (!Hls.isSupported()) {
        video.src = url;
        return;
      }
      // autoStartLoad stays off so the first picture can be the room's frame.
      // The television config is the web player's. Its own Authorization header
      // reads this browser's storage, which a Cast device does not have.
      hls = new Hls({
        ...liveHlsConfig("tv"),
        autoStartLoad: false,
        xhrSetup(xhr) {
          if (token) xhr.setRequestHeader("Authorization", "Bearer " + token);
        },
      });
      hls.attachMedia(video);
      hls.loadSource(url);
    },
    startAt(seconds) {
      if (hls) hls.startLoad(seconds ?? -1);
      else if (seconds != null) video.currentTime = seconds;
    },
    stop() {
      hls?.destroy();
      hls = null;
      video.removeAttribute("src");
      try {
        video.load();
      } catch {
        // The element can already be gone.
      }
    },
    fragments,
    mediaNow() {
      if (video.readyState < 2 || video.seeking) return null;
      return mediaTime(fragments(), video.currentTime);
    },
    timeFor(mediaMs) {
      return fragTime(fragments(), mediaMs);
    },
    forwardSec() {
      const at = video.currentTime;
      const ranges = video.buffered;
      let ahead = 0;
      for (let i = 0; i < ranges.length; i++) {
        const end = ranges.end(i);
        if (at >= ranges.start(i) - 0.05 && at <= end) ahead = Math.max(ahead, end - at);
      }
      return ahead;
    },
    paused() {
      return video.paused;
    },
    pause() {
      video.pause();
    },
    play() {
      void video.play().catch(() => undefined);
    },
    seek(seconds) {
      video.currentTime = seconds;
    },
    setRate(rate) {
      video.playbackRate = rate;
    },
  };
}
