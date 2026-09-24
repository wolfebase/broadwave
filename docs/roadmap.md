# Roadmap

> Superseded for execution by `docs/plan/MASTER_PLAN.md` (phases A-L) and its checklist `docs/plan/PROGRESS.md`. This file keeps the high-level history.

Phases run roughly in order. Later phases can start when their dependencies are in place.

## 1. Foundation

- [x] Git, CI, monorepo layout (`server/`, `web/`, `apple/`, `design/`), Makefile
- [x] Agent guidance: `AGENTS.md`, scoped rules, architecture and decisions
- [x] Fix-first bugs: macOS disk stats, periodic guide refresh, fan-out subscriber leak
- [x] Numbered SQL migrations
- [x] `/api/v1` contract in OpenAPI with generated TypeScript types (Swift models are hand-mirrored in BroadwaveKit for now)
- [x] Design tokens that generate CSS and Swift

## 2. Relay engine v2

- [ ] Per-frequency ring buffer shared by live, recordings, and exports
- [x] Rendition ladder with direct play and AC-3 passthrough (ADR 0002)
- [x] Capability-based stream decision with `streamInfo`
- [ ] Live captions
- [ ] Multi-tuner pool across devices; ATSC 3.0 (FLEX 4K) with AC-4 handling
- [x] Events WebSocket to replace polling
- [ ] Accounts, profiles, device pairing
- [x] Bonjour `_broadwave._tcp`
- [x] Sync clock and rooms (ADR 0003)
- [ ] Multi-arch image on GHCR, Unraid Community Apps, setup wizard, diagnostics

## 3. Web redesign

- [x] Feature folders, router, query layer, route code-splitting
- [x] New Home, Guide, Sports, Library, Player, Settings, and Setup on the design system
- [x] Player: fullscreen, PiP, stats, Whole-Home Sync (verified 15 ms between screens)
- [ ] Player: caption and audio-track pickers on web

## 4. Apple TV and iPhone MVP

- [x] `BroadwaveKit` and `BroadwaveUI` packages
- [x] Discover and pair, Home, Guide, Player, Recordings, record and passes
- [ ] TestFlight

## 5. Signature features

- [x] Whole-Home Sync (web + Apple engines)
- [ ] Group mode UI (shared pause/rewind) on Apple TV and iPhone
- [ ] Multiview (4 on Apple TV, 2 on iPhone landscape)
- [ ] Sports hub, team passes, game-aware recording extension
- [ ] Rich guide artwork and metadata

## 6. System integration

- [ ] Live Activities and Dynamic Island, widgets, Control Center control
- [ ] Top Shelf, App Intents and Siri, Spotlight, Now Playing
- [ ] SharePlay, commercial skip polish, intro and credits detection

## 7. Launch

- [ ] Name, icon, App Store, Community Apps, docs site

## Later

Secure remote access and downloads; iPad layouts; Apple Watch (remote, scores, recording alerts); Mac and visionOS.
