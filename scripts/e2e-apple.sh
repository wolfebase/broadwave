#!/usr/bin/env bash
# End to end on the iPhone and Apple TV simulators against a fresh local server
# with the fake tuner. For each platform: a new server that needs
# setup, the app finds it over Bonjour, finishes setup, shows the guide, plays
# a channel in sync while headless Chrome plays it too, records, and plays two
# channels side by side. Prints one line per step and the web/Apple sync numbers.
#
#   SIM_IOS, SIM_TV   simulator names or UDIDs. Unset: "Broadwave E2E iPhone"
#                     (iPhone 17) and "Broadwave E2E TV" (Apple TV 4K (3rd
#                     generation)) on the newest runtimes, created if missing.
#   PLATFORMS         "ios tvos" (default), or one of them.
#   E2E_PORT          server port (default 18641).
#   E2E_OUT           evidence directory (default .evidence/apple-e2e). Emptied
#                     first, so it must sit under .evidence, the temp folder,
#                     or the CI runner's temp folder.
#   SKIP_PREPARE=1    reuse the site and binaries in web/e2e/.run.
#   E2E_TYPED=1       skip Bonjour and type the server address.
#   SYNC_SECONDS      how long Chrome samples once both play (default 60).
#   DERIVED_DATA      keep builds here between runs (default: a /tmp dir removed at the end).
#
# The app is uninstalled from each simulator first, so its data there is lost.
# A defect the test notes, or a step 1 that had to type the address, fails the run.
set -uo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
port=${E2E_PORT:-18641}
out=${E2E_OUT:-$root/.evidence/apple-e2e}
platforms=${PLATFORMS:-ios tvos}
run="$root/web/e2e/.run"
bundle=com.wolfeup.broadwave
dd=${DERIVED_DATA:-$(mktemp -d /tmp/broadwave-e2e-dd.XXXXXX)}
serve_pid=""
sample_pid=""
booted=()
results=()
failed=0

say() { printf '[e2e-apple] %s\n' "$*"; }

cleanup() {
  [ -n "$sample_pid" ] && kill "$sample_pid" 2>/dev/null
  [ -n "$serve_pid" ] && kill "$serve_pid" 2>/dev/null
  for udid in ${booted[@]+"${booted[@]}"}; do xcrun simctl shutdown "$udid" 2>/dev/null; done
  [ -z "${DERIVED_DATA:-}" ] && rm -rf "$dd"
}
trap cleanup EXIT

for tool in xcodebuild xcrun xcodegen node go ffmpeg sips python3 curl; do
  command -v "$tool" >/dev/null || { say "missing $tool"; exit 1; }
done

free_gb() { df -k "$HOME" | awk 'NR==2 { print int($4 / 1048576) }'; }
check_disk() {
  local gb
  gb=$(free_gb)
  if [ "$gb" -lt 6 ]; then
    say "only ${gb} GB free; stopping"
    exit 1
  fi
}

# web/e2e/.run is shared with the browser tests. Wait for another harness to end.
for _ in $(seq 30); do
  pgrep -f '^([^ ]*/)?node [^ ]*e2e/serve\.mjs' >/dev/null || break
  say "another e2e server is running; waiting"
  sleep 30
done
if pgrep -f '^([^ ]*/)?node [^ ]*e2e/serve\.mjs' >/dev/null; then
  say "another e2e server is still running"
  exit 1
fi

case "$out" in
  "$root"/.evidence/?* | "${TMPDIR:-/tmp}"/?* | /tmp/?* | "${RUNNER_TEMP:-/nonexistent}"/?*) ;;
  *) say "E2E_OUT must be under .evidence, the temp folder, or the runner's temp folder: $out"; exit 1 ;;
esac
rm -rf "$out"
mkdir -p "$out"

(cd "$root/apple" && xcodegen generate --quiet) || exit 1
if [ "${SKIP_PREPARE:-}" != 1 ]; then
  say "building the site, server, and fake tuner"
  (cd "$root/web" && node e2e/prepare.mjs >"$out/prepare.log" 2>&1) || { say "prepare failed; see $out/prepare.log"; exit 1; }
fi

# Name or UDID -> UDID. Creates the default simulators when they are missing.
sim_udid() {
  local want=$1 kind=$2 model=$3
  local udid
  udid=$(xcrun simctl list devices available -j | python3 -c '
import json, sys
want = sys.argv[1]
for runtime, devices in json.load(sys.stdin)["devices"].items():
    for d in devices:
        if d["udid"] == want or d["name"] == want:
            print(d["udid"]); sys.exit(0)
' "$want")
  if [ -z "$udid" ] && [[ $want =~ ^[0-9A-Fa-f]{8}-[0-9A-Fa-f-]{27}$ ]]; then
    say "no simulator $want" >&2
    return 1
  fi
  if [ -z "$udid" ]; then
    local runtime
    runtime=$(xcrun simctl list runtimes available -j | python3 -c '
import json, sys
kind = sys.argv[1]
rts = [r for r in json.load(sys.stdin)["runtimes"] if r.get("platform") == kind and r.get("isAvailable", True)]
rts.sort(key=lambda r: [int(p) for p in r["version"].split(".")])
print(rts[-1]["identifier"] if rts else "")
' "$kind")
    [ -n "$runtime" ] || { say "no $kind runtime" >&2; return 1; }
    udid=$(xcrun simctl create "$want" "$model" "$runtime") || return 1
    say "created $want ($model, $runtime)" >&2
  fi
  echo "$udid"
}

boot() {
  local udid=$1
  local state
  state=$(xcrun simctl list devices -j | python3 -c '
import json, sys
for devices in json.load(sys.stdin)["devices"].values():
    for d in devices:
        if d["udid"] == sys.argv[1]:
            print(d["state"])
' "$udid")
  if [ "$state" != Booted ]; then
    xcrun simctl boot "$udid" || return 1
    booted+=("$udid")
  fi
  xcrun simctl bootstatus "$udid" -b >/dev/null
}

start_server() {
  local name=$1 log=$2
  node "$root/web/e2e/serve.mjs" >"$log" 2>&1 &
  serve_pid=$!
  for _ in $(seq 240); do
    if curl -fs "http://127.0.0.1:$((port + 9))/ready" >/dev/null 2>&1; then
      grep -q "^bonjour $name" "$log" && return 0
    fi
    kill -0 "$serve_pid" 2>/dev/null || break
    sleep 0.5
  done
  say "the server did not start"
  cat "$log"
  return 1
}

stop_server() {
  [ -n "$serve_pid" ] && kill "$serve_pid" 2>/dev/null && wait "$serve_pid" 2>/dev/null
  serve_pid=""
  # The harness exits before its server does. The next platform's server
  # needs the port, and must not hear the old one answer its health check.
  for _ in $(seq 50); do
    curl -fs "http://127.0.0.1:$port/api/v1/health" >/dev/null 2>&1 || return 0
    sleep 0.2
  done
  say "the old server is still answering on :$port"
}

run_platform() {
  local platform=$1 udid scheme target destination
  if [ "$platform" = ios ]; then
    udid=$(sim_udid "${SIM_IOS:-Broadwave E2E iPhone}" iOS "iPhone 17") || return 1
    scheme=Broadwave target=BroadwaveUITests destination="platform=iOS Simulator,id=$udid"
  else
    udid=$(sim_udid "${SIM_TV:-Broadwave E2E TV}" tvOS "Apple TV 4K (3rd generation)") || return 1
    scheme=BroadwaveTV target=BroadwaveTVUITests destination="platform=tvOS Simulator,id=$udid"
  fi
  local dir="$out/$platform"
  mkdir -p "$dir"
  say "$platform: simulator $udid"

  check_disk
  say "$platform: building"
  if ! xcodebuild build-for-testing -project "$root/apple/Broadwave.xcodeproj" -scheme "$scheme" \
    -destination "$destination" -derivedDataPath "$dd" CODE_SIGNING_ALLOWED=NO >"$dir/build.log" 2>&1; then
    say "$platform: build failed; see $dir/build.log"
    grep -E "error:" "$dir/build.log" | head -20
    return 1
  fi

  boot "$udid" || { say "$platform: simulator did not boot"; return 1; }
  # First run: no saved server.
  xcrun simctl terminate "$udid" "$bundle" >/dev/null 2>&1
  xcrun simctl uninstall "$udid" "$bundle" >/dev/null 2>&1
  # `simctl spawn defaults write` stores outside the app, where uninstall leaves it.
  xcrun simctl spawn "$udid" defaults delete "$bundle" >/dev/null 2>&1

  local name="Broadwave E2E $platform $$"
  local from=0
  [ -f "$run/server.log" ] && from=$(stat -f %z "$run/server.log")
  say "$platform: starting a fresh server on :$port"
  E2E_PORT=$port E2E_BONJOUR=1 E2E_LOOP=1 E2E_NAME="$name" BROADWAVE_E2E=1 start_server "$name" "$dir/serve.out" || return 1

  rm -f "$dir/watching" "$dir/sampled"
  SAMPLE_BASE="http://127.0.0.1:$port" SAMPLE_DIR="$dir" SAMPLE_LOG="$run/server.log" SAMPLE_LOG_FROM=$from \
    SAMPLE_SECONDS="${SYNC_SECONDS:-60}" SAMPLE_OUT="$dir/web-drift.jsonl" SAMPLE_LABEL="$platform" SAMPLE_WAIT=900 \
    node "$root/web/e2e/sync-sample.mjs" >"$dir/sync.log" 2>&1 &
  sample_pid=$!

  say "$platform: running EndToEndTests"
  local status=0
  TEST_RUNNER_BROADWAVE_E2E_SERVER="http://127.0.0.1:$port" \
    TEST_RUNNER_BROADWAVE_E2E_NAME="$name" \
    TEST_RUNNER_BROADWAVE_E2E_ADDRESS="127.0.0.1:$port" \
    TEST_RUNNER_BROADWAVE_E2E_DIR="$dir" \
    TEST_RUNNER_BROADWAVE_E2E_TYPED="${E2E_TYPED:-}" \
    TEST_RUNNER_BROADWAVE_E2E_SYNC_WAIT=$((${SYNC_SECONDS:-60} * 3 + 120)) \
    xcodebuild test-without-building -project "$root/apple/Broadwave.xcodeproj" -scheme "$scheme" \
    -destination "$destination" -derivedDataPath "$dd" -only-testing:"$target/EndToEndTests" -collect-test-diagnostics never \
    CODE_SIGNING_ALLOWED=NO >"$dir/test.log" 2>&1 || status=$?

  local sync=1
  if [ -e "$dir/sampled" ] || ! kill -0 "$sample_pid" 2>/dev/null; then
    # A sampler that is done exits within seconds. One that does not is stuck.
    for _ in $(seq 60); do kill -0 "$sample_pid" 2>/dev/null || break; sleep 0.5; done
    kill "$sample_pid" 2>/dev/null
    wait "$sample_pid"
    sync=$?
  else
    kill "$sample_pid" 2>/dev/null
  fi
  sample_pid=""
  stop_server
  tail -c +$((from + 1)) "$run/server.log" >"$dir/server.log" 2>/dev/null
  grep -h "sync report" "$dir/server.log" >"$dir/apple-reports.log"

  for png in "$dir"/*.png; do
    [ -e "$png" ] || continue
    sips -s format jpeg -Z 1200 "$png" --out "${png%.png}.jpg" >/dev/null 2>&1 && rm -f "$png"
  done

  local how
  how=$(grep -ho "broadwave-e2e note discovery [a-z]*" "$dir/test.log" | tail -1 | awk '{print $4}')
  local step
  for step in 1 2 3 4 5 6 7; do
    local line mark=PASS
    line=$(grep -h "broadwave-e2e pass $step " "$dir/test.log" | head -1 | sed "s/.*broadwave-e2e pass $step //")
    [ -n "$line" ] || mark=FAIL
    if [ "$step" = 7 ]; then
      line="$(grep -h "PASS\|FAIL" "$dir/sync.log" | tail -1 | sed -E 's/^sync-sample [a-z]*: (PASS|FAIL) //')"
      { [ $sync -eq 0 ] && grep -q "broadwave-e2e pass 7 " "$dir/test.log"; } || mark=FAIL
    fi
    if [ "$step" = 1 ] && [ "$how" = bonjour ] && ! grep -q "discovery: udp .*address already in use" "$dir/server.log"; then
      # This server also answered the app's UDP probe, which lists the same row.
      how="bonjour or the UDP probe"
    fi
    [ "$step" = 1 ] && [ -n "$how" ] && line="$line (via $how)"
    [ "$step" = 1 ] && [ "${E2E_TYPED:-}" != 1 ] && [[ $how != bonjour* ]] && mark=FAIL
    [ "$mark" = FAIL ] && failed=1
    results+=("$platform step $step $mark ${line:-see $dir/test.log}")
  done
  local defect
  while IFS= read -r defect; do
    failed=1
    results+=("$platform FAIL ${defect:0:240}")
  done < <(grep -ho "broadwave-e2e note defect.*" "$dir/test.log" | sed 's/^broadwave-e2e note //')
  if [ $status -ne 0 ]; then
    failed=1
    say "$platform: xcodebuild exited $status"
    grep -E "error: |XCTAssert|failed" "$dir/test.log" | grep -v "^Test Case.*passed" | head -8 | cut -c1-400
  fi
  grep -h "sync-sample" "$dir/sync.log" | sed 's/^/  /'
  # Shut down only a simulator this run booted, one platform at a time.
  local kept=() other
  for other in ${booted[@]+"${booted[@]}"}; do
    if [ "$other" = "$udid" ]; then
      xcrun simctl shutdown "$udid" 2>/dev/null
    else
      kept+=("$other")
    fi
  done
  booted=(${kept[@]+"${kept[@]}"})
  return 0
}

for platform in $platforms; do
  run_platform "$platform" || { failed=1; results+=("$platform FAIL before the test ran"); stop_server; }
done

echo
say "results (evidence in $out)"
for line in ${results[@]+"${results[@]}"}; do echo "  $line"; done
exit $failed
