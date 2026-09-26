#!/usr/bin/env bash
# Fetch four-minute Blender open-movie clips, build a looping HLS lineup,
# and serve it from a -staging Broadwave on 127.0.0.1:18520.
#
#   scripts/demo-lineup.sh        # build, serve, warm artwork
#   scripts/demo-lineup.sh stop   # stop the server and the HLS loops
#
# Channels are 4.1 Big Buck Bunny, 5.1 Sintel, 9.1 Tears of Steel, and
# 11.1 Elephants Dream (CC BY). Artwork is a bright frame that is not
# blood-red. Nothing here tunes the house HDHomeRun.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${DEMO_LINEUP_DIR:-$ROOT/data/demo-lineup}"
WWW="$OUT/www"
CFG="$OUT/config"
LOGS="$OUT/logs"
PIDS="$OUT/pids"
API_PORT="${DEMO_API_PORT:-18520}"
HTTP_PORT="${DEMO_HTTP_PORT:-18521}"
BASE="http://127.0.0.1:${API_PORT}"
FILES="http://127.0.0.1:${HTTP_PORT}"
BIN="$ROOT/bin/broadwave"

stop_lineup() {
  if [[ -f "$PIDS" ]]; then
    while read -r pid; do
      [[ -n "$pid" ]] || continue
      kill "$pid" 2>/dev/null || true
    done <"$PIDS"
    sleep 0.5
    while read -r pid; do
      [[ -n "$pid" ]] || continue
      kill -9 "$pid" 2>/dev/null || true
    done <"$PIDS"
    rm -f "$PIDS"
  fi
}

if [[ "${1:-}" == "stop" ]]; then
  stop_lineup
  echo "stopped"
  exit 0
fi

command -v ffmpeg >/dev/null
command -v ffprobe >/dev/null
command -v python3 >/dev/null
mkdir -p "$WWW/art" "$WWW/hls" "$WWW/media" "$CFG" "$LOGS" "$ROOT/.evidence/as3"
stop_lineup

# Refuse a port we did not record. A second copy must not kill a stranger.
for port in "$API_PORT" "$HTTP_PORT"; do
  if lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "port $port is already in use" >&2
    exit 1
  fi
done

if [[ ! -x "$BIN" ]]; then
  (cd "$ROOT" && go build -o "$BIN" ./server/cmd/broadwave)
fi

# Item ids from the plan. These files are the real pictures, not the
# mislabeled 360p mp4s archive.org also hosts under a 720p name.
# BBB 1280x720, Sintel 2048x872, Tears of Steel 1280x534, Elephants Dream 1024x576.
declare -a SLUGS=(bbb sintel tos ed)
declare -a NUMS=(4.1 5.1 9.1 11.1)
declare -a TITLES=("Big Buck Bunny" "Sintel" "Tears of Steel" "Elephants Dream")
declare -a URLS=(
  "https://archive.org/download/BigBuckBunny_124/Content/big_buck_bunny_720p_surround.avi"
  "https://archive.org/download/Sintel/sintel-2048-stereo.mp4"
  "https://archive.org/download/Tears-of-Steel/tears_of_steel_720p.mkv"
  "https://archive.org/download/ElephantsDream/ed_1024.avi"
)
# Open on a bright stretch. Artwork is chosen again from the encoded clip.
declare -a STARTS=(30 40 20 45)

encode_one() {
  local slug="$1" url="$2" start="$3" dest="$4"
  if [[ -f "$dest" && "${DEMO_REUSE:-}" == "1" ]]; then
    echo "reuse $dest"
    return
  fi
  echo "encoding $slug from ${start}s"
  ffmpeg -y -hide_banner -loglevel error \
    -user_agent "Broadwave demo-lineup" \
    -ss "$start" -t 240 -i "$url" \
    -vf "scale=1280:720:force_original_aspect_ratio=decrease,pad=1280:720:(ow-iw)/2:(oh-ih)/2:black,fps=30000/1001" \
    -c:v libx264 -preset veryfast -crf 20 -pix_fmt yuv420p \
    -c:a aac -ac 2 -b:a 160k \
    -f mpegts "$dest"
  local probe
  # MPEG-TS makes ffprobe print the same stream twice, with a blank line between.
  probe="$(ffprobe -v error -select_streams v:0 -show_entries stream=width,height -of csv=p=0 "$dest" | awk 'NF { print; exit }')"
  echo "$slug $probe"
  if [[ "$probe" != "1280,720" ]]; then
    echo "bad picture size for $slug: $probe" >&2
    exit 1
  fi
}

for i in 0 1 2 3; do
  encode_one "${SLUGS[$i]}" "${URLS[$i]}" "${STARTS[$i]}" "$WWW/media/${SLUGS[$i]}.ts"
done

python3 - "$WWW" <<'PY'
import subprocess, sys
from pathlib import Path
www = Path(sys.argv[1])

def stats(ts, t):
    raw = subprocess.check_output([
        "ffmpeg", "-v", "error", "-ss", str(t), "-i", str(ts),
        "-frames:v", "1", "-vf", "scale=64:36", "-f", "rawvideo", "-pix_fmt", "rgb24", "-",
    ])
    n = 64 * 36
    if len(raw) < n * 3:
        return None
    rs = gs = bs = 0
    for i in range(n):
        r, g, b = raw[i * 3], raw[i * 3 + 1], raw[i * 3 + 2]
        rs += r
        gs += g
        bs += b
    r, g, b = rs / n, gs / n, bs / n
    y = 0.2126 * r + 0.7152 * g + 0.0722 * b
    red = r - max(g, b)
    return y, red, r

for slug in ("bbb", "sintel", "tos", "ed"):
    ts = www / "media" / f"{slug}.ts"
    best = None
    for t in range(8, 230, 12):
        got = stats(ts, t)
        if got is None:
            continue
        y, red, r = got
        bloody = red > 28 and r > 90
        dark = y < 55
        score = y - (120 if bloody else 0) - (50 if dark else 0)
        if best is None or score > best[0]:
            best = (score, t, y, red, bloody, dark)
    if best is None:
        raise SystemExit(f"no frame in {ts}")
    _, t, y, red, bloody, dark = best
    if bloody:
        raise SystemExit(f"{slug} art at {t}s is still blood-red (red {red:.0f})")
    jpg = www / "art" / f"{slug}.jpg"
    subprocess.check_call([
        "ffmpeg", "-y", "-v", "error", "-ss", str(t), "-i", str(ts),
        "-frames:v", "1", "-vf", "scale=1280:720", "-q:v", "3", str(jpg),
    ])
    print(f"art {slug} t={t}s y={y:.0f} red={red:.0f} dark={dark}")
PY

python3 - "$WWW" "$FILES" <<'PY'
import sys
from datetime import datetime, timedelta, timezone
from pathlib import Path
www, files = Path(sys.argv[1]), sys.argv[2]
films = [
    ("bbb", "4.1", "Big Buck Bunny"),
    ("sintel", "5.1", "Sintel"),
    ("tos", "9.1", "Tears of Steel"),
    ("ed", "11.1", "Elephants Dream"),
]
lines = ['#EXTM3U url-tvg="%s/guide.xml"' % files]
for slug, number, title in films:
    logo = f"{files}/art/{slug}.jpg"
    lines.append(
        f'#EXTINF:-1 tvg-id="{number}" tvg-chno="{number}" tvg-name="{title}" '
        f'tvg-logo="{logo}" group-title="Demo",{title}'
    )
    lines.append(f"{files}/hls/{slug}/index.m3u8")
(www / "lineup.m3u").write_text("\n".join(lines) + "\n")

now = datetime.now(timezone.utc).replace(minute=0, second=0, microsecond=0) - timedelta(hours=1)

def stamp(when):
    return when.strftime("%Y%m%d%H%M%S +0000")

bits = ['<?xml version="1.0" encoding="UTF-8"?>', "<tv>"]
for slug, number, title in films:
    icon = f"{files}/art/{slug}.jpg"
    bits.append(f'  <channel id="{number}">')
    bits.append(f"    <display-name>{number}</display-name>")
    bits.append(f"    <display-name>{title}</display-name>")
    bits.append(f'    <icon src="{icon}" width="1280" height="720"/>')
    bits.append("  </channel>")
for slug, number, title in films:
    icon = f"{files}/art/{slug}.jpg"
    cursor = now
    for _ in range(16):
        end = cursor + timedelta(minutes=30)
        bits.append(f'  <programme start="{stamp(cursor)}" stop="{stamp(end)}" channel="{number}">')
        bits.append(f"    <title>{title}</title>")
        bits.append("    <sub-title>Open movie</sub-title>")
        bits.append("    <desc>A Blender Foundation film, under CC BY.</desc>")
        bits.append(f'    <icon src="{icon}" width="1280" height="720"/>')
        bits.append("  </programme>")
        cursor = end
bits.append("</tv>")
(www / "guide.xml").write_text("\n".join(bits) + "\n")
print("wrote lineup.m3u and guide.xml")
PY

: >"$PIDS"
nohup python3 -m http.server "$HTTP_PORT" --bind 127.0.0.1 --directory "$WWW" >>"$LOGS/http.log" 2>&1 &
echo $! >>"$PIDS"

for slug in "${SLUGS[@]}"; do
  mkdir -p "$WWW/hls/$slug"
  nohup ffmpeg -hide_banner -loglevel error -re -stream_loop -1 \
    -i "$WWW/media/$slug.ts" \
    -c copy -f hls -hls_time 4 -hls_list_size 8 \
    -hls_flags delete_segments+omit_endlist+independent_segments \
    -hls_segment_filename "$WWW/hls/$slug/seg%05d.ts" \
    "$WWW/hls/$slug/index.m3u8" >>"$LOGS/hls-$slug.log" 2>&1 &
  echo $! >>"$PIDS"
done

python3 - "$WWW" <<'PY'
import sys, time
from pathlib import Path
www = Path(sys.argv[1])
deadline = time.time() + 30
need = [www / "hls" / slug / "index.m3u8" for slug in ("bbb", "sintel", "tos", "ed")]
while time.time() < deadline:
    if all(p.exists() and "EXTINF" in p.read_text(errors="replace") for p in need):
        print("hls playlists are up")
        break
    time.sleep(0.5)
else:
    raise SystemExit("looping HLS did not publish a segment")
PY

rm -rf "$CFG"
mkdir -p "$CFG"
nohup "$BIN" -addr "127.0.0.1:${API_PORT}" -config "$CFG" -staging -bonjour=false -hdhr 127.0.0.1:9 \
  >>"$LOGS/broadwave.log" 2>&1 &
echo $! >>"$PIDS"

python3 - "$BASE" "$FILES" "$ROOT/.evidence/as3/demo-lineup.txt" <<'PY'
import json, sys, time, urllib.request
base, files, evidence = sys.argv[1], sys.argv[2], sys.argv[3]

def call(method, path, body=None, timeout=40):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(base + path, data=data, method=method, headers={
        "Content-Type": "application/json",
    })
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        raw = resp.read()
        return resp.status, dict(resp.headers), raw

deadline = time.time() + 25
while time.time() < deadline:
    try:
        urllib.request.urlopen(base + "/api/v1/server", timeout=2).read()
        break
    except Exception:
        time.sleep(0.4)
else:
    raise SystemExit("staging server did not answer")

status, _, raw = call("POST", "/api/v1/sources", {
    "kind": "m3u",
    "name": "Demo",
    "url": files + "/lineup.m3u",
    "xmltvUrl": files + "/guide.xml",
})
added = json.loads(raw)
print("source", status, added.get("name"), "channels field", list(added)[:8])
status, _, raw = call("PUT", "/api/v1/settings", {"setupComplete": "1"})
settings = json.loads(raw)
print("setup", settings.get("setupComplete"), settings.get("needsSetup"))
status, _, raw = call("PATCH", "/api/v1/server", {"name": "Living Room"})
info = json.loads(raw)
print("server", info.get("name"), info.get("id", "")[:8])

_, _, raw = call("GET", "/api/v1/channels")
channels = json.loads(raw).get("channels", [])
numbers = sorted(ch.get("guideNumber", "") for ch in channels)
print("channels", numbers)
want = {"4.1", "5.1", "9.1", "11.1"}
if set(numbers) != want:
    raise SystemExit(f"lineup numbers {numbers}")

_, _, raw = call("GET", "/api/v1/airings")
airings = json.loads(raw).get("airings", [])
print("airings", len(airings))
if len(airings) < 4:
    raise SystemExit("guide did not load")

warmed = []
for ch in channels:
    for width in (640, 1600):
        path = f"/media/art/channel/{ch['id']}?w={width}"
        status, headers, body = call("GET", path)
        ctype = headers.get("Content-Type", "")
        print(f"art channel {ch['guideNumber']} w={width} {status} {ctype} {len(body)} bytes")
        if "svg" in ctype or len(body) < 800:
            raise SystemExit(f"channel art missing for {ch['guideNumber']}")
        warmed.append((path, len(body), ctype))
seen = set()
for air in airings:
    if not air.get("imageUrl"):
        continue
    key = air["id"]
    if key in seen:
        continue
    seen.add(key)
    if len(seen) > 8:
        break
    for width in (640, 1600):
        path = f"/media/art/airing/{key}?w={width}"
        status, headers, body = call("GET", path)
        ctype = headers.get("Content-Type", "")
        print(f"art airing {key} w={width} {status} {ctype} {len(body)} bytes")
        if "svg" in ctype or len(body) < 800:
            raise SystemExit(f"airing art missing for {key}")

text = "\n".join([
    f"server {info.get('name')} at {base}",
    f"channels {', '.join(numbers)}",
    f"airings {len(airings)}",
    f"setupComplete {settings.get('setupComplete')} needsSetup {settings.get('needsSetup')}",
]) + "\n"
open(evidence, "w").write(text)
print(text)
print("left running. stop with: scripts/demo-lineup.sh stop")
PY
