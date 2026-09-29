#!/usr/bin/env bash
# End-to-end relay test with no tuner: ffmpeg serves a live test broadcast over
# HTTP, the server tunes it as a "link" source, and clients with different
# capabilities watch at once. Verifies renditions, the shared timeline, and the
# export path. Needs ffmpeg, curl, python3, sqlite3.
#
#   scripts/relay-smoke.sh
#   FAKE=1 scripts/relay-smoke.sh   # tune a local HDHomeRun fake instead of a link
set -uo pipefail
# This check opens a direct picture, a data-saver picture, and two tiles at
# once. The startup benchmark would refuse the extra pictures on a small host,
# so the check does not apply it. The budget has its own tests.
export BROADWAVE_BENCH=0
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PORT=18499
SRC_PORT=18500
T=$(mktemp -d)
BIN="$T/broadwave-smoke"
NAME="Smoke Broadcast"
DIRECT="copy.copy"
(cd "$ROOT" && go build -o "$BIN" ./server/cmd/broadwave) || exit 1

if [ "${FAKE:-}" = 1 ]; then
  # Two sound tracks, English and Spanish, carried in one encode.
  export BROADWAVE_ALTERNATES=1
  ffmpeg -hide_banner -loglevel error \
    -f lavfi -i "testsrc2=size=1280x720:rate=60000/1001" -f lavfi -i "sine=frequency=500" -f lavfi -i "sine=frequency=900" \
    -map 0 -map 1 -map 2 -t 30 -c:v libx264 -preset ultrafast -g 30 -pix_fmt yuv420p -c:a ac3 \
    -metadata:s:a:0 language=eng -metadata:s:a:1 language=spa \
    -f mpegts "$T/sample.ts" >"$T/src.log" 2>&1 || exit 1
  (cd "$ROOT" && go build -o "$T/fakehdhr" ./server/cmd/fakehdhr) || exit 1
  "$T/fakehdhr" -realtime -ts "$T/sample.ts" -source "$T/sample.ts" >"$T/fake.txt" 2>&1 &
  SRC=$!
  for _ in 1 2 3 4 5 6 7 8 9 10; do
    grep -q '^BASE=' "$T/fake.txt" 2>/dev/null && break
    sleep 0.3
  done
  BASE=$(sed -n 's/^BASE=//p' "$T/fake.txt")
  CONTROL_PORT=$(sed -n 's/^CONTROL_PORT=//p' "$T/fake.txt")
  export HDHR_CONTROL_PORT="$CONTROL_PORT"
  HDHR=${BASE#http://}
  NAME="KBWV"
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
if [ "$DIRECT" = "1080.copy.broadcast" ]; then
  HEVC=$(curl -s "$API/diagnostics" | python3 -c "import json,sys; print(json.load(sys.stdin).get('encoder',{}).get('hevc'))")
  if [ "$HEVC" = "True" ]; then
    DIRECT="1080.copy.broadcast.hevc"
  fi
fi
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
  # grep -q under pipefail exits before echo finishes on a long playlist,
  # and the resulting SIGPIPE fails a check that already matched.
  check "$k playlist has program date-times" '[[ "$PL" == *PROGRAM-DATE-TIME* ]]'
  check "$k is CMAF with init segment" '[[ "$PL" == *EXT-X-MAP* ]]'
  check "$k serves segment 0" '[[ "$PL" == *seg00000* ]]'
done
# Views: the picture alone and the sound alone, cut from the same encode.
for k in "$DIRECT" 540.aac2.broadcast; do
  M="http://127.0.0.1:$PORT/media/live/$ID/$k"
  VP=$(curl -s "$M/video.m3u8")
  AP=$(curl -s "$M/audio-2.m3u8")
  check "$k video view names its own media" '[[ "$VP" == *init.v.mp4* && "$VP" == *seg00000.v.m4s* && "$VP" == *RENDITION-REPORT* ]]'
  check "$k sound view names its own media" '[[ "$AP" == *init.a2.mp4* && "$AP" == *seg00000.a2.m4s* ]]'
  { curl -s "$M/init.v.mp4"; curl -s "$M/seg00000.v.m4s"; } >"$T/v.mp4"
  { curl -s "$M/init.a2.mp4"; curl -s "$M/seg00000.a2.m4s"; } >"$T/a.mp4"
  VS=$(ffprobe -v error -show_entries stream=codec_type -of default=nw=1:nk=1 "$T/v.mp4" | tr '\n' ' ')
  AS=$(ffprobe -v error -show_entries stream=codec_type -of default=nw=1:nk=1 "$T/a.mp4" | tr '\n' ' ')
  check "$k video view is the picture alone ($VS)" '[ "$VS" = "video " ]'
  check "$k sound view is the sound alone ($AS)" '[ "$AS" = "audio " ]'
  LAST=$(echo "$AP" | sed -n 's/.*LAST-MSN=\([0-9]*\).*/\1/p' | head -1)
  # A block ends after 1.5 s without the part, and a player asks again. On a
  # slow runner one part can take longer than that.
  NEXT=0
  for _ in 1 2 3; do
    NEXT=$(curl -s -m 5 "$M/audio-2.m3u8?_HLS_msn=$((LAST + 1))&_HLS_part=0" | sed -n 's/.*LAST-MSN=\([0-9]*\).*/\1/p' | head -1)
    [ "${NEXT:-0}" -gt "${LAST:-0}" ] && break
  done
  check "$k sound view blocks until the next segment ($LAST -> $NEXT)" '[ "${NEXT:-0}" -gt "${LAST:-0}" ]'
done
if [ "${FAKE:-}" = 1 ]; then
  M="http://127.0.0.1:$PORT/media/live/$ID/$DIRECT"
  MP=$(curl -s "$M/master.m3u8")
  check "$DIRECT master lists both sound tracks" '[[ "$MP" == *audio-2.m3u8* && "$MP" == *audio-3.m3u8* && "$MP" == *LANGUAGE=\"es\"* && "$MP" == *video.m3u8* ]]'
  { curl -s "$M/init.a3.mp4"; curl -s "$M/seg00000.a3.m4s"; } >"$T/a3.mp4"
  { curl -s "$M/init.mp4"; curl -s "$M/seg00000.m4s"; } >"$T/legacy.mp4"
  A3=$(ffprobe -v error -show_entries stream=codec_type -of default=nw=1:nk=1 "$T/a3.mp4" | tr '\n' ' ')
  LS=$(ffprobe -v error -show_entries stream=codec_type -of default=nw=1:nk=1 "$T/legacy.mp4" | tr '\n' ' ')
  check "$DIRECT second sound view is one sound ($A3)" '[ "$A3" = "audio " ]'
  check "$DIRECT legacy names carry one sound ($LS)" '[ "$LS" = "video audio " ]'
fi
curl -s -m 10 "http://127.0.0.1:$PORT/export/stream/$ID" -o "$T/export.ts"
SZ=$(stat -f%z "$T/export.ts" 2>/dev/null || stat -c%s "$T/export.ts")
check "export stream carries video ($SZ bytes)" "[ ${SZ:-0} -gt 100000 ]"
check "m3u export lists the channel" "curl -s http://127.0.0.1:$PORT/export/lineup.m3u | grep -q '$NAME'"
curl -s -XPOST "$API/watch/$ID/stop" -d '{}' >/dev/null
echo "logs: $T"
# A hosted runner keeps nothing from $T; show the server's side of a failure.
if [ "$fail" != 0 ]; then
  grep -v "GET /media" "$T/log.txt" | tail -60
fi
exit $fail
