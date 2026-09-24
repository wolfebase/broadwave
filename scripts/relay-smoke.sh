#!/usr/bin/env bash
# End-to-end relay test with no tuner: ffmpeg serves a live test broadcast over
# HTTP, the server tunes it as a "link" source, and clients with different
# capabilities watch at once. Verifies renditions, the shared timeline, and the
# export path. Needs ffmpeg, curl, python3, sqlite3.
#
#   scripts/relay-smoke.sh
#   FAKE=1 scripts/relay-smoke.sh   # tune a local HDHomeRun fake instead of a link
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PORT=18499
SRC_PORT=18500
T=$(mktemp -d)
BIN="$T/broadwave-smoke"
NAME="Smoke Broadcast"
DIRECT="copy.copy"
(cd "$ROOT" && go build -o "$BIN" ./server/cmd/broadwave) || exit 1

if [ "${FAKE:-}" = 1 ]; then
  ffmpeg -hide_banner -loglevel error \
    -f lavfi -i "testsrc2=size=1280x720:rate=60000/1001" -f lavfi -i "sine=frequency=500" \
    -t 4 -c:v libx264 -preset ultrafast -g 30 -pix_fmt yuv420p -c:a ac3 \
    -f mpegts "$T/sample.ts" >"$T/src.log" 2>&1 || exit 1
  (cd "$ROOT" && go build -o "$T/fakehdhr" ./server/cmd/fakehdhr) || exit 1
  "$T/fakehdhr" -realtime -ts "$T/sample.ts" >"$T/fake.txt" 2>&1 &
  SRC=$!
  for _ in 1 2 3 4 5 6 7 8 9 10; do
    grep -q '^BASE=' "$T/fake.txt" 2>/dev/null && break
    sleep 0.3
  done
  BASE=$(sed -n 's/^BASE=//p' "$T/fake.txt")
  CONTROL_PORT=$(sed -n 's/^CONTROL_PORT=//p' "$T/fake.txt")
  export HDHR_CONTROL_PORT="$CONTROL_PORT"
  HDHR=${BASE#http://}
  NAME="WDAF"
  DIRECT="1080.copy.broadcast"
  "$BIN" -config "$T" -addr "127.0.0.1:$PORT" -hdhr "$HDHR" -bonjour=false >"$T/log.txt" 2>&1 &
else
  ffmpeg -hide_banner -loglevel error -re \
    -f lavfi -i "testsrc2=size=1280x720:rate=60000/1001" -f lavfi -i "sine=frequency=500" \
    -c:v libx264 -preset ultrafast -g 30 -pix_fmt yuv420p -c:a ac3 \
    -f mpegts -listen 1 "http://127.0.0.1:$SRC_PORT/live.ts" >"$T/src.log" 2>&1 &
  SRC=$!
  sleep 1
  "$BIN" -config "$T" -addr "127.0.0.1:$PORT" -hdhr 127.0.0.1:1 -bonjour=false >"$T/log.txt" 2>&1 &
fi
SRV=$!
trap 'kill $SRV $SRC 2>/dev/null; wait 2>/dev/null' EXIT
sleep 2
API="http://127.0.0.1:$PORT/api/v1"
if [ "${FAKE:-}" != 1 ]; then
  curl -s -XPOST "$API/sources" -d "{\"kind\":\"link\",\"name\":\"Smoke Broadcast\",\"url\":\"http://127.0.0.1:$SRC_PORT/live.ts\"}" >/dev/null
fi
ID=$(curl -s "$API/channels" | python3 -c "import sys,json; print([c['id'] for c in json.load(sys.stdin)['channels'] if c['displayName']=='$NAME'][0])")
fail=0
check() { if eval "$2"; then echo "PASS $1"; else echo "FAIL $1"; fail=1; fi; }

TV=$(curl -s -XPOST "$API/watch" -d "{\"channelId\":$ID,\"caps\":{\"platform\":\"tvos\",\"video\":[\"h264\",\"hevc\"],\"audio\":[\"aac\",\"ac3\"]}}")
check "apple tv session" "echo '$TV' | grep -q rendition"
sleep 6
FO=$(sqlite3 "$T/broadwave.db" "select field_order from channels where id=$ID")
check "field-order probe recorded ($FO)" "[ -n '$FO' ]"
TV2=$(curl -s -XPOST "$API/watch" -d "{\"channelId\":$ID,\"caps\":{\"platform\":\"tvos\",\"video\":[\"h264\",\"hevc\"],\"audio\":[\"aac\",\"ac3\"]}}" | python3 -c "import sys,json;print(json.load(sys.stdin)['rendition'])")
check "progressive h264 goes direct ($DIRECT, got $TV2)" "[ '$TV2' = '$DIRECT' ]"
PH=$(curl -s -XPOST "$API/watch" -d "{\"channelId\":$ID,\"caps\":{\"platform\":\"ios\",\"video\":[\"h264\"],\"audio\":[\"aac\"]},\"prefs\":{\"quality\":\"saver\"}}" | python3 -c "import sys,json;print(json.load(sys.stdin)['rendition'])")
check "data saver gets its own rendition ($PH)" "[ '$PH' = '540.aac2.broadcast' ]"
TILE=$(curl -s -XPOST "$API/watch" -d "{\"channelId\":$ID,\"caps\":{\"platform\":\"web\",\"video\":[\"h264\"],\"audio\":[\"aac\"]},\"prefs\":{\"quality\":\"tile\",\"audio\":\"none\"}}" | python3 -c "import sys,json;print(json.load(sys.stdin)['rendition'])")
check "a tile is silent 540 ($TILE)" "[ '$TILE' = '540.none.broadcast' ]"
SMALL=$(curl -s -XPOST "$API/watch" -d "{\"channelId\":$ID,\"caps\":{\"platform\":\"web\",\"video\":[\"h264\"],\"audio\":[\"aac\"]},\"prefs\":{\"quality\":\"360\",\"audio\":\"none\"}}" | python3 -c "import sys,json;print(json.load(sys.stdin)['rendition'])")
check "a small tile is silent 360 ($SMALL)" "[ '$SMALL' = '360.none.broadcast' ]"
PLAN=$(curl -s -XPOST "$API/multiview/plan" -d "{\"channelIds\":[$ID]}")
check "the plan can play the channel" "echo '$PLAN' | grep -q '\"channelId\":$ID'"
sleep 6
for k in "$DIRECT" 540.aac2.broadcast 540.none.broadcast 360.none.broadcast; do
  PL=$(curl -s "http://127.0.0.1:$PORT/media/live/$ID/$k/index.m3u8")
  check "$k playlist has program date-times" "echo '$PL' | grep -q PROGRAM-DATE-TIME"
  check "$k is CMAF with init segment" "echo '$PL' | grep -q 'EXT-X-MAP'"
  check "$k withholds segment 0" "! echo '$PL' | grep -q 'seg00000'"
done
curl -s -m 10 "http://127.0.0.1:$PORT/export/stream/$ID" -o "$T/export.ts"
SZ=$(stat -f%z "$T/export.ts" 2>/dev/null || stat -c%s "$T/export.ts")
check "export stream carries video ($SZ bytes)" "[ ${SZ:-0} -gt 100000 ]"
check "m3u export lists the channel" "curl -s http://127.0.0.1:$PORT/export/lineup.m3u | grep -q '$NAME'"
curl -s -XPOST "$API/watch/$ID/stop" -d '{}' >/dev/null
echo "logs: $T"
exit $fail
