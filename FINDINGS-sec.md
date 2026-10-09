# Security findings left for the owners of these files

This lane does not change `server/internal/realtime/rooms.go` or `apple/Packages/BroadwaveKit/Sources/BroadwaveKit/SyncEngine.swift`. Both need a fix that cannot live anywhere else.

## rooms.go: stall times are never forgotten

`stepBackLocked` stores `r.stepped[room]` the first time a solo follow room or a `multiview:` room steps back. `Leave` deletes the room and leaves that timestamp in place. Nothing removes a key older than `stallQuiet`.

A socket can grow the map without staying in the rooms. Join a new `multiview:` name (those are not counted in the group cap), set latency to stable so the room sits ahead of its floor, send `stalled`, then leave. The room struct is freed and the key remains. The per-socket room cap does not apply, because the socket leaves before the next join. Repeat with a new name.

`stepped` is unexported. The bus cannot prune it. Drop the key in `Leave`, or drop keys older than `stallQuiet` when a new one is stored. Pruning, rather than deleting on leave, keeps the quiet window across a quick rejoin.

## SyncEngine.swift: a room state can crash or freeze the player

`sync.state` numbers are applied with no range check. `JSONDecoder` accepts a value such as `1e308`. A normal room does not send one.

`report` puts `Int(d)` in the health message, where `d` is the drift. `apply` does the same when sync logging is on, and `giveUp` converts its drift the same way. `Int` of a huge, infinite, or non-numeric drift aborts the process. The fault is in this process, before a report is sent. The server cannot catch it.

`decide` returns `pause(resumeAfter: driftMS / 1000)` for a positive drift past the seek threshold, with no ceiling. `apply` sets `holdUntil` to that and schedules `endHold`. Later states are stored and not applied until the hold ends. A drift of about `1e12` milliseconds is a hold of decades. A non-finite delay does not fire. The hold has to be capped or cleared in this file. The server cannot cancel a timer this client already scheduled.
