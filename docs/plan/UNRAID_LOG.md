# Unraid deployment log

Server: TUS (192.168.1.2), Unraid with Docker 29.x, i9-12900K + UHD 770 (`/dev/dri/renderD128`), 24 threads.
Container: `OTA-Viewer`, image `ota-viewer:latest` built on the server from `/mnt/cache/appdata/ota-viewer/build` (Dockerfile.runtime + prebuilt linux/amd64 binary).
Mounts: `/mnt/cache/appdata/ota-viewer/config -> /config`, `/mnt/user/media/ota-recordings -> /config/work/recordings`.
Template: `/boot/config/plugins/dockerMan/templates-user/my-OTA-Viewer.xml`.

## 2026-09-23 — state at handoff
- Running the pre-rebuild build (bridge network, `HDHR_HOST=192.168.1.252`, old UI). Catalog has live data (recordings from 2026-09-22).
- Not yet deployed: everything since the baseline commit. First task of the next session (plan A3): back up, deploy with `scripts/deploy-unraid.sh`, switch to host networking, update the template, verify.

## Entries
<!-- Append: date, commit, what was deployed, checks run, results, issues. -->
