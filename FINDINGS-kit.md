# Kit findings

Reviewed with the kit tests. These two are in `SyncEngine.swift`, which this lane does not change.

## A locked catch-up still counts as a miss

`apply` increments `misses` when a seek is still behind 3 to 15 seconds later. The `.play` arm, including a lock, never sets `misses` back to 0. Only `rejoin` does.

One catch-up that lands short, then locks, leaves `misses` at 1. The next time the screen is behind, the first 3-to-15-second window makes `misses` 2 and `giveUp` runs. That is one miss of the new incident, not two. `giveUp` sets `detached` and stops correcting until the viewer rejoins.

## A drift hold plays through a room pause

When the screen is ahead, `apply` pauses and schedules `endHold`. Until `holdUntil`, a newer `sync.state` is ignored, including a room rate of 0. `endHold` seeks only when the rate is not 0, then always calls `player.play()`.

A group or multiview room can pause while this screen is waiting out its lead. The timer starts playback again, and the next `apply` (up to a quarter second later) pauses it. A channel room does not take a pause command, so the single-channel player does not hit this.

## A failed socket open drops a fresh pause

`EventSocket.send` queues a `sync.command` only while `task` is nil. `connect` flushes that queue and clears it before the handshake finishes, and the send completion ignores the error. If that attempt never opens, the command is gone. A command sent after `task` is set and before `connected` is not queued at all. The web client keeps the command until `onopen` and only then applies the 3 second cutoff. A test would have to fail a handshake without calling `connect`, which opens a real socket.

## The demo's play jumps to live

`DemoServer.command` handles `"play"` and `"live"` in one arm and sets `anchorMedia` to about 10 seconds behind the wall clock. The server resumes `"play"` from the paused anchor and only `"live"` jumps. A paused demo room that is told to play leaves the frame it paused on.

## A probed server is never removed

`Discovery.update` removes a Bonjour withdrawal only when the id is not in `probed`. The first UDP answer inserts the id and nothing clears `probed`, so that row stays for the life of the `Discovery` object. A resolve that finishes after the service has left can also add the row back.
