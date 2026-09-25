---
name: broadwave-media-pipeline
description: Rules and hard-won gotchas for Broadwave's live relay, ffmpeg rendition arguments, CMAF/HLS playlists, the shared broadcast timeline, and the browser/AVPlayer sync engines. Use when changing server/internal/live, ffmpeg args, HLS output, stream decisions, renditions, multiview streams, LL-HLS, captions, or web/src/lib/sync.ts and BroadwaveKit SyncEngine.
---

# Broadwave media pipeline

## Architecture in one breath

`Hub` tunes a whole RF frequency once (`/tunerN/ch<freq>` on :5004). `mux.readLoop` fans raw TS to subscribers (renditions, recordings, exports, probes) and drops chunks for slow readers rather than stalling the tuner. A `feed` = one channel on a mux, with a `Timeline` and a map of `rendition`s (one ffmpeg each, keyed `video.audio[.mode]`). `Decide(Source, Caps, Prefs)` picks the rendition. Details: `docs/architecture.md`, ADR 0002/0003.

## Invariants (break these and playback or sync breaks)

1. **One tune per frequency.** Always reuse `h.muxes[freq]` before taking a tuner; release on every error path; set `/tunerN/channel none` on teardown.
2. **Renditions are independent.** Never restart a rendition because another viewer asked for something different.
3. **`-copyts` on every live rendition.** Timestamps must stay broadcast PTS so all renditions share one timeline. Consequences: do NOT use `aresample=first_pts=0`; force keyframes with `expr:if(isnan(prev_forced_t),1,gte(t,prev_forced_t+2))`, never `n_forced*2`.
4. **CMAF (fMP4) segments**, `-video_track_timescale 90000`, `-hls_segment_type fmp4`, `init.mp4` + `seg%05d.m4s`. With MPEG-TS segments hls.js corrupted its fragment table (negative durations ~-645 s) mid-stream. Segment times come from `tfdt` + first `trun` composition offset (`live/fmp4.go`), not from ffmpeg's PDT.
5. **Playlists are stamped by the server** (`playlistStamper`): drop ffmpeg PDT lines, insert `EXT-X-PROGRAM-DATE-TIME` = `Timeline.Wall(segmentPTS)`. Identical across renditions (test: `TestRenditionsShareOneTimeline`).
6. **Segment 0 is never served** (decoder warm-up; audio starts before video; browsers reject it) and `EXT-X-MEDIA-SEQUENCE` is bumped by one while it's listed. Watch waits for 3 `#EXTINF` before answering.
7. **VideoToolbox needs `-a53cc 0`** — its A/53 caption SEI makes every segment undecodable (0x0 frames). Captions for VT transcodes must come from a separate WebVTT path.
8. **Copy video only when progressive** (`channels.field_order == "progressive"`, learned by the background ffprobe in `live/probe.go`). Browsers don't deinterlace; interlaced H.264 subchannels get transcoded with bwdif. A header that misses the 800 ms window still rebuilds the running rendition (`applyProbeLocked` / `finishScan`). A stored "progressive" does not skip the packet scan: ffprobe calls soft 3:2 progressive, and that scan is what finds film.
8a. **Never deinterlace a progressive source, and keep its frame rate.** Most ABC and FOX stations send MPEG-2 720p59.94, which is progressive. Before 2026-09-24 every MPEG-2 channel was bobbed at field rate: on VAAPI that sent 720p60 out as 1920x1080 at 119.88 fps in 1 s segments (measured on Unraid, channel 4.1). The software path capped it at 29.97. Progressive sources now skip deinterlace, keep their own rate (no `fps=`), and scale on the GPU with `scale_vaapi=w='min(W,iw)'` so 720p is never upscaled. Tiles (`saver`, `tile`) cap at 29.97. `TestProgressive720pKeepsEveryFrame` guards this. Unscanned H.264 is not interlaced: an empty field order (HLS and any URL the rendition reads itself) keeps the source rate until ffprobe stores one. A stored `tt`/`bb`/`tb`/`bt` still field-deinterlaces. `TestUnscannedH264KeepsItsRate`.
8b. **Do not invent frames.** Smooth never runs `minterpolate` or `framerate`. A progressive source keeps its rate. 60 fps motion comes from field-rate deinterlace of interlaced video (ADR 0010). `minterpolate` blend ghosts, and `mci` was 0.09× on the UHD 770.
8c. **Film plays at 24p.** Field order `film` means MPEG-2 `repeat_first_field` on progressive pictures. The film picture mode runs `pullup` on the CPU (hard telecine has no flags; `fieldmatch` left combs) and scales on the GPU (`scale_vaapi`). Hard telecine stays on the field-rate path until that mode is selected.
8d. **A dead encode does not keep the tuner.** `watchRendition` gives a failed encode one restart in an empty directory: a VAAPI process that dies within 8 seconds comes back on the CPU (`libx264`, or `libx265` for HEVC), and any later death rebuilds the same command line once. The directory is wiped first so the new encode never serves the previous `init.mp4`. A second death, or a clean exit, drops the rendition and releases the tuner when nothing else is watching or recording. Stopping a rendition on purpose does not restart it.
9. **Sync engines never seek backward in a live buffer** (hls.js stalls/corrupts). Ahead -> pause for exactly the drift; behind -> seek forward; trim rate ±3% under 400 ms; at most one seek per 2 s; ignore fragments with `duration <= 0 || > 30`. Map position <-> PDT through the playlist fragments, not `hls.playingDate`.
10. **Room target** = `anchorMedia + (serverNow - anchorServer) * rate`; default latency 10 s behind real time (`balanced`). On a fresh tune the target is older than the window for a few seconds — the engine waits, it doesn't clamp-seek.

## Picture lab (real encodes on the Unraid iGPU)

- Capture samples through the running server (`curl -m 11 :8477/export/stream/<id>`) only when `/api/v1/tuners` is free and nothing records soon.
- Run encodes in throwaway containers named `bw-lab-*` with `--cpus 4 --device /dev/dri`, and clean up with `docker ps -aq --filter name=bw-lab | xargs -r docker rm -f`. Kill remote runners with `pkill -f "[r]unner-name"`; a plain `pkill -f name` matches your own SSH shell.
- **Feed samples at real time** (`ffmpeg -re` or a rate-limited pipe). Debian ffmpeg 5.1 with `-copyts` and VAAPI deinterlace, fed a file faster than real time, emits thousands of 0.0007 s segments and never exits. The image now uses jellyfin-ffmpeg 7 at `/usr/lib/jellyfin-ffmpeg/ffmpeg`. Production (live pipe) is fine. This cost an hour.
- Measure the result, not the args: `cat init.mp4 seg*.m4s`, then count frames from `frame=pts_time` (fps = frames / span), read width and height, and count decode errors.
- Staging beside production: container `Broadwave-Staging` on `:8490` runs `-staging -bonjour=false` (no recordings, no guide pulls, no idle scans, no emulator) with its own `server_identity`. Test there first, like a viewer would, in Chrome and the simulators; production stays untouched.

## Testing changes

- Unit tests: `go test ./server/internal/live` (includes an ffmpeg-backed shared-timeline test).
- `scripts/relay-smoke.sh` for end-to-end.
- Real tuner in the browser, then two-screen sync measurement (skill `broadwave-dev-loop`).
- Generate test broadcasts: `ffmpeg -f lavfi -i testsrc2=size=1280x720:rate=60000/1001 -f lavfi -i sine -c:v libx264 -g 30 -c:a ac3 -output_ts_offset 95000 -f mpegts x.ts` (the offset exercises big PTS values). Serve live with `-re ... -f mpegts -listen 1 http://127.0.0.1:18500/live.ts` and add it as a `link` source.
- Grab 8 s of a real channel: `curl -s -m 9 127.0.0.1:18477/export/stream/1 -o real.ts`.
- Inspect segments: `ffprobe -v quiet -show_entries stream=codec_name,width,height -of csv=p=0 seg.m4s` (needs init: `cat init.mp4 seg.m4s > x.mp4`), `ffmpeg -v error -i x -f null - | wc -l` counts decode errors.

## Adding a rendition type

1. Extend `Rendition.normalized()`/`Key()`/`ParseRenditionKey` and `Decide` with table tests.
2. Build args in `RenditionArgs` (keep `-copyts`, keyframe expr, CMAF flags).
3. `media` handler already serves `init.mp4`, `seg*.m4s`, `index.m3u8` per key.
4. Update `api/openapi.yaml` enums, `docs/decisions/0002`, and this skill.

## LL-HLS notes (Phase G3)

ffmpeg's HLS muxer can't emit `EXT-X-PART`. Plan: feed ffmpeg fragmented MP4 to a Go packager that cuts parts at `moof` boundaries (~330 ms), serves them, and implements blocking reload (`_HLS_msn`, `_HLS_part`) and `EXT-X-PRELOAD-HINT`. hls.js needs `lowLatencyMode: true`.
