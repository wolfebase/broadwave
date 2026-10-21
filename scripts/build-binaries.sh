#!/usr/bin/env bash
# Builds the server for each platform a release carries, with the web app
# embedded the way the Docker image has it, and writes SHA256SUMS.
# Usage: scripts/build-binaries.sh VERSION [OUT_DIR]
#   SKIP_WEB=1 reuses a web build already in server/cmd/broadwave/assets/web.
#   TARGETS="linux/amd64 darwin/arm64" builds only those.
# The binaries do not carry ffmpeg; see docs/guide/install.md.
set -euo pipefail
version="${1:?usage: build-binaries.sh VERSION [OUT_DIR]}"
root="$(cd "$(dirname "$0")/.." && pwd)"
out="${2:-$root/dist}"
targets="${TARGETS:-linux/amd64 linux/arm64 darwin/arm64 darwin/amd64 windows/amd64}"

if [ "${SKIP_WEB:-}" != "1" ]; then
  (cd "$root/web" && npm run build)
fi
if [ ! -f "$root/server/cmd/broadwave/assets/web/index.html" ]; then
  echo "build-binaries: no web build in server/cmd/broadwave/assets/web" >&2
  exit 1
fi

rm -rf "$out"
mkdir -p "$out"
stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT

for target in $targets; do
  goos="${target%/*}"
  goarch="${target#*/}"
  name="broadwave-${version#v}-${goos}-${goarch}"
  dir="$stage/$name"
  mkdir -p "$dir"
  exe=broadwave
  [ "$goos" = windows ] && exe=broadwave.exe
  echo "build-binaries: $goos/$goarch"
  (cd "$root/server" && CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags="-s -w -X main.version=$version" -o "$dir/$exe" ./cmd/broadwave)
  cp "$root/LICENSE" "$root/NOTICE" "$dir/"
  cat > "$dir/README.txt" <<EOF
Broadwave $version for $goos/$goarch

Run it:   ./$exe -config ./broadwave-data
Then open http://<this computer>:8477

ffmpeg and ffprobe must be on the PATH; they are not included. ffmpeg 6 or
newer works. jellyfin-ffmpeg 7 is what the Docker image uses: it adds the
AC-4 decoder ATSC 3.0 sound needs, and VAAPI, QSV, and NVENC encoders.
comskip (optional) finds commercial breaks.

Install guide: https://github.com/wolfebase/broadwave/blob/main/docs/guide/install.md#without-docker
EOF
  if [ "$goos" = windows ]; then
    (cd "$stage" && zip -qr "$out/$name.zip" "$name")
  else
    tar -C "$stage" -czf "$out/$name.tar.gz" "$name"
  fi
done

cd "$out"
if command -v sha256sum >/dev/null; then
  sha256sum broadwave-* > SHA256SUMS
else
  shasum -a 256 broadwave-* > SHA256SUMS
fi
cat SHA256SUMS
