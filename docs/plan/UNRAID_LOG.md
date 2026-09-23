# Unraid deployment log

Server: TUS (192.168.1.2), Unraid with Docker 29.x, i9-12900K + UHD 770 (`/dev/dri/renderD128`), 24 threads.
Container: `OTA-Viewer`, image `ota-viewer:latest` built on the server from `/mnt/cache/appdata/ota-viewer/build` (Dockerfile.runtime + prebuilt linux/amd64 binary).
Mounts: `/mnt/cache/appdata/ota-viewer/config -> /config`, `/mnt/user/media/ota-recordings -> /config/work/recordings`.
Template: `/boot/config/plugins/dockerMan/templates-user/my-OTA-Viewer.xml`.

## 2026-09-23 — state at handoff
- Running the pre-rebuild build (bridge network, `HDHR_HOST=192.168.1.252`, old UI). Catalog has live data (recordings from 2026-09-22).
- Not yet deployed: everything since the baseline commit. First task of the next session (plan A3): back up, deploy with `scripts/deploy-unraid.sh`, switch to host networking, update the template, verify.

## 2026-09-23 — product renamed to Waveguide
- The names above are the pre-rename install and still what's running. The next deploy (`scripts/deploy-unraid.sh`) moves `/mnt/cache/appdata/ota-viewer` to `/mnt/cache/appdata/waveguide`, replaces container `OTA-Viewer` with `Waveguide` (image `waveguide:latest`), and the server renames `ota-viewer.db` to `waveguide.db` on start. Replace template `my-OTA-Viewer.xml` with `my-Waveguide.xml`. Recordings stay in `/mnt/user/media/ota-recordings`.

## Entries
<!-- Append: date, commit, what was deployed, checks run, results, issues. -->

## 2026-09-23 14:55 CDT — R2 deploy v0.2.0

- Image `ghcr.io/wolfebase/waveguide:0.2.0` (tag `v0.2.0`, commit `83c8904`), pulled on TUS with `MODE=ghcr`. Container `Waveguide`, host network, `/dev/dri`, `TZ=America/Chicago`. `/api/v1/server` reports `version: v0.2.0`, encoder `h264_vaapi`.
- Startup: discovery found 1 device. Guide still 9 of 27 channels, 575 airings, listings through 2026-09-25. Tuners were free before and after the checks. Jeopardy on 4.1 was due at 20:00 UTC; both tuners were released at 19:54 UTC.
- Guide matching: listings on 4.1, 5.1, 9.1, 9.4, 29.1, 38.1, 39.7, 41.1, 62.1. Search for Jeopardy returned 4 airings and 1 recording.
- Sports: live scoreboard (WSH @ DET 4–1). Hide scores blanked that game, then the setting was turned back off and the score returned.
- Team pass: following "Waveguide Probe FC" with record on created a team pass; unfollowing removed it. The Jeopardy series pass was left as it was.
- Multiview 2-up: 4.1 and 9.1 as `540.none.broadcast` on VAAPI (`h264_vaapi`, deinterlace on 9.1). Both playlists carried program date times; a segment from each started with an fMP4 `styp` box. During the pair, `docker stats` showed CPU 47.62% and memory 341.5 MiB. Each ffmpeg was about 18% of a core. `intel_gpu_top`: render engine 12–20%, video enhance 11–20%, video codec 3–5%, GPU power about 0.5 W. Idle afterward: CPU 0%, memory 106 MiB. No Waveguide ffmpeg left running.
- iPhone 17 Pro and Apple TV 4K (1080p) simulators opened Home against `http://192.168.1.2:8477` and showed The Drew Barrymore Show on 4.1. They did not tune. Screenshots: `docs/screenshots/r2-iphone.jpg`, `docs/screenshots/r2-tv.jpg`.

## 2026-09-23 11:04 CDT — A3 migrate OTA-Viewer to Waveguide (v0.1.0)

- Appdata moved `/mnt/cache/appdata/ota-viewer` → `/mnt/cache/appdata/waveguide`. Catalog backups: `backups/ota-viewer-20260923-105232.db` and `backups/ota-viewer-20260923-105753.db`. On start the server renamed `config/ota-viewer.db` to `config/waveguide.db`. `schema_migrations` is 1, 2, 3, 4. Server name is "Waveguide on TUS".
- Container `OTA-Viewer` removed. `Waveguide` is `ghcr.io/wolfebase/waveguide:0.1.0`, host network, `/dev/dri`, `TZ=America/Chicago`, no `HDHR_HOST`. Mounts: `.../waveguide/config` → `/config`, `/mnt/user/media/ota-recordings` → `/config/work/recordings`.
- The first `scripts/deploy-unraid.sh` run died mid-upload (SSH over the tunnel drops large transfers). The image was pulled on the server instead. `MODE=ghcr` does that on purpose from here on.
- Template: `/boot/config/plugins/dockerMan/templates-user/my-Waveguide.xml` (this server's paths and `America/Chicago`). `my-OTA-Viewer.xml` removed.
- Startup log: `encoder: h264_vaapi`, `bonjour: _waveguide._tcp on port 8477`, `discovery: 1 device(s)` (HDHomeRun CONNECT DUO), `guide: source=silicondust-xmltv airings=609 channels=9`.
- Web at `http://192.168.1.2:8477`. Two browser tabs on channel 4.1 (WDAF): both 1080p, playing, sync offsets differed by 6 ms (`-10017` vs `-10011`), pill "2 screens".
- iPhone 17 Pro simulator launched `com.wolfeup.waveguide -OTAWatch 1` against this server. Diagnostics: one tuner, `ours: true`, viewers 3, renditions `1080.aac2.broadcast` (2) and `1080.copy.broadcast` (1).
- Bonjour: `avahi-browse` on TUS shows `br0 IPv4 Waveguide on TUS _waveguide._tcp`. From this Mac, `dns-sd` only sees the local dev server; the Mac reaches TUS through utun4, so LAN multicast does not arrive here.
