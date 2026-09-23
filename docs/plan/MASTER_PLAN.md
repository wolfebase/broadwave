# Waveguide — Master Plan v2

Written 2026-09-23 after reviewing run 1 (tasks A1–D6, 46 commits). This version supersedes v1 (see git history before `0984064`). Task IDs from v1 are kept so `PROGRESS.md` carries over; new tasks get new IDs (R*, B5, C7–C9, D7–D8, G10–G11, K6, M*).

The product is **Waveguide** (renamed from "OTA Viewer"; see `docs/brand.md`). The repo folder on disk is still `ota viewer`; quote paths and do not rename it.

Read in this order before touching code:

1. `AGENTS.md` (project guide + Lessons learned), then `.cursor/rules/*.mdc`
2. This file, then `docs/plan/PROGRESS.md` (start at "Resume here"), `docs/plan/BLOCKERS.md`, `docs/plan/UNRAID_LOG.md`
3. Skills: `waveguide-dev-loop`, `waveguide-media-pipeline`, `waveguide-apple`, `waveguide-multiview` (project, `.cursor/skills/`), `waveguide-unraid` (personal, `~/.cursor/skills/`)
4. `docs/architecture.md`, `docs/decisions/0001-0005`, `docs/research.md`

---

## 0. How to run this plan (read this section twice)

### 0.1 Nonstop protocol

Run 1 did good work but ended its turn after almost every phase; the user had to type "continue" eight times. That is the main thing to fix.

1. **Start with a goal.** First action: call `GetDynamicTools {"namespace":"cursor","toolName":"CreateGoal"}`, then `CallDynamicTool` `cursor/CreateGoal` with the objective: "Complete every task in docs/plan/PROGRESS.md for Waveguide (tick it or record a real blocker in BLOCKERS.md), verifying, committing, pushing, and deploying as the plan says." The user explicitly asks for this goal. Only call `UpdateGoal complete` when every line in `PROGRESS.md` is ticked or blocked.
2. **Never end your turn to report progress.** A finished task or phase is not a stopping point. Progress goes into `PROGRESS.md`, commit messages, and `UNRAID_LOG.md`, not into a chat summary. Write the next tool call instead of a recap.
3. **Background notifications are not stops.** When a background shell or subagent finishes, read its result, act on it, and continue with the current task in the same turn. Do not summarize it to the user.
4. **The only reasons to end a turn:** the plan is complete; every remaining task is blocked and recorded in `BLOCKERS.md`; or an irreversible action needs the user (App Store submission, deleting user data, spending money).
5. **Protect your context.** Delegate bulky work to subagents (Task tool) with self-contained prompts (paths, commands, acceptance, what to return): research (`docs-researcher`/`generalPurpose`), code exploration (`explore`), browser verification at three sizes (`browser-use`), Apple simulator screenshot runs, and code review (`code-reviewer`). Run independent subagents in parallel when they touch disjoint directories (for example `server/` vs `apple/`). Only the main agent commits. Never let two agents edit the same files at once.
6. **Crash-safe resume.** The top of `PROGRESS.md` has a "Resume here" block: the current task ID, what is half done, and the next command. Update it at the start of every task. If the session dies, a fresh agent reads it and continues without rediscovery.
7. **No silent deferrals.** If a task's acceptance can't be met in full, finish what can be, then add a new explicit task line in `PROGRESS.md` for the remainder, placed in the execution order (section 4). Prose notes alone are not enough (run 1 left the mosaic, the 18 empty channels, and TestFlight uploads only as notes).

### 0.2 Per-task loop

1. Update "Resume here". Read the relevant code and skill.
2. Implement. Match the surrounding style (`AGENTS.md` working agreements, copy voice).
3. `make check` (Go tests, web typecheck, eslint, swiftlint, swiftformat — the same gates as CI). Plus the task's gates: `scripts/relay-smoke.sh` for relay changes; the two-screen sync measurement for anything under `internal/live`, `web/src/lib/sync.ts`, or `OTAKit/SyncEngine.swift`; browser checks at phone (390×844), desktop (1440×900), and TV (1920×1080) for web UI; `make apple` + a simulator screenshot for Apple UI.
4. One commit per task that includes its `PROGRESS.md` tick (no separate "Tick X" commits). Message: what changed and why.
5. `git push`, then check the previous push's CI (`gh run list -L 3`). **Red CI is stop-the-line:** fix it before the next task. Run 1 left `main` red from D3 to D6 without noticing (swiftlint, plus an iOS 27-only API). CI builds with **Xcode 26.6** while this Mac has **Xcode 27** (Swift 6.4): guard iOS/tvOS 27 APIs with `#if compiler(>=6.4)` around the `#available` check.

### 0.3 Per-phase loop

At the end of every phase (R, C, K1, G1, F, E, D-extras, B5, I, G, H, J, K, L):

1. Add a `CHANGELOG.md` entry and tag `v0.N.0` (next minor). The release workflow publishes `ghcr.io/wolfebase/waveguide:<tag>` for amd64 and arm64.
2. Deploy to Unraid with `MODE=ghcr` (skill `waveguide-unraid`; large SSH uploads over the tunnel drop, so pull from GHCR). Smoke it: health, version, a channel plays, two tabs sync, the phase's features work. Log it in `UNRAID_LOG.md`. Run 1 deployed only once (A3); Unraid still runs v0.1.0.
3. If Apple code changed and A7 is unblocked, run `scripts/testflight.sh`.
4. Update `docs/architecture.md`, ADRs, `api/openapi.yaml`, `AGENTS.md` lessons, and skills where behavior changed.

### 0.4 Shared tuner etiquette

The Mac dev server and the Unraid server share one HDHomeRun CONNECT DUO (2 tuners). Unraid is the household's real DVR.

- Before any test that tunes, check Unraid: `curl -s http://192.168.1.2:8477/api/v1/diagnostics` (tuners in use, active recordings) and `/api/v1/schedule` (recordings in the next 30 minutes). If a recording is on or due, test with at most one tuner, or use the relay smoke test or the fake tuner (K1).
- Never cause a scheduled recording to fail. Never hold a tuner after a test; stop dev servers you started (`pkill -9 -x otav`) and leave at most one running.

### 0.5 Secrets and accounts

The user allows reading other projects under `~/Projects` and `~/.blitz` for keys and credentials. Never commit or log them. The App Store Connect API key is in `~/.blitz/asc-credentials.json`; the `asc` CLI is `~/.blitz/bin/asc`.

---

## 1. Where it stands (verified 2026-09-23)

| Thing | Value |
| --- | --- |
| GitHub | `wolfebase/waveguide` (public; `twolfekc` is not a GitHub account, see BLOCKERS) |
| Images | `ghcr.io/wolfebase/waveguide` (v0.1.0, amd64 + arm64, public) |
| Apple | team `D4MC63SS36`, bundle `com.wolfeup.waveguide` (iOS + tvOS), App Group `group.com.wolfeup.waveguide` (not yet on profiles) |
| Unraid | TUS `root@192.168.1.2`, container `Waveguide`, `ghcr.io/wolfebase/waveguide:0.1.0`, host network, VAAPI, appdata `/mnt/cache/appdata/waveguide`, recordings `/mnt/user/media/ota-recordings` |
| Tuner | CONNECT DUO `192.168.1.252`, 2 tuners, 27 channels, ATSC 1.0 |
| Network | This Mac reaches TUS through `utun4`, so Bonjour from TUS is not visible here. Check Bonjour on TUS itself (`avahi-browse -rt _waveguide._tcp`); point simulators at `http://192.168.1.2:8477` by address. |

**Done in run 1:** setup wizard and first-run detection (A1), SiliconDust refresh cadence (A2), Unraid migration and deploy (A3), lint in CI and web on `/api/v1` (A4), crash and SIGTERM recovery (A5), GitHub + GHCR (A6), Apple signing and archive (A7, upload blocked), multiview on web and Apple with tile renditions, tuner plan, and a shared room (B1–B4), guide matching, extra sources, artwork, rich programs, guide UX, and search (C1–C6), ESPN scores, game matching, game-aware recording, team passes, spoiler-safe scores, and the sports hub (D1–D6).

**Found in review (v2 fixes these):**

1. Turn-ending after each phase (section 0.1).
2. CI red from D3 to D6; fixed in `e411da1` and `0984064`. `make check` now mirrors CI.
3. Unraid not redeployed since A3.
4. **Guide coverage is the biggest product gap:** 18 of 27 channels say "No listing" (all of 14.x, plus 43.3, 46.7, and others), and the guide is only 2 days deep. The tuner guide cannot grow. → C7 (read the guide from the broadcast itself).
5. **Web boot:** every page load shows a full-screen "Finding your tuner…" until every dataset has loaded, including a 258 KB airings payload. The API answers in under 3 ms, so this is a client waterfall. JSON is not compressed. → R3.
6. **Artwork:** the Home hero stretches a small poster across the full width, so it looks blurry. → R4.
7. Multiview mosaic deferred (ADR 0004). → B5.
8. The 14.x frequency is unknown, so the multiview plan treats each 14.x subchannel as its own tuner. → C7 learns frequencies.
9. TestFlight upload blocked on the App Store Connect app record (needs one Apple ID login). → A7.

---

## 2. Research digest (sources in `docs/research.md`; re-verify version-sensitive facts)

- **ATSC PSIP (A/65) — the guide inside the broadcast.** Base PID `0x1FFB` carries MGT (`0xC7`), TVCT (`0xC8`), CVCT (`0xC9`), RRT (`0xCA`), and STT (`0xCD`). The MGT lists PIDs for EIT-0…EIT-127 (table types `0x0100+k`) and channel ETTs and EIT ETTs (`0x0200+k`). EIT (`0xCB`) sections carry events per `source_id` (the VCT maps `source_id` to major.minor); each EIT-k covers a 3-hour block, so EIT-0..3 (required) is 12 hours and stations can carry up to 16 days. ETT (`0xCC`) holds descriptions. Times are GPS seconds since 1980-01-06 minus the STT's GPS-UTC offset. Strings are `multiple_string_structure`, sometimes Huffman-compressed (A/65 Annex C tables, compression types 1 and 2). The genre descriptor (`0xAB`) gives categories, useful for sports matching. MythTV and TVHeadend both harvest EIT this way; Channels DVR does not.
- **Apple multiview (tvOS/iOS 26+):** `AVRoutingPlaybackArbiter` preferred participants route AirPlay and non-mixable audio to the focused tile; `networkResourcePriority` high for the focused tile. `AVPlaybackCoordinationMedium` is not used (it forces one timeline; see ADR 0004).
- **Channels DVR parity:** multiview up to 4 (live only, no buffer), intro/credits detection, Enhanced Commercial Detection (fingerprinting, idle backfill), commercial skip modes (auto, button, manual, double-forward inside a break), Personal Sections, Theater Mode. **AIRDVR** has side-by-side multiview and live scores; iOS app pending. We win with sync across screens, multiview with a buffer and sync, the broadcast-harvested guide, modern Apple-native design, open exports, and no subscription.
- **LL-HLS:** ffmpeg does not emit `EXT-X-PART`/`EXT-X-PRELOAD-HINT`; true LL-HLS needs our own packager (we already parse fMP4) with blocking reload (`_HLS_msn`, `_HLS_part`, `CAN-BLOCK-RELOAD=YES`). hls.js needs `lowLatencyMode` and a server that really blocks.
- **hls.js multiview:** 3-4 instances per page; cap back buffer per tile (20-30 s); `capLevelToPlayerSize`; ManagedMediaSource on iPhone Safari.
- **Captions:** OTA carries CEA-608/708 in the video SEI (A/53). VideoToolbox transcodes must run with `-a53cc 0`, which drops them, so captions must come from the source TS (a separate lightweight extractor reading the mux) into WebVTT aligned by PDT. `copy` renditions keep A/53 for AVPlayer's native CC.
- **ATSC 3.0:** HEVC + AC-4; jellyfin-ffmpeg has an AC-4 decoder; DRM stations cannot be decrypted, so show them as "Protected".
- **Live Activities:** broadcast push needs the developer's APNs `.p8`; without it, update locally while the app runs.
- **Top Shelf:** `TVTopShelfContentProvider` with carousel content; Swift 6 needs `@preconcurrency import TVServices`.
- **Unraid Community Apps:** public repo, OSI license, `ca_profile.xml`, template XMLs, real `icon.svg`, validate at ca.unraid.net/submit/new.
- **Commercial detection:** multi-signal (black frames, silence, scene cuts, logo presence) plus fingerprinting of repeated ads across recordings beats comskip alone.

---

## 3. Phases

Each task: **Do** / **Accept**. Tick it in `PROGRESS.md` in the same commit.

### Phase R — Repair and consolidate (first)

R1. **CI green and kept green.** Confirm run for `0984064` is green on all four jobs (`gh run list`). If Apple fails again, fix it. Add a short "CI" section to skill `waveguide-dev-loop` (what each job runs, Xcode 26 vs 27, `make check`). **Accept:** green `main`; skill updated.

R2. **Deploy A–D to Unraid.** Tag `v0.2.0` with a `CHANGELOG.md` (start it now; L1 expands it). Deploy with `MODE=ghcr`. On the LAN: multiview 2-up from Unraid (VAAPI tiles; record CPU and GPU use from `docker stats` and `intel_gpu_top` if present), search, sports hub, team pass, spoiler-safe scores, guide matching. iPhone and Apple TV simulators connect to `http://192.168.1.2:8477`. **Accept:** `UNRAID_LOG.md` entry with version, checks, and numbers.

R3. **Instant boot and lean data.**
- Web: render the shell immediately; show cached data from the last session (IndexedDB) and revalidate in the background; per-section skeletons. The full-screen boot screen appears only on the very first load. Keep first-run detection for setup.
- Load the guide by window: `GET /api/v1/airings?from=&to=` (and optional `channels=`). First paint needs now−30 min to +4 h; prefetch the rest when idle. Keep the full-range call for compatibility and update `api/openapi.yaml`.
- Server: gzip (or brotli) for JSON; precompressed `.br`/`.gz` static assets from the Vite build, served when accepted; `ETag` + `304` on channels, airings, and settings.
- Apple: cache the last snapshot (Codable in Caches) and show it at launch while refreshing.
- **Accept:** warm reload on LAN shows real content in < 300 ms and the guide is interactive in < 800 ms cold (CDP `Performance.getMetrics` and `performance.timing`, desktop and phone); airings transfer < 60 KB compressed for the first window; Go tests for windowing and compression; Lighthouse performance ≥ 90 on Home.

R4. **Artwork that never looks cheap.** The API returns image size and aspect with each image (probe once when caching in `/media/art`). Clients pick a layout by what the art can support: a full-bleed hero only with landscape art ≥ 1280 px wide; otherwise a composed hero (crisp poster or logo at native size over a blurred, darkened backdrop of the same art, or a live frame from R5). Never upscale an image beyond 1.25× its native size anywhere. Prefer the largest landscape icon when XMLTV lists several. **Accept:** Home, program sheet, sports, and recordings screenshots at three sizes with no blurry art; a unit test for layout choice.

R5. **Live preview frames.** For every frequency already tuned (a viewer, a recording, an export, or a C7 scan), grab a keyframe for each program in the mux every 60 s (`-skip_frame nokey`, scaled 480 w and 1280 w JPEG) into `work/frames/`. Never tune just for a frame. `GET /api/v1/channels/{id}/frame` with `Last-Modified`; frames older than 10 minutes are marked stale. Use frames on guide rows with no listing, Home "On now" cards, the multiview picker, the hero fallback (R4), and later Top Shelf (I1). **Accept:** frames appear for all subchannels of a tuned mux; CPU < 3% of one core per mux on Unraid; Go test for scheduling and staleness.

R6. **Apple review of B–D.** Screenshot every Apple screen that B–D touched (multiview 2-up/quad, guide, search, sports, Your teams, score bugs) on iPhone 17 Pro, iPad Pro 13", and Apple TV 4K, including Dynamic Type XL on iPhone and focus states on tvOS. Fix defects that take under an hour; add the rest as J1 sub-items. **Accept:** screenshots in `docs/screenshots/r6-*`; defects fixed or listed.

R7. **Code review of run 1.** Run a `code-reviewer` subagent over `8f91dfc..HEAD` (server, web, Apple): tuner leaks on error paths, goroutine leaks, missing `ctx` cancellation, SQL without indexes on hot paths, unbounded memory, race conditions (`go test -race ./server/...`), missing tests, copy-voice violations. Fix every real finding. **Accept:** `go test -race` clean; findings and fixes listed in the commit message.

### Phase C (continued) — Fill every channel

C7. **Guide from the broadcast (PSIP EIT harvesting).** Package `internal/psip`.
- First verify what the relay reads: capture 30 s of the mux the relay already tunes and list PIDs. If it is the full mux, PSIP (`0x1FFB` and the MGT-listed EIT/ETT PIDs) is already there. If the tuner URL filters to one program, switch that feed to the full-mux form and keep program filtering in our demux.
- Parser: MGT, TVCT/CVCT, STT, EIT, ETT, with Huffman string decoding and the genre descriptor. Table-driven tests on a captured sample filtered to PSIP PIDs only (keep `testdata` small: a few hundred KB).
- **Passive harvesting:** whenever a frequency is tuned for any reason, feed its PSIP to the harvester at no tuner cost.
- **Idle scan:** when no tuner is in use and no recording starts within 30 minutes, tune each frequency whose listings are missing or ending within 12 h, dwell 30-45 s, release (`/tunerN/channel none`). Preempt instantly: a watch or recording request cancels the scan before it tunes. At most one scan per frequency every 6 h, none between the user's quiet hours if set. It also learns every channel's frequency (fixes the 14.x note in B1).
- **Merge:** PSIP becomes a guide source with the lowest priority (user XMLTV / Schedules Direct > SiliconDust > PSIP), filling only channels and time ranges the others leave empty. Match by major.minor. Normalize ALL-CAPS titles to title case for display (keep the original). ETT text becomes the description. Genre feeds categories and sports matching (D2).
- Diagnostics shows per-channel guide source and depth, and the last scan per frequency. ADR 0006 (broadcast guide).
- **Accept:** ≥ 25 of 27 channels show a current and next listing on the real lineup (list any exceptions and why); guide depth per channel logged; a test proves a watch request preempts a scan; zero scan tunes while Unraid or the dev server has a viewer or recording.

C8. **Guide depth and freshness.** With C7, show honest depth ("Listings through Thursday"); the guide scrolls as far as any source goes; when a channel's data ends, its row says so instead of "No listing". Program sheet shows which source a listing came from (in the Stream Info style, not on every cell). **Accept:** screenshots; test for merge boundaries.

C9. **Antenna and signal tools.** A Settings > Tuners screen: per-channel signal strength, SNR quality, and symbol quality from `/tunerN/status` for channels on a tuned frequency; a "Check all channels" run that uses idle tuners the same way as C7 (preemptible). Show a simple verdict per channel (Great / OK / Weak / Lost) and a tip for weak ones in the copy voice. **Accept:** real readings from the DUO; the run yields to viewers.

### Phase K1 — Fake tuner early (before the heavy DVR and player work)

K1. **Fake HDHomeRun.** `internal/hdhr/fake` (or `internal/fakehdhr`): HTTP `discover.json`, `lineup.json`, `lineup_status.json`, a UDP control-protocol shim (`/tunerN/channel`, `/tunerN/status`, busy 805), and a TS streamer that loops a sample file with PSIP. Integration tests for tuning, frequency sharing, busy tuners, multiview planning, idle-scan preemption (C7), recordings start/stop/extend, restart recovery. `scripts/relay-smoke.sh` gains a mode that uses the fake instead of a real tuner, so CI can run it. **Accept:** integration tests in CI; relay smoke runs in CI with the fake.

### Phase G1 — Ring buffer (unblocks start-over, instant switching, and catch-up)

G1. **Ring buffer.** Per-mux raw TS ring on disk (default 60 min, configurable 30-240), shared by live renditions, recordings (record from the start of a show already in the buffer), exports, and new renditions (start instantly from the buffer instead of waiting for the tuner). Disk budget and cleanup honor the storage settings. **Accept:** Go tests with the fake tuner; a recording started 10 minutes into a show includes those 10 minutes; memory flat over 1 h.

### Phase F — Player excellence (all clients)

F1. **Captions.** Extract CEA-608/708 from the source TS into a WebVTT rendition aligned to the feed's PDT timeline, published in the master playlist (F2). Web caption picker (hls.js subtitle tracks, styling from settings); Apple via `AVMediaSelectionGroup` (the `copy` rendition keeps A/53 for native CC; transcoded renditions use the WebVTT track). **Accept:** captions on a real channel in web, iPhone, and Apple TV, in sync (±200 ms); a "Captions on by default" setting.

F2. **Master playlists.** `index.m3u8` per session listing the chosen rendition plus alternates (audio groups AC-3 vs AAC, subtitles), so clients switch without new watch calls. **Accept:** AVPlayer and hls.js switch audio and captions without a stall.

F3. **Web player extras.** Audio track picker, stats overlay (bitrate, dropped frames, buffer, latency, sync drift, rendition, encoder), keyboard help (`?`), last-channel toggle, number-pad entry, sleep timer, volume memory, theater mode. **Accept:** keyboard-only walkthrough; screenshots at three sizes.

F4. **Instant channel switching.** Keep the previous channel's rendition warm for 20 s; start renditions speculatively for the focused row in the mini guide when its frequency is already tuned; with G1, start from the buffer. Report time to first frame in Diagnostics. **Accept:** same-frequency switch < 1.0 s, already-tuned < 1.5 s, new tune < 3 s, measured on the DUO and recorded in `docs/dev-lab.md`.

F5. **Apple player.** tvOS info panel tabs (`customInfoViewControllers`: Info, Channels, Stream), contextual actions (Record, Start over, Multiview), clickpad swipe for channel up/down, frame-rate and dynamic-range matching verified, iPhone swipe to change channel and pinch to fill, PiP everywhere, AirPlay. **Accept:** tvOS and iPhone screenshots of each panel; frame-rate matching confirmed with a 59.94 and a 29.97 channel.

F6. **Group mode UI.** "Watch together" on web and Apple: who's in the room, shared pause/rewind, leave. **Accept:** two browsers plus one simulator pause and seek together.

F7. **Latency modes.** Lowest / Balanced / Stable in player options and settings, with a per-device default. **Accept:** measured latency per mode in `docs/dev-lab.md`.

### Phase E — DVR excellence

E1. Passes UI overhaul (web + Apple): series, team, keyword, category, time/day windows, channel limits, keep rules, priority drag-reorder, conflict preview.
E2. Conflict resolver: when tuners are short, suggest other airings, show what will be skipped, one-click fixes; never let live viewing starve a scheduled recording (warn the viewer first).
E3. Recording library redesign: shows with art (R4), seasons and episodes, watched state, continue watching, sort and filter, bulk actions, storage per show.
E4. Commercial detection v2: comskip in the image, plus a multi-signal detector (blackdetect + silencedetect + scene cuts + logo mask) with confidence scores, repeated-ad fingerprints across recordings, and an idle-time queue. Skip UX at Channels parity (auto, button, manual; double-forward inside a break skips it).
E5. Intro and credits detection for series (audio fingerprint across episodes); "Skip intro" and next-episode timing.
E6. Recording health: count TS continuity errors and signal drops during recording; mark the recording and offer to re-record the next airing.
E7. Start over and record from the buffer (uses G1): start-over on any show whose start is in the buffer; recordings started late include what the buffer holds.
E8. Storage manager: keep rules, auto-delete watched after N days, low-space policy, recordings on a separate path, move and rename via sidecars.
E9. Export and import: download the original TS; a Plex/Jellyfin-friendly folder layout with optional `.nfo`.
**Accept (phase):** each task has Go tests with the fake tuner (K1), web and Apple screenshots, and one real recording on Unraid exercising E2, E4, E6, and E7 (logged).

### Phase D (continued) — Sports that beat cable

D7. **Game Switcher.** In multiview with 2+ games, an optional auto mode moves the big tile and the audio to the game that matters most right now: a scoring chance (ESPN `situation.isRedZone` in football, power plays, late close games), a lead change, or the final minutes of a close game. A short banner says why ("Red zone: KC at LV"). Manual focus always wins for 2 minutes. **Accept:** tests with recorded ESPN live JSON and a fake clock; web and tvOS screenshots.

D8. **Game alerts.** Followed-team and close-game alerts: in the web app (toast), iPhone notifications while the app is active (and Live Activities after I3), with a one-tap Watch. Spoiler-safe: no score in an alert for a game you are recording and have not watched. **Accept:** fixture-driven tests; screenshots.

### Phase B (continued)

B5. **Mosaic rendition.** One ffmpeg `xstack` output combining 2-4 channels (`mosaic:<ids>:<layout>`) for AirPlay targets, older devices, low bandwidth, and as a virtual channel in exports (Plex/Jellyfin see "Multiview"). Same `-copyts`/CMAF/PDT invariants; audio from the chosen tile. **Accept:** relay smoke covers a 2-up mosaic; a mosaic plays in Safari and via the M3U export.

### Phase I — Apple platform integration

Start this phase by retrying A7 (app record + upload). Every Apple task below ends with a TestFlight build once A7 works.

I1. Top Shelf (tvOS): live sports and favorites now (with R5 frames) and continue watching; deep links.
I2. Widgets (iOS): On now, Your teams (scores), Recording now, Up next; interactive Record via App Intents. Needs the App Group on the profiles.
I3. Live Activities and Dynamic Island: recording in progress, followed game in progress (local updates; APNs broadcast behind a setting when the user supplies a key).
I4. App Intents, Siri, Shortcuts: "Watch channel 9", "Watch the Chiefs game", "Record Jeopardy", "What's on", "Start multiview with …"; Spotlight for recordings and channels; a Control Center control.
I5. Handoff / Move to Apple TV: continue the same live moment on another screen (sync makes it seamless).
I6. SharePlay for remote friends (GroupSession + playback coordinator).
I7. iPad: sidebar layout, guide beside a live preview, 4-up multiview, keyboard shortcuts.
I8. Apple Watch: remote (channel up/down, pause, record), followed-team scores, recording alerts.
I9. Now Playing adoption; CarPlay optional.
I10. App icon (Icon Composer, layered Liquid Glass, from J7), launch, onboarding, App Store assets.
**Accept (phase):** each feature screenshotted on the simulator, builds green on Xcode 26 and 27, TestFlight build uploaded (if A7 is unblocked).

### Phase G — Relay and infrastructure depth

G5. jellyfin-ffmpeg in the Docker image (broad hardware acceleration + AC-4); detect capabilities at startup; Diagnostics shows them.
G4. HEVC renditions for Apple devices with hardware encoders (VAAPI `hevc_vaapi`, QSV, VideoToolbox, NVENC).
G6. ATSC 3.0: detect 3.0 channels, HEVC copy + AC-4 to AAC/E-AC-3, mark DRM channels "Protected" and hide them by default. Test against sample files.
G7. Multi-device tuner pool: several HDHomeRuns, per-device priority, failover, reservations for scheduled recordings.
G2. ABR ladder with aligned segments for remote and cellular clients (one ffmpeg, several outputs); keep independent single renditions on the LAN.
G3. LL-HLS packager (ADR 0007): parts ~330 ms, `EXT-X-PART`, `EXT-X-PRELOAD-HINT`, blocking reload. Target glass-to-glass < 3 s on the LAN. Classic HLS stays default until it is proven.
G8. Metrics and logging: `slog` everywhere, per-feed stats, optional Prometheus `/metrics`, a log viewer in Diagnostics.
G9. Performance on Unraid: pprof under 4 renditions + a recording + a mosaic; ffmpeg thread tuning; memory caps.
G10. HDHomeRun firmware and health surface (read-only: version, tuner status, lock; never install firmware from the app).
G11. Verify exports in the real apps the user runs (check TUS for Plex, Jellyfin, or Channels containers): lineup, guide, and a stream through the HDHomeRun emulator. Log results; fix gaps.

### Phase H — Accounts, profiles, pairing, remote access

H1. Household profiles (name, color, kids flag + max rating), per-profile favorites, watched state, follows, saved multiview sets; profile picker on Apple TV and web.
H2. Device pairing: long-lived tokens (6-digit code or QR); trusted-LAN mode on by default; tokens required off the LAN; device list with revoke; signed short-lived media URLs when auth is on.
H3. Remote access: Tailscale docs first, then built-in (ADR 0008); bandwidth-aware defaults off the LAN (720/540, HEVC).
H4. Offline downloads of recordings on iPhone and iPad (`AVAssetDownloadTask`), with a progress Live Activity.

### Phase J — Design system and UX polish

J1. Full design review of every screen at phone, desktop, TV and on iPhone, iPad, Apple TV: spacing, type scale, color, motion, empty/loading/error states, copy voice; include R6 leftovers. Before/after screenshots in `docs/screenshots/`. Bar: it should look like Apple made it (TV app, Sports app) and feel faster than YouTube TV.
J2. Redesign the remaining legacy web screens (Recordings, Schedule/DVR, Settings with sections) to Home/Guide quality.
J3. Motion system: View Transitions for routes, shared-element transitions (guide cell to player, card to sheet), springs from tokens; Reduce Motion respected.
J4. Accessibility audit: labels, focus order, WCAG AA contrast, Dynamic Type, captions default.
J5. Light mode for iPhone and web (dark stays default); accent color picker.
J6. Localization readiness (String Catalogs on Apple, message catalog on web).
J7. Brand: logo, app icon (feeds I10), `icon.svg` for Unraid (feeds L3), marketing site in `site/` (static, GitHub Pages).

### Phase K — Quality engineering (continuous; dedicated pass here)

K2. Playwright e2e against the fake tuner (home, guide, player, multiview add/swap, setup wizard, search, sports) with visual snapshots at three sizes and a two-page sync assertion.
K3. Apple: Swift Testing for OTAKit (SyncEngine with a fake clock, discovery parsing, AppStore, caching from R3); XCUITest smoke for launch, connect, guide, player, multiview on iOS and tvOS.
K4. 24 h soak on Unraid: a recording schedule, intermittent viewers, multiview, idle scans; watch memory, file descriptors, ffmpeg count, tuner release. Log results.
K5. CI: Playwright job, Apple UI tests where feasible, container smoke (`relay-smoke.sh` in the image with the fake tuner).
K6. Performance budgets in CI: web bundle size, Home first paint (Playwright trace), API p95 under the fake tuner.

### Phase L — Release and distribution

L1. Semver tags per phase (already started in R2), `CHANGELOG.md`, release notes on GitHub.
L2. GHCR images verified multi-arch; image size budget; SBOM.
L3. **Unraid Community Apps.** Research the current process (ca.unraid.net/submit/help, the builder guide, the starter repository, the XML reference, recent forum guidance on requirements, icons, support threads, updates). Decide where templates live (this repo or a `wolfebase/unraid-templates` repo) per CA guidance. Make the template excellent (host network explained, `/dev/dri` optional, NVIDIA notes, copy-voice descriptions, WebUI, Support/Project, `Changes`, the J7 icon). Install it on TUS from its public URL the way a stranger would; Validate + Scan clean; submit; create the support thread if required. Record status in `BLOCKERS.md` (review is asynchronous) and keep working.
L4. **TestFlight and App Store readiness.** Final icon, screenshots for iPhone, iPad, Apple TV, description and keywords, privacy labels (skill `asc-privacy-nutrition-labels`), review notes explaining local-network use; submit for external TestFlight review. The App Store submission itself waits for the user.
L5. Docs site: install (Docker, Unraid, Mac), apps, multiview, sports, DVR, antenna tools, troubleshooting, FAQ.

### Phase M — Category-best extras (after L; keep going)

M1. Catch-up: rewind any channel you have had tuned up to the ring window; "Start over" wherever the buffer covers the start.
M2. Commercial skip while behind live (detect breaks in the live buffer and offer Skip).
M3. "Which channel has the game?": search a team or league from anywhere (Siri, search, widgets) and jump to the right channel, or to multiview when several games are on.
M4. Smart favorites: learn what the household watches by time of day to order Home and the mini guide (on-device, per profile, never sent anywhere).
M5. Anything the J1 review or K4 soak surfaced that makes the product better than Channels DVR and YouTube TV. Add tasks, then do them.

---

## 4. Execution order (dependency-aware)

R1 → R2 → R3 → R4 → R5 → R6 → R7 →
C7 → C8 → C9 → (tag + deploy) →
K1 → G1 → (tag + deploy) →
F1 → F2 → F3 → F4 → F5 → F6 → F7 → (tag + deploy + TestFlight) →
E1 … E9 → (tag + deploy) →
D7 → D8 → B5 → (tag + deploy) →
A7 retry → I1 … I10 → (tag + deploy + TestFlight) →
G5 → G4 → G6 → G7 → G2 → G3 → G8 → G9 → G10 → G11 → (tag + deploy) →
H1 → H2 → H3 → H4 → (tag + deploy + TestFlight) →
J1 … J7 → K2 … K6 → L1 … L5 → M1 … M5.

A7 is retried at the start of every phase: if `~/.blitz/bin/asc web auth status` (or an API call) shows a valid session, create the app records, attach the App Group to the profiles, and upload.

---

## 5. Definition of spectacular (final acceptance)

- A new user installs from the Unraid template or `docker run`, finishes setup in under 3 minutes, and sees a full, art-rich guide for every channel their antenna gets, including channels no online guide lists.
- The app opens instantly with real content; channel changes feel instant.
- Two games play side by side (web, Apple TV, iPad), in sync with each other and every other screen; audio follows focus; Game Switcher catches the big moments.
- Recordings of games end when the game ends; commercials skip reliably; followed teams record automatically.
- The Apple TV app feels native (Top Shelf, focus, remote gestures, info panels, frame-rate matching); the iPhone app has widgets, Live Activities, Siri, PiP, and the mini player.
- Everything runs on Unraid with hardware encoding, survives restarts, and a 24 h soak shows no leaks.
- `main` is green; images are published per phase; iOS and tvOS builds are in TestFlight; the Community Apps submission is in (or approved).
- Every line in `PROGRESS.md` is ticked or recorded in `BLOCKERS.md` with a clear reason.
