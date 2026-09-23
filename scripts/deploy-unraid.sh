#!/usr/bin/env bash
# Build OTA Viewer for linux/amd64 and deploy it to an Unraid server over SSH.
#
#   UNRAID_HOST=root@192.168.1.2 scripts/deploy-unraid.sh            # build + deploy + recreate
#   UNRAID_HOST=... MODE=image-only scripts/deploy-unraid.sh           # build image, keep container
#
# Layout on the server (matches the user's existing install):
#   $APPDATA/build/   Dockerfile + ota-viewer binary (image context)
#   $APPDATA/config/  mounted at /config (catalog, live buffers)
#   $RECORDINGS       mounted at /config/work/recordings
# The container runs with host networking (tuner discovery + Bonjour) and /dev/dri (VAAPI/QSV).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
: "${UNRAID_HOST:?set UNRAID_HOST, e.g. root@192.168.1.2}"
APPDATA="${APPDATA:-/mnt/cache/appdata/ota-viewer}"
RECORDINGS="${RECORDINGS:-/mnt/user/media/ota-recordings}"
NAME="${NAME:-OTA-Viewer}"
IMAGE="${IMAGE:-ota-viewer:latest}"
TZ_NAME="${TZ_NAME:-America/Chicago}"
VERSION="${VERSION:-$(cd "$ROOT" && git describe --tags --always --dirty 2>/dev/null || echo dev)}"
MODE="${MODE:-full}"

echo "==> building web + linux/amd64 binary ($VERSION)"
(cd "$ROOT/web" && npm run build >/dev/null)
(cd "$ROOT/server" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o "$ROOT/bin/ota-viewer-linux-amd64" ./cmd/ota-viewer)

echo "==> backing up catalog on $UNRAID_HOST"
ssh "$UNRAID_HOST" "mkdir -p $APPDATA/build $APPDATA/backups && cp $APPDATA/config/ota-viewer.db $APPDATA/backups/ota-viewer-\$(date +%Y%m%d-%H%M%S).db 2>/dev/null || true"

echo "==> uploading"
scp -q "$ROOT/bin/ota-viewer-linux-amd64" "$UNRAID_HOST:$APPDATA/build/ota-viewer"
scp -q "$ROOT/deploy/docker/Dockerfile.runtime" "$UNRAID_HOST:$APPDATA/build/Dockerfile"

echo "==> building image on server"
ssh "$UNRAID_HOST" "cd $APPDATA/build && docker build -q -t $IMAGE . >/dev/null && echo built"

if [[ "$MODE" == "image-only" ]]; then exit 0; fi

echo "==> recreating container $NAME (host network, /dev/dri)"
ssh "$UNRAID_HOST" "docker rm -f $NAME >/dev/null 2>&1 || true; docker run -d --name $NAME --restart unless-stopped \
  --network host --device /dev/dri \
  -e TZ=$TZ_NAME \
  -v $APPDATA/config:/config -v $RECORDINGS:/config/work/recordings \
  $IMAGE >/dev/null && echo started"

echo "==> waiting for health"
HOSTIP="${UNRAID_HOST#*@}"
for _ in $(seq 1 30); do
  if curl -fsS -m 2 "http://$HOSTIP:8477/api/v1/health" >/dev/null 2>&1; then
    curl -s "http://$HOSTIP:8477/api/v1/server"; echo
    ssh "$UNRAID_HOST" "docker logs --tail 8 $NAME 2>&1"
    exit 0
  fi
  sleep 2
done
echo "server did not become healthy; recent logs:"
ssh "$UNRAID_HOST" "docker logs --tail 40 $NAME 2>&1"
exit 1
