#!/usr/bin/env bash
# Build Broadwave for linux/amd64 and deploy it to an Unraid server over SSH.
#
#   UNRAID_HOST=root@192.168.1.2 scripts/deploy-unraid.sh            # build + deploy + recreate
#   UNRAID_HOST=... MODE=image-only scripts/deploy-unraid.sh           # build image, keep container
#   UNRAID_HOST=... MODE=ghcr scripts/deploy-unraid.sh                 # pull the public image on the server
#
# Layout on the server (matches the user's existing install):
#   $APPDATA/build/   Dockerfile + broadwave binary (image context)
#   $APPDATA/config/  mounted at /config (catalog, live buffers)
#   $RECORDINGS       mounted at /config/work/recordings
# The container runs with host networking (tuner discovery + Bonjour) and /dev/dri (VAAPI/QSV).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
: "${UNRAID_HOST:?set UNRAID_HOST, e.g. root@192.168.1.2}"
APPDATA="${APPDATA:-/mnt/cache/appdata/broadwave}"
RECORDINGS="${RECORDINGS:-/mnt/user/media/ota-recordings}"
NAME="${NAME:-Broadwave}"
IMAGE="${IMAGE:-broadwave:latest}"
TZ_NAME="${TZ_NAME:-America/Chicago}"
VERSION="${VERSION:-$(cd "$ROOT" && git describe --tags --always --dirty 2>/dev/null || echo dev)}"
MODE="${MODE:-full}"

# IPQoS=none: the path from this Mac to TUS is a tunnel, and the default QoS
# marking has dropped SSH mid-transfer. Short commands still win; large uploads
# should be avoided. MODE=ghcr pulls the public image on the server instead.
SSH=(ssh -o ServerAliveInterval=15 -o ServerAliveCountMax=8 -o ConnectTimeout=20 -o IPQoS=none)
SCP=(scp -o ServerAliveInterval=15 -o ServerAliveCountMax=8 -o ConnectTimeout=20 -o IPQoS=none)

if [[ "$MODE" == "ghcr" ]]; then
  IMAGE="${GHCR_IMAGE:-ghcr.io/wolfebase/broadwave:latest}"
  echo "==> pulling $IMAGE on $UNRAID_HOST (detached) and recreating $NAME"
  # The pull runs on the server. A marker file is the signal that the new
  # container was started; /health stays up on the old container during the pull.
  "${SSH[@]}" "$UNRAID_HOST" "mkdir -p $APPDATA && rm -f $APPDATA/deploy.ok && nohup sh -c 'docker pull $IMAGE && docker rm -f $NAME >/dev/null 2>&1 || true; docker run -d --name $NAME --restart unless-stopped --network host --device /dev/dri -e TZ=$TZ_NAME -v $APPDATA/config:/config -v $RECORDINGS:/config/work/recordings $IMAGE && touch $APPDATA/deploy.ok' > $APPDATA/deploy.log 2>&1 & echo launched"
  HOSTIP="${UNRAID_HOST#*@}"
  for _ in $(seq 1 90); do
    if "${SSH[@]}" "$UNRAID_HOST" "test -f $APPDATA/deploy.ok" 2>/dev/null; then
      curl -fsS -m 5 "http://$HOSTIP:8477/api/v1/server"; echo
      exit 0
    fi
    sleep 2
  done
  echo "deploy did not finish; see $APPDATA/deploy.log on the server"
  exit 1
fi

echo "==> building web + linux/amd64 binary ($VERSION)"
(cd "$ROOT/web" && npm run build >/dev/null)
(cd "$ROOT/server" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o "$ROOT/bin/broadwave-linux-amd64" ./cmd/broadwave)

echo "==> backing up catalog on $UNRAID_HOST"
"${SSH[@]}" "$UNRAID_HOST" "mkdir -p $APPDATA/build $APPDATA/backups && [ -f $APPDATA/config/broadwave.db ] && cp $APPDATA/config/broadwave.db $APPDATA/backups/broadwave-\$(date +%Y%m%d-%H%M%S).db; true"

echo "==> uploading"
"${SCP[@]}" -q "$ROOT/bin/broadwave-linux-amd64" "$UNRAID_HOST:$APPDATA/build/broadwave"
"${SCP[@]}" -q "$ROOT/deploy/docker/Dockerfile.runtime" "$UNRAID_HOST:$APPDATA/build/Dockerfile"

echo "==> building image on server"
"${SSH[@]}" "$UNRAID_HOST" "cd $APPDATA/build && docker build -q -t $IMAGE . >/dev/null && echo built"

if [[ "$MODE" == "image-only" ]]; then exit 0; fi

echo "==> recreating container $NAME (host network, /dev/dri)"
"${SSH[@]}" "$UNRAID_HOST" "docker rm -f $NAME >/dev/null 2>&1 || true; docker run -d --name $NAME --restart unless-stopped \
  --network host --device /dev/dri \
  -e TZ=$TZ_NAME \
  -v $APPDATA/config:/config -v $RECORDINGS:/config/work/recordings \
  $IMAGE >/dev/null && echo started"

echo "==> waiting for health"
HOSTIP="${UNRAID_HOST#*@}"
for _ in $(seq 1 30); do
  if curl -fsS -m 2 "http://$HOSTIP:8477/api/v1/health" >/dev/null 2>&1; then
    curl -s "http://$HOSTIP:8477/api/v1/server"; echo
    "${SSH[@]}" "$UNRAID_HOST" "docker logs --tail 8 $NAME 2>&1"
    exit 0
  fi
  sleep 2
done
echo "server did not become healthy; recent logs:"
"${SSH[@]}" "$UNRAID_HOST" "docker logs --tail 40 $NAME 2>&1"
exit 1
