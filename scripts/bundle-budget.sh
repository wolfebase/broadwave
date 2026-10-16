#!/usr/bin/env bash
# Gzip budget for the JS and CSS that the built index.html loads first.
# Lazy route chunks are not part of this ceiling.
# Ceilings are 10% above the entry sizes measured when this budget was added
# (node zlib level 9, same compressor the build writes next to each asset):
# JS 87653 -> 96419, CSS 7636 -> 8400.
# CSS rebaselined 2026-09-26 for the brand layer (theme/brand.css): 8967 -> 9860.
# JS given 200 bytes of slack 2026-10-07: the entry names every lazy chunk by
# its content hash, so a change inside a lazy page moves the entry's gzip size
# by a few bytes, and the entry sat exactly at its ceiling: 96419 -> 96619.
# JS lowered 2026-10-08: the copy table and the guide's program sheet load with
# the page that needs them. Measured entry gzip 92847, plus 300 bytes of slack.
# JS raised 2026-10-11: guide cells, channel buttons, and the recordings tab
# name their state for screen readers, and Back on a checkbox leaves. The guide
# loads up front, so those names are in the entry: 93137 -> 93507, plus 100
# bytes of slack for lazy-chunk hashes.
# JS raised 2026-10-11: a slow list read no longer replaces a newer one, a guide
# refresh keeps the two weeks already loaded, a settings save drops an older
# save's response and a failed disk read, and the entry sends a device token,
# opens pairing on any 401, and loads picture settings for a watch token. The
# pairing calls and code helpers load with the pages that use them. Measured
# entry gzip 95389 with all of these, plus 300 bytes of slack.
# JS raised 2026-10-11: the shell applies the saved theme and accent before
# the app loads. Measured entry gzip 96138, plus 300 bytes of slack for
# lazy-chunk hashes.
# CSS raised 2026-10-11: the light theme, accent fills, and the poster close
# plate load with the shell. Measured entry gzip 10880, plus 200 bytes.
# CSS raised 2026-10-11: guide bars, field edges, the sports card, the switch,
# Live on the hero, and the player's dark island on a light page load with the
# shell. Measured entry gzip 11660, plus 200 bytes.
# CSS raised 2026-10-15: the phone tab bar's icon-only layout, the guide's
# two-line channel cell, the hero name that truncates, and the Away from home
# copy rows load with the shell. Measured entry gzip 11957, plus 200 bytes.
# JS raised 2026-10-11: the shell listens for an install prompt and a TV remote
# walks each row in order. The offer itself loads only when the browser asks.
# Measured entry gzip 96517, plus 300 bytes of slack.
# JS raised 2026-10-11: Right at the end of a row continues to the control
# beside it, and the picture is left out of the arrow order while its buttons
# are up. Measured entry gzip 97061, plus 300 bytes of slack.
# JS raised 2026-10-12: Home, the guide, and the shell look up their words in
# an English catalog, with the plural helper. Measured entry gzip 97465, plus
# 300 bytes of slack for lazy-chunk hashes.
# JS and CSS raised 2026-10-16 for the watching work: link tabs with a More
# menu and a 404 page, the guide's empty states and page keys, rails with
# arrows and each show once, the light hero, a missing-tuner line, data change
# events with versions and /now first paint, and TV and phone target sizes.
# Measured entry gzip JS 101314 and CSS 13205, plus 300 and 200 bytes.
# JS and CSS raised 2026-10-16 for the set-up and manage work on top of the
# watching work: the shell names the server it cannot reach (offline line and
# stale guide), Safari on iPhone gets a Home Screen hint whose check runs
# before the app loads, and accent and red fills under white text are darkened
# to 4.5:1. Measured entry gzip JS 101790 and CSS 13408, plus 300 and 200 bytes.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WEB="$ROOT/server/cmd/broadwave/assets/web"
INDEX="$WEB/index.html"

JS_GZIP_MAX=102090
CSS_GZIP_MAX=13608

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
