# Progress

Tick items as they are verified and committed (`- [x] A1 ... (commit abc1234)`). Keep notes short. The executing agent updates this file after every task.

## Phase A — Stabilize and ship
- [ ] A1 Setup wizard verified at desktop/phone; robust first-run detection
- [ ] A2 Guide refresh follows SiliconDust terms (random 20-28 h, rate-limited manual)
- [ ] A3 Deployed to Unraid (host network, VAAPI, Bonjour, sync from LAN, iPhone sim connects)
- [ ] A4 Hygiene: dead CSS/strings, web on /api/v1 only, eslint + swiftlint in CI
- [ ] A5 Restart robustness: orphan ffmpeg cleanup, interrupted recordings, graceful SIGTERM

## Phase B — Multiview
- [ ] B1 Server: tile renditions (540/360, audio none), multiview plan endpoint, mosaic rendition, multiview sync room
- [ ] B2 Web multiview: 2-up side-by-side, 1+2, 1+3, quad, PiP; audio focus; quick guide; saved sets; TV keys
- [ ] B3 Apple multiview: tvOS 2-up/quad, iPad 4-up, iPhone landscape 2-up; focus audio; AirPlay arbiter
- [ ] B4 Multiview polish + accessibility

## Phase C — Guide and metadata
- [ ] C1 Channel matching; >= 25/27 channels listed (or documented)
- [ ] C2 Sources: SiliconDust JSON guide (if permitted), Schedules Direct UI, XMLTV URL/file, priorities
- [ ] C3 Artwork: channel logos, program images, image proxy/cache, TMDB fallback
- [ ] C4 Rich program model (season/episode, rating, flags, cast)
- [ ] C5 Guide UX (web day scrubber, logos, column virtualization; Apple focus, iPhone landscape grid, iPad split)
- [ ] C6 Search (FTS5) on web + Apple

## Phase D — Sports
- [ ] D1 Sports provider (ESPN scoreboard, cached, pluggable)
- [ ] D2 Airing <-> game matching
- [ ] D3 Game-aware recording extension
- [ ] D4 Team follows + team passes
- [ ] D5 Spoiler-safe score bugs everywhere
- [ ] D6 Sports hub redesign

## Phase E — DVR
- [ ] E1 Passes UI overhaul
- [ ] E2 Conflict resolver
- [ ] E3 Recording library redesign
- [ ] E4 Commercial detection v2
- [ ] E5 Intro/credits detection
- [ ] E6 Recording health + re-record
- [ ] E7 Record from buffer / start over
- [ ] E8 Storage manager
- [ ] E9 Export/import helpers

## Phase F — Player
- [ ] F1 Captions (WebVTT rendition, pickers)
- [ ] F2 Master playlists with alternates
- [ ] F3 Web player extras (audio picker, stats, shortcuts, last channel, theater)
- [ ] F4 Instant channel switching
- [ ] F5 Apple player (info panels, contextual actions, remote gestures, PiP, AirPlay)
- [ ] F6 Group mode UI
- [ ] F7 Latency modes

## Phase G — Relay and infrastructure
- [ ] G1 Ring buffer
- [ ] G2 ABR ladder
- [ ] G3 LL-HLS packager
- [ ] G4 HEVC renditions
- [ ] G5 jellyfin-ffmpeg in image + capability detection
- [ ] G6 ATSC 3.0 (AC-4, DRM handling)
- [ ] G7 Multi-device tuner pool + recording reservations
- [ ] G8 Metrics and structured logging
- [ ] G9 Performance profiling on Unraid

## Phase H — Accounts and remote
- [ ] H1 Profiles
- [ ] H2 Device pairing + tokens
- [ ] H3 Remote access (Tailscale docs, then built-in)
- [ ] H4 Offline downloads

## Phase I — Apple integration
- [ ] I1 Top Shelf
- [ ] I2 Widgets
- [ ] I3 Live Activities
- [ ] I4 App Intents / Siri / Spotlight / Control Center
- [ ] I5 Move to Apple TV (Handoff)
- [ ] I6 SharePlay
- [ ] I7 iPad layouts
- [ ] I8 Apple Watch
- [ ] I9 Now Playing / CarPlay (optional)
- [ ] I10 App icon, onboarding, App Store assets

## Phase J — Design polish
- [ ] J1 Full design review with screenshots
- [ ] J2 Legacy web screens redesigned
- [ ] J3 Motion system
- [ ] J4 Accessibility audit
- [ ] J5 Light mode + accent picker
- [ ] J6 Localization readiness
- [ ] J7 Brand + marketing site

## Phase K — Quality
- [ ] K1 Fake HDHomeRun + Go integration tests
- [ ] K2 Playwright e2e + visual snapshots + sync test
- [ ] K3 Apple tests (Swift Testing + XCUITest)
- [ ] K4 24 h Unraid soak
- [ ] K5 CI expansion

## Phase L — Release
- [ ] L1 Versioning + changelog
- [ ] L2 GHCR images verified
- [ ] L3 Community Apps repo
- [ ] L4 TestFlight prep
- [ ] L5 Docs site
