#!/usr/bin/env bash
# Installs or updates Broadwave in Docker.
#
#   curl -fsSL https://raw.githubusercontent.com/wolfebase/broadwave/main/scripts/install.sh | bash
#
# Run it again to update. Config and recordings stay in their folders.
# bash install.sh --help lists the options; --dry-run shows the plan and changes nothing.
set -euo pipefail

IMAGE_REPO="ghcr.io/wolfebase/broadwave"
UPDATER_IMAGE="nickfedor/watchtower:1"
UPDATER_NAME="broadwave-updater"
SCRIPT_URL="https://raw.githubusercontent.com/wolfebase/broadwave/main/scripts/install.sh"
DOCS_URL="https://github.com/wolfebase/broadwave/blob/main/docs/guide/install.md"
DEFAULT_PORT=8477

# Prefix for the files this script reads (/etc, /dev, /usr/share/zoneinfo). Tests point it at a fake tree.
ROOT="${BROADWAVE_INSTALL_ROOT:-}"

NAME="broadwave"
TAG="latest"
CONFIG_DIR=""
REC_DIR=""
TZ_OPT=""
PUID_OPT=""
PGID_OPT=""
TUNER=""
PORT=""
GPU="auto"
UPDATER=0
DRY_RUN=0
UNINSTALL=0
NOW=0
CHANGED=0

MODE=""          # host (Linux Docker Engine) or desktop (Docker Desktop: published ports)
HAVE_DOCKER=1
EXISTS=0
RUNNING=0
COMPOSE_DIR=""
COMPOSE_PROJECT=""
OLD_IMAGE=""
OLD_CONFIG=""
OLD_REC=""
OLD_PORT=""
OLD_DRI=0
OLD_NVIDIA_RUNTIME=0
OLD_GPUS=0
OLD_ENV=()
TZ_VALUE=""
PUID_VALUE=""
PGID_VALUE=""
RUN_ARGS=()

say() { printf '%s\n' "$*"; }
note() { printf '  %s\n' "$*"; }
warn() { printf 'Note: %s\n' "$*" >&2; }
die() {
  printf 'Error: %s\n' "$*" >&2
  exit 1
}

quoted() {
  local out="" a
  for a in "$@"; do
    out+="$(printf '%q' "$a") "
  done
  printf '%s' "${out% }"
}

# run prints a command that changes something, then runs it unless this is a dry run.
run() {
  printf '+ %s\n' "$(quoted "$@")"
  if [[ $DRY_RUN -eq 1 ]]; then
    return 0
  fi
  "$@"
}

usage() {
  cat <<EOF
Installs or updates Broadwave in Docker. Run it again to update.

Usage: bash install.sh [options]
       curl -fsSL $SCRIPT_URL | bash -s -- [options]

Options:
  --dir DIR          Config folder. Default: ~/broadwave, or the one the installed container uses.
  --recordings DIR   Recordings folder. Default: DIR/recordings.
  --tz ZONE          Time zone, such as America/Chicago. Default: this computer's.
  --puid N, --pgid N The user and group that own config and recordings. Default: you.
  --tuner ADDRESS    The tuner's address, when it is on another network or Docker can't hear it.
  --port N           Web port. Default: 8477.
  --tag TAG          Image tag. Default: latest (the newest release). A version such as 0.18.2 stays put.
  --no-gpu           Don't pass /dev/dri to the container, even when it exists.
  --updater          Also run the optional nightly updater (03:30; skips nights Broadwave is busy).
  --now              Update even while a tuner is in use or a recording is on or close.
  --name NAME        Container name. Default: broadwave.
  --dry-run          Show what would run. Changes nothing.
  --uninstall        Remove the container (and the updater). Folders and recordings stay.
  -h, --help         This text.

More: $DOCS_URL
EOF
}

need_value() {
  [[ $# -ge 2 && -n $2 && $2 != --* ]] || die "$1 needs a value. See --help."
}

parse_args() {
  local arg
  while [[ $# -gt 0 ]]; do
    arg=$1
    if [[ $arg == --*=* ]]; then
      set -- "${arg%%=*}" "${arg#*=}" "${@:2}"
      arg=$1
    fi
    case $arg in
      --dir) need_value "$@"; CONFIG_DIR=$2; CHANGED=1; shift 2 ;;
      --recordings) need_value "$@"; REC_DIR=$2; CHANGED=1; shift 2 ;;
      --tz) need_value "$@"; TZ_OPT=$2; CHANGED=1; shift 2 ;;
      --puid) need_value "$@"; PUID_OPT=$2; CHANGED=1; shift 2 ;;
      --pgid) need_value "$@"; PGID_OPT=$2; CHANGED=1; shift 2 ;;
      --tuner) need_value "$@"; TUNER=$2; CHANGED=1; shift 2 ;;
      --port) need_value "$@"; PORT=$2; CHANGED=1; shift 2 ;;
      --tag) need_value "$@"; TAG=${2#v}; CHANGED=1; shift 2 ;;
      --name) need_value "$@"; NAME=$2; shift 2 ;;
      --no-gpu) GPU="off"; CHANGED=1; shift ;;
      --updater) UPDATER=1; shift ;;
      --now) NOW=1; shift ;;
      --dry-run) DRY_RUN=1; shift ;;
      --uninstall) UNINSTALL=1; shift ;;
      -h | --help) usage; exit 0 ;;
      *) die "Unknown option $arg. See --help." ;;
    esac
  done
  if [[ -n $PORT ]] && ! [[ $PORT =~ ^[0-9]+$ && $PORT -ge 1 && $PORT -le 65535 ]]; then
    die "--port must be a number from 1 to 65535."
  fi
  if [[ -n $PUID_OPT ]] && ! [[ $PUID_OPT =~ ^[0-9]+$ ]]; then die "--puid must be a number."; fi
  if [[ -n $PGID_OPT ]] && ! [[ $PGID_OPT =~ ^[0-9]+$ ]]; then die "--pgid must be a number."; fi
  [[ $NAME =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]] || die "--name may use letters, digits, dots, dashes, and underscores."
  [[ $TAG =~ ^[A-Za-z0-9_][A-Za-z0-9_.-]*$ ]] || die "--tag is not an image tag: $TAG"
  if [[ -n $TUNER ]] && ! [[ $TUNER =~ ^[A-Za-z0-9._-]+(:[0-9]+)?$ ]]; then
    die "--tuner is an address such as 192.168.1.50, or name:port."
  fi
}

is_unraid() { [[ -f $ROOT/etc/unraid-version ]]; }

os_name() { printf '%s' "${BROADWAVE_INSTALL_UNAME:-$(uname -s)}"; }

is_utc() {
  case $1 in
    UTC | Etc/UTC | Etc/UCT | UCT | Universal | Etc/Universal | Zulu | Etc/Zulu | GMT | Etc/GMT | Etc/GMT0 | GMT0 | Greenwich | Etc/Greenwich) return 0 ;;
  esac
  return 1
}

valid_tz() {
  local tz=$1
  [[ $tz =~ ^[A-Za-z][A-Za-z0-9_+-]*(/[A-Za-z0-9_+-]+){0,2}$ ]] || return 1
  if [[ -d $ROOT/usr/share/zoneinfo ]]; then
    [[ -f $ROOT/usr/share/zoneinfo/$tz ]] || return 1
  fi
  return 0
}

# detect_tz prints this computer's time zone name, or nothing.
detect_tz() {
  local tz=""
  if [[ -n ${TZ:-} && ${TZ:-} != :* ]]; then
    tz=$TZ
  elif [[ -z $ROOT ]] && command -v timedatectl >/dev/null 2>&1 && tz=$(timedatectl show -p Timezone --value 2>/dev/null) && [[ -n $tz ]]; then
    :
  elif [[ -L $ROOT/etc/localtime ]]; then
    tz=$(readlink "$ROOT/etc/localtime")
    if [[ $tz == *zoneinfo/* ]]; then tz=${tz##*zoneinfo/}; else tz=""; fi
  elif [[ -r $ROOT/etc/timezone ]]; then
    tz=$(head -n 1 "$ROOT/etc/timezone")
  fi
  if [[ -n $tz ]] && valid_tz "$tz"; then
    printf '%s' "$tz"
  fi
}

can_prompt() {
  [[ -z ${BROADWAVE_INSTALL_NO_TTY:-} ]] && (exec </dev/tty) 2>/dev/null
}

choose_tz() {
  local tz="" detected
  if [[ -n $TZ_OPT ]]; then
    valid_tz "$TZ_OPT" || die "$TZ_OPT isn't a time zone name. Use one like America/New_York, America/Chicago, America/Denver, or America/Los_Angeles."
    TZ_VALUE=$TZ_OPT
    return
  fi
  tz=$(old_env TZ)
  if [[ -n $tz ]] && valid_tz "$tz"; then
    TZ_VALUE=$tz
    return
  fi
  detected=$(detect_tz)
  if [[ -n $detected ]] && ! is_utc "$detected"; then
    TZ_VALUE=$detected
    return
  fi
  if can_prompt; then
    if [[ -n $detected ]]; then
      say "This computer's clock is on $detected. Recordings follow the time zone you set here."
    fi
    local answer=""
    printf 'Your time zone, such as America/Chicago: ' >/dev/tty
    IFS= read -r answer </dev/tty || true
    answer=${answer// /}
    [[ -n $answer ]] || die "A time zone is needed. Run again with --tz America/Chicago (your zone)."
    valid_tz "$answer" || die "$answer isn't a time zone name. Run again with --tz America/Chicago (your zone)."
    TZ_VALUE=$answer
    return
  fi
  if [[ -n $detected ]]; then
    die "This computer's clock is on $detected, so recordings would start at the wrong hour. Run again with --tz America/Chicago (your zone), or --tz $detected if that is right."
  fi
  die "Couldn't tell your time zone. Run again with --tz America/Chicago (your zone)."
}

# user_home is the home folder of the person running this, even under sudo.
user_home() {
  local h=""
  if [[ ${EUID:-$(id -u)} -eq 0 && -n ${SUDO_USER:-} && ${SUDO_USER:-} != root ]]; then
    if command -v getent >/dev/null 2>&1; then
      h=$(getent passwd "$SUDO_USER" | cut -d: -f6)
    fi
    [[ -n $h ]] || h=$(eval "printf '%s' ~$SUDO_USER" 2>/dev/null || true)
  fi
  printf '%s' "${h:-$HOME}"
}

choose_ids() {
  PUID_VALUE=${PUID_OPT:-$(old_env PUID)}
  PGID_VALUE=${PGID_OPT:-$(old_env PGID)}
  if [[ -n $PUID_VALUE || -n $PGID_VALUE ]]; then
    [[ -n $PUID_VALUE && -n $PGID_VALUE ]] || die "Set --puid and --pgid together."
    return
  fi
  if [[ $MODE == desktop ]]; then
    return
  fi
  local uid gid
  uid=$(id -u)
  gid=$(id -g)
  if [[ $uid -eq 0 ]]; then
    if [[ -n ${SUDO_UID:-} && ${SUDO_UID:-0} -ne 0 ]]; then
      PUID_VALUE=$SUDO_UID
      PGID_VALUE=${SUDO_GID:-$SUDO_UID}
    fi
    return
  fi
  PUID_VALUE=$uid
  PGID_VALUE=$gid
}

has_dri() { [[ -d $ROOT/dev/dri ]] && compgen -G "$ROOT/dev/dri/renderD*" >/dev/null; }

selinux_enforcing() {
  [[ -z $ROOT ]] && command -v getenforce >/dev/null 2>&1 && [[ $(getenforce 2>/dev/null) == Enforcing ]]
}

port_busy() {
  local p=$1
  if command -v ss >/dev/null 2>&1; then
    ss -ltn "sport = :$p" 2>/dev/null | tail -n +2 | grep -q .
  elif command -v lsof >/dev/null 2>&1; then
    lsof -nP -iTCP:"$p" -sTCP:LISTEN >/dev/null 2>&1
  elif command -v netstat >/dev/null 2>&1; then
    netstat -ltn 2>/dev/null | awk '{print $4}' | grep -Eq "[:.]$p\$"
  else
    return 1
  fi
}

lan_address() {
  local ip="" iface
  if [[ $(os_name) == Darwin ]]; then
    iface=$(route -n get default 2>/dev/null | awk '/interface:/ {print $2}')
    [[ -n $iface ]] && ip=$(ipconfig getifaddr "$iface" 2>/dev/null || true)
  else
    if command -v ip >/dev/null 2>&1; then
      ip=$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{for (i = 1; i < NF; i++) if ($i == "src") {print $(i + 1); exit}}')
    fi
    if [[ -z $ip ]] && command -v hostname >/dev/null 2>&1; then
      ip=$(hostname -I 2>/dev/null | awk '{print $1}' || true)
    fi
  fi
  printf '%s' "$ip"
}

check_docker() {
  local out
  if ! command -v docker >/dev/null 2>&1; then
    if [[ $DRY_RUN -eq 1 ]]; then
      warn "Docker isn't installed here. This shows what would run on a computer that has it."
      HAVE_DOCKER=0
      return
    fi
    die "Docker isn't installed. Install Docker Engine (https://docs.docker.com/engine/install/) or Docker Desktop, then run this again."
  fi
  if ! out=$(docker info --format '{{.OperatingSystem}}' 2>&1); then
    case $out in
      *"permission denied"* | *"Permission denied"*)
        die "This user can't use Docker. Run it with sudo (curl -fsSL $SCRIPT_URL | sudo bash), or add yourself to the docker group and log in again."
        ;;
      *)
        die "Docker is installed but not running. Start it (sudo systemctl start docker, or open Docker Desktop), then run this again."
        ;;
    esac
  fi
  DOCKER_OS=$out
}

# check_arch stops on a system the image doesn't support. The image is x86-64 and ARM64.
check_arch() {
  if [[ $HAVE_DOCKER -eq 1 ]]; then
    ARCH=$(docker version --format '{{.Server.Arch}}' 2>/dev/null || true)
  fi
  if [[ -z ${ARCH:-} ]]; then
    case $(uname -m) in
      x86_64 | amd64) ARCH=amd64 ;;
      aarch64 | arm64) ARCH=arm64 ;;
      *) ARCH=$(uname -m) ;;
    esac
  fi
  case $ARCH in
    amd64 | arm64) ;;
    *) die "Broadwave runs on 64-bit systems (x86-64 or ARM64), and this Docker is $ARCH. On a Raspberry Pi, install the 64-bit Raspberry Pi OS." ;;
  esac
}

choose_mode() {
  case $(os_name) in
    Darwin | MINGW* | MSYS* | CYGWIN*) MODE=desktop ;;
    *) MODE=host ;;
  esac
  if [[ ${DOCKER_OS:-} == *"Docker Desktop"* ]]; then
    MODE=desktop
  fi
}

old_env() {
  local key=$1 kv
  for kv in ${OLD_ENV[@]+"${OLD_ENV[@]}"}; do
    if [[ ${kv%%=*} == "$key" ]]; then
      printf '%s' "${kv#*=}"
      return
    fi
  done
}

# read_existing fills OLD_* from a container that is already there, so a re-run keeps its settings.
read_existing() {
  [[ $HAVE_DOCKER -eq 1 ]] || return 0
  docker container inspect "$NAME" >/dev/null 2>&1 || return 0
  EXISTS=1
  local line prev=""
  [[ $(docker container inspect -f '{{.State.Running}}' "$NAME") == true ]] && RUNNING=1
  OLD_IMAGE=$(docker container inspect -f '{{.Image}}' "$NAME")
  COMPOSE_PROJECT=$(docker container inspect -f '{{index .Config.Labels "com.docker.compose.project"}}' "$NAME" 2>/dev/null || true)
  COMPOSE_DIR=$(docker container inspect -f '{{index .Config.Labels "com.docker.compose.project.working_dir"}}' "$NAME" 2>/dev/null || true)
  [[ $COMPOSE_PROJECT == "<no value>" ]] && COMPOSE_PROJECT=""
  [[ $COMPOSE_DIR == "<no value>" ]] && COMPOSE_DIR=""
  while IFS= read -r line; do
    case $line in
      /config=*) OLD_CONFIG=${line#*=} ;;
      /config/work/recordings=*) OLD_REC=${line#*=} ;;
    esac
  done < <(docker container inspect -f '{{range .Mounts}}{{.Destination}}={{if eq .Type "volume"}}{{.Name}}{{else}}{{.Source}}{{end}}{{println}}{{end}}' "$NAME")
  while IFS= read -r line; do
    case ${line%%=*} in
      TZ | PUID | PGID | UMASK | HDHR_HOST | BROADWAVE_* | NVIDIA_VISIBLE_DEVICES) OLD_ENV+=("$line") ;;
    esac
  done < <(docker container inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "$NAME")
  while IFS= read -r line; do
    [[ $line == /dev/dri* ]] && OLD_DRI=1
  done < <(docker container inspect -f '{{range .HostConfig.Devices}}{{println .PathOnHost}}{{end}}' "$NAME")
  [[ $(docker container inspect -f '{{.HostConfig.Runtime}}' "$NAME") == nvidia ]] && OLD_NVIDIA_RUNTIME=1
  [[ $(docker container inspect -f '{{len .HostConfig.DeviceRequests}}' "$NAME" 2>/dev/null || echo 0) != 0 ]] && OLD_GPUS=1
  while IFS= read -r line; do
    if [[ $prev == -addr ]]; then
      OLD_PORT=${line##*:}
    fi
    prev=$line
  done < <(docker container inspect -f '{{range .Config.Cmd}}{{println .}}{{end}}' "$NAME")
  return 0
}

# busy runs the server's own check: it exits 75 while a tuner is in use or a recording is on or within two hours.
busy() {
  [[ $RUNNING -eq 1 && $NOW -eq 0 && $HAVE_DOCKER -eq 1 ]] || return 1
  local why code=0
  why=$(docker exec "$NAME" broadwave -update-check 2>/dev/null) || code=$?
  if [[ $code -eq 75 ]]; then
    BUSY_WHY=$why
    return 0
  fi
  return 1
}

find_tuner_online() {
  command -v curl >/dev/null 2>&1 || return 0
  curl -fsS --max-time 5 https://api.hdhomerun.com/discover 2>/dev/null |
    grep -o '"LocalIP":"[^"]*"' | cut -d'"' -f4 || true
}

settle_values() {
  local home
  home=$(user_home)
  # A folder given here is made absolute. One read from the old container stays as it was:
  # it can be a Docker volume name.
  if [[ -n $CONFIG_DIR ]]; then
    case $CONFIG_DIR in /*) ;; *) CONFIG_DIR="$PWD/$CONFIG_DIR" ;; esac
  fi
  if [[ -n $REC_DIR ]]; then
    case $REC_DIR in /*) ;; *) REC_DIR="$PWD/$REC_DIR" ;; esac
  fi
  CONFIG_DIR=${CONFIG_DIR:-${OLD_CONFIG:-$home/broadwave}}
  if [[ -z $REC_DIR && -z $OLD_REC && -n $OLD_CONFIG && $CONFIG_DIR == "$OLD_CONFIG" ]]; then
    # The old container kept recordings inside its config folder. Mount that same folder.
    if [[ $OLD_CONFIG == /* ]]; then REC_DIR="$OLD_CONFIG/work/recordings"; fi
  fi
  REC_DIR=${REC_DIR:-${OLD_REC:-$CONFIG_DIR/recordings}}
  PORT=${PORT:-${OLD_PORT:-$DEFAULT_PORT}}
  TUNER=${TUNER:-$(old_env HDHR_HOST)}
  choose_tz
  choose_ids
  if [[ $MODE == desktop && -z $TUNER && $UNINSTALL -eq 0 ]]; then
    local found count
    found=$(find_tuner_online)
    count=$(printf '%s' "$found" | grep -c . || true)
    if [[ $count -eq 1 ]]; then
      TUNER=$found
      say "Found an HDHomeRun at $TUNER."
    elif [[ $DRY_RUN -eq 1 ]]; then
      TUNER="<tuner address>"
      warn "Docker Desktop can't search for tuners. A real run needs --tuner and the tuner's address."
    elif [[ $count -gt 1 ]]; then
      die "Docker Desktop can't search for tuners, and more than one answered: $(printf '%s' "$found" | tr '\n' ' ')Run again with --tuner and the one to use."
    else
      die "Docker Desktop can't search for tuners. Run again with --tuner and the tuner's address (your router's list of devices shows it)."
    fi
  fi
}

secure_port() {
  if [[ $1 -eq $DEFAULT_PORT ]]; then printf '8443'; else printf '%s' $(($1 + 1)); fi
}

# build_run_args fills RUN_ARGS with the docker run command for the settings chosen.
build_run_args() {
  local z="" kv key
  selinux_enforcing && z=":z"
  RUN_ARGS=(run -d --name "$NAME" --restart unless-stopped)
  if [[ $MODE == host ]]; then
    RUN_ARGS+=(--network host)
  else
    local sp
    sp=$(secure_port "$PORT")
    RUN_ARGS+=(-p "$PORT:$PORT" -p "$sp:$sp")
  fi
  RUN_ARGS+=(-e "TZ=$TZ_VALUE")
  if [[ -n $PUID_VALUE ]]; then
    RUN_ARGS+=(-e "PUID=$PUID_VALUE" -e "PGID=$PGID_VALUE")
  fi
  [[ -n $TUNER ]] && RUN_ARGS+=(-e "HDHR_HOST=$TUNER")
  if [[ $MODE == desktop && -z $(old_env BROADWAVE_DIRECT_ADDRS) ]]; then
    local ip
    ip=$(lan_address)
    [[ -n $ip ]] && RUN_ARGS+=(-e "BROADWAVE_DIRECT_ADDRS=$ip")
  fi
  for kv in ${OLD_ENV[@]+"${OLD_ENV[@]}"}; do
    key=${kv%%=*}
    case $key in
      TZ | PUID | PGID | HDHR_HOST) ;;
      *) RUN_ARGS+=(-e "$kv") ;;
    esac
  done
  RUN_ARGS+=(-v "$CONFIG_DIR:/config$z")
  # A config volume with no recordings mount keeps recordings inside the volume.
  if [[ $CONFIG_DIR == /* || $REC_DIR != "$CONFIG_DIR/recordings" ]]; then
    RUN_ARGS+=(-v "$REC_DIR:/config/work/recordings$z")
  fi
  if [[ $GPU != off && $MODE == host && ${ARCH:-amd64} == amd64 ]]; then
    if has_dri; then
      RUN_ARGS+=(--device /dev/dri)
    elif [[ $OLD_DRI -eq 1 ]]; then
      warn "The old container used /dev/dri, and it is gone now. Starting without it."
    fi
  fi
  [[ $OLD_NVIDIA_RUNTIME -eq 1 ]] && RUN_ARGS+=(--runtime nvidia)
  [[ $OLD_GPUS -eq 1 ]] && RUN_ARGS+=(--gpus all)
  RUN_ARGS+=(
    --label com.centurylinklabs.watchtower.enable=true
    --label "com.centurylinklabs.watchtower.lifecycle.pre-update=broadwave -update-check"
    --label com.centurylinklabs.watchtower.lifecycle.pre-update-timeout=1
    "$IMAGE_REPO:$TAG"
  )
  if [[ $PORT -ne $DEFAULT_PORT ]]; then
    RUN_ARGS+=(-config /config -addr ":$PORT")
  fi
  return 0
}

make_folders() {
  local d
  for d in "$CONFIG_DIR" "$REC_DIR"; do
    if [[ $d == /* && ! -d $d ]]; then
      run mkdir -p "$d" || die "Couldn't make $d. Pick another folder with --dir, or run this with sudo."
      if [[ $(id -u) -eq 0 && -n $PUID_VALUE ]]; then
        run chown "$PUID_VALUE:$PGID_VALUE" "$d"
      fi
    fi
  done
  if [[ $DRY_RUN -eq 0 && -d $REC_DIR ]]; then
    local free_kb
    free_kb=$(df -Pk "$REC_DIR" 2>/dev/null | awk 'NR == 2 {print $4}')
    if [[ -n $free_kb && $free_kb -lt 20971520 ]]; then
      warn "$REC_DIR has less than 20 GB free. An hour of HD is about 6 GB."
    fi
  fi
}

# wait_up returns 0 once the server answers, 1 when the container stopped, 2 when it is still starting.
wait_up() {
  [[ $DRY_RUN -eq 0 ]] || return 0
  local _
  for _ in $(seq 1 "${BROADWAVE_INSTALL_WAIT:-90}"); do
    if [[ $(docker container inspect -f '{{.State.Running}}' "$NAME" 2>/dev/null) != true ]]; then
      return 1
    fi
    if docker exec "$NAME" broadwave -healthcheck >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  return 2
}

# start_new returns 0 when the new container answers, 1 when it failed, 2 when it is still starting.
start_new() {
  run docker "${RUN_ARGS[@]}" || return 1
  local code=0
  wait_up || code=$?
  if [[ $code -ne 0 ]]; then
    docker logs --tail 30 "$NAME" >&2 2>&1 || true
  fi
  return "$code"
}

pull_image() {
  run docker pull "$IMAGE_REPO:$TAG" || die "Couldn't download $IMAGE_REPO:$TAG. Check the internet connection and the tag, then run this again. Nothing changed."
}

install_or_update() {
  local previous="$NAME-previous"
  build_run_args
  make_folders
  if [[ $EXISTS -eq 0 ]]; then
    if [[ $HAVE_DOCKER -eq 1 ]] && port_busy "$PORT"; then
      die "Port $PORT is in use by another program. Stop it, or run again with --port and a free port."
    fi
    pull_image
    local code=0
    start_new || code=$?
    case $code in
      0) ;;
      2) STILL_STARTING=1 ;;
      *) die "Broadwave didn't start. The log is above. More help: $DOCS_URL" ;;
    esac
    return
  fi
  pull_image
  if [[ $DRY_RUN -eq 0 && $CHANGED -eq 0 ]]; then
    local new_image
    new_image=$(docker image inspect -f '{{.Id}}' "$IMAGE_REPO:$TAG")
    if [[ $new_image == "$OLD_IMAGE" ]]; then
      if [[ $RUNNING -eq 0 ]]; then
        run docker start "$NAME"
      fi
      UP_TO_DATE=1
      return
    fi
  fi
  if [[ $HAVE_DOCKER -eq 1 ]] && docker container inspect "$previous" >/dev/null 2>&1; then
    run docker rm -f "$previous"
  fi
  [[ $RUNNING -eq 1 ]] && run docker stop -t 30 "$NAME"
  run docker rename "$NAME" "$previous"
  local code=0
  start_new || code=$?
  if [[ $code -eq 0 ]]; then
    run docker rm "$previous"
    return
  fi
  if [[ $code -eq 2 ]]; then
    # It may be updating a large catalog. Going back now could leave the old version on a newer catalog.
    STILL_STARTING=1
    KEPT_PREVIOUS=$previous
    return
  fi
  run docker rm -f "$NAME" || true
  run docker rename "$previous" "$NAME"
  [[ $RUNNING -eq 1 ]] && run docker start "$NAME"
  die "The new version didn't start, so the old one is back. The log is above. More help: $DOCS_URL"
}

install_updater() {
  if [[ $HAVE_DOCKER -eq 1 ]] && docker container inspect "$UPDATER_NAME" >/dev/null 2>&1; then
    run docker rm -f "$UPDATER_NAME"
  fi
  run docker run -d --name "$UPDATER_NAME" --restart unless-stopped \
    -e "TZ=$TZ_VALUE" \
    -e WATCHTOWER_LABEL_ENABLE=true \
    -e WATCHTOWER_LIFECYCLE_HOOKS=true \
    -e "WATCHTOWER_SCHEDULE=0 30 3 * * *" \
    -e WATCHTOWER_CLEANUP=true \
    -v /var/run/docker.sock:/var/run/docker.sock \
    "$UPDATER_IMAGE"
}

uninstall() {
  local updater=0
  if [[ $HAVE_DOCKER -eq 1 ]] && docker container inspect "$UPDATER_NAME" >/dev/null 2>&1; then
    updater=1
  fi
  if [[ $EXISTS -eq 0 && $updater -eq 0 ]]; then
    say "There is no container named $NAME. Nothing to remove."
    return
  fi
  if [[ $EXISTS -eq 1 ]]; then
    if busy; then
      die "Broadwave is busy: $BUSY_WHY Nothing was removed. Run this again later, or add --now."
    fi
    run docker rm -f "$NAME"
  fi
  [[ $updater -eq 1 ]] && run docker rm -f "$UPDATER_NAME"
  say ""
  if [[ $DRY_RUN -eq 1 ]]; then
    say "Dry run: nothing changed. Run without --dry-run to do it."
    return
  fi
  say "Broadwave is removed. Your folders are still there:"
  note "Config:     ${OLD_CONFIG:-unknown}"
  note "Recordings: ${OLD_REC:-${OLD_CONFIG:-unknown}/work/recordings}"
  say "Delete them yourself if you want them gone. To free the image's space: docker image rm $IMAGE_REPO:$TAG"
}

finish() {
  local host
  if [[ $MODE == desktop ]]; then
    host=localhost
  else
    host=$(lan_address)
    host=${host:-<this computer>}
  fi
  say ""
  if [[ $DRY_RUN -eq 1 ]]; then
    say "Dry run: nothing changed. Run without --dry-run to do it."
    return
  fi
  if [[ ${STILL_STARTING:-0} -eq 1 ]]; then
    say "Broadwave is still starting. Its log: docker logs -f $NAME"
    if [[ -n ${KEPT_PREVIOUS:-} ]]; then
      note "The old container is kept, stopped, as $KEPT_PREVIOUS. Once the new one works: docker rm $KEPT_PREVIOUS"
    fi
  elif [[ ${UP_TO_DATE:-0} -eq 1 ]]; then
    say "Broadwave is up to date."
  elif [[ $EXISTS -eq 1 ]]; then
    say "Broadwave is updated. Your settings, guide, and recordings are as they were."
  else
    say "Broadwave is running."
  fi
  say "Open http://$host:$PORT"
  note "Config:     $CONFIG_DIR"
  note "Recordings: $REC_DIR"
  if [[ $MODE == host && $GPU != off ]] && { ! has_dri || [[ ${ARCH:-amd64} != amd64 ]]; }; then
    note "No Intel or AMD graphics passed in, so the picture is made by the processor."
  fi
  say "Run this again to update. To remove it (folders stay): curl -fsSL $SCRIPT_URL | bash -s -- --uninstall"
}

main() {
  parse_args "$@"
  if is_unraid; then
    die "This is Unraid. Install Broadwave from its template so the Docker tab can edit and update it: $DOCS_URL#unraid"
  fi
  check_docker
  check_arch
  choose_mode
  read_existing
  if [[ -n $COMPOSE_PROJECT ]]; then
    say "Docker Compose started this Broadwave (project $COMPOSE_PROJECT). Update it there:"
    note "cd ${COMPOSE_DIR:-<the folder with compose.yaml>} && docker compose pull && docker compose up -d"
    exit 0
  fi
  if [[ $UNINSTALL -eq 1 ]]; then
    uninstall
    return
  fi
  settle_values
  if [[ $EXISTS -eq 1 ]] && busy; then
    say "Broadwave is busy: $BUSY_WHY"
    say "Nothing changed. Run this again later, or add --now to update anyway."
    exit 75
  fi
  if [[ $EXISTS -eq 1 ]]; then
    say "Updating the $NAME container. Config and recordings stay where they are."
  else
    say "Installing Broadwave ($IMAGE_REPO:$TAG)."
  fi
  install_or_update
  if [[ $UPDATER -eq 1 ]]; then
    install_updater
  fi
  finish
}

if [[ -z ${BROADWAVE_INSTALL_SOURCED:-} ]]; then
  main "$@"
fi
