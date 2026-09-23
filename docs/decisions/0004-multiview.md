# 0004: Multiview

Accepted, 2026-09-23.

## Goal

Watch two or more live channels at once, aligned to the same wall-clock moment, with audio on the focused tile. Channels on one broadcast frequency still share a single tuner.

## Design

**Tiles are renditions.** An unfocused tile asks for `tile` (540p) or `360` (360p) and `audio: none`. Those renditions keep `-copyts`, 2-second keyframes, and CMAF, so they sit on the same timeline as the full channel. The focused tile uses the normal decision, including the original picture when the device can play it. Adding a tile starts a rendition; it does not restart the others.

**Tuner budget.** `POST /api/v1/multiview/plan` takes channel ids and returns which ones fit. A known frequency costs one tuner no matter how many subchannels are in the set. A channel whose frequency has not been learned costs a tuner of its own. A frequency this server already has tuned is free. A link or a file does not take an antenna tuner. The blocked reason names what is already on.

**One room for the session.** Tiles join `multiview:<id>`. The room uses the same anchor math as any other room, so every tile aims at one program date-time. The room is a group: pause, play, and jump to live move every tile. Each tile still reads its own channel's playlist; the shared value is wall-clock time, which is the same on every broadcast timeline.

**Apple tiles are player layers.** Each tile is its own `AVPlayer`, not its own player controller. Mute, network priority, and `AVRoutingPlaybackArbiter` follow focus, so AirPlay takes the tile you are listening to. `AVPlaybackCoordinationMedium` is not attached: it seeks every player onto one timeline, and these are different live edges. The shared room already pauses them together.

**Mosaic later.** A single ffmpeg `xstack` of several channels (`mosaic:<ids>:<layout>`) is for AirPlay and older devices. It waits until the tiles themselves are solid, because a mosaic is a delivery shortcut, not the way the apps watch.

## Consequences

Small tiles cost a video transcode and no audio transcode. Four tiles on four frequencies need four tuners; four subchannels of one frequency need one. Clients mute every tile except the focused one even when a tile has audio, so a late rendition change never leaves two games audible.
