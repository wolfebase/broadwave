# Unraid deployment log

Server: TUS (192.168.1.2), Unraid with Docker 29.x, i9-12900K + UHD 770 (`/dev/dri/renderD128`), 24 threads.
Container: `Broadwave`, host network, `/dev/dri`, `TZ=America/Chicago`, image from `ghcr.io/wolfebase/broadwave`.
Mounts: `/mnt/cache/appdata/broadwave/config -> /config`, `/mnt/user/media/ota-recordings -> /config/work/recordings`.
Template: `/boot/config/plugins/dockerMan/templates-user/my-Broadwave.xml`.
Staging: `Broadwave-Staging` on `:8490` (`-staging`, iGPU), appdata `/mnt/cache/appdata/broadwave-staging`.

## Entries
<!-- Append: date, commit, what was deployed, checks run, results, issues. -->

## 2026-09-25 00:52 CDT — v0.7.1, captions stay on the graphics chip

- Image `ghcr.io/wolfebase/broadwave:0.7.1` (tag `v0.7.1`). Catalog backed up to `backups/broadwave-20260925-003303.db` before 0.7.0 and `backups/broadwave-20260925-005221.db` before 0.7.1. Container `Broadwave`, host network, `/dev/dri`, `TZ=America/Chicago`. `/api/v1/server` reports `v0.7.1`, encoder `h264_vaapi`, ffmpeg `7.1.4-Jellyfin`. Five recordings still listed. Tuners were free. Next recording is Jeopardy at 2026-09-25 20:00 UTC.
- `v0.7.0` (the same image family) played channel 1 only after `h264_vaapi` quit with "Access unit too large" and the rendition fell back to software. `v0.7.1` passes `-sei 0`. On production, channel 1 for 20 s: `1280x720 59.94`, segment 2.002 s, 240 frames, 0 decode errors, and the ffmpeg command was `h264_vaapi -sei 0`. Tuner released. Staging had the same numbers on the fix before the image was published.
- TestFlight build 147 (iOS and tvOS) uploaded from the 0.7.0 tree, which includes the iPad guide column. The server-only caption fix did not need another upload.

## 2026-09-24 16:06 CDT — v0.6.0, renamed to Broadwave

- Image `ghcr.io/wolfebase/broadwave:0.6.0` (tag `v0.6.0`). Catalog backed up to `backups/broadwave-20260924-160627-pre060.db`, then moved by hand: appdata to `/mnt/cache/appdata/broadwave`, catalog file to `broadwave.db`, server name to "Broadwave on TUS", template to `my-Broadwave.xml`, container to `Broadwave`. 27 channels, 5 recordings, and 1 pass carried over. Old images removed.
- `/api/v1/server` reports `v0.6.0`, encoder `h264_vaapi`. `avahi-browse` shows `Broadwave on TUS` on `_broadwave._tcp`.
- Channel 4.1 (720p MPEG-2) now plays 1280x720 at 59.94 fps in 2.002 s segments with 0 decode errors (v0.5.0 sent 1920x1080 at 119.88). Tuners free before and after. Next recording is Jeopardy on 2026-09-25 at 20:00 UTC.
- Staging is `Broadwave-Staging` on `:8490` with the same image and the branch binary.


## 2026-09-24 10:34 CDT — Phase C deploy v0.5.0

- Image `ghcr.io/wolfebase/broadwave:0.5.0` (tag `v0.5.0`, commit `2463a86`). Catalog backed up to `backups/broadwave-20260924-pre050.db` before recreate. Container `Broadwave`, host network, `/dev/dri`, `TZ=America/Chicago`. No PUID/PGID. `/api/v1/server` reports `version: v0.5.0`, encoder `h264_vaapi`. The deploy script's health curl raced container startup and exited 7; the container was Up healthy.
- `/api/v1/signals` returns the 8 present channels, so migration 0018 is applied. No stored readings yet; the earlier check ran against a catalog copy.
- Channel 4.1 (one tuner, signal 100) produced a playlist with program date times. Two browser tabs both reported the pill "2 screens". Sync drift was 5607 ms and 5619 ms, 12 ms apart. Tuners were free before the check and after the idle release. Next Jeopardy is 20:00 UTC. A7 is still unauthenticated.

## 2026-09-24 09:20 CDT — Phase S deploy v0.4.0

- Image `ghcr.io/wolfebase/broadwave:0.4.0` (tag `v0.4.0`, commit `6442a79`). Catalog backed up to `backups/broadwave-20260924-pre040.db` before recreate. Container `Broadwave`, host network, `/dev/dri`, `TZ=America/Chicago`. No PUID/PGID: the catalog and recordings are owned by root, and dropping to uid 99 would not be able to open them. `/api/v1/server` reports `version: v0.4.0`, encoder `h264_vaapi`.
- Sources list still returns the DUO, and that query reads `last_refresh`, so migration 0016 is applied. Guide still 9 channels with listings, 336 airings, through 2026-09-25. Diagnostics has one note: recordings are owned by root. That matches the ownership above.
- Channel 4.1 (one tuner, 593 MHz, signal 100) produced a playlist with program date times. Two browser tabs both reported the pill "2 screens". Sync offsets differed by 5–8 ms. Tuners were free before the check and after the idle release. Next Jeopardy is 20:00 UTC. A7 is still unauthenticated.

## 2026-09-23 22:05 CDT — Phase R deploy v0.3.3

- Image `ghcr.io/wolfebase/broadwave:0.3.3` (commit `58793cf`). Container `Broadwave`, host network, `/dev/dri`. `/api/v1/server` reports `version: v0.3.3`, encoder `h264_vaapi`. CI and the release build for that commit are green.
- `v0.3.0` killed preview grabs before a keyframe. `v0.3.1` waits 4 seconds. `v0.3.2` names the JPEG format, because ffmpeg will not write a `.jpg.part` file. On `v0.3.2`, channel 4.1 wrote `1.jpg` (21,091 bytes) and `1-1280.jpg` (74,824 bytes). Both frame URLs returned 200. The grab used about 6% of one core for under 2 seconds, which is about 0.2% of one core over the minute. Under the 3% budget.
- `v0.3.2` left the live playlist as a 0-byte `init.mp4`. The same encode against a saved mux wrote segments, so the rendition was missing the start of the live stream. `v0.3.3` keeps a few seconds of that stream.
- On `v0.3.3`, channel 4.1 (one tuner, 593 MHz, signal 100) produced `init.mp4` and segments. `GET /media/live/1/1080.aac2.broadcast/index.m3u8` returned 200 with program date times. Two browser tabs both reported sync offset `-4464` (0 ms apart) and the pill "2 screens". Drift differed by 1 ms.
- Tuners were free before the checks and after the idle release. No recording was in progress. Next Jeopardy is 20:00 UTC.

## 2026-09-23 14:55 CDT — R2 deploy v0.2.0

- Image `ghcr.io/wolfebase/broadwave:0.2.0` (tag `v0.2.0`, commit `83c8904`), pulled on TUS with `MODE=ghcr`. Container `Broadwave`, host network, `/dev/dri`, `TZ=America/Chicago`. `/api/v1/server` reports `version: v0.2.0`, encoder `h264_vaapi`.
- Startup: discovery found 1 device. Guide still 9 of 27 channels, 575 airings, listings through 2026-09-25. Tuners were free before and after the checks. Jeopardy on 4.1 was due at 20:00 UTC; both tuners were released at 19:54 UTC.
- Guide matching: listings on 4.1, 5.1, 9.1, 9.4, 29.1, 38.1, 39.7, 41.1, 62.1. Search for Jeopardy returned 4 airings and 1 recording.
- Sports: live scoreboard (WSH @ DET 4–1). Hide scores blanked that game, then the setting was turned back off and the score returned.
- Team pass: following "Broadwave Probe FC" with record on created a team pass; unfollowing removed it. The Jeopardy series pass was left as it was.
- Multiview 2-up: 4.1 and 9.1 as `540.none.broadcast` on VAAPI (`h264_vaapi`, deinterlace on 9.1). Both playlists carried program date times; a segment from each started with an fMP4 `styp` box. During the pair, `docker stats` showed CPU 47.62% and memory 341.5 MiB. Each ffmpeg was about 18% of a core. `intel_gpu_top`: render engine 12–20%, video enhance 11–20%, video codec 3–5%, GPU power about 0.5 W. Idle afterward: CPU 0%, memory 106 MiB. No Broadwave ffmpeg left running.
- iPhone 17 Pro and Apple TV 4K (1080p) simulators opened Home against `http://192.168.1.2:8477` and showed The Drew Barrymore Show on 4.1. They did not tune. Screenshots: `docs/screenshots/r2-iphone.jpg`, `docs/screenshots/r2-tv.jpg`.

## 2026-09-23 11:04 CDT — A3 first host-network deploy (v0.1.0)

- Catalog backed up twice before the move. `schema_migrations` is 1, 2, 3, 4. Server name is "Broadwave on TUS".
- `Broadwave` is `ghcr.io/wolfebase/broadwave:0.1.0`, host network, `/dev/dri`, `TZ=America/Chicago`, no `HDHR_HOST`. Mounts: `.../broadwave/config` → `/config`, `/mnt/user/media/ota-recordings` → `/config/work/recordings`.
- The first `scripts/deploy-unraid.sh` run died mid-upload (SSH over the tunnel drops large transfers). The image was pulled on the server instead. `MODE=ghcr` does that on purpose from here on.
- Template: `/boot/config/plugins/dockerMan/templates-user/my-Broadwave.xml` (this server's paths and `America/Chicago`).
- Startup log: `encoder: h264_vaapi`, `bonjour: _broadwave._tcp on port 8477`, `discovery: 1 device(s)` (HDHomeRun CONNECT DUO), `guide: source=silicondust-xmltv airings=609 channels=9`.
- Web at `http://192.168.1.2:8477`. Two browser tabs on channel 4.1 (WDAF): both 1080p, playing, sync offsets differed by 6 ms (`-10017` vs `-10011`), pill "2 screens".
- iPhone 17 Pro simulator launched `com.wolfeup.broadwave -BroadwaveWatch 1` against this server. Diagnostics: one tuner, `ours: true`, viewers 3, renditions `1080.aac2.broadcast` (2) and `1080.copy.broadcast` (1).
- Bonjour: `avahi-browse` on TUS shows `br0 IPv4 Broadwave on TUS _broadwave._tcp`. From this Mac, `dns-sd` only sees the local dev server; the Mac reaches TUS through utun4, so LAN multicast does not arrive here.
