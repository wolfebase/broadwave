#!/usr/bin/env bash
# Gzip budget for the JS and CSS that the built index.html loads first.
# Lazy route chunks are not part of this ceiling.
# Ceilings are 10% above the entry sizes measured when this budget was added
# (node zlib level 9, same compressor the build writes next to each asset):
# JS 87653 -> 96419, CSS 7636 -> 8400.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WEB="$ROOT/server/cmd/broadwave/assets/web"
INDEX="$WEB/index.html"

JS_GZIP_MAX=96419
CSS_GZIP_MAX=8400

if [[ ! -f "$INDEX" ]]; then
  echo "bundle budget: missing $INDEX" >&2
  echo "run: cd web && npm run build" >&2
  exit 1
fi

if ! command -v node >/dev/null 2>&1; then
  echo "bundle budget: node is required to gzip-size the bundle" >&2
  exit 1
fi

flat=$(tr '\n' ' ' < "$INDEX")

tag_attr() {
  local kind="$1" attr="$2" tag="$3"
  local ref
  ref=$(printf '%s\n' "$tag" | sed -n "s/.*${attr}=\"\\([^\"]*\\)\".*/\\1/p")
  ref="${ref%%\?*}"
  ref="${ref%%#*}"
  if [[ -z "$ref" || "$ref" == *..* ]]; then
    echo "bundle budget: no built $kind in $INDEX" >&2
    exit 1
  fi
  case "$ref" in
    http://*|https://*)
      echo "bundle budget: $kind is not a built file: $ref" >&2
      exit 1
      ;;
    /*) printf '%s%s\n' "$WEB" "$ref" ;;
    *) printf '%s/%s\n' "$WEB" "$ref" ;;
  esac
}

js_tag=$(printf '%s\n' "$flat" | grep -o '<script[^>]*>' | grep 'type="module"' | head -n 1 || true)
css_tag=$(printf '%s\n' "$flat" | grep -o '<link[^>]*>' | grep 'rel="stylesheet"' | head -n 1 || true)
if [[ -z "$js_tag" || -z "$css_tag" ]]; then
  echo "bundle budget: index.html is missing its module script or stylesheet" >&2
  exit 1
fi

js_path=$(tag_attr js src "$js_tag")
css_path=$(tag_attr css href "$css_tag")

for asset in "$js_path" "$css_path"; do
  if [[ ! -f "$asset" ]]; then
    echo "bundle budget: missing $asset" >&2
    exit 1
  fi
done

gzip_bytes() {
  BW_ASSET=$1 node -e 'const fs=require("node:fs");const zlib=require("node:zlib");process.stdout.write(String(zlib.gzipSync(fs.readFileSync(process.env.BW_ASSET),{level:9}).length));'
}

js_bytes=$(gzip_bytes "$js_path")
css_bytes=$(gzip_bytes "$css_path")
echo "bundle budget: js gzip ${js_bytes} bytes (ceiling ${JS_GZIP_MAX})"
echo "bundle budget: css gzip ${css_bytes} bytes (ceiling ${CSS_GZIP_MAX})"

fail=0
if [[ "$js_bytes" -gt "$JS_GZIP_MAX" ]]; then
  echo "bundle budget: js gzip ${js_bytes} exceeds ${JS_GZIP_MAX}" >&2
  fail=1
fi
if [[ "$css_bytes" -gt "$CSS_GZIP_MAX" ]]; then
  echo "bundle budget: css gzip ${css_bytes} exceeds ${CSS_GZIP_MAX}" >&2
  fail=1
fi
exit "$fail"
