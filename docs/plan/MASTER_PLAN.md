# Broadwave — Master Plan v2

Written 2026-09-23 after reviewing run 1 (tasks A1–D6, 46 commits). This version supersedes v1 (see git history before `0984064`). Task IDs from v1 are kept so `PROGRESS.md` carries over; new tasks get new IDs (R*, S*, P*, N*, B5, C7–C9, D7–D8, G10, K6, M*). Revised the same day to add Phase S (every source, auto-found), Phase P (server and Apple apps as one), and Phase N (every other screen).

The product is **Broadwave** (see `docs/brand.md`). The repo is `/Users/tyler/Projects/active/broadwave`.

Read in this order before touching code:

1. `AGENTS.md` (project guide + Lessons learned), then `.cursor/rules/*.mdc`
2. This file, then `docs/plan/PROGRESS.md` (start at "Resume here"), `docs/plan/BLOCKERS.md`, `docs/plan/UNRAID_LOG.md`
3. Skills: `broadwave-dev-loop`, `broadwave-media-pipeline`, `broadwave-apple`, `broadwave-multiview`, `broadwave-sources` (project, `.cursor/skills/`), `broadwave-unraid` (personal, `~/.cursor/skills/`)
4. `docs/architecture.md`, `docs/decisions/0001-0005`, `docs/research.md`

---

## 0. How to run this plan (read this section twice)

### 0.1 Nonstop protocol

Run 1 did good work but ended its turn after almost every phase; the user had to type "continue" eight times. That is the main thing to fix.

1. **Start with a goal (interactive sessions only).** In the Cursor TUI, first call `GetDynamicTools {"namespace":"cursor","toolName":"CreateGoal"}`, then `CallDynamicTool` `cursor/CreateGoal` with the objective: "Complete every task in docs/plan/PROGRESS.md for Broadwave (tick it or record a real blocker in BLOCKERS.md), verifying, committing, pushing, and deploying as the plan says." Headless rounds from `scripts/agent-loop.sh` skip this; the loop is the goal.
2. **Never end your turn to report progress.** A finished task or phase is not a stopping point. Progress goes into `PROGRESS.md`, commit messages, and `UNRAID_LOG.md`, not into a chat summary. Write the next tool call instead of a recap.
3. **Background notifications are not stops.** When a background shell or subagent finishes, read its result, act on it, and continue with the current task in the same turn. Do not summarize it to the user.
4. **The only reasons to end a turn:** the plan is complete; every remaining task is blocked and recorded in `BLOCKERS.md`; or an irreversible action needs the user (App Store submission, deleting user data, spending money).
5. **Protect your context.** Delegate bulky work to subagents (Task tool) with self-contained prompts (paths, commands, acceptance, what to return): research (`docs-researcher`/`generalPurpose`), code exploration (`explore`), browser verification at three sizes (`generalPurpose` running `playwright-cli open --browser=chrome`; there is no `browser-use` subagent), Apple simulator screenshot runs, and code review (`code-reviewer`). Run independent subagents in parallel when they touch disjoint directories (for example `server/` vs `apple/`). Only the main agent commits. Never let two agents edit the same files at once.
6. **Crash-safe resume.** The top of `PROGRESS.md` has a "Resume here" block: the current task ID, what is half done, and the next command. Update it at the start of every task. If the session dies, a fresh agent reads it and continues without rediscovery.
7. **No silent deferrals.** If a task's acceptance can't be met in full, finish what can be, then add a new explicit task line in `PROGRESS.md` for the remainder, placed in the execution order (section 4). Prose notes alone are not enough (run 1 left the mosaic, the 18 empty channels, and TestFlight uploads only as notes).

### 0.1a Review 2 (2026-09-24, after v0.5.0): what changes from here

A review of R1–P1 found good, real work (CI green every commit, real containers tested, three clean Unraid deploys) and five habits to fix. These override anything below that disagrees.

1. **Every tick carries its evidence.** A `PROGRESS.md` tick lists each **Accept** bullet with the proof: a number (ms, KB, %, count), a test name, a screenshot path, or an `UNRAID_LOG` entry. If a bullet was not met, say so on the line and add a follow-up task line (0.1 rule 7). The commit body says what was verified and how. Phase S ticked ten tasks in two hours, and most commit bodies are one line, so nobody can tell which acceptance criteria actually ran.
2. **Apple is the flagship, so Apple depth is the bar.** When a task names iPhone, iPad, or Apple TV, the Apple part must do what the web part does. A screen that only shows text where the web version has actions is a placeholder, not done. S8's Apple setup is a placeholder: it never searches for tuners, scans, adds a playlist, or checks the recordings folder. Its "big four" favorites match `ABC|CBS|FOX|NBC` in the channel name, but broadcast names are call signs (WDAF, KCTV), so nothing gets starred. → S8b and P5. Recordings and Settings on Apple live inside `SportsView.swift` and are thin. → P5 and E3 must bring them to parity.
3. **No partial work hidden in comments.** `internal/psip/text.go` skips Huffman-compressed strings "until a capture needs them". A stranger's stations will use them, and their titles will come out blank. Anything left out of a task's Do list becomes a task line, never a code comment. → C7b.
4. **`make check` now mirrors CI** (commit after 97d3ade): gofmt, go vet, the API drift check (`apigen -check`), and `FAKE=1 scripts/relay-smoke.sh` were in CI or missing entirely, not in `make check`. `internal/httpapi/server.go` was committed unformatted; run `gofmt -w server` before the next commit. → R8 adds gofmt to CI once that file is clean.
5. **Round-trip every secret.** 97d3ade fixed a bug the tests missed. `maskURL` percent-encodes its dots, so `FetchURL`'s `strings.Contains(public, "••••")` never matched. Every source with a password fetched the masked address on its daily refresh and went offline. Xtream guides also lost their password, because only one secret per source was stored. Tests now fetch through `FetchURL`. Any new credential path needs a test that stores, lists (masked), and fetches (real). Logins live only in `source_secrets`. They never appear in `sources`, channel JSON (`json:"-"`), logs, events, or exports.

### 0.1b Review 3 (2026-09-24): test like a person, on staging, with the iGPU

The owner's priorities, in order: **playback that is the best available**, **setup that finds everything in the house by itself**, **multiview done right on web, iPhone, iPad, and Apple TV**, and **every device a stranger might own, not just this one DUO**. Phases PB, HOME, MV, and HW below cover them and come first in section 4.

How every task is verified from now on:

1. **Staging beside production.** TUS runs a second container, `Broadwave-Staging`, on `http://192.168.1.2:8490`. It uses the production image with the branch binary mounted, host network, `/dev/dri` (the UHD 770 iGPU), `--cpus 6 --memory 3g`, `-staging -bonjour=false`, a catalog copied from the latest backup with passes deleted, and its own `server_identity` ("Broadwave Staging"). `-staging` (8dac5dd) never records, never pulls the guide, never runs the idle scan, and never starts the emulator, so it tunes only when someone presses play. Every change goes to staging first (U1 scripts this). **Never** touch the `Broadwave` container except in a phase deploy. Nothing else on TUS may be restarted, and that includes `channelsdvr_intel`, a Channels DVR that can also use the DUO.
2. **Tuner etiquette with two servers.** Before tuning, check both `:8477/api/v1/tuners` and `/api/v1/schedule`. Don't tune within 20 minutes of a scheduled recording. Use one tuner at most when production has a viewer. Stop what you start (`POST /api/v1/watch/{channelId}/stop`) and confirm `ours:false` afterward.
3. **Click like a person, then measure.**
   - Web: real Chrome through Playwright (`playwright-cli open --browser=chrome`; the bundled Chromium has no H.264 or AAC) at 390×844, 1440×900, and 1920×1080. Go Home → Watch, open a guide cell and play it, add a tile in multiview, then open settings. Read `getVideoPlaybackQuality()` (dropped frames), `videoWidth`/`videoHeight`, and the console; 404 noise counts as a defect.
   - Apple: dedicated simulators "Broadwave Staging iPhone" (iOS 26.5) and "Broadwave Staging TV" (tvOS 26.5, 1080p) point at staging through the `server` default. Leave the other booted simulators alone. Drive them with XCUITest (focus and remote on tvOS, taps on iPhone), not only launch arguments. Screenshot every screen the task touched.
   - Server output: download the rendition's `init.mp4` plus segments and measure frames/span, WxH, and decode errors (method in skill `broadwave-media-pipeline`, section "Picture lab").
4. **Use the iGPU and prove it.** Every playback measurement names the encoder and says whether decode ran on the GPU, with CPU per rendition taken from `docker stats`.

### 0.1c How the run is operated (2026-09-24)

Run 2's first session lasted 23 hours in one conversation. It made about 3,300 tool calls, with 2 subagent calls. The owner typed "continue" 9 times, because each stop followed a text-only "I'll do X next" message. It finally died on `[canceled] http/2 stream closed with error code CANCEL (0x8)` with P2 uncommitted. From now on:

- **Run it with `scripts/agent-tmux.sh`** (the loop in a tmux session, a readable live view of every tool call, and a status pane), or `scripts/agent-loop.sh` alone. Each round is a fresh headless session (`agent -p --force`) started with `docs/plan/AGENT_PROMPT.md`. A dropped stream or an early stop costs one round. The loop forces `network.useHttp1ForAgent` (long HTTP/2 streams die through this Mac's VPN tunnel) and stops when every line is ticked or blocked, when `~/.broadwave-agent-stop` exists, or after 6 rounds with no commit. Logs: `~/Library/Logs/broadwave-agent/`.
- **Steer by editing `AGENT_PROMPT.md` or this plan.** The next round reads them. Do not run two agent sessions on the repo at once.
- **Every message carries a tool call.** Delegate as `AGENT_PROMPT.md` section 2 lists. `best-of-n-runner` gives parallel tasks their own worktree; only the main agent commits to `main`.
- **CI also builds the Docker image,** which `make check` does not. A web import from outside `web/` (like `api/fixtures`) must be copied in the Dockerfile's web stage (fixed in the commit after 8843648).

### 0.1d Review 4 (2026-09-24, evening): Broadwave 1.0 is in App Review

What changed today: the product is Broadwave everywhere, with no compatibility shims. Production runs v0.6.0, including the 720p60 fix. The App Store has 1.0 for iOS and tvOS waiting for review, with screenshots from a demo lineup of Blender open movies (CC BY). The repo moved to `~/Projects/active/broadwave`. These rules apply from here to the end:

1. **App Review follow-through (AS1) comes before everything else.** At the start of every round, run `~/.blitz/bin/asc versions list --app 6815795649` and write any state change into "Resume here".
   - REJECTED, METADATA_REJECTED, or DEVELOPER_ACTION_NEEDED: make fixing it the current task. Read what `asc review status --app 6815795649` shows. The full message lives in the Resolution Center, which needs the owner's web sign-in, so record it in BLOCKERS and send a desktop notification (`osascript -e 'display notification "..." with title "Broadwave"'`). Then fix whatever the rejection names that is in our control (code, metadata, notes, screenshots, demo), and resubmit.
   - Never reply in the Resolution Center; only the owner does.
   - READY_FOR_SALE: record the date, add the App Store link to README and SUPPORT.md, and continue.
2. **Release cadence.** Every phase ends with a tag `vX.Y.0`, a TUS deploy (0.3), and `scripts/testflight.sh`, which puts a TestFlight build on both platforms. An App Store update is submitted only where section 4 says "App Store update", and only when:
   - `asc validate` shows 0 errors on both platforms;
   - the screenshots were regenerated with `scripts/appstore-shots.sh` (JPEG, no alpha, demo content only);
   - "What's New" is written in the copy voice;
   - the review notes are current.
   The owner authorized submissions. Anything that needs the owner's facts (legal, money, rights, contact) waits in BLOCKERS.
3. **Store and legal hygiene.**
   - Store assets never show broadcast TV or real team logos.
   - Screenshots are JPEG. A PNG with alpha got stuck "upload in progress" and blocked a submission for 40 minutes; the `associatedErrors` field of a failed `reviewSubmissionItems` POST names the stuck asset.
   - A new network call, stored data, or permission updates `PRIVACY.md` and the App Privacy answers in the same task.
   - Third-party content keeps its attribution (Blender CC BY).
4. **Product review every phase.** At the end of each phase:
   - a `generalPurpose` subagent screenshots every screen on web (3 sizes), iPhone, iPad, and Apple TV against staging, and adds defects as J1 sub-items;
   - `code-reviewer` reviews the phase diff;
   - `security-review` reviews anything touching auth, network exposure, secrets, or file paths.
   Fix what takes under an hour, and turn the rest into task lines.
5. **The owner watches through `scripts/agent-tmux.sh`.** The live view shows tool calls and messages, and the status pane shows "Resume here", commits, CI, App Review, and the TUS tuners. Keep "Resume here" and commit messages honest and current. They are the owner's window into the run.

### 0.2 Per-task loop

1. Update "Resume here". Read the relevant code and skill.
2. Implement. Match the surrounding style (`AGENTS.md` working agreements, copy voice).
3. `make check` (Go tests, web typecheck, eslint, swiftlint, swiftformat — the same gates as CI). Plus the task's gates: `scripts/relay-smoke.sh` for relay changes; the two-screen sync measurement for anything under `internal/live`, `web/src/lib/sync.ts`, or `BroadwaveKit/SyncEngine.swift`; browser checks at phone (390×844), desktop (1440×900), and TV (1920×1080) for web UI; `make apple` + a simulator screenshot for Apple UI.
4. One commit per task that includes its `PROGRESS.md` tick (no separate "Tick X" commits). Message: what changed and why.
5. `git push`, then check the previous push's CI (`gh run list -L 3`). **Red CI is stop-the-line:** fix it before the next task. Run 1 left `main` red from D3 to D6 without noticing (swiftlint, plus an iOS 27-only API). CI builds with **Xcode 26.6** while this Mac has **Xcode 27** (Swift 6.4): guard iOS/tvOS 27 APIs with `#if compiler(>=6.4)` around the `#available` check.

### 0.3 Per-phase loop

At the end of every phase (R, S, C, K1, P, G1, F, E, D-extras, B5, I, G, H, N, J, K, L):

1. Add a `CHANGELOG.md` entry and tag `v0.N.0` (next minor). The release workflow publishes `ghcr.io/wolfebase/broadwave:<tag>` for amd64 and arm64.
2. Deploy to Unraid with `MODE=ghcr` (skill `broadwave-unraid`; large SSH uploads over the tunnel drop, so pull from GHCR). Smoke it: health, version, a channel plays, two tabs sync, the phase's features work. Log it in `UNRAID_LOG.md`. Run 1 deployed only once (A3); Unraid still runs v0.1.0.
3. If Apple code changed, run `scripts/testflight.sh` (A7 is done; uploads need no sign-in). App Store updates follow 0.1d rule 2.
4. Update `docs/architecture.md`, ADRs, `api/openapi.yaml`, `AGENTS.md` lessons, and skills where behavior changed.

### 0.4 Shared tuner etiquette

The Mac dev server and the Unraid server share one HDHomeRun CONNECT DUO (2 tuners). Unraid is the household's real DVR.

- Before any test that tunes, check Unraid: `curl -s http://192.168.1.2:8477/api/v1/diagnostics` (tuners in use, active recordings) and `/api/v1/schedule` (recordings in the next 30 minutes). If a recording is on or due, test with at most one tuner, or use the relay smoke test or the fake tuner (K1).
- Never cause a scheduled recording to fail. Never hold a tuner after a test; stop dev servers you started (`pkill -9 -x broadwave-dev`) and leave at most one running.

### 0.5 Secrets and accounts

The user allows reading other projects under `~/Projects` and `~/.blitz` for keys and credentials. Never commit or log them. The App Store Connect API key is in `~/.blitz/asc-credentials.json`; the `asc` CLI is `~/.blitz/bin/asc`.

---

## 1. Where it stands (verified 2026-09-24, evening)

| Thing | Value |
| --- | --- |
| Product | **Broadwave** (renamed 2026-09-24; retired names are blocked by `scripts/check-names.sh` in `make check` and CI) |
| Repo | `/Users/tyler/Projects/active/broadwave`, GitHub `wolfebase/broadwave` (public) |
| Images | `ghcr.io/wolfebase/broadwave` (latest `0.6.0`, amd64 + arm64, public) |
| Apple | team `D4MC63SS36`, app `6815795649` "Broadwave" (iOS + tvOS), bundle `com.wolfeup.broadwave`, App Group `group.com.wolfeup.broadwave`. Version 1.0 build 2 is **waiting for App Review** on both platforms. `asc` CLI with the API key in `~/.blitz` does uploads, metadata, screenshots, and submissions. New app records, App Groups, and first-time availability need the website (the owner signs in to Chrome, the agent drives it). |
| Production | TUS `root@192.168.1.2`, container `Broadwave` on `:8477`, `ghcr.io/wolfebase/broadwave:0.6.0`, host network, VAAPI (UHD 770), appdata `/mnt/cache/appdata/broadwave`, recordings `/mnt/user/media/ota-recordings`, template `my-Broadwave.xml` |
| Staging | TUS container `Broadwave-Staging` on `:8490` (`-staging`, branch binary at `/mnt/cache/appdata/broadwave-staging/broadwave`), lab samples in `/mnt/cache/appdata/broadwave-lab` |
| Tuner | CONNECT DUO `192.168.1.252`, 2 tuners, 27 channels, ATSC 1.0. A Channels DVR container on TUS (`channelsdvr_intel`) can also use it. |
| Network | This Mac reaches TUS through `utun4`; Bonjour from TUS is not visible here (check on TUS with `avahi-browse -rt _broadwave._tcp`). SSH over the tunnel flaps: retry, and send files with `gzip -c | ssh ... 'gunzip -c > file'` plus a checksum. |
| Run | `scripts/agent-tmux.sh` (loop + live view + status pane). Logs in `~/Library/Logs/broadwave-agent/`. |

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
9. TestFlight upload blocked on the App Store Connect app record. → A7 (done 2026-09-24; 1.0 is in App Review).
10. **Sources are thin.** Discovery is only the HDHomeRun UDP broadcast, which fails when Docker runs in bridge mode or the tuner is on another VLAN. M3U parsing ignores `tvg-id`, `tvg-chno`, `tvg-logo`, `group-title`, and `url-tvg`, numbers every channel from 801, never refreshes, and has no stream limit. An XMLTV link is applied to every playlist channel instead of its own source, and fetch errors are swallowed. No Xtream Codes, tvheadend, Channels DVR, HDHomeRun-emulator, or free-channel sources. → Phase S.
11. **Apps can watch but not set up.** The iPhone and Apple TV apps connect (Bonjour or typed address) but cannot add a tuner, scan channels, add a playlist, or run setup; Swift models are hand-mirrored from the API with no contract tests, so a server change can silently break decoding. → Phase P.
12. **Only Apple screens and browsers.** People also watch on Fire TV, Android/Google TV, Roku, smart TVs, and inside Channels, Plex, Jellyfin, and IPTV players. The HDHomeRun emulator and M3U/XMLTV exports exist but were never verified against those apps. → Phase N.

---

## 2. Research digest (sources in `docs/research.md`; re-verify version-sensitive facts)

- **The source ecosystem (verify in S0; details in `docs/research.md`).** Channels DVR officially supports only HDHomeRun tuners plus M3U "Custom Channels" (MPEG-TS or HLS, 750 channels per playlist) and TV Everywhere; Tablo was dropped and AirTV was never supported. HDHomeRun: UDP 65001 broadcast discovery (same L2 segment, host networking), mDNS hostnames `hdhomerun.local` / `hdhr-<id>.local`, SSDP (`upnp:rootdevice`, `Server: HDHomeRun/1.0`), and `api.hdhomerun.com/discover` (SiliconDust calls it unsupported and asks apps not to rely on it; behind CGNAT it lists strangers' devices — last resort only, verified locally). `discover.json`, `lineup.json` (Tags `favorite`, `drm`), `lineup_status.json`, scan with `POST /lineup.post?scan=start&source=Antenna`, `status.json`. Streams on port 5004: `/auto/v<ch>` is a filtered single program (device-built PAT/PMT; PSIP probably absent), **`/auto/ch<freq>` is the unfiltered full mux**, `/auto/ch<freq>-<prog>` one program; 503 busy or DRM with `X-HDHomeRun-Error`. Models today: FLEX DUO/QUATRO/4K (4K: 2 of 4 tuners ATSC 3.0, 3.0 channels numbered 100+, DRM channels fall back to 1.0), PRIME (CableCARD); EXTEND and SCRIBE discontinued; SCRIBE/SERVIO are storage devices that list recordings at `recorded_files.json`. Discovery of others: Channels DVR advertises `_channels_dvr._tcp` (8089), tvheadend `_htsp._tcp`, Jellyfin answers UDP 7359; many emulators answer SSDP. HDHomeRun emulators that Plex/Channels users run: tvheadend (9981, `/playlist/channels.m3u`, `/xmltv/channels`, HTSP 9982), Threadfin/xTeVe (34400), ErsatzTV (8409), Dispatcharr, Antennas; many answer SSDP and serve `discover.json`. Channels DVR itself serves `http://host:8089/devices/ANY/channels.m3u` and `/devices/ANY/guide/xmltv`. IPTV: extended M3U (`#EXTM3U url-tvg=`, `tvg-id`, `tvg-name`, `tvg-logo`, `tvg-chno`/`channel-number`, `group-title`, Channels' `tvc-guide-stationid`, `tvg-shift`, catchup attributes, `#EXTVLCOPT:http-user-agent/referrer`) and Xtream Codes (`player_api.php?username=&password=&action=get_live_categories|get_live_streams`, `xmltv.php`, streams at `/live/<user>/<pass>/<id>.ts|.m3u8`). Free channels: the hosted i.mjh.nz lists were withdrawn in 2024 and Pluto now needs a login; people run generator containers instead — FastChannels (:5523, 25+ services, `/feeds/<name>/m3u` and `/epg.xml`), Pluto for Channels (:7777 or 8080), Samsung TV Plus for Channels (:8182). None announce themselves; find them with the opt-in port probe. Channels-specific M3U tags: `channel-id`, `channel-number`, `tvc-guide-stationid` (Gracenote), fallback `tvc-guide-title/-description/-art/-tags/-genres`, `tvc-stream-vcodec/-acodec`. Tablo Gen 1-3 has a local REST API on 8885 (discovery UDP 8881/8882) returning HLS; Gen 4 needs Tablo's cloud and signed requests (unsupported). Channels DVR as a source: `/devices/ANY/channels.m3u?format=ts&codec=copy`, `/devices/ANY/guide/xmltv` (it returns 403 to requests that look external, which Docker bridge networking can trigger). Channels DVR's M3U source UX: URL or file, stream format (HLS/MPEG-TS/auto), stream limit, "prefer channel numbers from M3U", XMLTV URL, refresh interval.
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

R1. **CI green and kept green.** Confirm run for `0984064` is green on all four jobs (`gh run list`). If Apple fails again, fix it. Add a short "CI" section to skill `broadwave-dev-loop` (what each job runs, Xcode 26 vs 27, `make check`). **Accept:** green `main`; skill updated.

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

### Phase U — Staging and hotfix (first)

U1. **Staging as a script.** The 720p fix already shipped as v0.6.0 (UNRAID_LOG, 2026-09-24).
- Add `MODE=staging` to `scripts/deploy-unraid.sh`, doing exactly what section 0.1b rule 1 describes. It must be idempotent and refuse to touch `Broadwave`. Today staging was replaced by hand: the binary is sent with `gzip | ssh`, the name goes in with `sqlite3` using single quotes, and the container runs `-staging -bonjour=false` on `:8490`. TUS has no Python, so do any checks on the Mac.
- Add `scripts/staging-watch.sh <channelId> [seconds] [host:port]`. It watches through the API (`POST /api/v1/watch`, then `POST /api/v1/watch/<channelId>/stop`), measures the rendition (WxH, fps from frame timestamps, segment length, decode errors), and confirms the tuner was released.
**Accept:** `staging-watch.sh 1 15 192.168.1.2:8477` prints 1280x720 59.94 against production, and the same against `:8490`.

### Phase AS — App Store life

AS1. **Review follow-through** (continuous; 0.1d rule 1). **Accept:** every state change is in "Resume here" and a rejection is handled in the round that sees it.
AS2. **"Try Broadwave" demo mode in the apps.** Reviewers and people who haven't set up a server can use every screen.
- `BroadwaveKit` gets `DemoServer`: an in-process HTTP server (Network.framework, loopback only) that serves a catalog, a guide with art, and live sessions, built from `api/fixtures` plus a demo lineup.
- Short HLS loops of the Blender open movies are bundled (≤ 25 MB total, 720p, CMAF), with attribution in About.
- ConnectView gets "Try the demo". Leaving the demo is one tap in Settings. Sync and multiview work between two simulators in demo.
**Accept:** in airplane mode, the iPhone and Apple TV simulators run Home, Guide, Player, and Multiview from the demo. The app size change is reported, and the review notes say how to enter the demo.
AS3. **`scripts/appstore-shots.sh` and `scripts/demo-lineup.sh`.** Script what was done by hand on 2026-09-24:
- `demo-lineup.sh` fetches 4-minute clips of Big Buck Bunny, Sintel, Tears of Steel, and Elephants Dream from archive.org (`BigBuckBunny_124`, `Sintel`, `Tears-of-Steel`, `ElephantsDream`). It encodes them as 720p TS, picks bright, calm art frames (never blood or gore), builds looping HLS, and writes M3U + XMLTV (four channels 4.1/5.1/9.1/11.1 with art). It then runs a `-staging` server on `:18520`, adds the source, marks setup done, names the server "Living Room", and warms `/media/art` at 640 and 1600.
- `appstore-shots.sh` creates the "Broadwave Shots" simulators (iPhone 17 Pro Max, iPad Pro 13-inch M5, Apple TV 4K 1080p) and sets the status bar to 9:41. For each device it captures Home, Guide (`-BroadwaveTab guide`), Watch (`-BroadwaveWatch 1`), and Multiview (`-BroadwaveMultiview 1,3`; iPad only once MV1 is done). It converts the captures to JPEG and uploads them with `asc screenshots upload --replace` (IPHONE_67, IPAD_PRO_3GEN_129, APPLE_TV). After AS2 lands, it uses the in-app demo instead of the server.
**Accept:** one command regenerates and uploads every set, and `asc validate` is clean afterward.
AS4. **App Store update 1.1** at its section 4 milestone: build, What's New, new screenshots (with iPad multiview), review notes (demo mode), and submit on both platforms. **Accept:** WAITING_FOR_REVIEW recorded; later updates follow the same recipe.
AS5. **Public TestFlight.** An external group "Public" with a public link on both platforms, a beta description, beta review info, and submission for beta review. **Accept:** the link is in README and SUPPORT.md once beta review approves it.
AS6. **Sports data rights.** The scores come from ESPN's unofficial, unlicensed scoreboard, and the owner attested to content rights. Research licensed or clearly permitted sources (official league APIs, TheSportsDB terms, others) and make the provider pluggable. Write ADR 0011. Put the owner's decision on the default in BLOCKERS as a question, and keep scores on until it is answered. **Accept:** ADR, pluggable provider, and the owner's answer recorded.

### Phase AP — Apple apps at web depth (quick wins before 1.1)

AP1. **Program art everywhere on Apple.** Wide guide cells, the program sheet hero, search results, sports cards, and recording posters (art fallback) all show art. Use `ArtLayout` and `store.artURL(_:width:)`; never upscale past 1.25×. The Home hero and On now cards already do this (f22d4ab). **Accept:** screenshots per platform against the demo lineup and against staging.
AP2. **iPhone portrait player.** The channel and program are shown, controls have labels, and there's a clear way to reach the mini guide and multiview (overlaps F5; do the portrait layout here). **Accept:** screenshots plus VoiceOver labels.
AP3. **Preview frames on Apple.** On now cards and the hero use R5 frames when a mux is tuned and there's no art, like the web. **Accept:** a staging screenshot with a tuned channel.

### Phase OPS — Running it for years

OPS1. **Update notifier.** The server checks GitHub releases once a day; it's opt-out and sends nothing but the request. Web and Apple show "Broadwave 0.7 is available", and the release notes link to the CHANGELOG. P3 carries the minimum app version. **Accept:** a test with a fake releases feed, and screenshots.
OPS2. **Catalog backup and restore.** A nightly backup (keep 7 daily and 4 weekly) in `config/backups`, plus a backup before any version change. Settings > Backups lists, downloads, and restores them. **Accept:** a restore tested on staging, and Go tests.
OPS3. **Support bundle.** One click exports logs, versions, doctor results, and a redacted config, with no secrets (reuse `maskURL`; a test proves no password is present). It's linked from SUPPORT.md. **Accept:** a redaction test and a sample bundle.
OPS4. [done 2026-09-24] `scripts/check-names.sh` in `make check` and CI.
OPS5. **README and site.** A README with screenshots and installs for Docker, Unraid, and Mac, and the App Store badge once live. A GitHub Pages site from `site/` (merges J7's site and L5's docs), on no paid domain until the owner buys one (BLOCKERS). **Accept:** the site is live at `wolfebase.github.io/broadwave`.

### Phase LEGAL — Licenses, notices, security

LEGAL1. **Licenses and notices.**
- Confirm the repo LICENSE is OSI (needed for Community Apps).
- Write a NOTICE listing Go, npm, and Swift dependencies.
- Meet ffmpeg (and later jellyfin-ffmpeg) GPL/LGPL obligations for the image: source offer, notice, and OCI labels.
- Credit the Blender open movies (CC BY) in the app's About and in README.
- Keep trademark-safe copy (HDHomeRun, Apple TV, Plex, and Channels are named only to describe compatibility).
**Accept:** NOTICE, image labels, the About screen, and a short `docs/legal.md`.
LEGAL2. **Security pass before remote access.** Before H2/H3: a threat model in `docs/security.md`, then a `security-review` subagent over the server (auth, CSRF on admin routes, path traversal in media and file routes, SSRF in playlist and XMLTV fetch, secret handling, rate limits). Fix every real finding with tests. **Accept:** findings and fixes listed, `go test -race` clean.

### Phase PB — Playback that beats everything

The picture is the product. OTA broadcasts are MPEG-2 or H.264: 1080i (29.97 frames = 59.94 fields), 720p59.94, and 480i SD. **Real 60-frame motion comes from field-rate deinterlacing (bob, motion-adaptive or motion-compensated on the iGPU), which rebuilds all 60 real moments per second from 1080i.** Frame interpolation invents frames and is only for true 30p/24p sources, and only if it passes a visual review (PB4).

PB1. [done, 8dac5dd] A progressive source is never deinterlaced and keeps its frame rate (720p60 was sent at 119.88 fps on VAAPI and 29.97 in software). A `-staging` flag was added.
PB2. **The first tune is right.** On a fresh install the field order is unknown until ffprobe finishes, so the first rendition of a 720p station still takes the interlaced path. Read the scan type straight from the mux within 200 ms: MPEG-2 `sequence_extension.progressive_sequence` and `picture_coding_extension.progressive_frame`, and H.264 SPS `frame_mbs_only_flag`. Pass it to `Source` before the rendition starts, and store it. **Accept:** an empty-catalog staging tune of 4.1 is 1280x720 59.94 on the first try; unit tests on captured headers.
PB3. **Scan-type matrix.** Fixtures made with ffmpeg (`tinterlace`, `telecine`, `-flags +ildct+ilme`, PAFF and MBAFF H.264), plus real captures from 1080i, 720p, 480i SD MPEG-2, and the H.264 subchannels (14.x). Each case goes through every encoder path (VAAPI, VideoToolbox, libx264). Assert output fps, frame count, `idet` on the output (≈0 interlaced frames), and `mpdecimate` (no duplicate frames at 60). Detect 3:2 film cadence automatically and switch to 24p on its own (hybrid: `fieldmatch,decimate` on the CPU, then GPU scale/encode). **Accept:** table in `docs/dev-lab.md`; Go table tests on the args; lab runs on TUS.
PB4. **Honest "30 to 60".** Measure `minterpolate` (mci and blend), `framerate`, and any VAAPI/QSV frame-rate conversion that jellyfin-ffmpeg exposes on the UHD 770: CPU/GPU cost at 1080p, and ghosting on a sports clip (stills plus a 10 s clip per method, reviewed side by side). Keep a method only if it holds real time within one core and looks better; otherwise remove the `blend` path and say why in ADR 0010. Also confirm the Apple TV "Match Frame Rate" switch at 59.94 (F5).
PB5. **GPU end to end.** Decode on the GPU: `-hwaccel vaapi -hwaccel_output_format vaapi`, dropping `format=nv12,hwupload` when frames are already on the GPU (the lab run failed with "Impossible to convert between the formats"), with a software-decode fallback on error. Move the image to jellyfin-ffmpeg 7 (pulls G5 forward; the image's Debian ffmpeg 5.1 misbehaves with VAAPI deinterlace when fed faster than real time). Encoder tuning on VAAPI: `-rc_mode`, `-profile:v high`, B-frames, low-power. Choose bitrates by VMAF (libvmaf in the lab; ≥ 95 at 1080p60 on the LAN, ≥ 90 at 720p on cellular). Add HEVC renditions (`hvc1`) for Apple TV/iPhone (G4 pulled forward). **Accept:** CPU per 1080p60 rendition on TUS before/after (baseline about 18% of a core), VMAF table, and decode errors = 0 over a 10-minute live run.
PB5c. **HEVC stays on the iGPU.** On staging, an Apple HEVC rendition exits `hevc_vaapi` with "Access unit too large: 8192 < 9312" (ffmpeg -28; the cache had 193G free) and restarts on libx265. Find the arg or ffmpeg fix, and keep the software fallback for a real GPU failure. **Accept:** staging Apple channel 1 stays `hevc_vaapi` with GPU decode for 30s, 0 decode errors, and the stream panel says so.
PB6. **Sound.** AC-3 5.1 passthrough to Apple TV and AVPlayer (AC-3 in fMP4 HLS) when the route supports it; E-AC-3. Pick audio by PMT, not stream order: ISO-639 language plus AC-3 descriptor `bsmod`. Here, 4.1 carries Spanish SAP on PID 0x102 and 41.1 a second English stereo track (probably described video). Add an audio picker in every player (main / second language / described video) and an optional "Even volume" (light `loudnorm`), off by default. **Accept:** fixtures for PMT parsing; Apple TV sim plays 5.1 passthrough; the web picker switches without a stall (F2).
PB7. **Player tuning.** hls.js buffer and back-buffer settings per device; AVPlayer `preferredForwardBufferDuration`, no peak-bitrate cap on the LAN, `AVDisplayCriteria` frame-rate and range matching on tvOS. Measure time to first frame and stalls per hour on web, iPhone, and Apple TV (staging).
PB7b. **Stall soak.** Watch one channel on staging for an hour on web, iPhone, and Apple TV. Record stalls and stall time. Tuner etiquette applies.
PB8. **Show the truth.** A Stream panel in the web and Apple players (pulls F3's stats forward): source codec, WxH, scan type, and fps; output WxH, fps, encoder, bitrate, and whether decode ran on the GPU; client dropped frames and buffer; and the sync offset. **Accept:** screenshots on all three platforms at staging.
PB9. **Picture lab in the repo.** `scripts/picture-lab.sh` captures samples (etiquette rules), runs candidate args in `bw-lab-*` containers on TUS at real time (`-re`), and outputs a table (fps, frames, WxH, decode errors, speed, CPU, VMAF) plus stills. Every PB task attaches its table. Do this first in Phase PB.

### Phase HOME — The house sets itself up

HOME1. **Your home.** One scan that finds every tuner or source (S2), plus every screen and server that can use Broadwave:
- Apple TVs and iPhones running the app (clients announce themselves over the event socket);
- Chromecast and Google TV (`_googlecast._tcp`), Fire TV and Android TV (DIAL/SSDP), smart TVs (UPnP MediaRenderer);
- AirPlay targets (`_airplay._tcp`);
- Plex, Jellyfin, Emby, and Channels DVR servers (offer "Use Broadwave as your tuner", N1).

Show it in setup step 1 and in Settings > Your home, with one action per item. It's read-only and local-subnet only; nothing is added without a tap, except a new install's first HDHomeRun. **Accept:** on the real LAN it finds the DUO, `channelsdvr_intel`, Plex, and the simulators. Screenshots on web and Apple TV.
HOME2. **Setup finishes itself.** After the tuner step, setup runs on its own:
- a scan if the lineup is empty;
- guide sources (SiliconDust, then PSIP harvesting scheduled);
- the recordings folder check;
- big-four favorites by affiliation (S8b);
- a live encoder self-test ("Intel GPU found: 1080p60 at 11x real time");
- one signal check (C9), summarized ("6 channels great, 1 weak").

It ends on "Ready: 27 channels, guide for 21, 2 tuners, Intel GPU". **Accept:** empty-catalog staging to first live channel in < 90 s with zero typing, on web, Apple TV, and iPhone; screenshots of every step.
HOME3. **Later arrivals.** A device that appears later (a second HDHomeRun, a new Apple TV, a Plex server) raises one activity event and a banner ("New tuner found: HDHomeRun FLEX 4K. Add it?"). **Accept:** a fake device started mid-run produces exactly one banner.

### Phase MV — Multiview, configured right everywhere

Seen on 2026-09-24 in `docs/screenshots/pb-tv-mv.jpg` and in Chrome at 1920×1080: side-by-side tiles fill the full height, so each picture sits in a band with large black bars. The tvOS focused layout button is an empty blue pill (its label vanishes on focus). "Select a tile to hear it" stays after a tile has sound. The focused web tile dropped 8 of its first 129 frames.

MV1. **Geometry.** Tiles are 16:9 boxes sized to the screen for 2-up, 1+2, 1+3, quad, and PiP, on web (phone, desktop, TV), iPhone portrait and landscape, iPad, and Apple TV. **Accept:** screenshots of every layout × platform with no letterbox bars inside a tile.
MV2. **Remote and focus on tvOS.** Visible focus labels. Swipe moves between tiles, click moves audio focus, long-press opens the tile menu, Play/Pause pauses all, and Menu leaves. **Accept:** an XCUITest drives it.
MV3. **Tuner-aware picker.** The add-a-channel picker says "same tune as 9.1", "uses a tuner", or "no tuner free". It works with 1, 2, 4, and 8 tuners and with several devices (HW1 fakes). A scheduled recording always wins, and the viewer is warned before a tile stops. **Accept:** fake-tuner tests; web and tvOS screenshots.
MV4. **Tile quality.** The focused tile runs at 60 fps (540p60 or 720p60) and the others at 30, within the host budget from HW4. No dropped frames after warm-up. Hide the hint once a tile has sound. **Accept:** per-tile dropped frames measured in Chrome and on the Apple TV simulator.
MV5. **Sync and sound.** All tiles and every other screen stay within 50 ms of the broadcast timeline. Audio focus follows to AirPlay and HomePod. Saved sets work, and Sports "Watch together" opens the right layout. **Accept:** sync numbers from two browsers plus the TV simulator.
MV6. **Parity.** For each platform and layout, a screenshot and a 10 s screen recording; the multiview rows in `docs/parity.md` are complete.

### Phase HW — Every device, not just this DUO

The owner has one CONNECT DUO (ATSC 1.0, 2 tuners, MPEG-2). Strangers own everything else. Nothing may assume one device, two tuners, ATSC 1.0, MPEG-2, or this lineup.

HW1. **Fake device fleet** (extends the K1 fake). Each profile answers discover, lineup, status, and control the way the real model does, and streams the right codecs:
- HDHR3-US, CONNECT DUO/QUATRO, FLEX DUO/QUATRO;
- FLEX 4K (2 of 4 tuners ATSC 3.0, channels 100+, DRM tags, HEVC + AC-4 samples);
- PRIME (CableCARD, copy-once/never flags, 3 tuners);
- EXTEND (transcode profiles) and SCRIBE/SERVIO (storage);
- old firmware, and 1 to 8 tuners.

A table test runs discovery, scan, tune, multiview plan, recording, and failover for every profile. `docs/hardware.md` lists each model as verified on real hardware, verified on a fake, or untested.
HW2. **Pools.** Two DUOs plus a FLEX 4K, and 8-tuner pools. 3.0-capable tuners are kept for 3.0 channels. A device that vanishes mid-stream hands the stream to another device within 5 s.
HW3. **Servers and emulators for real.** tvheadend, Threadfin, ErsatzTV, and Dispatcharr in containers; the Channels DVR already on TUS (read-only); Plex Live TV reading Broadwave (N1). CI keeps the tvheadend and Threadfin fakes.
HW4. **Server hardware.** Cover Intel VAAPI/QSV (TUS), AMD VAAPI, NVIDIA NVENC, Apple VideoToolbox, and software-only (Raspberry Pi 5 arm64; a Synology-class J4125). A startup self-benchmark picks the rendition and tile budget per host and shows it in Diagnostics. Test the arm64 image under QEMU.
HW5. **Client matrix.** Apple TV HD (1080p, no 10-bit HEVC), each Apple TV 4K generation, older iPhones, iPad, Safari, Chrome, Edge, and Firefox (no HEVC; no AC-3 in Chrome). A table test shows each gets the right rendition from `Decide`.

### Phase S — Every source, found automatically

The bar: a stranger with any common tuner or playlist gets a working guide without typing an IP address. Anything Channels DVR accepts as a source, Broadwave accepts too, and it finds more of it on its own.

S0. **Research and ADR.** Verify the ecosystem facts in section 2 against current docs and forums (a `docs-researcher` subagent): discovery for each device family, stream URL forms (including whether `/auto/v<ch>` carries PSIP and how to get the full mux), M3U attribute conventions, Xtream endpoints, what Channels DVR supports today (including Tablo, AirTV, TV Everywhere) and what has no open API (write those down as unsupported, with the reason). Record it in `docs/research.md` and write ADR 0009 (sources and discovery). **Accept:** ADR merged; a support matrix in `docs/sources.md` (device or service, how found, how streamed, guide, status).

S1. **One source model.** Replace the ad hoc `src-*` device ids with a real `sources` abstraction: kind (`hdhomerun`, `hdhr-compatible`, `m3u`, `xtream`, `tvheadend`, `channels-dvr`, `link`, `folder`, and later `tablo` if S0 finds a usable API), a stable id, display name, enabled flag, priority, capabilities (tuner count or stream limit, stream format TS/HLS, whether it carries its own guide, whether it needs a tuner), credentials stored separately (never in logs, masked in every URL the API or Diagnostics returns), refresh policy, and health. Channels keep stable ids across refreshes (key by device + guide number, `tvg-id`, or stream URL). New numbered migration that moves existing channels, recordings, and passes without loss (test it on a copy of the real catalog and the Unraid backup). **Accept:** migration test on real catalogs; every existing screen still works; OpenAPI updated.

S2. **Auto-find engine** (`internal/discovery`). Runs at startup, every 5 minutes, and on demand, and streams results to clients over the WebSocket as they arrive:
- HDHomeRun UDP broadcast on every interface (exists), plus unicast discovery to hosts on the local /24 of each interface when broadcast gets no answer (bridge networking, VLANs).
- mDNS hostnames `hdhomerun.local` / `hdhr-<id>.local`, and SSDP (HDHomeRun answers `upnp:rootdevice`).
- SiliconDust cloud lookup (`api.hdhomerun.com/discover`) only as a last resort inside "Look harder" (SiliconDust asks apps not to rely on it, and behind CGNAT it lists strangers' devices): offer a result only if its `LocalIP` answers locally with the same device id.
- SSDP `M-SEARCH` for HDHomeRun emulators and UPnP media servers.
- mDNS browse: `_channels_dvr._tcp` (Channels DVR), `_htsp._tcp` (tvheadend), plus any found in S0; Jellyfin's UDP 7359 probe for N1.
- **"Look harder"** (explicit, user-started): probe the local /24 for well-known ports — 80/5004 (HDHomeRun, Antennas, cetonproxy), 9981 tvheadend, 34400 Threadfin, 8409 ErsatzTV, 9191 Dispatcharr, 8089 Channels DVR, 5523 FastChannels, 7777/8182 Pluto/Samsung generators, 8885 legacy Tablo, 32400 Plex, 8096 Jellyfin — fingerprinting each (`/discover.json` or a known endpoint), rate-limited, local subnets only.
- Results are a list of found things with kind, name, address, and channel count, each with one-tap Add. A brand-new install adds a found HDHomeRun automatically; everything else waits for a tap. Devices are tracked by device id, so a DHCP address change is followed automatically.
- **Accept:** Go tests with fake responders for each method; on the real network the DUO is found by broadcast, SSDP, and mDNS; a bridged `docker run` on the Mac still finds it (unicast or SSDP, without the cloud); results appear in setup within 5 s.

S3. **HDHomeRun family, complete.** Multi-device tuner pool (moves G7 here): several devices, per-device priority, failover, recording reservations. Channel scan from the UI (`lineup.post?scan=start`, live progress from `lineup_status.json`, then refresh the lineup). Model awareness: FLEX 4K ATSC 3.0 channels (hand to G6), PRIME CableCARD copy-protection flags (mark "Copy protected" and don't offer them), EXTEND transcode profiles as an alternative to server transcoding on weak servers. Lineup `drm` tags mark channels "Protected" (not offered). SCRIBE/SERVIO storage devices: list their recordings (`recorded_files.json`) as a library source. Firmware version shown read-only. **Accept:** fake-device tests for two devices, failover, and scan; a real rescan on the DUO.

S4. **M3U done right.** Parse `#EXTM3U url-tvg`/`x-tvg-url`, `tvg-id`, `tvg-name`, `tvg-logo`, `tvg-chno`/`channel-number`, `channel-id`, `group-title` (semicolon lists), `tvg-shift`, `catchup`/`catchup-source`/`catchup-days`, Channels' `tvc-guide-stationid` and fallback `tvc-guide-title/-description/-art/-tags/-genres` (used as a guide when no XMLTV covers the channel), `tvc-stream-vcodec/-acodec` hints, and `#EXTVLCOPT`/`#KODIPROP` user agent and referrer (sent when streaming). XMLTV may be gzip or xz. Add from a URL, an uploaded file, or a path on the server; gzip. Options matching Channels DVR: stream format (auto/HLS/MPEG-TS, probed), stream limit, numbering (use playlist numbers, or start at N), include/exclude groups, and an XMLTV URL (auto-filled from `url-tvg`) matched by `tvg-id` to **this source's** channels only (fixes the current bug). Refresh on a schedule (playlist daily; XMLTV every 3, 6, or 24 h) with a diff that keeps channel ids, favorites, and passes. **Source priority with rollover:** the same channel from two sources (an HDHomeRun and a Channels DVR or emulator playlist) stacks into one guide row, and playback rolls over to the next source when one is busy or at its limit. Large playlists (> 300 channels) open a channel picker instead of flooding the guide. The relay reads HLS inputs with reconnect and uses their program date-times when present so sync still works; TS inputs go through the same mux path as tuners. Errors say what failed and what to try. **Accept:** parser table tests over real-world samples (anonymized); a 5,000-channel playlist imports in < 3 s; an HLS and a TS channel play, record, and sync in two tabs.

S5. **Xtream Codes.** Server URL, username, password: import live categories and streams, EPG from `xmltv.php`, the same options as S4 (groups, numbering, limit). Credentials are masked everywhere. **Accept:** tests against a fake Xtream server; a live channel plays.

S6. **Servers and emulators as sources.** tvheadend (M3U + XMLTV with auth, or its HDHomeRun emulation), Threadfin/xTeVe/ErsatzTV/Dispatcharr/Antennas (HDHomeRun emulation or M3U + XMLTV), a Channels DVR server (its M3U and XMLTV; say "uses your Channels DVR tuners" so the user knows who owns the tuner), and "HDHomeRun-compatible device at an address". Legacy Tablo (Gen 1-3) through its local API on 8885 if S0 confirms it still works (optional; Gen 4 needs Tablo's cloud and is unsupported). AirTV, Fire TV Recast, and TV Everywhere are listed as unsupported in `docs/sources.md` with the reason. Pass `format=ts&codec=copy` to Channels DVR and explain its 403 for bridged containers. **Accept:** one integration test per kind against fakes; tvheadend or ErsatzTV verified for real in a local container.

S7. **Free channels.** "Add free channels": detect generator containers already on the network (FastChannels, Pluto for Channels, Samsung TV Plus for Channels via the port probe) and add their feeds in one tap; otherwise show a short guide to running FastChannels next to Broadwave (a compose snippet and the Unraid app name), then add it. Each is a normal M3U source underneath, labeled "Streamed from the internet", in its own guide group, never taking a tuner; DRM-protected streams are skipped with a note. Never scrape services ourselves. **Accept:** a FastChannels container on the Mac is detected and one feed adds channels with a guide and art.

S8. **Setup wizard v2 (web, Apple TV, iPhone).** Step 1 "Looking for your tuner…" fills in live as S2 finds things (HDHomeRun preselected), with Add a playlist (URL, file, Xtream), Free channels, Look harder, and Enter an address. Step 2 channels: scan if the lineup is empty, show logos, preselect favorites for the big four networks the user gets, and offer to hide duplicates and shopping channels. Step 3 guide: shows coverage per source (and later PSIP from C7) in one line per channel group. Step 4 recordings: where they go, with the volume check from S9. Step 5 apps: QR for iPhone, "Open Broadwave on your Apple TV" instructions. The same flow runs natively on Apple TV and iPhone (Phase P5). **Accept:** fresh install to first live channel with an HDHomeRun in < 90 s and zero typing; screenshots of every step at three sizes and on tvOS/iPhone.

S9. **Setup doctor.** Detect and explain, in one line each with the fix: bridge networking (container IP in a Docker range and no broadcast replies), missing `/dev/dri` when the host has an iGPU, the recordings path not on a mounted volume (`/proc/mounts`), low disk, time zone unset, clock skew (Whole-Home Sync needs a sane clock), file ownership (support `PUID`/`PGID`/`UMASK` so Unraid recordings are `99:100`, not root), and a tuner that stopped answering. Shown in setup, Diagnostics, and the apps. **Accept:** tests for each check; the Unraid template and compose file carry the right defaults.

S10. **Source health.** Per-source status: online, last refresh, errors, streams in use against the limit, next refresh. A source that goes away raises one activity event and a banner, and comes back on its own. **Accept:** fake-source tests for offline, back online, and limit reached ("All 2 streams from this playlist are in use. Stop one or raise the limit.").

### Phase P — Server and Apple apps, working as one

The Docker server and the iPhone and Apple TV apps must behave like one product: find each other instantly, never disagree about data, recover from anything, and let the living-room TV do everything the web console can.

P1. **Generated clients.** Generate Swift types and client from `api/openapi.yaml` (swift-openapi-generator, or a small generator if that fits better) into BroadwaveKit, and TypeScript types for the web; CI fails when generated code is out of date. Retire the hand-mirrored models (keep thin wrappers where the UI needs them). **Accept:** BroadwaveKit builds from generated code; drift check in CI.

P2. **Contract tests.** A test mode of the server (fake tuner from K1, fixed clock) records golden responses for every endpoint and WebSocket event; BroadwaveKit and web decode tests run against them in CI, so a server change that breaks a client fails the build. **Accept:** golden fixtures in the repo; decode tests in `make check`.

P3. **Compatibility.** `apiVersion` + `features` negotiation: the apps hide what the server lacks and say "Update your Broadwave server to use this" when needed; the server keeps working with the previous app version for one release. `GET /api/v1/server` reports a minimum app version. **Accept:** tests with an older fixture set; a clear screen for incompatible versions.

P4. **Find each other, always.** Bonjour first. Fallback discovery for networks that filter mDNS: the server answers a small UDP broadcast probe on a documented port with its id, name, and URL; the apps send it when Bonjour finds nothing in 3 s. Remembered servers, reconnect by server id when the address changes, a Local Network permission explainer on iOS and tvOS, and QR/deep link connect. tvOS: typing an address is the last resort (Continuity keyboard works). **Accept:** simulators find the dev server with Bonjour disabled on the server; an address change is followed without user action.

P5. **Set up and manage from the apps.** Native SwiftUI for the S8 setup flow (Apple TV first: most people set up in the living room), sources and discovery results, channel scan, favorites and hiding, passes, recordings management, settings, and a Diagnostics screen with the S9 doctor. Keep `docs/parity.md`: every feature × web / iPhone / iPad / Apple TV, updated in the same commit as any feature. Move Recordings and Settings out of `SportsView.swift` into their own files and bring them up to the web's depth: every setting the web Settings page has, grouped the same way. The antenna check (C9), guide source and depth (C8), sources with health (S10), and the doctor (S9) all appear on Apple. **Accept:** a fresh server set up entirely from the Apple TV simulator; parity doc complete with **no "web only" cell except where the platform forbids it** (each such cell gives the reason); screenshots per section on tvOS and iPhone.

P6. **Realtime and resilience.** EventSocket reconnect with backoff and room re-join; clock re-sync after sleep; an offline banner with cached data (R3); the player survives a server restart or container upgrade (recovers within 5 s, same channel, same sync room); correct behavior on iOS background/foreground and tvOS sleep/wake; guide and recordings update live from events, never by polling. **Accept:** scripted test restarts the container while web and simulators play; all recover; timings logged.

P7. **End-to-end in CI.** A job that runs the built Docker image with the fake tuner, then XCUITest on iOS and tvOS simulators: find the server, finish setup, guide renders, a channel reaches playing, multiview 2-up, record creates a recording, and web + simulator stay in sync (< 100 ms). **Accept:** the job is green and required.

P8. **The container is the reference server.** Every end-to-end and soak test runs against the image, not `go run`, so ffmpeg builds, hardware fallback, permissions, `PUID`/`PGID`, time zone, and healthcheck problems show up in CI. Image variants documented (Intel/AMD VAAPI default; NVIDIA with `--runtime=nvidia` notes). **Accept:** CI e2e uses the image; `docs/sources.md` and README list image options.

### Phase N — Every screen people already own

Apple stays the flagship. Everyone else in the house still gets a great way in.

N1. **Be the best tuner for Channels, Plex, Jellyfin, and Emby.** Harden the HDHomeRun emulator and the M3U/XMLTV exports so those apps see Broadwave as a tuner with the merged guide (tuner, PSIP, M3U, and free channels), unlimited streams from one tune, and stable channel ids. Verify for real with each app in a container on the Mac or TUS (moves G11 here): add as a tuner, guide maps, live plays, a recording in their DVR works. Settings shows ready-to-copy URLs for each app. **Accept:** a log of each app working; fixes for every gap found.

N2. **IPTV players.** Per-profile M3U + XMLTV with tokens (after H2), and an Xtream Codes-compatible output (`player_api.php`, `xmltv.php`, `/live/...`) so TiviMate, IPTV Smarters, Kodi, and VLC on Fire TV, Android TV, and phones get channels, guide, and logos. **Accept:** TiviMate or IPTV Smarters (on an Android TV emulator) loads channels and guide and plays.

N3. **Web app on TV browsers.** Installable PWA; the TV layout driven fully by D-pad on Fire TV Silk, Android/Google TV browsers, and LG/Samsung browsers; checked in Chrome with TV emulation and on a real Fire TV if one is on the network. **Accept:** D-pad walkthrough of Home, Guide, player, multiview.

N4. **Google Cast.** Cast from the web app (and later Android) to Chromecast and Google TV: HLS with AAC, CORS set, a custom receiver that runs our sync engine so a cast screen joins Whole-Home Sync. **Accept:** casts from Chrome to a Cast device or the Cast emulator; receiver in sync with a web tab.

N5. **DLNA/UPnP media server (optional setting).** Live channels and recordings as items for smart TVs, VLC, and game consoles. **Accept:** VLC's UPnP browser lists and plays a channel and a recording.

N6. **Android and Android TV app (after Apple is flagship-complete).** Kotlin + Media3/ExoPlayer against the same generated API (P1): Home, Guide, player, multiview, sync. Fire TV build from the same code. If time runs out, write the design and API gaps into `docs/android.md` instead. **Accept:** emulator screenshots, or the design doc.

N7. **Roku.** Design notes only (`docs/roku.md`): SceneGraph client scope, HLS constraints, sync feasibility.

### Phase C (continued) — Fill every channel

C7. **Guide from the broadcast (PSIP EIT harvesting).** Package `internal/psip`.
- First verify what the relay reads: capture 30 s of what it tunes today and list PIDs. `/auto/v<ch>` is filtered by the device and probably lacks PSIP; `/auto/ch<freq>` is the unfiltered full mux. Learn each channel's frequency first (after any `/auto/v` tune, `get /tunerN/channel` reports it; a scan reports all), then move the relay's tune to `/auto/ch<freq>` (it already fans one frequency out to every subchannel, so demux programs ourselves), and confirm (relay smoke + two-screen sync, since this touches `internal/live`) that playback is unchanged and that `0x1FFB` plus the MGT-listed EIT/ETT PIDs arrive. Some stations use non-standard EIT PIDs; always follow the MGT. The HTTP stream has no PID filter, so filtering stays in our demux.
- Parser: MGT, TVCT/CVCT, STT, EIT, ETT, with Huffman string decoding and the genre descriptor. Table-driven tests on a captured sample filtered to PSIP PIDs only (keep `testdata` small: a few hundred KB).
- **Passive harvesting:** whenever a frequency is tuned for any reason, feed its PSIP to the harvester at no tuner cost.
- **Idle scan:** when no tuner is in use and no recording starts within 30 minutes, tune each frequency whose listings are missing or ending within 12 h, dwell 30-45 s, release (`/tunerN/channel none`). Preempt instantly: a watch or recording request cancels the scan before it tunes. At most one scan per frequency every 6 h, none between the user's quiet hours if set. It also learns every channel's frequency (fixes the 14.x note in B1).
- **Merge:** PSIP becomes a guide source with the lowest priority (user XMLTV / Schedules Direct > SiliconDust > PSIP), filling only channels and time ranges the others leave empty. Match by major.minor. Normalize ALL-CAPS titles to title case for display (keep the original). ETT text becomes the description. Genre feeds categories and sports matching (D2).
- Diagnostics shows per-channel guide source and depth, and the last scan per frequency. ADR 0006 (broadcast guide).
- **Accept:** ≥ 25 of 27 channels show a current and next listing on the real lineup (list any exceptions and why); guide depth per channel logged; a test proves a watch request preempts a scan; zero scan tunes while Unraid or the dev server has a viewer or recording.

C8. **Guide depth and freshness.** With C7, show honest depth ("Listings through Thursday"); the guide scrolls as far as any source goes; when a channel's data ends, its row says so instead of "No listing". Program sheet shows which source a listing came from (in the Stream Info style, not on every cell). **Accept:** screenshots; test for merge boundaries.

C9. **Antenna and signal tools.** A Settings > Tuners screen: per-channel signal strength, SNR quality, and symbol quality from `/tunerN/status` for channels on a tuned frequency; a "Check all channels" run that uses idle tuners the same way as C7 (preemptible). Show a simple verdict per channel (Great / OK / Weak / Lost) and a tip for weak ones in the copy voice. **Accept:** real readings from the DUO; the run yields to viewers.

C7b. **Every string a station sends.** Implement A/65 Annex C Huffman decoding (compression types 1 and 2, both title and program-description tables) in `internal/psip/text.go`, plus multi-language strings (prefer the device locale, then `eng`). Put a table-driven test on the Annex C examples, and on a real compressed string if any station on the DUO sends one. Also handle mode `0x3F` (UTF-16) and ISO-8859 modes where the spec allows them. **Accept:** no event decodes to an empty title while its ETT or EIT has bytes; test names in the tick.

S8b. **Apple setup, for real.** Replace `apple/App/Shared/SetupWizard.swift` with the same flow as the web wizard. Discovery results arrive live over the event socket, with one-tap Add, Look harder, and Enter an address. Then a channel scan with live progress, Add a playlist (URL or Xtream, with the Continuity keyboard on tvOS), Free channels, favorites for the big four chosen by **network affiliation**, and the recordings volume check from the S9 doctor. Pick the affiliation the same way on web and Apple: from the guide (XMLTV `display-name` or the station's network), then from a call-sign table the server exposes. The server decides; the clients never guess from the name. Setup reruns from Settings. **Accept:** a fresh server (fake tuner, empty catalog) gets set up from the Apple TV simulator and from the iPhone simulator with zero typing, to a playing channel, in < 90 s. Screenshots of every step: `docs/screenshots/s8b-*`.

R8. **Lint in CI matches `make check`.** After `server.go` is gofmt-clean, add `test -z "$(gofmt -l server)"` to the Go job in `.github/workflows/ci.yml`. **Accept:** CI green with the step.

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

Every Apple task below ends with a TestFlight build (`scripts/testflight.sh`). The phase ends with App Store update 1.3 (0.1d rule 2).

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
**Accept (phase):** each feature screenshotted on the simulator, builds green on Xcode 26 and 27, TestFlight build uploaded.

### Phase G — Relay and infrastructure depth

G5. jellyfin-ffmpeg in the Docker image (broad hardware acceleration + AC-4); detect capabilities at startup; Diagnostics shows them.
G4. HEVC renditions for Apple devices with hardware encoders (VAAPI `hevc_vaapi`, QSV, VideoToolbox, NVENC).
G6. ATSC 3.0: detect 3.0 channels, HEVC copy + AC-4 to AAC/E-AC-3, mark DRM channels "Protected" and hide them by default. Test against sample files.
G7. (Moved to S3.)
G2. ABR ladder with aligned segments for remote and cellular clients (one ffmpeg, several outputs); keep independent single renditions on the LAN.
G3. LL-HLS packager (ADR 0007): parts ~330 ms, `EXT-X-PART`, `EXT-X-PRELOAD-HINT`, blocking reload. Target glass-to-glass < 3 s on the LAN. Classic HLS stays default until it is proven.
G8. Metrics and logging: `slog` everywhere, per-feed stats, optional Prometheus `/metrics`, a log viewer in Diagnostics.
G9. Performance on Unraid: pprof under 4 renditions + a recording + a mosaic; ffmpeg thread tuning; memory caps.
G10. HDHomeRun firmware and health surface (read-only: version, tuner status, lock; never install firmware from the app).
G11. (Moved to N1.)

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
K3. Apple: Swift Testing for BroadwaveKit (SyncEngine with a fake clock, discovery parsing, AppStore, caching from R3); XCUITest smoke for launch, connect, guide, player, multiview on iOS and tvOS.
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
S0 → S1 → S2 → S3 → S4 → S5 → S6 → S7 → S8 → S9 → S10 → (tag + deploy) →
C7 → C8 → C9 → (tag + deploy) →
K1 → P1 → P2 → R8 → **U1** →
PB9 → PB2 → PB3 → PB3b → PB5 → PB6 → PB4 → PB7 → PB7b → PB8 → PB5c → (tag + deploy + TestFlight) →
C7b → S8b → HOME1 → HOME2 → HOME3 → (tag + deploy + TestFlight) →
MV1 → MV2 → MV3 → MV4 → MV5 → MV6 → AP1 → AP2 → AP3 → AS2 → AS3 → (tag + deploy + TestFlight + **App Store update 1.1 = AS4**) →
OPS1 → OPS2 → OPS3 → LEGAL1 → AS6 → (tag + deploy) →
HW1 → HW2 → HW3 → HW4 → HW5 → (tag + deploy) →
P2b → P3 → P4 → P5 → P6 → P7 → P8 → G1 → (tag + deploy + TestFlight) →
F1 → F2 → F3 → F4 → F5 → F6 → F7 → (tag + deploy + TestFlight + **App Store update 1.2**) →
E1 … E9 → (tag + deploy) →
D7 → D8 → B5 → (tag + deploy) →
I1 … I10 → (tag + deploy + TestFlight + **App Store update 1.3**) →
G5 → G4 → G6 → G2 → G3 → G8 → G9 → G10 → (tag + deploy) →
LEGAL2 → H1 → H2 → H3 → H4 → (tag + deploy + TestFlight + **App Store update 1.4**) →
N1 → N2 → N3 → N4 → N5 → N6 → N7 → (tag + deploy) →
J1 … J7 → K2 … K6 → L1 → L2 → L3 → AS5 → L4 → OPS5 → L5 → (**App Store update 2.0**) → M1 … M5.

AS1 runs continuously at the start of every round. Each phase ends with the product review in 0.1d rule 4.

---

## 5. Definition of spectacular (final acceptance)

- A new user installs from the Unraid template or `docker run`, and Broadwave finds their HDHomeRun (or other tuner, emulator, or Channels DVR server) by itself. Setup takes under 90 seconds with no typing for an HDHomeRun, and they see a full, art-rich guide for every channel their antenna gets, including channels no online guide lists.
- Any M3U, Xtream, tvheadend, emulator, or free-channel source that works in Channels DVR works here, with its guide, logos, numbering, and stream limits.
- The iPhone and Apple TV apps find the server instantly, can do the whole setup and administration, recover from a server restart or upgrade on their own, and never break on a server update (contract tests in CI).
- Channels, Plex, Jellyfin, Emby, and IPTV players on Fire TV, Android TV, and smart TVs can use Broadwave as their tuner and guide.
- The app opens instantly with real content; channel changes feel instant.
- Two games play side by side (web, Apple TV, iPad), in sync with each other and every other screen; audio follows focus; Game Switcher catches the big moments.
- Recordings of games end when the game ends; commercials skip reliably; followed teams record automatically.
- The Apple TV app feels native (Top Shelf, focus, remote gestures, info panels, frame-rate matching); the iPhone app has widgets, Live Activities, Siri, PiP, and the mini player.
- Everything runs on Unraid with hardware encoding, survives restarts, and a 24 h soak shows no leaks.
- `main` is green; images are published per phase; iOS and tvOS builds are in TestFlight; the Community Apps submission is in (or approved).
- Every OTA format (1080i, 720p, 480i, H.264 subchannels, film cadence) plays at its true motion rate on the GPU, measured, with 0 dropped frames after warm-up on web, iPhone, and Apple TV.
- A stranger's house is found by itself: tuners, servers, and screens appear in setup with one action each, on hardware we never owned (HW matrix).
- Broadwave is live on the App Store for iPhone, iPad, and Apple TV, with a demo mode anyone can try. Every milestone update is approved, and a public TestFlight is open.
- The server tells people when an update is out, backs itself up, and exports a support bundle without secrets. Licenses, notices, attributions, and the privacy policy are accurate.
- Every line in `PROGRESS.md` is ticked with its evidence (0.1a) or recorded in `BLOCKERS.md` with a clear reason.
- `docs/parity.md` shows the Apple apps doing everything the web app does.
