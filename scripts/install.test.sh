#!/usr/bin/env bash
# Tests scripts/install.sh with a fake docker on PATH. Nothing here needs Docker or the network.
#   bash scripts/install.test.sh
# The unit tests set the sourced script's globals, and a && pass || fail line is the point.
# shellcheck disable=SC2015,SC2034,SC2329
# shellcheck source-path=SCRIPTDIR
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
script="$here/install.sh"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
fails=0
passes=0

pass() { passes=$((passes + 1)); }
fail() {
  fails=$((fails + 1))
  printf 'FAIL: %s\n' "$*" >&2
}
expect_contains() { # name haystack needle
  if [[ $2 == *"$3"* ]]; then pass; else fail "$1: wanted '$3' in:"$'\n'"$2"; fi
}
expect_missing() {
  if [[ $2 != *"$3"* ]]; then pass; else fail "$1: did not want '$3' in:"$'\n'"$2"; fi
}
expect_eq() {
  if [[ $2 == "$3" ]]; then pass; else fail "$1: got '$2', want '$3'"; fi
}

# A fake root: a zoneinfo tree, a localtime link, and optionally /dev/dri.
root="$tmp/root"
mkdir -p "$root/usr/share/zoneinfo/America" "$root/usr/share/zoneinfo/Etc" "$root/etc"
touch "$root/usr/share/zoneinfo/America/Chicago" "$root/usr/share/zoneinfo/America/Denver" "$root/usr/share/zoneinfo/UTC" "$root/usr/share/zoneinfo/Etc/UTC"
ln -s ../usr/share/zoneinfo/America/Chicago "$root/etc/localtime"

# The fake docker. FAKE_* variables pick its answers; every call is appended to $FAKE_LOG.
bin="$tmp/bin"
mkdir -p "$bin"
cat >"$bin/docker" <<'FAKE'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$FAKE_LOG"
fmt=""
args=("$@")
for ((i = 0; i < ${#args[@]}; i++)); do
  [[ ${args[i]} == -f || ${args[i]} == --format ]] && fmt=${args[i + 1]}
done
last=${args[${#args[@]} - 1]}
# Containers are files in $FAKE_LOG.state, so run, rename, and rm change what inspect sees.
state="$FAKE_LOG.state"
mkdir -p "$state"
case $1 in
  info)
    if [[ -n ${FAKE_INFO_ERROR:-} ]]; then echo "$FAKE_INFO_ERROR" >&2; exit 1; fi
    echo "${FAKE_OS:-Ubuntu 24.04 LTS}"
    ;;
  container)
    [[ -e $state/$last ]] || exit 1
    case $fmt in
      "") echo '[{}]' ;;
      *State.Running*) echo "${FAKE_RUNNING:-true}" ;;
      *"{{.Image}}"*) echo sha256:old ;;
      *compose.project.working_dir*) echo "${FAKE_COMPOSE_DIR:-}" ;;
      *compose.project*) echo "${FAKE_COMPOSE:-}" ;;
      *Mounts*) printf '%b' "${FAKE_MOUNTS-/config=$FAKE_SRV/bw\n/config/work/recordings=$FAKE_SRV/rec\n}" ;;
      *Config.Env*) printf '%b' "PATH=/usr/bin\nNVIDIA_DRIVER_CAPABILITIES=compute,video,utility\nTZ=America/Denver\nPUID=1001\nPGID=1002\nBROADWAVE_HOSTS=tv.example.com\n" ;;
      *Devices*) printf '%b' "${FAKE_DEVICES-/dev/dri\n}" ;;
      *Runtime*) echo runc ;;
      *DeviceRequests*) echo 0 ;;
      *Config.Cmd*) printf '%b' "-config\n/config\n-addr\n${FAKE_ADDR:-:8477}\n" ;;
      *) echo "fake docker: unknown format $fmt" >&2; exit 2 ;;
    esac
    ;;
  image) echo "${FAKE_NEW_IMAGE:-sha256:new}" ;;
  version) echo "${FAKE_ARCH:-amd64}" ;;
  exec)
    case "$*" in
      *-update-check*) echo "A recording is running."; exit "${FAKE_BUSY:-0}" ;;
      *-healthcheck*) exit 0 ;;
    esac
    ;;
  run)
    [[ ${FAKE_RUN_EXIT:-0} == 0 ]] || exit "$FAKE_RUN_EXIT"
    for ((i = 0; i < ${#args[@]}; i++)); do
      [[ ${args[i]} == --name ]] && touch "$state/${args[i + 1]}"
    done
    ;;
  rename) mv "$state/$2" "$state/$3" ;;
  rm) rm -f "$state/$last" ;;
  logs) echo "fake log line" ;;
  *) exit 0 ;;
esac
exit 0
FAKE
cat >"$bin/ss" <<'FAKE'
#!/usr/bin/env bash
echo "State Recv-Q Send-Q Local Address:Port Peer Address:Port"
[[ ${FAKE_PORT_BUSY:-0} == 1 ]] && echo "LISTEN 0 4096 *:8477 *:*"
exit 0
FAKE
chmod +x "$bin/docker" "$bin/ss"

# install runs the script as a stranger's Linux box would, with the fake docker first on PATH.
install() {
  : >"$tmp/log"
  rm -rf "$tmp/log.state"
  mkdir -p "$tmp/log.state"
  local a
  for a in "$@"; do
    [[ $a == FAKE_EXISTS=1 ]] && touch "$tmp/log.state/broadwave"
  done
  env -u TZ PATH="$bin:$PATH" FAKE_LOG="$tmp/log" FAKE_SRV="$tmp/srv" BROADWAVE_INSTALL_ROOT="$root" BROADWAVE_INSTALL_UNAME=Linux \
    BROADWAVE_INSTALL_NO_TTY=1 HOME="$tmp/home" "$@" bash "$script" ${ARGS[@]+"${ARGS[@]}"} 2>&1
}
calls() { cat "$tmp/log"; }
mutations() { grep -E '^(pull|run|stop|rename|rm|start) ' "$tmp/log" || true; }

# --- functions, sourced ---
(
  export BROADWAVE_INSTALL_SOURCED=1 BROADWAVE_INSTALL_ROOT="$root"
  unset TZ
  # shellcheck source=install.sh
  source "$script"
  set +e
  valid_tz America/Chicago && pass || fail "valid_tz America/Chicago"
  valid_tz America/Chicagoo && fail "valid_tz accepted a missing zone" || pass
  valid_tz '../../etc/passwd' && fail "valid_tz accepted a path" || pass
  is_utc Etc/UTC && pass || fail "is_utc Etc/UTC"
  is_utc America/Denver && fail "is_utc America/Denver" || pass
  expect_eq "detect_tz from the localtime link" "$(detect_tz)" America/Chicago
  expect_eq "detect_tz from TZ" "$(TZ=America/Denver detect_tz)" America/Denver
  expect_eq "detect_tz skips a bad TZ" "$(TZ=Nowhere/Here detect_tz)" ""
  rm "$root/etc/localtime"
  echo America/Denver >"$root/etc/timezone"
  expect_eq "detect_tz from /etc/timezone" "$(detect_tz)" America/Denver
  rm "$root/etc/timezone"
  ln -s ../usr/share/zoneinfo/America/Chicago "$root/etc/localtime"

  has_dri && fail "has_dri with no /dev/dri" || pass
  mkdir -p "$root/dev/dri" && touch "$root/dev/dri/renderD128"
  has_dri && pass || fail "has_dri with renderD128"
  rm -rf "${root:?}/dev"

  expect_eq "secure port beside 8477" "$(secure_port 8477)" 8443
  expect_eq "secure port beside 9000" "$(secure_port 9000)" 9001

  MODE=host TZ_VALUE=America/Chicago PUID_VALUE=1000 PGID_VALUE=1000 CONFIG_DIR=/srv/bw REC_DIR=/srv/rec PORT=8477 TUNER=""
  build_run_args
  args="${RUN_ARGS[*]}"
  expect_contains "host networking" "$args" "--network host"
  expect_missing "no published port on host networking" "$args" "-p 8477"
  expect_contains "time zone" "$args" "-e TZ=America/Chicago"
  expect_contains "user" "$args" "-e PUID=1000 -e PGID=1000"
  expect_contains "config mount" "$args" "-v /srv/bw:/config"
  expect_contains "recordings mount" "$args" "-v /srv/rec:/config/work/recordings"
  expect_missing "no /dev/dri when the host has none" "$args" "--device"
  expect_contains "updater label" "$args" "com.centurylinklabs.watchtower.lifecycle.pre-update=broadwave -update-check"
  expect_eq "image last when the port is the default" "${RUN_ARGS[${#RUN_ARGS[@]} - 1]}" "ghcr.io/wolfebase/broadwave:latest"

  mkdir -p "$root/dev/dri" && touch "$root/dev/dri/renderD128"
  build_run_args
  expect_contains "/dev/dri when present" "${RUN_ARGS[*]}" "--device /dev/dri"
  GPU=off build_run_args
  expect_missing "--no-gpu" "${RUN_ARGS[*]}" "--device"
  rm -rf "${root:?}/dev"

  MODE=desktop PORT=9000 TUNER=10.0.0.9 PUID_VALUE="" PGID_VALUE=""
  build_run_args
  args="${RUN_ARGS[*]}"
  expect_contains "desktop publishes the port" "$args" "-p 9000:9000 -p 9001:9001"
  expect_missing "desktop has no host networking" "$args" "--network"
  expect_contains "desktop tuner" "$args" "-e HDHR_HOST=10.0.0.9"
  expect_missing "desktop sets no PUID" "$args" "PUID"
  n=${#RUN_ARGS[@]}; expect_eq "port flag after the image" "${RUN_ARGS[n-5]} ${RUN_ARGS[n-4]} ${RUN_ARGS[n-3]} ${RUN_ARGS[n-2]} ${RUN_ARGS[n-1]}" "ghcr.io/wolfebase/broadwave:latest -config /config -addr :9000"

  # A config volume with no recordings mount keeps both inside the volume.
  MODE=host PORT=8477 CONFIG_DIR=3f9a REC_DIR=3f9a/recordings
  build_run_args
  expect_contains "volume config" "${RUN_ARGS[*]}" "-v 3f9a:/config"
  expect_missing "volume config adds no recordings mount" "${RUN_ARGS[*]}" "/config/work/recordings"

  OLD_ENV=(TZ=America/Denver PUID=5 PGID=6)
  expect_eq "old_env" "$(old_env PGID)" 6
  expect_eq "old_env missing" "$(old_env HDHR_HOST)" ""
  OLD_ENV=()

  PUID_OPT=7 PGID_OPT="" MODE=host
  out=$( (choose_ids) 2>&1)
  expect_contains "puid without pgid" "$out" "Set --puid and --pgid together"
  PUID_OPT="" MODE=host SUDO_UID=1003 SUDO_GID=1004
  id() { if [[ $1 == -u ]]; then echo 0; else echo 0; fi; }
  choose_ids
  expect_eq "sudo user owns the folders" "$PUID_VALUE:$PGID_VALUE" "1003:1004"
  unset -f id

  BROADWAVE_INSTALL_NO_TTY=1 TZ_OPT=""
  rm "$root/etc/localtime" && ln -s ../usr/share/zoneinfo/Etc/UTC "$root/etc/localtime"
  out=$( (choose_tz) 2>&1)
  expect_contains "UTC clock asks for --tz" "$out" "clock is on Etc/UTC"
  rm "$root/etc/localtime" && ln -s ../usr/share/zoneinfo/America/Chicago "$root/etc/localtime"
  TZ_OPT=UTC choose_tz
  expect_eq "--tz UTC is allowed" "$TZ_VALUE" UTC

  ( parse_args --tag v0.18.2 --dir=/x --no-gpu; expect_eq "tag drops the v" "$TAG" 0.18.2; expect_eq "--dir=" "$CONFIG_DIR" /x; expect_eq "--no-gpu" "$GPU" off )
  out=$( (parse_args --tuner 'a;b') 2>&1)
  expect_contains "tuner is checked" "$out" "--tuner is an address"
  out=$( (parse_args --dir) 2>&1)
  expect_contains "missing value" "$out" "--dir needs a value"

  PATH="$bin:$PATH" FAKE_PORT_BUSY=1
  export FAKE_PORT_BUSY
  port_busy 8477 && pass || fail "port_busy with a listener"
  FAKE_PORT_BUSY=0
  port_busy 8477 && fail "port_busy with nothing listening" || pass

  printf '%s %s\n' "$passes" "$fails" >"$tmp/unit"
)
read -r p f <"$tmp/unit"
passes=$((passes + p))
fails=$((fails + f))

# --- whole runs against the fake docker ---

ARGS=(--tz America/Chicago --dir "$tmp/bw")
out=$(install FAKE_EXISTS=0)
expect_contains "fresh: pulls" "$(mutations)" "pull ghcr.io/wolfebase/broadwave:latest"
expect_contains "fresh: host networking" "$(mutations)" "--network host"
expect_contains "fresh: recordings folder" "$(mutations)" "-v $tmp/bw/recordings:/config/work/recordings"
expect_contains "fresh: says where" "$out" "Broadwave is running."
[[ -d $tmp/bw/recordings ]] && pass || fail "fresh: made the recordings folder"

ARGS=(--tz America/Chicago)
out=$(install FAKE_EXISTS=0 FAKE_PORT_BUSY=1) && fail "fresh: busy port should fail" || pass
expect_contains "fresh: busy port" "$out" "Port 8477 is in use"
expect_eq "fresh: busy port runs nothing" "$(mutations)" ""

ARGS=()
out=$(install FAKE_EXISTS=1)
m=$(mutations)
expect_contains "update: stops" "$m" "stop -t 30 broadwave"
expect_contains "update: keeps the old one aside" "$m" "rename broadwave broadwave-previous"
expect_contains "update: same config" "$m" "-v $tmp/srv/bw:/config"
expect_contains "update: same recordings" "$m" "-v $tmp/srv/rec:/config/work/recordings"
expect_contains "update: same time zone" "$m" "-e TZ=America/Denver"
expect_contains "update: same user" "$m" "-e PUID=1001 -e PGID=1002"
expect_contains "update: keeps other settings" "$m" "-e BROADWAVE_HOSTS=tv.example.com"
expect_missing "update: drops image env" "$m" "NVIDIA_DRIVER_CAPABILITIES"
expect_contains "update: removes the old one" "$m" "rm broadwave-previous"
expect_contains "update: says so" "$out" "Broadwave is updated."
expect_contains "update: missing /dev/dri noted" "$out" "/dev/dri, and it is gone now"

out=$(install FAKE_EXISTS=1 FAKE_NEW_IMAGE=sha256:old)
expect_eq "up to date: changes nothing" "$(mutations)" "pull ghcr.io/wolfebase/broadwave:latest"
expect_contains "up to date: says so" "$out" "up to date"

code=0
out=$(install FAKE_EXISTS=1 FAKE_BUSY=75) || code=$?
expect_eq "busy: exit 75" "$code" 75
expect_contains "busy: says why" "$out" "Broadwave is busy: A recording is running."
expect_eq "busy: changes nothing" "$(mutations)" ""
ARGS=(--now)
install FAKE_EXISTS=1 FAKE_BUSY=75 >/dev/null
expect_contains "busy --now: updates" "$(mutations)" "rename broadwave broadwave-previous"
expect_missing "busy --now: no busy check" "$(calls)" "exec broadwave broadwave -update-check"

ARGS=(--dry-run)
out=$(install FAKE_EXISTS=1)
expect_eq "dry run: docker changes nothing" "$(mutations)" ""
expect_contains "dry run: shows the plan" "$out" "+ docker rename broadwave broadwave-previous"
expect_contains "dry run: says so" "$out" "Dry run: nothing changed."

ARGS=()
code=0
out=$(install FAKE_EXISTS=1 FAKE_RUN_EXIT=1) || code=$?
m=$(mutations)
expect_eq "failed start: exit 1" "$code" 1
expect_contains "failed start: removes the new one" "$m" "rm -f broadwave"
expect_contains "failed start: puts the old one back" "$m" "rename broadwave-previous broadwave"
expect_contains "failed start: starts it" "$m" "start broadwave"
expect_contains "failed start: says so" "$out" "the old one is back"

out=$(install FAKE_EXISTS=1 FAKE_COMPOSE=bw FAKE_COMPOSE_DIR=/opt/bw)
expect_contains "compose: points there" "$out" "cd /opt/bw && docker compose pull && docker compose up -d"
expect_eq "compose: changes nothing" "$(mutations)" ""

out=$(install FAKE_EXISTS=1 FAKE_MOUNTS="/config=$tmp/srv/old\n")
expect_contains "recordings inside config stay there" "$(mutations)" "-v $tmp/srv/old/work/recordings:/config/work/recordings"

out=$(install FAKE_EXISTS=1 FAKE_ADDR=:9000)
expect_contains "update keeps the port" "$(mutations)" "-addr :9000"

ARGS=(--uninstall)
out=$(install FAKE_EXISTS=1)
expect_eq "uninstall: removes the container only" "$(mutations)" "rm -f broadwave"
expect_contains "uninstall: names the folders" "$out" "Recordings: $tmp/srv/rec"
code=0
out=$(install FAKE_EXISTS=1 FAKE_BUSY=75) || code=$?
expect_eq "uninstall while busy: refused" "$(mutations)" ""
expect_contains "uninstall while busy: says why" "$out" "Nothing was removed"

ARGS=(--tz America/Chicago)
touch "$root/etc/unraid-version"
out=$(install FAKE_EXISTS=0) && fail "unraid should stop" || pass
expect_contains "unraid: points at the template" "$out" "This is Unraid"
rm "$root/etc/unraid-version"

out=$(install FAKE_INFO_ERROR="permission denied while trying to connect to the Docker daemon socket") && fail "permission should stop" || pass
expect_contains "docker permission" "$out" "can't use Docker"
out=$(install FAKE_INFO_ERROR="Cannot connect to the Docker daemon") && fail "daemon down should stop" || pass
expect_contains "docker not running" "$out" "not running"

mkdir -p "$tmp/empty"
out=$(env -u TZ PATH="$tmp/empty" BROADWAVE_INSTALL_ROOT="$root" "$BASH" "$script" --tz America/Chicago 2>&1) && fail "no docker should stop" || pass
expect_contains "no docker" "$out" "Docker isn't installed"

ARGS=(--tz America/Chicago --tuner 192.168.50.20)
out=$(install FAKE_EXISTS=0 FAKE_OS="Docker Desktop")
expect_contains "Docker Desktop on Linux publishes ports" "$(mutations)" "-p 8477:8477"
expect_contains "Docker Desktop opens localhost" "$out" "http://localhost:8477"

ARGS=(--tz America/Chicago)
out=$(install FAKE_EXISTS=0 FAKE_ARCH=arm) && fail "32-bit should stop" || pass
expect_contains "32-bit: says why" "$out" "64-bit Raspberry Pi OS"
mkdir -p "$root/dev/dri" && touch "$root/dev/dri/renderD128"
install FAKE_EXISTS=0 FAKE_ARCH=arm64 >/dev/null
expect_missing "arm64: no /dev/dri" "$(mutations)" "--device"
install FAKE_EXISTS=0 >/dev/null
expect_contains "amd64: /dev/dri" "$(mutations)" "--device /dev/dri"
rm -rf "${root:?}/dev"

printf '%d passed, %d failed\n' "$passes" "$fails"
[[ $fails -eq 0 ]]
