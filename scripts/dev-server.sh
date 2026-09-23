#!/usr/bin/env bash
# Rebuild the web app + server and (re)start a dev server safely.
#
#   scripts/dev-server.sh                 # real catalog copy in /tmp/otav-live, port 18477
#   CONFIG=/tmp/otav-fresh FRESH=1 scripts/dev-server.sh   # empty catalog (first-run / setup wizard)
#   SKIP_WEB=1 scripts/dev-server.sh      # server-only change
#
# Why this exists: a killed server can linger long enough that the next one fails
# with "address already in use", and `pkill -f otav` matches the calling shell.
# This script kills by exact process name, waits for the port, then starts in the
# foreground (run it with the Shell tool's block_until_ms: 0 to background it).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PORT="${PORT:-18477}"
CONFIG="${CONFIG:-/tmp/otav-live}"
BIN="${BIN:-/tmp/otav}"

if [[ -z "${SKIP_WEB:-}" ]]; then
  (cd "$ROOT/web" && npm run build >/dev/null)
fi
(cd "$ROOT" && go build -o "$BIN" ./server/cmd/waveguide)

pkill -9 -x "$(basename "$BIN")" 2>/dev/null || true
for _ in $(seq 1 40); do
  lsof -nP -iTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1 || break
  sleep 0.25
done

if [[ -n "${FRESH:-}" ]]; then
  rm -rf "$CONFIG"
fi
mkdir -p "$CONFIG"
if [[ ! -f "$CONFIG/waveguide.db" && ! -f "$CONFIG/ota-viewer.db" && -z "${FRESH:-}" ]]; then
  # The server adopts a pre-rename ota-viewer.db on open.
  for name in waveguide ota-viewer; do
    if [[ -f "$ROOT/data/$name.db" ]]; then cp "$ROOT/data/$name".db* "$CONFIG"/; break; fi
  done
fi

echo "starting $BIN on :$PORT with $CONFIG (log: $CONFIG/stderr.log)"
exec "$BIN" -config "$CONFIG" -addr ":$PORT" 2>"$CONFIG/stderr.log"
