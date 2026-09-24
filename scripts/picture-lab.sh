#!/usr/bin/env bash
# Picture lab. Capture a short broadcast sample, then encode candidate ffmpeg
# graphs in throwaway bw-lab-* containers on the Unraid host at real time.
#
#   scripts/picture-lab.sh capture [channelId] [seconds]
#   scripts/picture-lab.sh run <sample.ts> [candidates-dir]
#   scripts/picture-lab.sh [channelId] [seconds]
#
# A candidate is a *.args file. Blank lines and lines starting with # are
# dropped. The remaining tokens are the ffmpeg arguments between the input
# and the output file. The lab always feeds the sample with -re.
#
# Prints a table: fps, frames, WxH, decode errors, encode speed, CPU, VMAF.
# Writes one still per candidate. Defaults talk to staging (:8490), never
# production. Refuses to tune within 20 minutes of a recording.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
UNRAID_HOST="${UNRAID_HOST:-root@192.168.1.2}"
PROD="${PROD:-http://192.168.1.2:8477}"
BASE="${BASE:-http://192.168.1.2:8490}"
IMAGE="${IMAGE:-ghcr.io/wolfebase/broadwave:0.6.0}"
OUT="${OUT:-$ROOT/docs/lab/runs/latest}"
CANDIDATES="${CANDIDATES:-$ROOT/scripts/picture-lab}"
SSH=(ssh -o ServerAliveInterval=15 -o ServerAliveCountMax=8 -o ConnectTimeout=20 -o IPQoS=none)

usage() {
  sed -n '2,16p' "$0" | sed 's/^# \{0,1\}//'
  exit 2
}

etiquette() {
  python3 - "$PROD" "$BASE" <<'PY'
import datetime, json, sys, urllib.request
prod, base = sys.argv[1], sys.argv[2]

def get(url):
    with urllib.request.urlopen(url, timeout=8) as r:
        return json.load(r)

sched = get(prod + "/api/v1/schedule")
now = datetime.datetime.now(datetime.timezone.utc)
for item in sched.get("items", []):
    if item.get("skipped"):
        continue
    start = (item.get("airing") or {}).get("start")
    if not start:
        continue
    when = datetime.datetime.fromisoformat(start.replace("Z", "+00:00"))
    if datetime.timedelta(0) <= when - now <= datetime.timedelta(minutes=20):
        sys.exit(f"refusing: recording starts at {start}")
for label, url in (("production", prod), ("target", base)):
    body = get(url + "/api/v1/tuners")
    tuners = body.get("tuners", body if isinstance(body, list) else [])
    ours = sum(1 for t in tuners if t.get("ours"))
    print(f"{label} tuners ours={ours}", flush=True)
    if ours:
        sys.exit(f"refusing: {label} is already using a tuner")
PY
}

capture() {
  local channel="${1:?channel id}"
  local seconds="${2:-12}"
  etiquette
  mkdir -p "$ROOT/docs/lab/samples"
  local dest="$ROOT/docs/lab/samples/ch${channel}.ts"
  echo "capturing channel $channel for ${seconds}s from $BASE"
  curl -fsS -m "$seconds" "$BASE/export/stream/$channel" -o "$dest" || true
  local bytes
  bytes=$(wc -c < "$dest" | tr -d ' ')
  if [[ "$bytes" -lt 100000 ]]; then
    echo "capture too small ($bytes bytes)" >&2
    exit 1
  fi
  echo "sample $dest ($bytes bytes)"
  # The export ends when this connection closes. Wait until staging lets go.
  local released=0
  for _ in $(seq 1 20); do
    if python3 - "$BASE" <<'PY'
import json, sys, urllib.request
with urllib.request.urlopen(sys.argv[1] + "/api/v1/tuners", timeout=8) as r:
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
    echo "tuner still held after capture" >&2
    exit 1
  fi
  echo "tuner released"
  printf '%s\n' "$dest"
}

run_lab() {
  local sample="${1:?sample.ts}"
  local dir="${2:-$CANDIDATES}"
  [[ -f "$sample" ]] || { echo "no sample $sample" >&2; exit 1; }
  mkdir -p "$OUT/stills"
  local remote="/tmp/bw-lab-sample.ts"
  echo "uploading sample"
  gzip -c "$sample" | "${SSH[@]}" "$UNRAID_HOST" "gunzip -c > '$remote'"
  local rows=()
  local name file
  for file in "$dir"/*.args; do
    [[ -f "$file" ]] || continue
    name="$(basename "$file" .args)"
    rows+=("$(run_one "$name" "$file" "$remote")")
  done
  "${SSH[@]}" "$UNRAID_HOST" "rm -f '$remote'; docker ps -aq --filter name=bw-lab | xargs -r docker rm -f >/dev/null"
  {
    echo "| candidate | fps | frames | size | decode errors | speed | cpu | vmaf |"
    echo "| --- | ---: | ---: | --- | ---: | ---: | ---: | ---: |"
    printf '%s\n' "${rows[@]}"
  } | tee "$OUT/table.md"
  echo "table $OUT/table.md"
}

run_one() {
  local name="$1" file="$2" remote="$3"
  local args_b64
  args_b64=$(grep -v -E '^(#|[[:space:]]*$)' "$file" | tr '\n' ' ' | base64 | tr -d '\n')
  local cname="bw-lab-$name"
  echo "running $name" >&2
  # Encode at real time. docker stats samples CPU while ffmpeg runs.
  # -progress writes into the mounted dir; a detached container has no pipe.
  "${SSH[@]}" "$UNRAID_HOST" "bash -s" <<EOF
set -euo pipefail
cname="$cname"
image="$IMAGE"
remote="$remote"
args=\$(printf '%s' '$args_b64' | base64 -d)
mkdir -p /tmp/\$cname
docker rm -f "\$cname" >/dev/null 2>&1 || true
docker run -d --name "\$cname" --cpus 4 --device /dev/dri \
  -v "\$remote":/in.ts:ro -v /tmp/\$cname:/out \
  --entrypoint ffmpeg "\$image" \
  -hide_banner -nostats -progress /out/progress.txt -re -i /in.ts \
  \$args -t 8 -f mp4 -movflags +faststart /out/out.mp4 >/dev/null
cpu="n/a"
maxcpu=0
for _ in \$(seq 1 60); do
  if ! docker inspect -f '{{.State.Running}}' "\$cname" 2>/dev/null | grep -q true; then
    break
  fi
  sample=\$(docker stats --no-stream --format '{{.CPUPerc}}' "\$cname" 2>/dev/null | tr -d '%' || true)
  if [[ -n "\$sample" ]]; then
    cpu="\${sample}%"
    awk -v s="\$sample" -v m="\$maxcpu" 'BEGIN{ if (s+0 > m+0) print s; else print m }' > /tmp/\$cname/maxcpu
    maxcpu=\$(cat /tmp/\$cname/maxcpu)
  fi
  sleep 1
done
if [[ "\$maxcpu" != "0" ]]; then cpu="\${maxcpu}%"; fi
code=\$(docker wait "\$cname" || true)
speed=\$(grep -E '^speed=' /tmp/\$cname/progress.txt 2>/dev/null | tail -1 | cut -d= -f2 || true)
speed=\${speed:-n/a}
if [[ "\$code" != "0" ]]; then
  echo "ffmpeg exit \$code" >&2
  docker logs "\$cname" >&2 || true
fi
echo "\$cpu \$speed" > /tmp/\$cname/meta
EOF

  local meta cpu speed
  meta=$("${SSH[@]}" "$UNRAID_HOST" "cat /tmp/$cname/meta 2>/dev/null || echo 'n/a n/a'")
  cpu=${meta%% *}
  speed=${meta#* }

  local local_mp4="$OUT/$name.mp4"
  "${SSH[@]}" "$UNRAID_HOST" "cat /tmp/$cname/out.mp4" > "$local_mp4"
  ffmpeg -y -v error -ss 2 -i "$local_mp4" -frames:v 1 "$OUT/stills/$name.jpg" >/dev/null 2>&1 || true

  local measured
  measured=$(python3 - "$local_mp4" "$sample" "$IMAGE" <<'PY'
import subprocess, sys
clip, sample = sys.argv[1], sys.argv[2]
def run(args):
    return subprocess.run(args, text=True, capture_output=True)
probe = run(["ffprobe", "-v", "error", "-select_streams", "v:0",
             "-show_entries", "stream=width,height", "-of", "csv=p=0", clip])
wh = probe.stdout.strip().replace("\n", "")
pts = run(["ffprobe", "-v", "error", "-select_streams", "v:0",
           "-show_entries", "frame=pts_time", "-of", "csv=p=0", clip]).stdout.split()
times = []
for raw in pts:
    try:
        times.append(float(raw))
    except ValueError:
        pass
frames = len(times)
fps = 0.0
if frames >= 2 and times[-1] > times[0]:
    fps = (frames - 1) / (times[-1] - times[0])
err = run(["ffmpeg", "-v", "error", "-i", clip, "-f", "null", "-"])
errors = len([ln for ln in err.stderr.splitlines() if ln.strip()])
vmaf = "n/a"
vf = run(["ffmpeg", "-hide_banner", "-filters"])
if "libvmaf" in vf.stdout:
    scored = run(["ffmpeg", "-v", "error", "-i", clip, "-i", sample,
                  "-lavfi", "[0:v][1:v]libvmaf=log_fmt=json:log_path=/dev/stdout",
                  "-f", "null", "-"])
    text = scored.stdout + scored.stderr
    key = '"vmaf":'
    if key in text:
        vmaf = text.split(key, 1)[1].split(",", 1)[0].strip()
print(f"{fps:.2f}|{frames}|{wh}|{errors}|{vmaf}")
PY
)
  "${SSH[@]}" "$UNRAID_HOST" "docker rm -f '$cname' >/dev/null 2>&1 || true; rm -rf /tmp/$cname"
  IFS='|' read -r fps frames wh errors vmaf <<<"$measured"
  printf '| %s | %s | %s | %s | %s | %s | %s | %s |\n' \
    "$name" "$fps" "$frames" "$wh" "$errors" "$speed" "$cpu" "$vmaf"
}

cmd="${1:-both}"
case "$cmd" in
  -h|--help|help) usage ;;
  capture)
    capture "${2:-1}" "${3:-12}"
    ;;
  run)
    run_lab "${2:?sample.ts}" "${3:-$CANDIDATES}"
    ;;
  both)
    sample=$(capture "${2:-1}" "${3:-12}")
    sample=$(printf '%s\n' "$sample" | tail -1)
    run_lab "$sample" "$CANDIDATES"
    ;;
  *)
    if [[ "$cmd" =~ ^[0-9]+$ ]]; then
      sample=$(capture "$cmd" "${2:-12}")
      sample=$(printf '%s\n' "$sample" | tail -1)
      run_lab "$sample" "$CANDIDATES"
    else
      usage
    fi
    ;;
esac
