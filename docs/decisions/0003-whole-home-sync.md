# 0003: Whole-Home Sync

Accepted, 2026-09-22.

## Goal

Every screen watching the same channel shows the same frame within about 50 ms on a home network, so audio from different rooms never echoes and nobody sees a play before anyone else. An optional group mode lets anyone pause, rewind, or jump to live for the whole room.

## Design

**Shared timeline.** All viewers of a channel already read the same HLS segments. Each segment carries `EXT-X-PROGRAM-DATE-TIME`, so any frame has a wall-clock timestamp (its broadcast time) that is identical on every client.

**Server clock.** Clients estimate their offset from the server clock with an NTP-style exchange over the events WebSocket: send `t0`, the server replies with `t1`, the client records `t2`, and keeps the sample with the smallest round trip. Offset = `t1 - (t0 + t2) / 2`. Clients re-sample periodically.

**Rooms.** The server keeps one sync room per channel (plus private rooms for group mode). A room state is:

- `anchorServerTime`: server time when the state was set
- `anchorMediaTime`: the program date-time playing at that moment
- `rate`: 1 when playing, 0 when paused
- `mode`: `follow` (independent controls, synced playback) or `group` (controls apply to everyone)

At any server time `T`, the target media time is `anchorMediaTime + (T - anchorServerTime) * rate`. By default the anchor sits a fixed latency behind the live edge (the `Balanced` latency target), so every client aims at the same point.

**Clients.**

- Apple: start with `AVPlayer.setRate(1, time: target, atHostTime: hostTime)` for a frame-accurate start. Then hold sync by nudging `rate` between 0.97 and 1.03 when drift exceeds 20 ms, and seek when it exceeds 400 ms. The `BroadwaveKit` sync engine owns this logic.
- Web: map `video.currentTime` to program date-time through the playlist's segments (both directions), trim `playbackRate` up to 3% under 400 ms of drift, and above that pause for exactly the drift when ahead or seek forward when behind. Backward seeks in a live buffer stall hls.js, so the engine never makes them outside group rewinds, and it seeks at most every 2 s.

Measured on a real ATSC broadcast (2026-09-22): two browser screens locked 15 ms apart, each within 5 ms of the room target.

**Group mode.** Pause, seek, and jump-to-live become room commands sent over the WebSocket. The server rewrites the anchor and broadcasts it, and every client converges. Because everyone reads the same buffer, rewinding for the room needs no extra tuner or transcode.

**Remote friends.** SharePlay (`AVPlaybackCoordinator`) covers watching together outside the home and is independent of room sync.

## Consequences

Sync needs accurate program date-times and aligned renditions (see 0002). It adds a WebSocket per client, which the events channel needs anyway.
