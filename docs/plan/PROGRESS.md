# Progress

Tick items as they are verified and committed, in the same commit as the work (`- [x] R3 ... (commit abc1234)`). Keep notes short. Order of work is section 4 of `MASTER_PLAN.md`, not the order of this file.

## Resume here
- Current task: **S5**. Xtream Codes.
- Half done: S4 is in this commit. `make check` passed.
- Next command: import live categories from a fake Xtream server.
- Unraid is on `v0.3.3`. Both tuners are free. Next recording is Jeopardy at 20:00 UTC.
- A7 retry at phase S: `asc web auth status` is unauthenticated. Still blocked (see BLOCKERS).

## Phase R — Repair and consolidate
- [x] R1 CI green on all four jobs at 0984064 (lint fixes e411da1, Xcode 26 guard 0984064); `make check`; CI section in the dev-loop skill
- [x] R2 v0.2.0 tagged with CHANGELOG, deployed to Unraid via GHCR, A–D features checked on the LAN (tag v0.2.0, log in UNRAID_LOG)
- [x] R3 Instant boot: cached shell, windowed airings, gzip/brotli + ETag, Apple snapshot cache (< 300 ms warm)
- [x] R4 Artwork layouts by size and aspect; no upscaling past 1.25x
- [x] R5 Live preview frames from tuned muxes (no extra tunes). Unraid CPU sample is the phase R deploy line.
- [x] R6 Apple screenshots of B–D on iPhone, iPad, and Apple TV (`docs/screenshots/r6-*`). iPhone multiview stays 2-up. Quad layout is on iPad and Apple TV; those tiles timed out when :8477 flapped, and the 2-up shots are the live picture. Scoreboard was empty, so no score was on screen.
- [x] R7 Code review of run 1. `go test -race ./server/...` clean. Hiding a score no longer edits the cached board. Multiview waits for the tuner plan before a tile starts, on web and Apple.
- [x] Phase R deploy: tag, Unraid smoke, and preview-frame CPU under 3% of one core per mux

## Phase S — Every source, found automatically
- [x] S0 Research sources and discovery; ADR 0009; `docs/sources.md` support matrix
- [x] S1 One source model (kinds, capabilities, masked credentials, stable channel ids) with a lossless migration
- [x] S2 Auto-find engine: broadcast, unicast, SSDP, mDNS, Look harder, cloud last resort; the DUO answered on the LAN and from a bridged container; a new address keeps the channel id (commit 4c3da1c)
- [x] S3 HDHomeRun family: two devices with failover, a tuner held for a recording, copy-protected channels hidden, scan on a fake tuner and a finished scan on the DUO. FLEX ATSC 3.0 stays on G6. EXTEND adds transcode=mobile when the server encoder is software. SCRIBE recordings parse from recorded_files.json. (commit d18c9af)
- [x] S4 M3U done right: tvg-* / group / url-tvg / tvc-guide-stationid, file + URL + path, stream format and limit, numbering, groups, per-source XMLTV, refresh with stable ids, picker for big lists
- [ ] S5 Xtream Codes source
- [ ] S6 tvheadend, Threadfin/xTeVe/ErsatzTV/Dispatcharr/Antennas, Channels DVR server, HDHomeRun-compatible by address; unsupported devices documented
- [ ] S7 Free channels: detect FastChannels / Pluto / Samsung generator containers, or guide the user to run FastChannels
- [ ] S8 Setup wizard v2 on web, Apple TV, and iPhone (< 90 s, no typing for HDHomeRun)
- [ ] S9 Setup doctor: bridge network, /dev/dri, volumes, disk, TZ, clock, PUID/PGID/UMASK
- [ ] S10 Source health and limits

## Phase P — Server and Apple apps, working as one
- [ ] P1 Generated Swift and TypeScript clients from OpenAPI; drift check in CI
- [ ] P2 Golden-response contract tests for OTAKit and web in `make check`
- [ ] P3 apiVersion/features negotiation and minimum app version
- [ ] P4 Bonjour + UDP fallback discovery, remembered servers, follow address changes, permission explainer
- [ ] P5 Full setup and management from the Apple apps; `docs/parity.md`
- [ ] P6 Realtime resilience: reconnect, re-join, survive server restart/upgrade, sleep/wake
- [ ] P7 End-to-end CI: Docker image + fake tuner + XCUITest on iOS and tvOS + sync check
- [ ] P8 The container is the reference server for e2e and soak

## Phase N — Every screen people already own
- [ ] N1 Verified as a tuner in Channels, Plex, Jellyfin, Emby (was G11); copyable URLs in Settings
- [ ] N2 Tokened M3U/XMLTV and an Xtream-compatible output for IPTV players (after H2)
- [ ] N3 Web app as a PWA with full D-pad control on TV browsers
- [ ] N4 Google Cast with a synced custom receiver
- [ ] N5 DLNA/UPnP media server (optional)
- [ ] N6 Android / Android TV / Fire TV app (or `docs/android.md` design)
- [ ] N7 Roku design notes

## Phase A — Stabilize and ship
- [x] A1 Setup wizard verified at desktop/phone; robust first-run detection (commit d6bd17f)
- [x] A2 Guide refresh follows SiliconDust terms (random 20-28 h, rate-limited manual) (commit ce5d6a6)
- [x] A3 Deployed to Unraid: host network, VAAPI, catalog migrated, 6 ms two-tab sync, iPhone sim playing (see UNRAID_LOG)
- [x] A4 Hygiene: unused copy removed, web on /api/v1 only, eslint and swiftlint clean in CI (commit ca94523)
- [x] A5 Restart kills leftover ffmpeg, fails a cut-off recording, resumes a show still on, and SIGTERM releases the tuner
- [x] A6 Public repo wolfebase/waveguide (twolfekc does not exist; see BLOCKERS), CI green, ghcr.io/wolfebase/waveguide:0.1.0 public for amd64 and arm64 (commit 5c9308b, tag v0.1.0)
- [ ] A7 Apple signing done (team D4MC63SS36, bundle IDs, profiles, `scripts/testflight.sh`, iOS archive signs). Remaining: app records, App Group on profiles, iOS + tvOS uploads. Blocked on one Apple ID login (see BLOCKERS); retry at the start of every phase

## Phase B — Multiview
- [x] B1 Tile renditions (540.none, 360.none), tuner plan, and a shared multiview room (commit feb8c17). Mosaic waits until the tiles exist (ADR 0004). 14.x still has no stored frequency, so the plan treats those as separate tuners until one tune locks.
- [x] B2 Web multiview: 2-up, 1+2, 1+3, quad, PiP; sound follows focus; quick guide; saved sets; TV keys (commit efd9bf4). Real DUO 4.1 and 9.1 stayed 4 ms apart on 540 tiles; tuners released after leaving.
- [x] B3 Apple multiview: tvOS 2-up and quad, iPad up to 4, iPhone portrait stacked; sound and AirPlay follow focus (commit fd51f07). Simulator check on the real DUO: Apple TV showed 4.1 and 9.1 side by side and in the top row of quad; iPhone stacked both with sound on 4.1.
- [x] B4 Multiview polish + accessibility (commit 58af4e3). First open says to select a tile. The focused tile shows how it is sent. Make big is in the tile menu. VoiceOver reads the channel and the program.
- [ ] B5 Mosaic rendition (xstack) for AirPlay, older devices, and exports

## Phase C — Guide and metadata
- [x] C1 Channel matching (commit 716382d). SiliconDust XMLTV and the JSON guide both publish the same 9 of 27 (4.1, 5.1, 9.1, 9.4, 29.1, 38.1, 39.7, 41.1, 62.1). 14.1–14.16, 43.3, and 46.7 are absent, so they stay empty until another source is added. Call signs that differ by DT or HD still match, and Settings can pin a guide match.
- [x] C2 Guide sources (commit 82a80a7). The JSON guide lists the same 9 channels as XMLTV, so it is not a second listing source. Settings takes a Schedules Direct account (14 days, one day if the account refuses) and an XMLTV link. Both fill only channels the tuner guide skipped. No Schedules Direct account was found in other projects. A lineup list and a file upload wait on an account.
- [x] C3 Artwork (commit e246413). Eight of the nine listed channels have logos (38.1 does not). All 609 of their programs have an image. `/media/art` caches a resized copy, and a blank program gets a category placeholder. Settings can store a movie artwork key for programs the guide left blank. No key was required for this lineup.
- [x] C4 Rich program model (commit f291b7d). Season and episode, the onscreen label, original air date, series id, live, premiere, finale, rating, and cast are stored and shown. This feed sends season, episode, and a date on most programs, and live on a couple. It does not send ratings or cast, so those stay empty. Listings already saved pick this up on the next guide refresh.
- [x] C5 Guide UX (commit 37be574). The grid covers the listings on hand (up to two days). Now, Tonight, and Tomorrow jump the time. A Now button returns when now has scrolled away. Wide cells show a picture. Drag a channel to reorder it on this screen; right-click or H hides it. iPhone landscape and Apple TV use the grid, play/pause on Apple TV starts the focused show, and iPad keeps the program beside the guide.
- [x] C6 Search (commit d7def66). Full-text search covers titles, subtitles, descriptions, and cast, plus recordings. Search on the web and on Apple lists the matches. Record every airing makes a pass for that title.
- [ ] C7 Guide from the broadcast: relay on full mux `/auto/ch<freq>`, PSIP EIT/ETT parser, passive harvest, preemptible idle scan, merge; >= 25/27 channels listed
- [ ] C8 Honest guide depth and per-listing source
- [ ] C9 Antenna and signal tools (Settings > Tuners)

## Phase D — Sports
- [x] D1 Sports provider (commit 7923323). ESPN scoreboard for twelve leagues, with F1 and NASCAR as schedules only. A live board refreshes every 30 seconds; a quiet board waits two hours. A failed fetch backs off and keeps the last good board.
- [x] D2 Airing <-> game matching (commit ce1fcb7). A listing matches a game when both teams, or the race name, appear in the title or subtitle and the start is within 90 minutes. The game id is stored on the airing.
- [x] D3 Game-aware recording extension (commit c444120). A matched game keeps recording while it is on, and for a delay before the start. It stops 8 minutes after the game ends. A sports listing that did not match records an extra hour. The activity log says when a recording was extended.
- [x] D4 Team follows + team passes (commit 2a6777a). Follow a team from Sports. Home shows Your teams. Record every game is a pass that matches the team on any channel. The activity log announces the next game once.
- [x] D5 Spoiler-safe score bugs (commit ee8f7a4). Scores show on the guide, sports cards, the player, and multiview. Hide scores in Settings blanks every result. A recorded game stays blank until it is watched.
- [x] D6 Sports hub redesign (commit 9db0294). Sports shows what is on now, with the score and how long is left when the game is linked. A matched game uses its logo and color. Watch together plays the games that are on at once.
- [ ] D7 Game Switcher (auto focus on the game that matters in multiview)
- [ ] D8 Game alerts (followed teams, close games; spoiler-safe)

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
- [ ] G1 Ring buffer (do right after K1; unblocks E7, F4, M1)
- [ ] G2 ABR ladder
- [ ] G3 LL-HLS packager
- [ ] G4 HEVC renditions
- [ ] G5 jellyfin-ffmpeg in image + capability detection
- [ ] G6 ATSC 3.0 (AC-4, DRM handling)
- [x] G7 moved to S3 (tracked there)
- [ ] G8 Metrics and structured logging
- [ ] G9 Performance profiling on Unraid
- [ ] G10 HDHomeRun health surface (read-only)
- [x] G11 moved to N1 (tracked there)

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
  - R6: iPad side-by-side tiles stretch to the full height, so the picture sits in a short band (`r6-ipad-multiview-2.jpg`).
  - R6: a blocked tile's note can name a channel that is not on (4.1 listed while that tile said both tuners were busy).
  - R6: tvOS focused layout control can read as an empty pill. Confirm the label is actually missing.
- [ ] J2 Legacy web screens redesigned
- [ ] J3 Motion system
- [ ] J4 Accessibility audit
- [ ] J5 Light mode + accent picker
- [ ] J6 Localization readiness
- [ ] J7 Brand: name decided (Waveguide, renamed everywhere); logo, app icon, marketing site to do

## Phase K — Quality
- [ ] K1 Fake HDHomeRun + Go integration tests + relay smoke in CI (do right after C9)
- [ ] K2 Playwright e2e + visual snapshots + sync test
- [ ] K3 Apple tests (Swift Testing + XCUITest)
- [ ] K4 24 h Unraid soak
- [ ] K5 CI expansion
- [ ] K6 Performance budgets in CI

## Phase L — Release
- [ ] L1 Versioning + changelog
- [ ] L2 GHCR images verified
- [ ] L3 Community Apps: researched, template installed on TUS via URL, Validate + Scan clean, submitted
- [ ] L4 TestFlight external + App Store readiness (icon, screenshots, privacy labels, review notes)
- [ ] L5 Docs site

## Phase M — Category-best extras
- [ ] M1 Catch-up rewind and start over from the ring buffer
- [ ] M2 Commercial skip while behind live
- [ ] M3 "Which channel has the game?" from search, Siri, and widgets
- [ ] M4 Smart favorites by time of day (on-device)
- [ ] M5 Improvements found in J1 and K4 (add tasks, then do them)
