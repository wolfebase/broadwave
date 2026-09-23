---
name: waveguide-multiview
description: Design and implementation guide for Waveguide multiview (two or more live channels side by side, synced, with audio focus) across the Go server, web app, and iPhone/iPad/Apple TV apps, including tuner budgeting, tile renditions, layouts, and verification. Use when building or changing multiview, side-by-side viewing, quad view, picture-in-picture tiles, or mosaic streams.
---

# Waveguide multiview

The user's must-have: watch one channel and a different channel **right next to each other, clean and seamless**. Beat Channels DVR (live-only, no buffer, no sync) with synced tiles, instant audio focus, buffer-aware controls, and great layouts everywhere.

## Product rules

- Layouts: `2up` (side by side, default when adding the second channel), `1+2`, `1+3`, `quad`, `pip` (small over big). Phone portrait: stacked 2-up. Transitions animate (web View Transitions/FLIP; SwiftUI matchedGeometryEffect).
- Audio follows focus; exactly one tile unmuted. Focused tile gets the best rendition; others get tile renditions without audio.
- Adding a tile never interrupts the others (independent renditions already guarantee this server-side).
- All tiles are aligned in wall-clock terms: each channel's PDT timeline is broadcast wall time, so targeting "now - latency" for every tile shows the same real-world moment on both games.
- Tuner limits are explained, not hidden: channels on the same frequency share a tuner (e.g. 14.1-14.16); busy tuners name what's using them.
- Entry points: guide sheet "Add to multiview", context menus, player toolbar, `m` key, Sports "Watch together" for simultaneous games, saved sets on Home.

## Server work (Phase B1)

1. Tile renditions: add `360` video and `none` audio (`540.none`, `360.none`); keep `-copyts`, CMAF, keyframe expr (skill `waveguide-media-pipeline`). Table-test keys and `Decide` with a `tile: true` pref (or `quality: "tile"`).
2. `POST /api/v1/multiview/plan {channelIds}` -> `{playable:[...], blocked:[{channelId, reason, holders:[...]}], tunersNeeded, tunersFree}`; compute via frequency grouping (`channels.frequency_hz`, learned on tune) and live tuner status. Unknown frequency -> assume one tuner each.
3. Multiview sync room `multiview:<id>`: same RoomState math; all tiles join it. Optional group controls (pause all).
4. Mosaic rendition (optional, after web/Apple tiles work): one ffmpeg with multiple inputs from different feeds' pipes is hard because each feed is a separate mux subscriber — implement as a special feed reading N feeds' `copy` renditions (local HLS) or N pipe subscribers into `xstack=inputs=N:layout=...`, audio from the focused input; key `mosaic:<ids>:<layout>`.
5. Update OpenAPI + drift test + ADR 0004.

## Web work (Phase B2)

- `features/multiview/`: `MultiviewScreen` (route `/multiview?ch=1,3&layout=2up&focus=1`), `Tile` (own hls.js, reuses the LivePlayer loading logic factored into a hook `useLiveStream(channel, prefs)`), `QuickGuide` strip, `LayoutPicker`.
- Per-tile hls.js config: `capLevelToPlayerSize: true`, `backBufferLength: 20`, `maxBufferLength: 10` for small tiles; destroy on remove. Each tile runs a `SyncEngine` against the multiview room.
- Keyboard/TV: arrows move tile focus, Enter = focus audio/make big, `o` = tile menu, `g` = quick guide, Esc = exit to single view of focused tile.
- Persist last layout and saved sets (localStorage now, server profiles in Phase H).
- The persistent mini player concept: leaving multiview keeps the focused tile playing in the mini player.

## Apple work (Phase B3)

- `MultiviewScreen` with N `AVPlayer`s in a SwiftUI grid; `AVPlayerLayer`-backed views (not N `AVPlayerViewController`s) for tiles; full-screen single tile uses `AVPlayerViewController`.
- Focus: tvOS `@FocusState` per tile; Select toggles audio focus; long-press menu (Replace, Remove, Record, Full screen); swipe up shows a quick guide row; play/pause affects all.
- `player.isMuted = !focused`; `networkResourcePriority = focused ? .high : .low`; `AVRoutingPlaybackArbiter.shared.preferredParticipantForExternalPlayback = focusedPlayer`.
- Sync per tile via `OTAKit.SyncEngine` against `multiview:<id>`; evaluate `AVPlaybackCoordinationMedium` for coordinated pause/stall (don't let it fight live-edge alignment).
- iPhone landscape 2-up; iPad 4-up; Apple TV up to 4 (check decoder limits; downgrade tiles to 540/360 automatically).

## Verification

- Real DUO: 4.1 + 9.1 side by side (two tuners) and 14.1 + 14.2 + 14.3 (one tuner). Screenshot each layout at desktop, phone, TV sizes and in the tvOS/iPad simulators.
- Measure tile-to-tile wall-clock alignment: in each tile compare `data-sync-offset` (web) — target < 100 ms.
- 30-minute run: memory stable (Chrome task manager / `performance.memory`), no hls errors, no tuner leaks (`/api/v1/tuners` after exit shows ours=false within 25 s).
