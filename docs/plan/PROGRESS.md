# Progress

Tick items as they are verified and committed, in the same commit as the work (`- [x] R3 ... (commit abc1234)`). Keep notes short. Order of work is section 4 of `MASTER_PLAN.md`, not the order of this file.

## Resume here
- Current task: **MV3** tuner-aware add-channel picker. App Review unchanged at 2026-09-25 16:06Z: tvOS 1.0 REJECTED (Guideline 2.1), iOS 1.0 WAITING_FOR_REVIEW. The owner sends the reply.
- Lane ledger: P2b merged `4a411d3`. LEGAL1 merged `0743bc6`. OPS3 merged `02554ea`. HW1 merged `8a815e8`, review fix `4c04373`. MV2 merged `cc726eb`. P2c merged `086265d`. OPS1 merged `a9abe62` (CI green). AS6 is ready in `…/706f9af145de` (registry plus ADR 0011, ESPN stays the default, no second client). The parent writes its BLOCKERS question; do not take a PROGRESS edit. G10 is ready in `…/707af1c4a73f` (`GET /api/v1/devices/health`, Diagnostics row, fake-tuner test). Take only the health files. Drop its copy of the update notifier, lab logs, and any PROGRESS edit. No lanes are running.
- Production `Broadwave` `:8477` is `v0.8.0`. Staging has the MV1 web bundle. Next recording Jeopardy 2026-09-25 20:00 UTC. Lane A is **MV3**.
- App Review: tvOS 1.0 REJECTED (Guideline 2.1), iOS 1.0 WAITING_FOR_REVIEW, checked 2026-09-25 16:06Z (AS1). Check it at the start of every round.
- Production `Broadwave` on TUS `:8477` is v0.8.0. Staging `Broadwave-Staging` on `:8490` is `v0.8.0-4-g6545f65` (encoder h264_vaapi). Next recording: Jeopardy, 2026-09-25 20:00 UTC.
- The repo is `~/Projects/active/broadwave`.

## Read first (reviews 2–4; details in MASTER_PLAN 0.1a–0.1d)
- **Order:** U1 → PB → PB10–PB16 → C7b, S8b, HOME → MV, AP, AS2, AS3 → **App Store update 1.1** → OPS, LEGAL1, AS6 → HW → P2b–P8 … (MASTER_PLAN section 4).
- **Evidence:** every tick lists the proof for each Accept bullet (numbers, test names, screenshot paths). A shortfall becomes a new task line.
- **Verify like a person on staging:** Chrome through `playwright-cli --browser=chrome` at three sizes, plus the "Broadwave Staging iPhone" and "Broadwave Staging TV" simulators. Measure the output. Production changes only in phase deploys.
- **Checks:** `make check` mirrors CI (gofmt, vet, tests, lint, API drift, relay smoke, retired-name guard). CI also builds the image. Run `docker build --target web` when web imports change.
- **Apple:** App Store screenshots are JPEG only (alpha PNGs got stuck). No broadcast TV or real logos in store assets. Updates follow MASTER_PLAN 0.1d rule 2.
- **Next migration is 0020.** Never edit an old one.

## Phase AS — App Store life
- [ ] AS1 Review follow-through every round (continuous; record state changes; handle rejections first). 2026-09-25: tvOS 1.0 is Guideline 2.1 Information Needed. Reply and shot list in `docs/appstore/review-2.1-reply.md`. Notes updated on both versions. The owner sends the reply and the recordings.
- [ ] AS2 "Try Broadwave" demo mode in the apps (DemoServer on loopback, bundled CC BY clips, attribution, airplane-mode test)
- [ ] AS3 `scripts/demo-lineup.sh` + `scripts/appstore-shots.sh` regenerate and upload every screenshot set (JPEG)
- [ ] AS4 App Store update 1.1 (demo mode, Apple art, multiview fixes, iPad multiview screenshots) submitted on both platforms
- [ ] AS5 Public TestFlight group with a public link on both platforms
- [ ] AS6 Sports data rights: ADR 0011, pluggable provider, owner's decision on the default

## Phase AP — Apple apps at web depth
- [ ] AP1 Program art on Apple guide cells, program sheet, search, sports cards, recordings (Home done in f22d4ab)
- [ ] AP2 iPhone portrait player with channel/program info and labeled controls
- [ ] AP3 R5 preview frames on Apple Home when a mux is tuned

## Phase OPS — Running it for years
- [x] OPS1 Update notifier (GitHub releases, opt-out, banners on web and Apple). `TestReleaseFeed` uses a fake releases document: a newer tag is "Broadwave 0.7 is available", the same and older tags stay quiet, opt-out makes zero requests, a failed fetch keeps the last good notice, and a notes link outside `https://github.com/wolfebase/broadwave/` falls back to the tag page (including a `..` path). `TestServerUpdateField`. `TestSettingsHideUpdateCache` checks GET settings for `updateVersion`, `updateNotesURL`, `updateMessage`, and `updateCheckedAt`. `TestUpdateSetting`. A local 0.7.0 server against the real GitHub feed returned version 0.8.0, notes `https://github.com/wolfebase/broadwave/releases/tag/v0.8.0`, message "Broadwave 0.8.0 is available". Chrome showed that banner, Release notes, and Not now at 390×844, 1440×900, and 1920×1080 (console: the existing preview-frame 404 only; `.evidence/ops1/web-phone.jpg`, `web-desktop.jpg`, `web-tv.jpg`). Settings shows Check for updates On and "Once a day. Nothing else is sent." (`.evidence/ops1/web-settings.jpg`). iPhone and Apple TV home show the same banner (`.evidence/ops1/iphone-home.jpg`, `tv-home.jpg`). Turning the check off sends nothing; turning it on checks now. A failed fetch retries in 15 minutes instead of waiting out the day. PRIVACY.md effective date is September 25, 2026. App Privacy stays Data Not Collected: the apps still talk only to the home server. `make check` green.
- [ ] OPS2 Nightly catalog backups with retention, pre-upgrade backup, restore from Settings
- [x] OPS3 Support bundle with redacted config. `GET /api/v1/support` returns a zip of versions, doctor, redacted config, and logs. `TestSupportBundleLeavesOutSecrets` stores an Xtream password and a `DeviceAuth` token, then checks the zip bytes, every entry, and the log ring contain neither (it does not look for the mask dots, which `net/url` percent-encodes). `TestMaskURLMatchesTheStoredForm`. Web Settings links the download (`copy.settings.support`). SUPPORT.md links the route. Sample zip `.evidence/ops3/broadwave-support.zip` (not committed). OpenAPI `downloadSupport`. No admin auth, same as backup. `make check` green.
- [ ] OPS3b Apple Settings downloads the support bundle, the way the web Settings button does.
- [x] OPS4 Retired-name guard `scripts/check-names.sh` in `make check` and CI (2026-09-24)
- [ ] OPS5 README with screenshots and installs; GitHub Pages site at wolfebase.github.io/broadwave

## Phase LEGAL — Licenses, notices, security
- [x] LEGAL1 NOTICE, ffmpeg obligations, CC BY credit, trademark-safe copy, `docs/legal.md`. `LICENSE` is the full Apache-2.0 text (the old file was the appendix only, so GitHub reported NOASSERTION). NOTICE lists the direct Go, npm, and Swift dependencies. Both Dockerfiles and `.github/workflows/release.yml` set `org.opencontainers.image.licenses=Apache-2.0 AND GPL-3.0-or-later`. The image source offer names jellyfin-ffmpeg7 7.1.4-3 and tag `v7.1.4-3` (`--enable-gpl`, `--enable-version3`). The GHCR Dockerfile copies LICENSE, NOTICE, and `docs/legal.md`. About shows "Blender Foundation films under CC BY" on web at 390×844, 1440×900, and 1920×1080 (Chrome, 0 console errors, `.evidence/legal1/web-phone.jpg`, `web-desktop.jpg`, `web-tv.jpg`), iPhone (`.evidence/legal1/iphone-about.jpg`), and Apple TV (`.evidence/legal1/tv-about.jpg`, sidebar collapsed, full sentence). README credit waits on `readme-launch` (do not edit README). `make check` green.
- [ ] LEGAL2 Threat model and security review before H2/H3; every finding fixed with tests

## Phase U — Staging and hotfix
- [x] U1 `MODE=staging` in `scripts/deploy-unraid.sh` + `scripts/staging-watch.sh`. Staging recreate left `Broadwave` alone (`docker inspect` name `/Broadwave-Staging`, args `-staging -bonjour=false`, `:8490`, identity "Broadwave Staging"). `staging-watch.sh 1 15 192.168.1.2:8477` printed `1280x720 59.94`, segment 2.002s, 0 decode errors, tuner released. The same against `:8490`.

## Phase PB — Playback
- [x] PB1 Progressive broadcasts keep every frame and are never bobbed; `-staging` flag. Staging on TUS iGPU: 4.1 1280x720 59.94 in 2.002 s segments, 0 decode errors (v0.5.0: 1920x1080 119.88 in 1.001 s); 9.1 1920x1080 59.94 unchanged; Chrome 0 dropped frames on both; iPhone and Apple TV sims played 4.1 and a 4.1+9.1 multiview (`docs/screenshots/pb-*`); `TestProgressive720pKeepsEveryFrame` (commit 8dac5dd)
- [x] PB2 First tune reads scan type from the mux (no interlaced guess on a fresh install). Staging with `field_order` cleared on 4.1: `scan type progressive for 4.1 in 800ms`, then 1280x720 59.94, segment 2.002s, 0 decode errors, 240 frames, tuner released. A sequence header can sit ~460 ms into the GOP, so the read waits up to 800 ms and returns when the header arrives. `TestScanTypeCapturedHeaders`, `TestScanTypeFFmpegHeaders`.
- [x] PB3 Scan-type matrix. Soft 3:2 (`repeat_first_field`) is field order `film` and plays at 24p (`fieldmatch,decimate` on the CPU, `scale_vaapi` after). `TestScanTypeMatrix` covers VAAPI, libx264, and VideoToolbox for 1080i, 720p, 480i, H.264 PAFF/MBAFF, and film. `TestScanMatrixPicture`: libx264 and VideoToolbox, 59.940 fps, 60 frames, idet 0/60, mpdecimate kept them. TUS VAAPI: 10s fixture 59.94 fps, 479 frames, 0 decode errors, idet 4/476; real 5.1 1920×1080 59.94, 0 decode errors, idet matches `bwdif` (209/103); real 4.1 stays 1280×720 59.94. Table in `docs/dev-lab.md`. Hard telecine idet is PB3b. 14.x and 480i were not on the air, so those rows are fixtures.
- [x] PB3b Hard telecine plays clean at 24p. `pullup` replaces `fieldmatch,decimate` (that graph was idet 23/0 on a 1s fixture). `TestScanMatrixPicture` telecine: libx264 and VideoToolbox, 23.976 fps, 23 frames, idet 0 interlaced / 23 progressive. Fixture is a top-field 3:2 weave of a moving vertical bar (`-top 1`). No header flag, so it stays on the field-rate path until the picture mode is film. Soft 3:2 is still `TestScanTypeCapturedHeaders`.
- [x] PB4 Honest 30→60. TUS, jellyfin-ffmpeg 7.1.4, 4 s of 1080p30 from channel 2 (`bwdif=send_frame`) at `-re`. Filter-only: duplicate 9.83% / 0.883×, blend 25.43% / 0.881×, framerate 19.79% / 0.881×. `mci` 0.09×. `vpp_qsv` failed (MFX -3). Blend with `h264_vaapi` was 41.68% and real time, and the invented frame ghosts (`docs/lab/pb4/blend-box.jpg`, `rate-box.jpg`; sharp duplicate `dup-box.jpg`). No VAAPI interpolator. Blend path removed. `TestSmoothDoesNotInventFrames`. ADR 0010. Apple TV Match Frame Rate stays on PB7 (no `AVDisplayCriteria` yet).
- [x] PB5 GPU decode on VAAPI (`-hwaccel vaapi -hwaccel_output_format vaapi`, no `hwupload` except film/`pullup`). jellyfin-ffmpeg 7.1.4-3 in the image (`ffmpeg version 7.1.4-Jellyfin` on staging). Encoder `-rc_mode VBR -profile:v high -bf 2 -low_power 0`. Apple transcodes are `hevc_vaapi` with `-tag:v hvc1` (`1080.copy.broadcast.hevc`). Staging 5.1: 1920×1080 59.94, 0 decode errors over 30 polls / 292 segments (~10 min), CPU 10.51% (25 s sample 10.83%; baseline was ~18%). HEVC `hvc1` 60000/1001, 0 decode errors, CPU 10.87%. Software fallback restarts the rendition on `libx264` if VAAPI ffmpeg exits within 8 s. VMAF bitrates are PB5b (no libvmaf in this ffmpeg). `TestHEVCRenditionUsesHVC1`, `TestScanTypeMatrix`, `TestProgressive720pKeepsEveryFrame`. Table in `docs/dev-lab.md`.
- [ ] PB5b Score the 14M 1080p60 and 8M 720p60 bitrates with libvmaf (≥ 95 LAN, ≥ 90 cellular). jellyfin-ffmpeg 7.1.4 has no libvmaf.
- [x] PB6 AC-3 passthrough and PMT audio pick. Staging channel 2 (5.1) with tvOS caps: `1080.copy.broadcast.hevc`, ffprobe `ac3` 6ch `5.1(side)`, tuner released. PMT on the full mux: 5.1 main pid 52 English, Spanish pid 53; 4.1 main pid 52 English, second track pid 53 described (no separate language tag, so it is not the filtered-stream pid 0x102). Web and Apple players pick Main, Second language, or Described video, plus Even volume off by default (`loudnorm` only when on, which re-encodes). Switching joins that rendition, so the picture reloads; gapless alternates stay on F2. `TestAudioTracksPMT`, `TestAudioTracksSameLanguageAlternate`, `TestAudioTracksISOAudioType`, `TestSourceForPicksSAP`, `TestRenditionMapsChosenPID`.
- [x] PB7 Player tuning. hls.js buffers follow the layout: phone 16s/30s back, desktop 24s/90s, TV 30s/120s, tiles 16s/20s (`liveHlsConfig`). AVPlayer forward buffer is 8s on the LAN and 12s on cellular; `preferredPeakBitRate` is 0 on the LAN and 4 Mb/s on cellular (`lanLiveHasNoPeakBitrateCap`, `cellularCapsThePeakBitrate`). tvOS sets `AVDisplayManager.preferredDisplayCriteria` from an SDR format description at the asset frame rate (59.94 until the track reports otherwise) and clears it when the player closes. Staging channel 1 (4.1), Chrome: desktop cold TTFF 7770 ms, 1 stall of 16 ms, 1280×720, buffer 24/90; phone cold TTFF 9320 ms, 0 stalls, dropped 5/217, buffer 16/30; TV warm TTFF 33 ms (rendition already up), 0 stalls, dropped 4/338, buffer 30/120. Console error is favicon 404 only. iPhone and Apple TV sims played 4.1 (`docs/screenshots/pb7-web-phone.jpg`, `pb7-web-desktop.jpg`, `pb7-web-tv.jpg`, `pb7-iphone.jpg`, `pb7-tv.jpg`). An hour-long stall count was not run.
- [x] PB7b Hour-long stall count on staging channel 1 (4.1). Web Chrome 01:55:53Z–02:55:59Z (60 min): TTFF 8535 ms, 1280×720, 0 `waiting` stalls (`dataset.stalls` never set), minute samples tracked wall clock within 25 ms through 02:50Z, dropped 684/215546, hls.js `bufferSeekOverHole` from the first sample (startup hole), console favicon 404 only; the 02:55:59Z sample was paused with currentTime 3596 s. iPhone TTFF 340 ms and Apple TV TTFF 550 ms: 41 one-minute beats, stalls 0, stall time 0 (`docs/lab/pb7b/`). The simulator apps were already gone at 21:50 (nothing to terminate, no crash report), so their logs stop at 21:38. Viewers stopped; tuners `ours:false`. Screenshots `docs/screenshots/pb7b-iphone.jpg`, `pb7b-tv.jpg`.
- [x] PB8 Stream panel. Staging channel 1. Web: MPEG2 1280×720 Progressive 59.94 in, 1280×720 59.94 h264_vaapi 10 Mb/s GPU out. Phone 0/12416 dropped, buffer 9.1s, Locked −14 ms (`pb8-web-phone.jpg`). Desktop after idle (panel opacity 1, dock 0): 1/704 dropped, buffer 8.4s, Locked −16 ms (`pb8-web-desktop.jpg`). TV Locked −11 ms, buffer 9.2s (`pb8-web-tv.jpg`). iPhone and Apple TV via `-BroadwaveStream`: hevc_vaapi 10 Mb/s GPU, dropped 0, buffer 9.5s/9.3s, Syncing −63 ms and −57 ms (`pb8-iphone.jpg`, `pb8-tv.jpg`). The earlier −49183 ms was real catch-up drift, not a unit bug. `TestStreamFacts`, `TestMuxPictureKeepsTheWindowOpen`, `TestPictureFacts`, `TestPictureFactsFFmpeg`. One Apple HEVC start fell back (PB5c).
- [x] PB5c Keep Apple HEVC on the iGPU. The VAAPI packed-header buffer is 1024 bytes. An OTA A/53 caption SEI overflows it (`Access unit too large: 8192 < N`, errno 28) and `hevc_vaapi` exits, so the rendition restarted on libx265. `-sei 0` skips that header. The software fallback is unchanged (`TestStreamFacts` still expects libx265). `TestHEVCRenditionUsesHVC1`. Staging, three `1080.copy.broadcast.hevc` starts, no libx265: channel 1 (4.1) panel `hevc_vaapi` GPU 1280×720 59.94, and at 30s `hvc1` 360 frames, 0 decode errors, process still `-sei 0`; a second channel 1 start the same; channel 2 (5.1) panel `hevc_vaapi` GPU 1920×1080 59.94 interlaced, still `-sei 0` at 12s. Logs: 0 "Access unit too large", 0 libx265, 0 "Encode failed". Tuners released. Note in `docs/dev-lab.md`.
- [x] Phase PB deploy: tag v0.7.1, Unraid smoke on jellyfin-ffmpeg 7.1.4, TestFlight build 147. Production `/api/v1/server` is `v0.7.1`, encoder `h264_vaapi`. Channel 1: 1280x720 59.94, segment 2.002s, 240 frames, 0 decode errors, ffmpeg command `h264_vaapi -sei 0`, tuner released. `v0.7.0` had fallen back to software after "Access unit too large". Catalog backups `broadwave-20260925-003303.db` (before 0.7.0) and `broadwave-20260925-005221.db` (before 0.7.1). Five recordings kept. Log in UNRAID_LOG. TestFlight 147 uploaded for iOS and tvOS (`Uploaded Broadwave`, `Uploaded BroadwaveTV`). GitHub releases exist for v0.1.0 through v0.7.1.
- [x] PB9 `scripts/picture-lab.sh` on TUS at real time. Staging capture of channel 1 (12 s, tuner released, no recording within 20 min). `bw-lab-field-deint` 1280x720 119.88 fps, 959 frames, 0 decode errors, 0.993x, CPU 21.34%. `bw-lab-progressive-scale` 1280x720 59.94 fps, 479 frames, 0 decode errors, 0.993x, CPU 19.96%. VMAF n/a: image ffmpeg has no libvmaf (PB5). Table `docs/lab/picture-lab.md`. Stills `docs/lab/runs/latest/stills/`.
- [x] PB review (2026-09-25). Every screen on staging v0.7.1: web at 390×844, 1440×900, and 1920×1080, plus iPhone, iPad, and Apple TV. Shots in `.evidence/pb-review/` (not committed). Channel 1 played on all three web sizes (1280×720, readyState 4, Synced/Live; phone dropped 9/1463, desktop 20/1768, TV 446/1313) and on the three simulators with a real picture. Desktop 2-up of channels 1 and 3 showed both pictures at 960×540. Tuners `ours:false` after. `make check` green. Fixes in this commit: tvOS guide channel column (4.1 WDAF-DT, 5.1 KCTVDT1, 9.1 KMBC-HD visible in `.evidence/pb-review/fix-tv-guide-left.jpg`), favicon (Vite `GET /favicon.ico` 200), phone recordings title and buttons (`.evidence/pb-review/fix-web-phone-recordings.jpg`), sports chips and card actions (`.evidence/pb-review/fix-web-phone-sports.jpg`), tuner rows read "1 Free Idle", phone player title "The Goldbergs" on its own line (`.evidence/pb-review/fix-web-phone-watch.jpg`). Web fixes were checked with Vite against the staging API; the staging binary is still v0.7.1 until the next staging deploy.
- [x] PB10 Recordings and library playback use the real scan type. `fileGraph` sets Progressive from the recording's headers, and uses the channel's stored scan only when the file has no header yet. `TestProgressiveRecordingStays720p60`: 1280×720, 59.940 fps, 60 frames, idet clean, mpdecimate kept them. `TestProgressiveRecordingKeepsNativeRate`: libx264, VAAPI, and VideoToolbox skip bwdif, deinterlace, and fps; an interlaced recording still uses `bwdif=mode=send_field` and `fps=60000/1001`; film uses pullup at 24000/1001; the playlist stamp changes so a bobbed cache is not reused. `TestRecordingOrderPrefersTheFile`, `TestFileScanReadsProgressiveHeaders`. Real file `20260923_145831_4.1_WDAF-DT.ts`: ffprobe 1280×720 progressive 60000/1001, `fileScanOrder` progressive on the first 4 MB. Staging `:8490` healthy with `h264_vaapi`; the container does not mount recordings, so that file was not played there. `make check` green.
- [x] PB11 A sequence header that misses the 800 ms window still corrects the running graph, and a stored "progressive" does not skip the scan that finds film. `TestProbeRebuildsTheRunningGraph`: an interlaced rendition rebuilt off `bwdif` when the probe said progressive (2 viewers kept); a later film header rebuilt onto `pullup` and stored `tt`. `TestStoredProgressiveStillScansForFilm`, `TestLateHeaderAppliesAfterTheWindow`, `TestProbeDoesNotOverrideTheHeader`. Staging channel 1 (4.1) was scanned (`scan type progressive for 4.1 in 500ms`), 1280x720 59.94, segment 2.002s, 240 frames, 0 decode errors, tuner released. `make check` green.
- [x] PB12 Unscanned H.264, including HLS, is not assumed interlaced. Empty field order keeps the source rate; Lace is only a stored tt/bb/tb/bt. `TestUnscannedH264KeepsItsRate`: 30p 30.000 fps, 30 frames; 60p 60.000 fps, 60 frames. `TestUnscannedH264ProbeRebuilds` starts without bwdif and rebuilds onto `bwdif=mode=send_field` when the probe says tt. `TestInputProbeStoresProgressive`, `TestHLSProbeTargetFindsTheSegment`. Staging channel 1 stayed 1280x720 59.94, segment 2.002s, 240 frames, 0 decode errors, tuner released. Temporary 320×180 playlists on `:8490` (removed after): 60p output 60/1 and 240 frames in two segments, panel 10M and scan progressive; 30p output 30/1 and 120 frames, same panel. Both stored `field_order=progressive`. Tuners `ours:false` on `:8477` and `:8490`. `make check` green.
- [x] PB13 A PMT that spans TS packets keeps every audio elementary stream. `sections()` appends the pointer-field tail onto the open section before the next one starts (ISO/IEC 13818-1). `TestSplitPMTKeepsTheTailAudio`: 387-byte PMT, 183 bytes plus one full continuation, last ES only in the pointer field (described pid 0x103). The old parser returned English + Spanish and dropped Described video. Staging `:8490`: 4.1 `main pid 52 English, described pid 53 Described video`, progressive in 62 ms; 5.1 `main pid 52 English, language pid 53 Spanish`, `tt` in 5 ms. Tuners `ours:false` on `:8477` and `:8490`. `make check` green.
- [x] PB14 Apple TV display criteria follow the asset's frame size and rate, and clear when the player closes. `matchingDisplay` replaces an early 59.94 with 23.976 and 1920×1080 with 1280×720; a blank sample does not invent 1080p60 (`aLateFilmRateReplaces59_94`, `a720pPictureReplaces1080p`, `aBlankSampleDoesNotInvent1080p60`). Live HLS reports `nominalFrameRate` 0, so a server hint fills only that missing rate and does not put the picture back to 1080p (`aServerHintFillsTheRateAndKeepsTheAssetSize`). Staging Apple TV sim, channel 1: `broadwave display 1280x720 59.94`. Criteria are cleared when the item is removed and in `dismantleUIViewController`. Tuners `ours:false`. iOS and tvOS builds succeeded. `make check` green.
- [x] PB15 A VAAPI encode that dies after 8 seconds restarts cleanly. One restart, in an empty directory. A second death, or a clean exit, drops the rendition and releases the tuner unless a recording or another rendition still needs it. An early VAAPI death comes back on libx264 (libx265 for HEVC) and does not keep the old init.mp4. `TestEarlyVAAPIFallbackReplacesInit`, `TestEarlyHEVCFallbackUsesLibx265`, `TestLateVAAPIDeathRebuildsWithoutTheOldInit`, `TestSecondDeathReleasesTheTuner`, `TestDeathDuringARecordingReleasesTheRenditionOnly`, `TestCleanExitReleasesTheTuner`, `TestStoppedRenditionDoesNotRestart`. Staging channel 1 (4.1), `1080.aac2.broadcast`: killed h264_vaapi at 10.462s, log `restarted`, new process still h264_vaapi, init.mp4 inode 5721638→5720952 and sha256 c2656d44…→7dcf9f30…, playlist returned `seg00001.m4s`, tuner stayed ours with 1 viewer; a second kill logged `released after 5.776s` and both servers went `ours:false`. Killed h264_vaapi at 2.648s after a 0-byte init (inode 5722730, sha256 e3b0c442…): log `restarted`, process `-c:v libx264`, session encoder libx264 decode cpu, new init inode 5723952 size 1370 sha256 913d26c0…. Idle stop then both servers `ours:false`. A restart also clears the playlist timestamp cache (`TestRestartDropsStaleSegmentTimes`), and a stop does not signal a process that has already been waited (`TestStopSkipsAProcessThatAlreadyExited`). `make check` green.
- [x] PB16 A second same-language complete main stays Main, and passthrough keeps the 5.1. The PMT on 38.1 and 41.1 copies one AC-3 descriptor onto both English streams, so mix width and bsmod come from the frame (two agreeing headers; one false sync does not count). `TestSecondCompleteMainStaysMain`, `TestMeasuredSurroundBeatsAnEarlierStereo` (stereo listed first, 5.1 wins), `TestFrameMarksDescribedVideo`, `TestOneFalseAC3SyncDoesNotCount`, `TestAudioTracksSameLanguageAlternate` (an unlabeled extra English is still described). Staging channel 24 (41.1): `main pid 52 English 6ch, main pid 53 English 2ch` in 61 ms. tvOS caps, `1080.copy.broadcast.hevc`: ac3 6ch `5.1(side)` 384 kb/s English, HEVC 1920×1080 59.94. A Spanish track whose descriptor says visually impaired but whose frames are a complete main is Second language (4.1, 9.1, 29.1 captures). Tuners `ours:false` on `:8477` and `:8490`. `make check` green.

## Phase HOME — The house sets itself up
- [x] HOME1 Your home. `GET /api/v1/home` scans the local subnet and does not add anything (`TestHomeListsTunersServersAndScreens` device count unchanged). Staging at 11:27Z found HDHomeRun CONNECT DUO `10611B4C` at 192.168.1.252 (Added, 2 tuners), Plex "TUS" at 192.168.1.2 (Use as tuner; `binhex-plexpass` on that host), Channels "tus" at 192.168.1.2 (Use as tuner; `channelsdvr_intel` on that host, `_channels_dvr._tcp`), Chromecast "Kitchen Display", and AirPlay Bedroom, Entertainment Room, Living Room, MacBook Pro 16", and Q-Series Soundbar. Apps announce with `here`: Broadwave Staging TV, Broadwave Staging iPhone, and This browser, each "On this server". No Jellyfin, Emby, Fire TV, or MediaRenderer answered; those parsers are `TestClassifySSDPKeepsScreensAndServers`, `TestParsePlexAndJellyfin`, `TestAssembleOneActionEach`, `TestUnescapeDNSNames`, `TestHereAnnouncesAScreen`. A public address is dropped (`TestOnLANDropsThePublicInternet`). Use as tuner on staging (emulator off) says "Turn on Act as an HDHomeRun, then add 192.168.1.2:8478." Shots `.evidence/home1/web-desktop-settings.png`, `web-phone-settings.png`, `web-tv-settings.png`, `web-desktop-setup.png`, `tv-settings.jpg`, `iphone-settings.jpg`. Console errors 0. Tuners `ours:false` on both. `make check` green.
- [x] HOME2 Setup finishes itself. `POST /api/v1/setup/finish` runs the lineup, SiliconDust listings, the recordings folder, big-four favorites, a one-second 1080p60 encoder test, and one antenna check, then a Ready line. A fresh lineup has no stored frequency, so the check tunes one channel per mux and learns the rest (`TestNextSignalChannelLearnsAFrequency`). `TestSetupFinishRunsItself`, `TestSignalSummaryAndReadyLine`, `TestFormatEncoderLine`, `TestBenchEncoderParsesAScript`. Empty catalog on staging, zero typing. Web: Ready at 26.6 s (12:05:40.651Z to 12:06:07.286Z), first frame 10.3 s after Watch, 1280×720 readyState 4, desktop dropped 60/1407, console 0. The recorded goto-to-frame was 87.1 s because Watch was clicked late; a click when it appeared is about 37 s. Ready: "Ready: 8 channels, guide for 8, 2 tuners, Intel GPU". Signal: "7 channels great, 1 ok." Picture: "Intel GPU found: 1080p60 at 5.4x real time." iPhone: launch 12:17:26Z to Ready 12:18:23Z (57 s) with the same signal line and Watch on screen; the debug walk then played FOX 4 (tuner in 5 s, `.evidence/home2/iphone-watch.jpg`). Apple TV: launch 12:21:31Z to a held tuner and Ready at 12:21:56Z (25 s), picture on screen by 12:22:04Z (`.evidence/home2/tv-watch.jpg`, Live, 4.1 WDAF-DT). Shots `.evidence/home2/`. The Mac blocked synthetic clicks, so Apple playback used the debug `BroadwaveSetup=walk` path; the Watch button was on screen. Tuners `ours:false` after. Staging catalog restored. `make check` green.
- [ ] A setup channel scan that is still running after 40s is left on the tuner (`setup_finish.go` breaks without `scan=abort`). The household lineup already has channels, so v0.8.0 does not take that branch.
- [ ] A signal reading and the field-order probe can both drop the same feed. If a recording takes that frequency in the gap, the second drop sets the tuner to none. Check the feed is still the one held before releasing.
- [ ] A live HLS field-order read shares one 8s budget with the playlist probe, so the segment probe is often cancelled. Antenna tunes do not use this path.
- [ ] `normCall` strips `DT2` and `DT3`, so setup can star a subchannel such as WDAF-DT2 as FOX. Stop stripping numbered DT suffixes.
- [x] Web guide titles sit in the channel column on 38.1, 39.7, and 62.1. The title inset was capped at `width - 120`, so a show that started off screen slid back under the name. It now starts at the visible edge, and the channel column paints above the cells. Staging after the fix, Chrome: 0 titles with a left edge inside the column at 1440×900 (column right 237) and 1920×1080 (column right 381). Shot `.evidence/home-review/guide-fix-desktop.jpg`. iPhone guide did not show it.
- [ ] Apple TV sidebar stays open over Home, Sports, and Recordings (`.evidence/home-review/tv-home.jpg`, `tv-sports.jpg`, `tv-recordings.jpg`). Guide and Settings collapse it.
- [ ] Apple TV guide time ruler is clipped at the channel column (`tv-guide.jpg`).
- [ ] The phone tab bar covers the next row on iPhone Home and Settings and on the web phone Settings (`.evidence/home-review/iphone-home.jpg`, `iphone-settings.jpg`, `web-phone-settings.jpg`).
- [ ] The Home hero scrim is too light, so the title sits on the picture (`.evidence/home-review/web-desktop-home.jpg`, `web-tv-home.jpg`).
- [x] Diagnostics says the tuner stopped answering while a tune is on and the signal is full (`.evidence/home-review/web-desktop-diagnostics.jpg`). LastSeen older than 3 minutes set the note even during a tune. A busy hub is not quiet (`TestATuneMeansTheTunerIsAnswering`). An idle tuner last seen 10 minutes ago still is.
- [x] Phase HOME deploy: tag v0.8.0 (`d92b021`), GitHub release, Unraid image `ghcr.io/wolfebase/broadwave:0.8.0`. Production `/api/v1/server` is `v0.8.0`, encoder `h264_vaapi`. Channel 1: 1280x720 59.94, segment 2.002s, 240 frames, 0 decode errors, tuner released. Catalog backup `broadwave-20260925-084110.db`. Five recordings kept. Staging setup finish did not tune (`ours:false` throughout; signal "7 channels great, 1 ok" from stored readings). TestFlight 171 uploaded for iOS and tvOS. The release workflow failed afterward on the Actions cache export; the amd64 and arm64 manifests were already pushed. Log in UNRAID_LOG. Phase review on the pre-tag staging binary: desktop watch 1280×720, readyState 4, dropped 68/1787, console clean (`.evidence/home-review/`).
- [x] HOME3 A device that shows up later gets one banner. The first network scan is the baseline (`TestLaterArrivalBannersOnce`: a saved tuner and this browser do not seed it; a catalog-only look stays quiet). A speaker the first scan misses stays quiet (`TestAMissedSpeakerStaysQuietAtStartup`). One new tuner is one event, and the same tuner on a second id is not another (`TestTunerAnsweredTwoWaysIsOneBanner`). A Channels server that also answers on a bridge address is one place (`TestSameServerOnTwoAddressesIsOne`). `TestLaterArrivalBannersOnce` in httpapi writes one `home` event, "New tuner found: HDHomeRun FLEX 4K. Add it?". Staging: a fake HDHomeRun started after the baseline, log `home: New tuner found: HDHomeRun FLEX 4K. Add it?` once, and the next scans added none. Web Chrome showed that line at 1440×900, 390×844, and 1920×1080 (`.evidence/home3/web-desktop.png`, `web-phone.png`, `web-tv.png`); Your home opened Settings, Not now cleared it. Console errors were six existing 404s for preview frames and posters, no banner errors. iPhone and Apple TV showed the home banner over the player (`.evidence/home3/iphone.jpg`, `tv.jpg`). Tuners `ours:false` on both. `make check` green.

## Phase MV — Multiview everywhere
- [x] MV1 16:9 tile geometry for every layout × platform (no black bands inside tiles). Web: each tile is the largest 16:9 box in its cell, video `object-fit: cover`. Chrome on staging, channels 1 and 3: every layout at 390×844, 1440×900, and 1920×1080 measured ratio 1.778, video box matched the tile within 0 px, readyState 4. Desktop 2up 707×398, dropped 6/379 and 4/340. Phone 2up 370×208, dropped 0. TV-size 2up 947×533. Console 0. Shots `.evidence/mv1/web-*`. Apple: `TileGeometry` and `resizeAspectFill`. iPhone portrait, iPad, and Apple TV played 4.1 and 9.1 with the picture filling the tile and the layout buttons below it (`.evidence/mv1/iphone-*`, `ipad-*`, `tv-*`). Quad and 1+3 show two tiles because the DUO has two tuners and those channels are different frequencies. iPhone landscape stayed 1206×2622: `simctl ui` has no orientation, and `-BroadwaveLandscape` did not rotate the framebuffer. Tuners released, `ours:false` on `:8477` and `:8490`. `make check` green.
- [ ] iPhone landscape multiview screenshots. The staging iPhone stayed portrait (1206×2622).
- [x] MV2 tvOS focus labels and remote gestures, proven by XCUITest. A tile shows "Focused" while the remote is on it, and "Sound on" after click. Swipe right moves that focus to the other tile. Play/Pause shows "Paused". Long-press opens Make big, Record, Remove, and Full screen. Menu leaves multiview. `BroadwaveTVUITests` `testSwipeClickPauseMenu` and `testLongPressOpensTheTileMenu` passed on Apple TV 4K (3rd generation) simulator `4AA9998C-4556-43C7-A36E-5F46C6805C2B`, offline, with `-BroadwaveMultiviewTest`. No tuner.
- [ ] MV3 Tuner-aware add-channel picker with 1/2/4/8 tuners and several devices
- [ ] MV4 Focused tile at 60 fps, others 30; no drops after warm-up; hint hides once a tile has sound
- [ ] MV5 Tiles and other screens within 50 ms; audio focus to AirPlay; saved sets; Watch together
- [ ] MV6 Screenshot + 10 s recording per layout × platform; parity rows complete

## Phase HW — Every device, not just this DUO
- [x] HW1 Fake fleet (HDHR3, DUO/QUATRO, FLEX, FLEX 4K ATSC 3.0, PRIME, EXTEND, SCRIBE, SERVIO, old firmware, 1–8 tuners) with a table test; `docs/hardware.md`. `TestProfileFleet` runs discovery, scan, tune, multiview plan, recording plan, and failover for every named profile. SERVIO has no tuners: scan is refused, status is empty, failover is not run, and `recorded_files.json` returns News / At 6 / news.ts. FLEX 4K: 104.1 is HEVC+AC-4 and tuner 2 returns 806; 105.1 returns 811. PRIME copy-once and copy-never return 811 and the client marks both protected. EXTEND `transcode=mobile` is an AVC+AAC marker; `transcode=nope` returns 802. Old firmware has no DeviceAuth and no VideoCodec. Tuner counts 1–8 are capped at 8. `TestTuneStatusAndBusy` still locks 8vsb on the original fake. `TestControlTuneStreamsOnThatTuner`: a CONNECT DUO control tune of 4.1 then `/tuner0/ch593000000` returns MPEG2, the way the live hub probes and then opens the mux. `TestNoneStopsTheStreamAndTheNextTuneStays`: `vchannel none` ends that stream, and the next tune of 5.1 is still on tuner 0. `go test -race ./server/internal/hdhr/fake/` is clean. The first `lineup_status.json` after `lineup.post` is still scanning (`api/fixtures/scan-status.json` found 0); the original fake stays tunable while that scan is open, so `TestContractFixtures` watch is still `tuner returned 404 Not Found: no sample`. Table in `docs/hardware.md`: each row is verified on a fake, and the models this house has not read are listed as untested. CONNECT DUO discover, lineup, and status were already read on the real device (`docs/dev-lab.md`); this task did not retune it. `make check` green.
- [ ] HW2 Multi-device pools, 3.0 tuner reservation, mid-stream failover < 5 s
- [ ] HW3 tvheadend, Threadfin, ErsatzTV, Dispatcharr, Channels DVR, and Plex verified for real
- [ ] HW4 Server hardware matrix (Intel, AMD, NVIDIA, Apple, software/arm64) with a startup self-benchmark
- [ ] HW5 Client matrix table tests (Apple TV HD, Apple TV 4K, older iPhones, Safari, Chrome, Firefox)

## Phase R — Repair and consolidate
- [x] R1 CI green on all four jobs at 0984064 (lint fixes e411da1, Xcode 26 guard 0984064); `make check`; CI section in the dev-loop skill
- [x] R2 v0.2.0 tagged with CHANGELOG, deployed to Unraid via GHCR, A–D features checked on the LAN (tag v0.2.0, log in UNRAID_LOG)
- [x] R3 Instant boot: cached shell, windowed airings, gzip/brotli + ETag, Apple snapshot cache (< 300 ms warm)
- [x] R4 Artwork layouts by size and aspect; no upscaling past 1.25x
- [x] R5 Live preview frames from tuned muxes (no extra tunes). Unraid CPU sample is the phase R deploy line.
- [x] R6 Apple screenshots of B–D on iPhone, iPad, and Apple TV (`docs/screenshots/r6-*`). iPhone multiview stays 2-up. Quad layout is on iPad and Apple TV; those tiles timed out when :8477 flapped, and the 2-up shots are the live picture. Scoreboard was empty, so no score was on screen.
- [x] R7 Code review of run 1. `go test -race ./server/...` clean. Hiding a score no longer edits the cached board. Multiview waits for the tuner plan before a tile starts, on web and Apple.
- [x] R8 gofmt check in the CI Go job; `gofmt -l server` is empty at 8843648
- [x] Phase R deploy: tag, Unraid smoke, and preview-frame CPU under 3% of one core per mux

## Phase S — Every source, found automatically
- [x] S0 Research sources and discovery; ADR 0009; `docs/sources.md` support matrix
- [x] S1 One source model (kinds, capabilities, masked credentials, stable channel ids) with a lossless migration
- [x] S2 Auto-find engine: broadcast, unicast, SSDP, mDNS, Look harder, cloud last resort; the DUO answered on the LAN and from a bridged container; a new address keeps the channel id (commit 4c3da1c)
- [x] S3 HDHomeRun family: two devices with failover, a tuner held for a recording, copy-protected channels hidden, scan on a fake tuner and a finished scan on the DUO. FLEX ATSC 3.0 stays on G6. EXTEND adds transcode=mobile when the server encoder is software. SCRIBE recordings parse from recorded_files.json. (commit d18c9af)
- [x] S4 M3U done right: tvg-* / group / url-tvg / tvc-guide-stationid, file + URL + path, stream format and limit, numbering, groups, per-source XMLTV, refresh with stable ids, picker for big lists (commit b14a82e)
- [x] S5 Xtream Codes source (commit b9e48ae)
- [x] S6 tvheadend, Threadfin/xTeVe/ErsatzTV/Dispatcharr/Antennas, Channels DVR server, HDHomeRun-compatible by address; unsupported devices documented (commit e464ec9)
- [x] S7 Free channels: detect FastChannels / Pluto / Samsung generator containers, or guide the user to run FastChannels (commit 1eababe)
- [x] S8 Setup wizard v2 on web, Apple TV, and iPhone (< 90 s, no typing for HDHomeRun) (commit 0991df2)
- [x] S8b Apple setup at web depth. The server decides ABC/CBS/FOX/NBC: a guide name that is the network, "FOX 4", or "(NBC)" wins, then a call sign with DT/HD stripped (`TestAffiliationGuideNameWins`, `TestAffiliationCallSign`, `TestAffiliationDoesNotGuess`, `TestNetworksFromGuide`, `TestStarBigFour`). WDAF-DT is FOX and KCTV is CBS; WDAF2 and "FOX NEWS" are not starred. `GET /affiliations` exposes the table (KSHB is NBC). The table is the primary affiliates in the large markets, Kansas City included. Web and Apple both call `POST /channels/star`. Fresh fake tuner, empty catalog, zero typing: iPhone walk 12.7s and Apple TV walk 12.6s, both under 90s, to a playing 4.1 (color bars, Live). Shots `.evidence/s8b/iphone-sources.jpg`, `iphone-channels.jpg`, `iphone-guide.jpg`, `iphone-recordings.jpg`, `iphone-apps.jpg`, `iphone-playing.jpg`, and the same `tv-*`. Settings has "Run setup again". Doctor showed the low-disk note. `make check` green.
- [x] S9 Setup doctor: bridge network, /dev/dri, volumes, disk, TZ, clock, PUID/PGID/UMASK (commit 9f70a73)
- [x] S10 Source health and limits (commit 56451fc)
- [x] Phase S deploy: tag v0.4.0, Unraid smoke, two-screen sync (commit 6442a79, log in UNRAID_LOG)

## Phase P — Server and Apple apps, working as one
- [x] P1 Generated Swift and TypeScript clients from OpenAPI; drift check in CI. BroadwaveKit models come from `api/openapi.yaml`. `go run ./server/cmd/apigen -check` fails when the generated files drift.
- [x] P2 Golden responses recorded from the fake tuner at a fixed clock: 28 fixtures in `api/fixtures` (25 of 49 paths plus the ws hello, clock, and sync events). `TestContractFixtures` fails on drift (`CONTRACT_UPDATE=1` re-records). BroadwaveKit `swift test` and `web/src/api/contract.ts` decode them, and `make check` runs both.
- [x] P2b Golden responses for watch, discovery, scan, signals check, the preview frame, Xtream and free sources, setup finish, and the remaining ws events (`sources.found`, `live.changed`, `activity`). `TestContractFixtures` on the in-process fake (127.0.0.1 only; `LookAt` and `FreeHosts` skip the LAN). 19 new JSON fixtures plus `api/fixtures/frame.jpg`: a missing preview is 404, then `image/jpeg` matches those 332 bytes. Setup finish on this tree is `Ready: 5 channels, guide for 1, 2 tuners, Software` (`setup-finish-done.json`). Watch is `tuner returned 404 Not Found: no sample`. The Xtream fixture URL is `lab:%E2%80%A2%E2%80%A2%E2%80%A2%E2%80%A2@127.0.0.1:9` (the password `secret` is not in the file). BroadwaveKit `decodesServerResponses` and `web/src/api/contract.ts` decode the same files. Write paths that still have no golden response are P2c.
- [x] P2c Golden responses for the remaining write paths. `TestContractFixtures` records rename (`server-rename.json`), settings save, guide refresh (4 airings via `GuidePull`, so the test does not call the public XMLTV host), schedule skip, recording progress/watched/play/detect/stop/delete, pass create/delete, marker create/delete, team unfollow, virtual create, and backup restore. GET `/backup` is a SQLite file checked in memory (200, `application/octet-stream`, `SQLite format 3`), not a fixture. GET `/recordings/1/file` is MPEG-TS checked in memory. Recording create is 500 `tuner returned 404 Not Found: no sample` (the fake has no sample). `DELETE /virtuals/{id}` has no route. BroadwaveKit `decodesServerResponses` and `web/src/api/contract.ts` decode the new files. `make check` green.
- [ ] P3 apiVersion/features negotiation and minimum app version
- [ ] P4 Bonjour + UDP fallback discovery, remembered servers, follow address changes, permission explainer
- [ ] P5 Full setup and management from the Apple apps; `docs/parity.md` with no unexplained "web only" cells; Recordings and Settings out of `SportsView.swift` at web depth
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
- [x] A6 Public repo wolfebase/broadwave (twolfekc does not exist; see BLOCKERS), CI green, ghcr.io/wolfebase/broadwave:0.1.0 public for amd64 and arm64 (commit 5c9308b, tag v0.1.0)
- [x] A7 App record `6815795649` (Broadwave, `com.wolfeup.broadwave`, App Group `group.com.wolfeup.broadwave`); `scripts/testflight.sh` uploaded iOS and tvOS builds 1 and 2, all VALID

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
- [x] C7 Guide from the broadcast: relay on full mux `/tunerN/ch<freq>` (PSIP present; `/auto/v` is not), EIT parser, passive harvest, preemptible idle scan, gap merge. Nine of 27 stored channels have current and next listings. An antenna scan on 2026-09-24 did not find 14.1–14.16, 9.4, 43.3, or 46.7, and tuning them returns not found, so they stay empty (ADR 0006). (commit 7d7a49a)
- [x] C7b Every string a station sends. A/65 Annex C Huffman types 1 and 2, built from Tables C4 (913 codes) and C6 (832). `TestAnnexCEveryCode` walks every encode row. `TestAnnexCExamples`: title "The" `41`, "News" `35ef`, "Hello" `fd2bf2ff`, C.2.1 escape "Sqpa" `168e122f`, "Café" `b95be7a403`, "Café!" `b95be7a487`, "A" `71007f`; description "The show." `d7f985373f` and "News" `22ff4e007f`. Modes 0x00 and 0xFF. `TestPlainModes`: Latin-1 Café, UTF-16 Café, surrogate U+1F600, mode 0x04 byte 0x10 is U+0410, BOM stripped. `TestLanguagePreference`, `TestDeviceLanguage` (es locale picks spa, missing locale falls back to eng, `LC_ALL=C` uses `LANG`). `TestCompressedEITTitleIsNotEmpty` ("News" via parseEIT and parseETT). `TestWDAFTitlesNotBlank`. Live DUO 8 s captures, tuner released: 533 MHz 2247, 563 1414, 575 1392, 587 1022, 593 696, 605 757 strings, all compression 0 mode 0x00 eng, 0 blank. No station sent Huffman.
- [x] C8 Honest guide depth and per-listing source. The grid runs to the last listing (up to 14 days). A row past its data says "Listings through Saturday" instead of "No listings". The program sheet says where the listing came from. (commit b123bdd)
- [x] C9 Antenna and signal tools. Settings > Tuners reads strength, quality, and symbols, then says Great, OK, Weak, or Lost. Check all channels uses an idle tuner and stops when a viewer starts. On the DUO, 4.1, 5.1, 9.1, 29.1, 38.1, and 41.1 were Great and 39.7 was OK. (commit 4e51316)
- [x] Phase C deploy: tag v0.5.0, Unraid smoke, two-screen sync (commit 2463a86, log in UNRAID_LOG)

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
- [ ] F2 Master playlists with alternates (PB6's audio picker reloads the rendition; a gapless switch belongs here)
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
- [ ] G5 AC-4 in the jellyfin-ffmpeg image (the 7.1.4 image and startup diagnostics landed in PB5)
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
  - R6: tvOS focused layout control can read as an empty pill. Confirmed 2026-09-24 (`pb-tv-mv.jpg`); fixed in MV2.
  - 2026-09-24 web Home: only the tuned channel's "On now" card has a picture; the others are mostly empty space. A faint oversized channel-number outline sits over the hero image.
  - 2026-09-24 web: no favicon (404). Home asks for `/channels/{id}/frame` on untuned channels and logs a 404 for each; the API should say which channels have a frame.
  - 2026-09-24 iPhone portrait player: a small band of video under unlabeled buttons, with no channel or program info.
- 2026-09-25 iPad guide: the channel column was off screen on open, and four channels left the rest of the iPad empty. The column stays pinned, and a short lineup grows its rows (`.evidence/ipad-guide-before-small.jpg`, `.evidence/ipad-guide-after-small.jpg`). Confirmed again on staging (`.evidence/pb-review/apple-ipad-guide.jpg`). The store shot is regenerated in AS3.
- 2026-09-25 tvOS guide: same class of bug, fixed in the PB review. The channel column is pinned beside the scroller. `.evidence/pb-review/fix-tv-guide-left.jpg` shows 4.1 WDAF-DT, 5.1 KCTVDT1, 9.1 KMBC-HD.
- 2026-09-25 PB review leftovers (shots in `.evidence/pb-review/`):
  - iPhone settings: the floating tab bar covers the Whole-Home Sync explanation (`apple-iphone-settings.jpg`).
  - iPad player: picture is up, with no channel number or program title, and the leftmost control is clipped (`apple-ipad-player.jpg`). iPhone portrait player is still the thin band already listed (AP2).
  - A program only a few minutes wide stacks its title into a column of letters on the iPad guide (5.1 and 9.1).
  - Web settings on a phone clips the playlist address and the group field (`web-phone-settings-bottom.jpg`). The Layout control sits under the Storage heading.
  - Schedule prints two "Schedule" headings, and an activity line has no space ("4:24 PMSporting is on at 6:30 PM.").
  - Diagnostics says the guide refreshes at a time that has already passed, and "9/8 channels listed".
  - Phone watch failed once with a playlist 404 (`1080.aac2.broadcast`), then played on retry. TV layout dropped 446/1313 frames in one ~21 s sample.
  - Complete recordings on staging show "0 B", and `/media/poster/{id}` is 404, so library art is a blank rectangle. Favicon 404 is fixed in this commit; staging serves it on the next deploy.
- [ ] J2 Legacy web screens redesigned
- [ ] J3 Motion system
- [ ] J4 Accessibility audit
- [ ] J5 Light mode + accent picker
- [ ] J6 Localization readiness
- [ ] J7 Brand: name decided (Broadwave, renamed everywhere); logo, app icon, marketing site to do

## Phase K — Quality
- [x] K1 Fake HDHomeRun + Go integration tests + relay smoke in CI. `FAKE=1 scripts/relay-smoke.sh` tunes the fake and checks renditions, program date-times, and export. The Go test covers a shared frequency, a busy tuner, a recording that extends and restarts, and a scan that yields. (commit 13274ea)
- [ ] K2 Playwright e2e + visual snapshots + sync test
- [ ] K3 Apple tests (Swift Testing + XCUITest)
- [ ] K4 24 h Unraid soak
- [ ] K5 CI expansion
- [ ] K6 Performance budgets in CI

## Phase L — Release
- [ ] L1 Versioning + changelog
- [ ] L2 GHCR images verified
- [ ] L3 Community Apps: researched, template installed on TUS via URL, Validate + Scan clean, submitted
- [ ] L4 TestFlight external + App Store readiness. Done 2026-09-24: 1.0 listing, 11 screenshots (`docs/screenshots/appstore`, Blender open movies), privacy labels (Data Not Collected), age rating 12+, US and Canada, review notes and demo videos, submitted for review. Remaining: external TestFlight group, and redo the iPad screenshots with multiview once MV1 fixes the tile bands
- [ ] L5 Docs site

## Phase M — Category-best extras
- [ ] M1 Catch-up rewind and start over from the ring buffer
- [ ] M2 Commercial skip while behind live
- [ ] M3 "Which channel has the game?" from search, Siri, and widgets
- [ ] M4 Smart favorites by time of day (on-device)
- [ ] M5 Improvements found in J1 and K4 (add tasks, then do them)
