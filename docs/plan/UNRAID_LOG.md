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

## 2026-09-23 11:04 CDT — A3 migrate OTA-Viewer to Waveguide (v0.1.0)

- Appdata moved `/mnt/cache/appdata/ota-viewer` → `/mnt/cache/appdata/waveguide`. Catalog backups: `backups/ota-viewer-20260923-105232.db` and `backups/ota-viewer-20260923-105753.db`. On start the server renamed `config/ota-viewer.db` to `config/waveguide.db`. `schema_migrations` is 1, 2, 3, 4. Server name is "Waveguide on TUS".
- Container `OTA-Viewer` removed. `Waveguide` is `ghcr.io/wolfebase/waveguide:0.1.0`, host network, `/dev/dri`, `TZ=America/Chicago`, no `HDHR_HOST`. Mounts: `.../waveguide/config` → `/config`, `/mnt/user/media/ota-recordings` → `/config/work/recordings`.
- The first `scripts/deploy-unraid.sh` run died mid-upload (SSH over the tunnel drops large transfers). The image was pulled on the server instead. `MODE=ghcr` does that on purpose from here on.
- Template: `/boot/config/plugins/dockerMan/templates-user/my-Waveguide.xml` (this server's paths and `America/Chicago`). `my-OTA-Viewer.xml` removed.
- Startup log: `encoder: h264_vaapi`, `bonjour: _waveguide._tcp on port 8477`, `discovery: 1 device(s)` (HDHomeRun CONNECT DUO), `guide: source=silicondust-xmltv airings=609 channels=9`.
- Web at `http://192.168.1.2:8477`. Two browser tabs on channel 4.1 (WDAF): both 1080p, playing, sync offsets differed by 6 ms (`-10017` vs `-10011`), pill "2 screens".
- iPhone 17 Pro simulator launched `com.wolfeup.waveguide -OTAWatch 1` against this server. Diagnostics: one tuner, `ours: true`, viewers 3, renditions `1080.aac2.broadcast` (2) and `1080.copy.broadcast` (1).
- Bonjour: `avahi-browse` on TUS shows `br0 IPv4 Waveguide on TUS _waveguide._tcp`. From this Mac, `dns-sd` only sees the local dev server; the Mac reaches TUS through utun4, so LAN multicast does not arrive here.
