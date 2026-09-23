# OTA Viewer — Master Plan: from great to spectacular

Written 2026-09-23 as a handoff. The executing agent works through **every phase in order without stopping for approval between milestones**, committing as it goes, and only stops when the whole plan is done or a hard blocker (listed in `docs/plan/BLOCKERS.md`) makes further progress impossible everywhere.

Read in this order before touching code:

1. `AGENTS.md` (project guide), then `.cursor/rules/*.mdc`
2. This file, then `docs/plan/PROGRESS.md` (live checklist; update it as you go)
3. Skills: `ota-viewer-dev-loop`, `ota-viewer-media-pipeline`, `ota-viewer-apple`, `ota-viewer-unraid` (personal), `ota-viewer-multiview`
4. `docs/architecture.md`, `docs/decisions/0001-0003`, `docs/research.md`

---

## 0. Operating mode (non-negotiable)

- **Autonomous run.** Do not pause at phase boundaries to ask. Finish a task, verify it, commit it, tick it in `PROGRESS.md`, move to the next. The user explicitly wants the agent to "rip through the whole thing."
- **Verify, don't assume.** Every task has acceptance checks. Server: Go tests + `scripts/relay-smoke.sh`. Web: typecheck + build + browser check at desktop, phone, and TV sizes with the real tuner. Apple: both schemes build + simulator screenshot of the changed screen. Unraid: deploy and hit it from the LAN.
- **Commit small and often** with descriptive messages (what + why). Never leave the tree broken at a commit.
- **When blocked**, write the blocker (what, why, what was tried, what would unblock) to `docs/plan/BLOCKERS.md`, stub or feature-flag around it, and continue with the next task. Only things needing the user's hands (Apple developer team ID, APNs key, TestFlight upload, buying hardware) are legitimate blockers.
- **Protect what works.** The relay, Whole-Home Sync (15 ms measured), CMAF pipeline, and the real-tuner playback paths are proven. Any change to `internal/live`, `web/src/lib/sync.ts`, or `OTAKit/SyncEngine.swift` must re-run the relay smoke test and the two-screen sync measurement (see skill `ota-viewer-dev-loop`).
- **Keep the docs true.** Update `docs/architecture.md`, ADRs, `api/openapi.yaml` (drift test enforces routes), `AGENTS.md`, and the skills when behavior changes. Add ADRs for big decisions (multiview, guide sources, LL-HLS, auth).
- **Real hardware.** HDHomeRun CONNECT DUO at 192.168.1.252 (2 tuners, ATSC 1.0, 27 channels). Unraid TUS at 192.168.1.2 (i9-12900K, UHD 770, VAAPI). The dev Mac has VideoToolbox. The user's live catalog is in `data/` (never delete it; copy it for tests).

---

## 1. Where it stands (verified 2026-09-23)

**Server (Go, `server/`)**
- One tune per RF frequency; subchannels, renditions, recordings, and exports read one stream (`internal/live/hub.go`).
- Renditions are independent ffmpeg processes keyed `video.audio[.mode]` (`copy`, `1080`, `720`, `540` × `copy`, `aac2`, `aac6`). One viewer's change never restarts another (tested).
- `Decide()` picks per device: Apple gets original H.264 + AC-3; browsers get copy video + AAC; MPEG-2 is transcoded; unprobed H.264 is deinterlaced until the background ffprobe learns field order (stored in `channels.field_order`).
- CMAF/fMP4 HLS with `-copyts`; the server reads each segment's first video PTS from `tfdt`/`trun` and stamps `EXT-X-PROGRAM-DATE-TIME` from a per-feed Timeline, identical across renditions. Segment 0 withheld. Watch waits for 3 segments.
- Encoders: NVENC, QSV, VideoToolbox (with `-a53cc 0`), VAAPI, libx264 fallback.
- Realtime WebSocket `/api/v1/ws`: activity, live.changed, clock sync, sync rooms (follow + group).
- Bonjour `_otaviewer._tcp` with id/name/version TXT. HDHomeRun emulator on :8478 (whole lineup), `/export/lineup.m3u`, `/export/guide.xml`, `/export/stream/{id}`.
- Numbered SQL migrations (0001-0003), server identity, `/api/v1/diagnostics`, `-healthcheck` flag, index.html no-cache.
- DVR: passes (title/contains/category, pads, priority, keep, new-only), conflicts, comskip/blackdetect markers, virtual (library) channels.

**Web (`web/`)**: React 19 + Vite, design tokens (`design/tokens.json` -> CSS + Swift), glass shell, Home (hero, On now, Sports, Tonight, recordings), Guide (virtualized grid, tally now line, category tints, progress fill, recording badges, jump, program sheet, phone On-now list), Sports hub, persistent live player (full <-> mini on one `<video>`), stream info, mini guide, options, Whole-Home Sync engine (`lib/sync.ts`), setup wizard (not yet visually verified), diagnostics page, route code-splitting (81 KB gz initial).

**Apple (`apple/`)**: XcodeGen project (iOS + tvOS 26.1), `OTAKit` (models tested against real JSON, API client, Bonjour discovery, event socket, AVPlayer SyncEngine, AppStore, guide logic), `OTAUI` (tokens, components). Screens: Connect, Home, Guide (grid with pinned column/header; iPhone On-now list), Sports, Recordings, Settings, AVPlayerViewController live player (sync pill, channel up/down, record, tvOS Channels menu), recording player, deep links (`otaviewer://watch/<id>`, `connect?url=`), debug `-OTAWatch <id>`. Verified live playback on iPhone 17 Pro and Apple TV 4K simulators.

**Deploy**: Dockerfile (web + Go multi-stage, VA drivers, HEALTHCHECK), Dockerfile.runtime (prebuilt binary), compose + Unraid template (host network), GHCR release workflow (amd64+arm64), CI (Go, web, Apple, Docker). `scripts/dev-server.sh`, `scripts/relay-smoke.sh`, `scripts/deploy-unraid.sh`.

**Known gaps and debt (fix early)**
1. **Guide coverage is poor:** only 9 of 27 channels have listings, and only ~2 days deep. SiliconDust's free XMLTV gives 2 days (14 needs their DVR subscription) and asks for refreshes at randomized 20-28 h intervals; we refresh every 4 h. See Phase 3.
2. The Unraid container still runs the **old build, bridge network, old template** — no Bonjour, no new UI. See Phase 1.
3. Setup wizard not visually verified; first-run detection heuristic (`setupComplete` / no recordings / no passes).
4. Accounts/pairing/remote access not started; the API is open on the LAN.
5. Captions: live renditions don't carry a caption track the web UI can select; VideoToolbox transcodes drop A/53 captions entirely.
6. Only one rendition per client (no ABR ladder); no LL-HLS.
7. No multiview anywhere (user's must-have).
8. Swift models are hand-mirrored (no generator yet); Apple apps are MVP-level in polish.
9. Old web screens (Library, Schedule, Sources, Settings forms) got the new styles but not a redesign.
10. Debug data attributes on `<video>` (`data-sync-drift`, `data-sync-offset`, `data-hls-error`) and `video.hls` are intentional diagnostics; keep them but document them.

---

## 2. Research digest (sources in `docs/research.md`; re-verify anything version-sensitive)

- **Apple multiview (tvOS/iOS 26+):** `AVPlaybackCoordinationMedium` synchronizes rate changes, seeks, stalls, and startup across multiple `AVPlayer`s (`player.playbackCoordinator.coordinate(using: medium)`). `AVRoutingPlaybackArbiter.shared.preferredParticipantForExternalPlayback` / `preferredParticipantForNonMixableAudioRoutes` pick which tile goes to AirPlay/HomePod. Set `AVPlayer.networkResourcePriority` high for the focused tile, low for others. Apple sample: "Creating a seamless multiview playback experience" (WWDC25 session 302). AVFoundation types are `@Observable` on 26+.
- **Channels DVR parity:** multiview up to 4 (live only, no buffer, Quick Guide to add, hold-to-replace, layouts cycle with Select), intro/credits detection (preview, per-show opt-in, needs 10+ episodes), Enhanced Commercial Detection (fingerprinting, re-fingerprint, idle backfill), Personal Sections, Theater Mode, Sports/News sections, "upcoming airings" context menu, Live Activities for downloads. We beat them with: sync across screens, multiview with a buffer and sync, modern design, open exports, free.
- **LL-HLS:** ffmpeg's HLS muxer does not emit `EXT-X-PART` / `EXT-X-PRELOAD-HINT`; true LL-HLS needs our own packager (we already parse fMP4 boxes) and blocking playlist reload (`_HLS_msn`, `_HLS_part`, `CAN-BLOCK-RELOAD=YES`). hls.js needs `lowLatencyMode: true` and a server that really blocks. AVPlayer supports it fully.
- **hls.js multiview:** 3-4 instances per page is reasonable; fMP4 is key for CPU; Chrome MSE budget ~150 MB video / 12 MB audio **per SourceBuffer**, so cap back buffer per tile (`backBufferLength` 20-30 s) and use `capLevelToPlayerSize`. On iPhone Safari use ManagedMediaSource (hls.js 1.6 does automatically) and `disableRemotePlayback`.
- **ATSC 3.0:** HEVC video + AC-4 audio. Stock ffmpeg lacks an AC-4 decoder; **jellyfin-ffmpeg** has one (experimental; resample to 48 kHz). DRM (A3SA/Widevine) stations cannot be decrypted by ffmpeg; show them as "Protected."
- **Guide data:** SiliconDust XMLTV at `api.hdhomerun.com/api/xmltv?DeviceAuth=` (read DeviceAuth fresh each call; accept gzip; randomized 20-28 h cadence) includes `<icon src>` channel logos and program images. There is also a JSON guide endpoint `api.hdhomerun.com/api/guide?DeviceAuth=...` (fields include `ImageURL`, `EpisodeNumber`, `Synopsis`; paged by start time) — investigate coverage/terms for the missing channels. Schedules Direct remains the paid, complete, 14-day option. TMDB for fallback artwork.
- **Live sports status:** ESPN's unofficial `site.api.espn.com/apis/site/v2/sports/{sport}/{league}/scoreboard[?dates=YYYYMMDD]` gives events with `status.type.state` (`pre`/`in`/`post`), `completed`, clock, period, competitors (names, abbreviations, logos, colors, scores), broadcasts. Unofficial: cache, back off, degrade gracefully.
- **Live Activities:** iOS 18+ broadcast push via channels (`api-manage-broadcast.push.apple.com` to create; `apns-push-type: liveactivity` + `apns-channel-id`) needs the developer's APNs .p8 key — a self-hosted server cannot push without it. Plan: local Live Activities updated by the app while running + optional APNs relay later.
- **Top Shelf:** `TVTopShelfContentProvider` returning `TVTopShelfCarouselContent` (`.actions` / `.details`), action URLs deep link; Swift 6 needs `@preconcurrency import TVServices` or the completion-handler override.
- **Unraid Community Apps:** public repo with OSI LICENSE, `ca_profile.xml` (non-empty `<Profile>`), template XMLs (e.g. `templates/ota-viewer.xml`), real `icon.svg`; validate at ca.unraid.net/submit/new.
- **Commercial detection:** multi-signal (black frames, silence, uniform frames, scene cuts, logo presence) + audio fingerprinting of repeated ads across recordings beats comskip-only; Channels re-fingerprints recordings.

---

## 3. Phases

Each task: **Do** / **Accept**. Tick them in `PROGRESS.md`.

### Phase A — Stabilize, verify, and ship what exists (first)

A1. **Setup wizard verification.** Start `FRESH=1 CONFIG=/tmp/otav-fresh scripts/dev-server.sh`; load with cache disabled (CDP `Network.setCacheDisabled`); walk all 4 steps at desktop and phone sizes; fix layout/logic bugs. First-run detection: prefer an explicit server-side flag (`setupComplete` unset AND server age < 1 day OR no favorites/passes/recordings) — make it robust and testable. **Accept:** screenshots of each step; completing sets `setupComplete=1`; existing installs never see it unasked.

A2. **Guide refresh compliance.** SiliconDust XMLTV: next pull at a random 20-28 h after success; store `lastGuidePull` + `nextGuidePull` in settings; manual refresh allowed but rate-limited (e.g. 1/hour). Log source and counts per refresh. **Accept:** unit test for the scheduler; diagnostics shows next refresh time.

A3. **Deploy to Unraid** with `scripts/deploy-unraid.sh` (see skill `ota-viewer-unraid`): back up the catalog, switch to host networking, drop `HDHR_HOST` (discovery works on host network; keep the variable optional), update `/boot/config/plugins/dockerMan/templates-user/my-OTA-Viewer.xml` to match `deploy/unraid/ota-viewer.xml` (host network, no personal defaults except values the user needs), confirm VAAPI encoder detected, migrations applied to the real catalog, Bonjour visible from the Mac (`dns-sd -B _otaviewer._tcp`), web UI at `http://192.168.1.2:8477`, live playback + sync from two browser tabs against the Unraid server, and the iPhone simulator connecting to it. **Accept:** all of that, recorded in `docs/plan/UNRAID_LOG.md` with timestamps.

A4. **Hygiene.** Remove stale `.player*`-era CSS, unused `strings.ts` entries, legacy `/api` callers in the web app (move everything to `/api/v1`), dead code (`live/file.go` PictureArgs paths stay for recordings). Add `web` lint (eslint + typescript-eslint, react-hooks) and `swiftformat`/`swiftlint` config. **Accept:** lint clean in CI.

A5. **Crash/restart robustness.** On server start: kill orphaned ffmpeg children from a previous run (track PIDs in `work/pids`), clear stale `work/live/*`, mark interrupted recordings `failed` or resume them if their airing is still on. Graceful shutdown on SIGTERM (stop renditions, finish recordings cleanly, release tuners). **Accept:** kill -9 the server mid-recording, restart, see correct state; tuners released.

### Phase B — MULTIVIEW (the user's must-have; make it the best in the category)

Read skill `ota-viewer-multiview` first. Write ADR 0004 (multiview).

B1. **Server: multiview-aware relay.**
- Add a `tile` quality class: `540` and a new `360` video rendition (`360.aac2`), both with aligned 2 s keyframes; allow `copy` tiles when bandwidth allows (Apple TV on LAN).
- Tuner budgeting: an endpoint `POST /api/v1/multiview/plan {channelIds:[...]}` returns which channels can play together given tuners in use (channels on the same frequency cost one tuner), with explanations ("5.1 and 5.2 share one tuner"). Busy responses name what holds the tuners.
- Audio-follows-focus stays client-side (mute non-focused tiles), but offer `audio=none` renditions (`540.none`) so unfocused tiles cost no audio transcode.
- Optional **server-composited mosaic**: one ffmpeg `xstack` rendition combining 2-4 channels into a single 1080p stream (for AirPlay targets, older devices, low bandwidth, and exporting a mosaic channel to Plex). Key `mosaic:<ids>:<layout>`.
- Sync: every tile already has PDT on its channel's timeline; add a multiview room type `multiview:<sessionId>` whose target is shared across tiles (all tiles aim at "now - latency" in wall-clock terms, so the same moment in real time across different channels — critical for watching two games at once).
- **Accept:** Go tests for planning and keys; relay smoke extended to two channels + mosaic; tuner math verified on the real DUO (e.g. 14.1+14.2+14.3 on one tuner).

B2. **Web multiview.**
- Layouts: side-by-side 2-up (the user's primary ask: two channels next to each other, clean and seamless), 1 big + 2 small, 1 big + 3 small, 2×2 quad, picture-in-picture (small tile over big). Smooth animated transitions between layouts (FLIP/View Transitions API).
- Entry points: "Add to multiview" in the guide sheet, program context menu, player toolbar button, Sports hub "Watch together" on simultaneous games, keyboard `m`.
- Tile chrome: channel badge, live dot, title, score bug (Phase D), audio indicator; hover/focus shows swap, make big, remove, record. Click/Enter on a tile moves audio focus (and makes it big in 1+N layouts). Drag to reorder.
- A "Quick Guide" strip at the bottom to add/replace tiles without leaving multiview.
- Performance: one hls.js per tile, `capLevelToPlayerSize`, `backBufferLength: 20`, request smaller renditions for small tiles (`540`/`360` + `none` audio), upgrade the focused tile to `copy`/`1080`; pause decode of fully hidden tiles.
- Sync: all tiles join one multiview room; each tile uses the existing SyncEngine logic (pause-to-align, forward seeks only).
- Tuner-limit UX: disabled "Add" with a clear reason; suggest same-frequency channels that are free.
- Saved sets: "Sunday games", persisted per profile; a Multiview entry on Home when 2+ sports are live.
- Full-screen and TV layout (arrow keys move focus between tiles; Select swaps audio; long-press/`o` opens tile menu).
- **Accept:** two real channels side by side from the DUO, synced (measure tile-to-tile offset < 100 ms in wall-clock terms), audio focus switching instant, 60 fps UI, memory stable over 30 minutes (Chrome task manager), works at phone (stacked 2-up), desktop, TV sizes.

B3. **Apple multiview (tvOS first, then iPad/iPhone).**
- `MultiviewScreen` with N `AVPlayer`s (N ≤ 4 on Apple TV 4K, 2 on iPhone landscape, 4 on iPad), layouts matching web, focus-driven audio (`isMuted` on non-focused), `networkResourcePriority` high for focused.
- Coordination: channels are different live streams, so use our SyncEngine per tile against the shared multiview room for wall-clock alignment; use `AVPlaybackCoordinationMedium` for pause/resume/stall behavior across tiles (evaluate: if it conflicts with independent live edges, restrict it to group pause/play).
- AirPlay: `AVRoutingPlaybackArbiter` preferred participant = focused tile.
- tvOS interactions: Select swaps focus/audio; play/pause pauses all; long-press tile -> Replace/Remove/Record/Make full screen; swipe up shows Quick Guide row; Menu exits to single view of the focused tile.
- iPhone: landscape 2-up side by side; portrait stacked with a draggable divider; PiP of the focused tile.
- **Accept:** tvOS simulator screenshots of 2-up and quad with real channels; focus + audio switching; builds with strict concurrency.

B4. **Multiview polish.** Layout animation tuning, per-tile stream info, "swap with main" gesture, remember last layout, onboarding hint the first time. Accessibility: VoiceOver announces tile channel + program, focus order logical.

### Phase C — Guide and metadata excellence

C1. **Coverage.** Diagnose why 18/27 channels lack listings (compare `lineup.json` guide numbers/call signs vs XMLTV `<channel>` ids/display-names; subchannels like 14.x may be listed under call signs). Build a channel-matching layer (station id, call sign, number, name fuzzy) with manual override in Settings > Channels ("Match guide data").
C2. **More sources.** Implement the SiliconDust JSON guide (`/api/guide`, paged by `Start`) if it covers more channels/days within terms; full Schedules Direct integration with a Settings UI (account, lineup picker, 14 days, images); user XMLTV URL/file; per-channel source priority; merge rules. ADR 0005 for guide sources.
C3. **Artwork.** Parse XMLTV `<icon>` for channels and programmes; store image URLs; server-side image proxy + resize cache (`/media/art/...?w=`), placeholders by category. TMDB fallback (requires user API key; optional). Use art everywhere: guide cells (optional thumbnails in TV layout), program sheet hero, Home hero backdrop, recordings posters, Top Shelf.
C4. **Rich program model.** Series/season/episode numbers, original air date, rating, cast, genres, `new`/`live`/`premiere`/`finale` flags, sports teams (Phase D). Migration + API + clients.
C5. **Guide UX.** Web: time scrubber to any day, "jump to prime time", mini-thumbnails, channel logos, column virtualization, sticky "now" return button, reorder/hide channels inline, per-profile channel lists. Apple: guide focus behavior perfection (tvOS: focus follows time, swipe to change day, play/pause to watch focused), iPhone landscape full grid, iPad split view (guide + preview).
C6. **Search.** Server full-text search over titles/subtitles/descriptions/cast (SQLite FTS5) across 14 days + recordings; web and Apple search tabs; "search results -> record every airing".
**Accept (phase):** ≥ 25/27 channels with listings on the real lineup (or documented reason), 7+ days where the source allows, art on most programs, search fast (< 50 ms).

### Phase D — Sports: the reason people cancel YouTube TV

D1. **Sports data provider** (`internal/sports`): ESPN scoreboard client for NFL, NCAAF, NBA, WNBA, NCAAB, MLB, NHL, MLS, NWSL, EPL, F1/NASCAR (schedule-only), with caching (30 s during live windows, hours otherwise), backoff, and a provider interface so it can be swapped.
D2. **Matching.** Link guide airings to games (league from category/title, teams from title/subtitle vs competitor names/abbreviations/aliases, start time ±90 min, broadcaster hints). Store `game_id` on airings.
D3. **Game-aware recording.** When recording a matched game: keep extending while `state == "in"` (poll), stop 5-10 min after `post`/`completed`; fall back to generous padding when unmatched. Handle overtime and delays before start. Event log entries ("Extended 22 min for overtime").
D4. **Team follows + team passes.** Follow teams (profile); "Record every game" pass type `team` matching across leagues/channels; Home "Your teams" row; notifications of upcoming games.
D5. **Scores UI (spoiler-safe).** Score bugs on guide cells, sports cards, multiview tiles, player info; global "Hide scores" and per-recording spoiler protection (never show scores for games you're recording and haven't watched).
D6. **Sports hub redesign.** Live now with scores and time left, today by league, matchup tiles with team colors/logos (from provider), "Watch together" (multiview) for simultaneous games, standings later.
**Accept:** unit tests with recorded ESPN JSON fixtures; a real game on the DUO matched and extended correctly (or simulated with fixtures + fake clock).

### Phase E — DVR excellence

E1. Passes UI overhaul (web + Apple): series, team, keyword, category, time/day windows, channel restrictions, keep rules, priority drag-reorder, conflict preview.
E2. Conflict resolver: when tuners are short, suggest alternate airings, show what will be skipped, one-click fixes.
E3. Recording library redesign: shows with art, seasons/episodes, watched state per profile, "continue watching", sort/filter, bulk actions, storage usage per show.
E4. Commercial detection v2: bundle comskip in the image (or build), multi-signal detector (blackdetect + silencedetect + scene + logo mask), per-show learning (repeat-ad fingerprints across recordings), confidence scores, background queue with idle scheduling; skip UX parity with Channels (auto/button/manual, double-forward skip).
E5. Intro/credits detection for series (audio fingerprint across episodes), "skip intro" button, next-episode timing.
E6. Recording health: detect signal drops/CC errors during recording (`mpegts` corrupt packet counts), mark and optionally re-record next airing.
E7. Start-over and record-from-buffer: when starting a recording mid-show, include what's already in the live buffer (ring buffer, Phase G).
E8. Storage manager: per-pass keep rules enforcement, auto-delete watched after N days, low-space policy, recordings on a separate path, move/rename via sidecars.
E9. Export/import: download original TS, share to Plex library folder structure (optional `.nfo`/sidecars).

### Phase F — Player excellence (all clients)

F1. Captions: extract CEA-608/708 server-side into a WebVTT rendition (`ffmpeg ... -f webvtt` via `movie=...[out+subcc]` or `-c:s webvtt` from `eia_608` streams) linked from a master playlist; web caption picker (hls.js subtitle tracks), Apple via `AVMediaSelectionGroup`. Keep A/53 in `copy` renditions for AVPlayer native CC.
F2. Master playlists: publish `index.m3u8` master per session listing the chosen rendition plus alternates (audio groups for AC-3 vs AAC, subtitles), so clients can switch without new watch calls.
F3. Web player: audio track picker, stats overlay (bitrate, dropped frames, buffer, latency, sync drift, rendition, encoder), keyboard help overlay, last-channel toggle, number-pad entry overlay, sleep timer UI, volume memory, theater mode.
F4. Instant channel switching: pre-warm the previous channel's rendition for 20 s (already via RenditionIdle), and if the next channel is on an already-tuned frequency, start its rendition speculatively on hover/focus in the mini guide.
F5. Apple player: custom tvOS info panel tabs (`customInfoViewControllers`: Info, Channels, Stream), contextual actions (Record, Start Over, Multiview), Siri Remote clickpad swipe up/down for channels, frame-rate/range matching verified, iPhone gestures (swipe up/down to change channel, pinch to fill), PiP everywhere, AirPlay.
F6. Group mode UI (shared pause/rewind) on web and Apple: "Watch together" toggle, who's in the room, host controls optional.
F7. Latency modes (Lowest/Balanced/Stable) exposed in player options and settings; per-device default.

### Phase G — Relay and infrastructure depth

G1. Ring buffer: per-mux raw TS ring on disk (configurable 30-240 min), shared by live, recordings (record-from-start), exports, and new renditions (start instantly from buffer).
G2. ABR ladder with aligned segments (one ffmpeg producing multiple outputs via `-map` + `-var_stream_map` or tee) for remote/cellular clients; master playlist with bandwidths; keep independent single-rendition mode for LAN.
G3. LL-HLS: custom Go packager that splits fMP4 fragments into ~330 ms parts (ffmpeg `-frag_duration` / `movflags frag_every_frame` fed via pipe), emits `EXT-X-PART`, `EXT-X-PRELOAD-HINT`, `EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,PART-HOLD-BACK`, supports `_HLS_msn/_HLS_part` blocking reloads. Target glass-to-glass < 3 s on LAN. ADR 0006. Keep classic HLS as default until proven.
G4. HEVC renditions for Apple devices (lower bitrate for same quality; fMP4 required) with hardware encoders (VAAPI `hevc_vaapi`, QSV, VT `hevc_videotoolbox`, NVENC).
G5. jellyfin-ffmpeg in the Docker image (broad HW accel + AC-4 decoder); detect capabilities at startup; diagnostics shows them.
G6. ATSC 3.0: detect FLEX 4K / ATSC 3.0 channels (lineup flags), HEVC copy + AC-4 -> AAC/E-AC-3 transcode, mark DRM channels "Protected" and hide from guide by default.
G7. Multi-device tuner pool: multiple HDHomeRuns, per-device priority, failover, tuner reservation for scheduled recordings (never let live viewing starve a recording; warn viewers).
G8. Metrics and logging: structured `slog`, per-feed stats (bitrate, errors, ffmpeg restarts), Prometheus `/metrics` optional, log viewer in Diagnostics.
G9. Performance: profile Go (pprof) under 4 simultaneous renditions + recording on Unraid; ffmpeg thread tuning; memory caps.

### Phase H — Accounts, profiles, pairing, remote access

H1. Household profiles (name, avatar color, kids flag + max rating), per-profile favorites, watched/progress, follows, saved multiview sets. Profile picker on Apple TV (tvOS user integration) and web.
H2. Device pairing: apps get long-lived tokens (6-digit code or QR from web); LAN trust mode default on; tokens required for non-LAN clients; device list with revoke. Media URLs carry signed short-lived tokens when auth is on.
H3. Remote access: document Tailscale first (zero code); then built-in secure remote (TLS via Let's Encrypt with DNS challenge or a relay option) — ADR 0007; bandwidth-aware defaults (cellular -> 720/540, HEVC).
H4. Downloads for offline (recordings) on iPhone/iPad (AVAssetDownloadTask of HLS, progress Live Activity).

### Phase I — Apple platform integration (make it feel built by Apple)

I1. Top Shelf (tvOS): carousel of live sports/favorites now + continue watching; deep links.
I2. Widgets (iOS): On Now, Your Teams (scores), Recording now, Up next; interactive buttons (Record) via App Intents.
I3. Live Activities + Dynamic Island: recording in progress, followed game in progress (local updates while app runs; APNs broadcast channel support behind a setting when the user supplies an APNs key).
I4. App Intents + Siri + Shortcuts: "Watch channel 9", "Watch the Chiefs game", "Record Jeopardy", "What's on", "Start multiview with ..."; Spotlight indexing of recordings and channels; Control Center control (Watch favorite).
I5. Handoff / "Move to Apple TV": continue the same live moment on another screen (sync makes it seamless) via NSUserActivity + deep link.
I6. SharePlay for remote friends (AVPlayer `playbackCoordinator` with GroupSession).
I7. iPad: sidebar layout, split guide + preview, 4-up multiview, keyboard shortcuts.
I8. Apple Watch: remote control (channel up/down, pause, record), scores for followed teams, recording alerts.
I9. Now Playing framework (iOS 27) adoption where available; CarPlay video browsing (iOS 27, parked) optional.
I10. App icon (Icon Composer, multi-layer Liquid Glass), launch experience, onboarding, App Store assets.

### Phase J — Design system and UX polish (continuous, with a dedicated pass)

J1. Design review of every screen at phone/desktop/TV and iPhone/tvOS: spacing, type scale, color, motion, empty/loading/error states, copy voice. Produce before/after screenshots in `docs/screenshots/`.
J2. Redesign legacy web screens (Library -> Recordings, Schedule -> DVR, Sources/Settings -> Settings with sections) to match Home/Guide quality.
J3. Motion system: View Transitions for route changes, shared-element transitions (guide cell -> player, card -> sheet), spring presets from tokens; honor Reduce Motion.
J4. Accessibility audit: VoiceOver/TalkBack labels, focus order, contrast (WCAG AA), Dynamic Type, captions default setting.
J5. Light mode for iPhone/web (optional, dark default), accent color picker in settings (tokens support it).
J6. Localization readiness (String Catalogs on Apple, message catalog on web).
J7. Brand: name decision (keep "OTA Viewer" as working name; propose 3-5 names with rationale in `docs/brand.md`), logo, app icon, marketing site in `site/` (static, deployable to GitHub Pages).

### Phase K — Quality engineering

K1. Go: fake HDHomeRun (HTTP lineup/status + control protocol shim + TS streamer from a sample file) for integration tests of tuning, busy tuners, multiview planning, recordings.
K2. Web: Playwright e2e (home, guide nav, open player, multiview add/swap, setup wizard) against a server using the fake tuner; visual regression snapshots at 3 sizes; sync measurement test (two pages, assert drift).
K3. Apple: Swift Testing for OTAKit (SyncEngine math with a fake clock, discovery parsing, AppStore), XCUITest smoke for launch/connect/guide/player on both platforms.
K4. Soak test on Unraid: 24 h with a recording schedule + intermittent viewers + multiview; watch memory, fds, ffmpeg counts, tuner releases. Log results in `docs/plan/UNRAID_LOG.md`.
K5. CI: add Playwright job, Apple UI tests (simulator) where feasible, Docker image build + smoke (`scripts/relay-smoke.sh` inside container).

### Phase L — Release and distribution

L1. Versioning (semver tags), CHANGELOG.md, release notes.
L2. GHCR images via release workflow (verify multi-arch), image size budget, SBOM.
L3. Unraid Community Apps repo layout (`ca_profile.xml`, `templates/ota-viewer.xml`, `icon.svg`, LICENSE) — validate with ca.unraid.net scan (blocked on a public repo: record in BLOCKERS if not public).
L4. TestFlight builds for iOS + tvOS (blocked on the user's Apple developer team; prepare everything: bundle ids, entitlements, privacy manifest `PrivacyInfo.xcprivacy`, export compliance, screenshots).
L5. Docs site: install (Docker, Unraid, Mac), apps, multiview, sports, DVR, troubleshooting (diagnostics), FAQ.

---

## 4. Suggested execution order (dependency-aware)

A1 -> A2 -> A3 (deploy early, then redeploy after each phase) -> A5 -> A4 ->
B1 -> B2 -> B3 -> B4 (multiview end to end; redeploy; verify on Unraid) ->
C1 -> C2 -> C3 -> C4 -> C5 -> C6 ->
D1 -> D2 -> D3 -> D4 -> D5 -> D6 ->
F1 -> F2 -> F3 -> F4 -> F5 -> F6 -> F7 ->
E1 ... E9 ->
G5 -> G1 -> G4 -> G6 -> G7 -> G2 -> G3 -> G8 -> G9 ->
I1 ... I10 ->
H1 -> H2 -> H3 -> H4 ->
J (dedicated pass; also continuous) -> K (continuous; dedicated pass at the end) -> L.

After every phase: run all tests, relay smoke, two-screen sync check, deploy to Unraid, smoke it from the LAN, update `PROGRESS.md`, `UNRAID_LOG.md`, docs, and skills.

---

## 5. Definition of spectacular (final acceptance)

- A new user installs from the Unraid template or `docker run`, opens the web UI, finishes setup in under 3 minutes, and sees a full, art-rich guide for every channel.
- Two games play side by side (web, Apple TV, iPad), in sync with each other and with every other screen in the house; audio follows focus; adding a tile never interrupts others.
- Recordings of games end when the game ends; commercials skip reliably; followed teams record automatically.
- The Apple TV app feels native: Top Shelf, focus, remote gestures, info panels, frame-rate matching; the iPhone app has widgets, Live Activities, Siri, PiP, and the mini player.
- Everything runs on Unraid with hardware encoding, survives restarts, and a 24 h soak shows no leaks.
- Docs, tests, CI, and releases are in place; the plan's checklist is fully ticked or each gap is recorded in `BLOCKERS.md` with a clear reason.
