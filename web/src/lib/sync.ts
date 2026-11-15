import Hls from "hls.js";
import { events, type RoomState } from "./events.ts";
import { roomStart, type Frag } from "./roomStart.ts";

export { SyncEngine, type SyncEnv, type SyncStatus } from "./syncEngine.ts";

/**
 * Starts hls.js on the room's frame instead of its live edge. A screen joining
 * a room that plays well behind the edge otherwise held its first frame for
 * the whole gap. hls must be created with autoStartLoad off; this starts it
 * once, at the edge when the room's state or the playlist is not in within
 * waitMs, or the playlist does not hold the room's frame. A master loads no
 * level before startLoad, so its caller passes the primed level's fragments.
 */
export function startOnRoom(hls: Hls, video: HTMLVideoElement, room: string, waitMs = 800, primed?: Frag[]) {
  const bus = events();
  let st = bus.roomState(room) as RoomState | undefined;
  let frags: Frag[] | null = null;
  let timer = 0;
  let started = false;
  const off = bus.on("sync.state", (data) => {
    if ((data as RoomState).room !== room) return;
    st = data as RoomState;
    if (frags) start();
  });
  const done = () => {
    started = true;
    window.clearTimeout(timer);
    off();
  };
  const start = () => {
    if (started) return;
    done();
    const pos = st && frags ? roomStart(frags, st, bus.serverNow()) : null;
    video.dataset.syncStart = pos == null ? "edge" : "room";
    hls.startLoad(pos ?? -1);
  };
  hls.once(Hls.Events.MANIFEST_PARSED, () => {
    if (started) return;
    if (primed) {
      frags = primed;
      if (st) return start();
    }
    timer = window.setTimeout(start, waitMs);
  });
  if (!primed)
    hls.once(Hls.Events.LEVEL_UPDATED, (_e, data) => {
      frags = data.details.live ? data.details.fragments : [];
      if (st || !data.details.live) start();
    });
  hls.once(Hls.Events.DESTROYING, done);
}
