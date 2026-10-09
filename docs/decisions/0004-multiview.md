# 0004: Multiview

Accepted, 2026-09-23.

## Goal

Watch two or more live channels at once, aligned to the same wall-clock moment, with audio on the focused tile. Channels on one broadcast frequency still share a single tuner.

## Design

**Tiles are renditions.** Side by side, every tile asks for the large-tile picture with stereo. A quad asks for 360p with stereo on every tile. A small tile beside a large one, including the small picture-in-picture tile, asks for `tile` (540p) or `360` (360p) and `audio: none`. Those renditions keep `-copyts` and CMAF, and they close on the broadcast's keyframes (`-force_key_frames source`), so they sit on the same timeline as the full channel. The focused tile in a layout with one large picture uses the normal decision, including the original picture when the device can play it. Adding a tile starts a rendition; it does not restart the others.

**Tuner budget.** `POST /api/v1/multiview/plan` takes channel ids and returns which ones fit. A known frequency costs one tuner no matter how many subchannels are in the set. A channel whose frequency has not been learned costs a tuner of its own. A frequency this server already has tuned is free. A link or a file does not take an antenna tuner. The blocked reason names what is already on.

**Each tile has its own room.** A tile joins `multiview:<id>:<channel>`. A shared room stepped every tile back when any of them stalled. Pause, play, and jump to live are sent to every tile. Each tile still reads its own channel's playlist.

**Apple tiles are player layers.** Each tile is its own `AVPlayer`, not its own player controller. Mute, network priority, and `AVRoutingPlaybackArbiter` follow focus, so AirPlay takes the tile you are listening to. `AVPlaybackCoordinationMedium` is not attached: it seeks every player onto one timeline, and these are different live edges. Pause is sent to every tile.

**Mosaic for one-stream screens.** A single ffmpeg `xstack` of 2 to 4 channels, keyed by its channel ids with the sound channel first (`4-12`), is for AirPlay, older devices, and other apps through the exports. It is a delivery shortcut, not the way the apps watch: tiles stay separate players, each in its own room. The mosaic reads each channel's shared tune like an export, so it costs no tuner a channel already has; it costs two pictures of the budget (software decode of every input and one 1080p60 encode). Its inputs drop their broadcast timestamps (no `-copyts`), since stations' clocks are unrelated and only arrival lines them up; tiles can sit up to one group of pictures apart.

## Consequences

A small tile beside a large picture costs a video transcode and no audio transcode. A quad's 360p tiles transcode stereo as well. Four tiles on four frequencies need four tuners; four subchannels of one frequency need one. Clients mute every tile except the focused one even when a tile has audio, so a late rendition change never leaves two games audible.
