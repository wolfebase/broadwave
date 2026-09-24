#!/usr/bin/env bash
# Watch one channel through the API, measure the rendition, and stop.
#
#   scripts/staging-watch.sh <channelId> [seconds] [host:port]
#
# Prints "<width>x<height> <fps>" plus segment length, decode errors, and
# whether the tuner was released. Refuses to tune within 20 minutes of a
# production recording. Runs on the Mac (the Unraid host has no Python).
set -euo pipefail

CHANNEL="${1:?channel id}"
DURATION="${2:-15}"
HOSTPORT="${3:-192.168.1.2:8490}"
BASE="http://${HOSTPORT}"
PROD="http://192.168.1.2:8477"

python3 - "$PROD" "$BASE" <<'PY'
import json, sys, urllib.request
prod, base = sys.argv[1], sys.argv[2]
def get(url):
    with urllib.request.urlopen(url, timeout=8) as r:
        return json.load(r)
sched = get(prod + "/api/v1/schedule")
import datetime
now = datetime.datetime.now(datetime.timezone.utc)
for item in sched.get("items", []):
    if item.get("skipped"):
        continue
    air = item.get("airing") or {}
    start = air.get("start")
    if not start:
        continue
    when = datetime.datetime.fromisoformat(start.replace("Z", "+00:00"))
    if datetime.timedelta(0) <= when - now <= datetime.timedelta(minutes=20):
        sys.exit(f"refusing: recording starts at {start}")
for label, url in (("production", prod), ("target", base)):
    body = get(url + "/api/v1/tuners")
    busy = [t for t in body.get("tuners", body if isinstance(body, list) else []) if t.get("ours")]
    print(f"{label} tuners ours={len(busy)}", flush=True)
PY

WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

SESSION=$(curl -fsS -m 20 -X POST "$BASE/api/v1/watch" \
  -H 'Content-Type: application/json' \
  -d "{\"channelId\":$CHANNEL}")
PLAYLIST=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["playlist"])' <<<"$SESSION")
RENDITION=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["rendition"])' <<<"$SESSION")
echo "watching $PLAYLIST ($RENDITION) for ${DURATION}s"

sleep "$DURATION"

curl -fsS -m 15 "$BASE$PLAYLIST" -o "$WORKDIR/index.m3u8"
python3 - "$WORKDIR/index.m3u8" "$WORKDIR/files.txt" <<'PY'
import sys
text = open(sys.argv[1]).read().splitlines()
init = None
segs = []
for line in text:
    if line.startswith("#EXT-X-MAP:") and 'URI="' in line:
        init = line.split('URI="', 1)[1].split('"', 1)[0]
    elif line and not line.startswith("#"):
        segs.append(line.strip())
segs = [s for s in segs if "seg00000" not in s]
use = segs[-2:] if len(segs) >= 2 else segs
if not init or not use:
    sys.exit("playlist has no init or media segments")
open(sys.argv[2], "w").write(init + "\n" + "\n".join(use) + "\n")
print(f"segments {len(use)} of {len(segs)}")
PY

ORIGIN="${PLAYLIST%/*}"
while IFS= read -r name; do
  curl -fsS -m 20 "$BASE$ORIGIN/$name" -o "$WORKDIR/$name"
done < "$WORKDIR/files.txt"

cat "$WORKDIR"/init.mp4 "$WORKDIR"/seg*.m4s > "$WORKDIR/clip.mp4"
ffmpeg -v error -i "$WORKDIR/clip.mp4" -f null - >/dev/null 2>"$WORKDIR/ff.err" || true
ERRS=$(wc -l < "$WORKDIR/ff.err" | tr -d ' ')

python3 - "$WORKDIR/clip.mp4" "$WORKDIR/index.m3u8" "$ERRS" <<'PY'
import subprocess, sys
clip, playlist, errs = sys.argv[1], sys.argv[2], int(sys.argv[3])
probe = subprocess.check_output([
    "ffprobe", "-v", "error", "-select_streams", "v:0",
    "-show_entries", "stream=width,height", "-of", "csv=p=0", clip,
], text=True).strip()
w, h = probe.split(",")[:2]
pts = subprocess.check_output([
    "ffprobe", "-v", "error", "-select_streams", "v:0",
    "-show_entries", "frame=pts_time", "-of", "csv=p=0", clip,
], text=True).split()
times = [float(x) for x in pts if x.strip()]
if len(times) < 2:
    sys.exit(f"only {len(times)} frames")
fps = (len(times) - 1) / (times[-1] - times[0])
extinf = []
for line in open(playlist):
    if line.startswith("#EXTINF:"):
        extinf.append(float(line.split(":", 1)[1].split(",")[0]))
seg = sum(extinf) / len(extinf) if extinf else 0
print(f"{w}x{h} {fps:.2f}")
print(f"segment {seg:.3f}s  decode_errors {errs}  frames {len(times)}")
PY

curl -fsS -m 10 -X POST "$BASE/api/v1/watch/$CHANNEL/stop" \
  -H 'Content-Type: application/json' \
  -d "{\"rendition\":\"$RENDITION\"}" >/dev/null

released=0
for _ in $(seq 1 20); do
  if python3 - "$BASE" <<'PY'
import json, sys, urllib.request
url = sys.argv[1] + "/api/v1/tuners"
with urllib.request.urlopen(url, timeout=8) as r:
    body = json.load(r)
tuners = body.get("tuners", body if isinstance(body, list) else [])
sys.exit(0 if not any(t.get("ours") for t in tuners) else 1)
PY
  then
    released=1
    break
  fi
  sleep 2
done
if [[ "$released" != 1 ]]; then
  echo "tuner still held" >&2
  exit 1
fi
echo "tuner released"
