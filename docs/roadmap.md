# Roadmap

> This file keeps the high-level history. Current work is tracked in the release notes (CHANGELOG.md).

Phases run roughly in order. Later phases can start when their dependencies are in place.

## 1. Foundation

- [x] Git, CI, monorepo layout (`server/`, `web/`, `apple/`, `design/`), Makefile
- [x] Architecture notes and decision records
- [x] Fix-first bugs: macOS disk stats, periodic guide refresh, fan-out subscriber leak
- [x] Numbered SQL migrations
- [x] `/api/v1` contract in OpenAPI, with generated TypeScript and Swift types
- [x] Design tokens that generate CSS and Swift

## 2. Relay engine v2

- [x] Per-frequency ring buffer shared by live, recordings, and exports
- [x] Rendition ladder with direct play and AC-3 passthrough (ADR 0002)
- [x] Capability-based stream decision with `streamInfo`
- [x] Live captions
- [x] Multi-tuner pool across devices; ATSC 3.0 (FLEX 4K) with AC-4 handling
- [x] Events WebSocket to replace polling
- [ ] Accounts and profiles
- [x] Bonjour `_broadwave._tcp`
- [x] Sync clock and rooms (ADR 0003)
- [x] Multi-arch image on GHCR, setup wizard, and diagnostics
- [ ] Unraid Community Apps

## 3. Web redesign

- [x] Feature folders, router, query layer, route code-splitting
- [x] New Home, Guide, Sports, Library, Player, Settings, and Setup on the design system
- [x] Player: fullscreen, PiP, stats, Whole-Home Sync (verified 15 ms between screens)
- [x] Player: caption and audio-track pickers on web

## 4. Apple TV and iPhone MVP

- [x] `BroadwaveKit` and `BroadwaveUI` packages
- [x] Discover and pair, Home, Guide, Player, Recordings, record and passes
- [x] App Store

## 5. Signature features

- [x] Whole-Home Sync (web + Apple engines)
- [ ] Group mode UI (shared pause/rewind) on Apple TV and iPhone
- [x] Multiview (4 on Apple TV and iPad, 2 on iPhone)
- [x] Sports hub, team passes, game-aware recording extension
- [x] Rich guide artwork and metadata

## 6. System integration

- [x] Widgets on iPhone and iPad
- [ ] Live Activities, Dynamic Island, and a Control Center control
- [x] Top Shelf
- [x] App Intents for Record on a widget
- [ ] Siri, Spotlight, and system Now Playing
- [ ] SharePlay
- [x] Commercial skip, with intro and credits detection

## 7. Launch

- [x] Name, icon, and the App Store
- [ ] Unraid Community Apps
- [x] Docs site (`docs/guide/`, built by `docs/sitegen/`)

## Later

Secure remote access and downloads; Mac and visionOS.
