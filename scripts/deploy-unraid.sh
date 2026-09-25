#!/usr/bin/env bash
# Build Broadwave for linux/amd64 and deploy it to an Unraid server over SSH.
#
#   UNRAID_HOST=root@192.168.1.2 scripts/deploy-unraid.sh            # build + deploy + recreate
#   UNRAID_HOST=... MODE=image-only scripts/deploy-unraid.sh           # build image, keep container
#   UNRAID_HOST=... MODE=ghcr scripts/deploy-unraid.sh                 # pull the public image on the server
#   UNRAID_HOST=... MODE=staging scripts/deploy-unraid.sh              # branch binary on Broadwave-Staging :8490
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

if [[ "$MODE" == "staging" ]]; then
  # Staging sits beside production. This branch never names the production
  # container, so a rerun cannot remove it.
  STAGING_NAME="Broadwave-Staging"
  STAGING_APPDATA="${STAGING_APPDATA:-/mnt/cache/appdata/broadwave-staging}"
  STAGING_PORT="${STAGING_PORT:-8490}"
  if [[ "$STAGING_NAME" == "$NAME" || "$STAGING_APPDATA" == "$APPDATA" ]]; then
    echo "refusing to touch $NAME ($APPDATA)" >&2
    exit 1
  fi
  echo "==> building web + linux/amd64 binary for staging ($VERSION)"
  if [[ "${SKIP_WEB:-}" != "1" ]]; then
    (cd "$ROOT/web" && npm run build >/dev/null)
  fi
  (cd "$ROOT/server" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o "$ROOT/bin/broadwave-linux-amd64" ./cmd/broadwave)
  LOCAL_SUM=$(shasum -a 256 "$ROOT/bin/broadwave-linux-amd64" | awk '{print $1}')
  echo "==> uploading binary to $STAGING_APPDATA (not $APPDATA)"
  REMOTE_SUM=""
  for _try in 1 2; do
    REMOTE_SUM=$(gzip -c "$ROOT/bin/broadwave-linux-amd64" | "${SSH[@]}" "$UNRAID_HOST" "mkdir -p '$STAGING_APPDATA' && gunzip -c > '$STAGING_APPDATA/broadwave.new' && sha256sum '$STAGING_APPDATA/broadwave.new'" | awk '{print $1}')
    [[ "$REMOTE_SUM" == "$LOCAL_SUM" ]] && break
    echo "checksum mismatch (try $_try); retrying"
  done
  if [[ "$REMOTE_SUM" != "$LOCAL_SUM" ]]; then
    echo "binary upload checksum mismatch" >&2
    exit 1
  fi
  echo "==> recreating $STAGING_NAME on :$STAGING_PORT"
  "${SSH[@]}" "$UNRAID_HOST" "bash -s" <<EOF
set -euo pipefail
NAME="$STAGING_NAME"
APPDATA="$STAGING_APPDATA"
PROD="$APPDATA"
PORT="$STAGING_PORT"
TZ_NAME="$TZ_NAME"
if [ "\$NAME" = "Broadwave" ] || [ "\$APPDATA" = "\$PROD" ]; then
  echo "refusing to touch Broadwave" >&2
  exit 1
fi
mkdir -p "\$APPDATA/config"
if [ ! -f "\$APPDATA/config/broadwave.db" ]; then
  latest=\$(ls -1t "\$PROD/backups/"*.db 2>/dev/null | head -1 || true)
  if [ -z "\$latest" ]; then
    echo "no production backup to copy" >&2
    exit 1
  fi
  cp "\$latest" "\$APPDATA/config/broadwave.db"
  sqlite3 "\$APPDATA/config/broadwave.db" "UPDATE server_identity SET server_id=lower(hex(randomblob(16))) WHERE id=1;"
fi
sqlite3 "\$APPDATA/config/broadwave.db" "DELETE FROM passes; UPDATE server_identity SET name='Broadwave Staging' WHERE id=1;"
mv "\$APPDATA/broadwave.new" "\$APPDATA/broadwave"
chmod +x "\$APPDATA/broadwave"
IMAGE="${STAGING_IMAGE:-\$(docker inspect -f '{{.Config.Image}}' "\$NAME" 2>/dev/null || docker inspect -f '{{.Config.Image}}' Broadwave 2>/dev/null || echo ghcr.io/wolfebase/broadwave:0.6.0)}"
docker rm -f "\$NAME" >/dev/null 2>&1 || true
docker run -d --name "\$NAME" --restart no --network host --device /dev/dri \\
  --cpus 6 --memory 3g --no-healthcheck -e TZ="\$TZ_NAME" \\
  -v "\$APPDATA/config:/config" \\
  -v "\$APPDATA/broadwave:/usr/local/bin/broadwave-staging:ro" \\
  --entrypoint /usr/local/bin/broadwave-staging \\
  "\$IMAGE" -config /config -addr ":\$PORT" -bonjour=false -staging >/dev/null
echo started "\$IMAGE"
EOF
  HOSTIP="${UNRAID_HOST#*@}"
  echo "==> waiting for health on :$STAGING_PORT"
  for _ in $(seq 1 30); do
    if curl -fsS -m 2 "http://$HOSTIP:$STAGING_PORT/api/v1/health" >/dev/null 2>&1; then
      curl -s "http://$HOSTIP:$STAGING_PORT/api/v1/server"; echo
      "${SSH[@]}" "$UNRAID_HOST" "docker inspect -f '{{.Name}} {{.Config.Entrypoint}} {{.Args}}' $STAGING_NAME"
      exit 0
    fi
    sleep 2
  done
  echo "staging did not become healthy; recent logs:"
  "${SSH[@]}" "$UNRAID_HOST" "docker logs --tail 40 $STAGING_NAME 2>&1"
  exit 1
fi

if [[ "$MODE" == "ghcr" ]]; then
  IMAGE="${GHCR_IMAGE:-ghcr.io/wolfebase/broadwave:latest}"
  echo "==> backing up catalog on $UNRAID_HOST"
  "${SSH[@]}" "$UNRAID_HOST" "mkdir -p $APPDATA/backups && [ -f $APPDATA/config/broadwave.db ] && cp $APPDATA/config/broadwave.db $APPDATA/backups/broadwave-\$(date +%Y%m%d-%H%M%S).db; true"
  echo "==> pulling $IMAGE on $UNRAID_HOST (detached) and recreating $NAME"
  # The pull runs on the server. A marker file is the signal that the new
  # container was started; /health stays up on the old container during the pull.
  "${SSH[@]}" "$UNRAID_HOST" "mkdir -p $APPDATA && rm -f $APPDATA/deploy.ok && nohup sh -c 'docker pull $IMAGE && docker rm -f $NAME >/dev/null 2>&1 || true; docker run -d --name $NAME --restart unless-stopped --network host --device /dev/dri -e TZ=$TZ_NAME -v $APPDATA/config:/config -v $RECORDINGS:/config/work/recordings $IMAGE && touch $APPDATA/deploy.ok' > $APPDATA/deploy.log 2>&1 & echo launched"
  HOSTIP="${UNRAID_HOST#*@}"
  for _ in $(seq 1 90); do
    if "${SSH[@]}" "$UNRAID_HOST" "test -f $APPDATA/deploy.ok" 2>/dev/null; then
      # The marker lands before the process is listening. Keep trying.
      if curl -fsS -m 5 "http://$HOSTIP:8477/api/v1/server"; then
        echo
        exit 0
      fi
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
