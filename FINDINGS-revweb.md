# revweb findings

## A channel room rejects Live

`Rooms.Apply` in `server/internal/realtime/rooms.go` returns `ErrFollowRoom` for a playback command, including `"live"`, when the room is not a group. A channel room is a follow room. The web Live button must not send that command. Sync already holds the live frame. That file was not changed.
