#!/usr/bin/env bash
# A Playwright trace is the real first-paint measurement and is not in CI
# because Playwright is not a dependency. This check parses the built
# index.html and fails if the JS entry it points at is missing.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WEB="$ROOT/server/cmd/broadwave/assets/web"
INDEX="$WEB/index.html"

if [[ ! -f "$INDEX" ]]; then
  echo "home paint: missing $INDEX" >&2
  echo "run: cd web && npm run build" >&2
  exit 1
fi

flat=$(tr '\n' ' ' < "$INDEX")
js_tag=$(printf '%s\n' "$flat" | grep -o '<script[^>]*>' | grep 'type="module"' | head -n 1 || true)
js_ref=$(printf '%s\n' "$js_tag" | sed -n 's/.*src="\([^"]*\)".*/\1/p')
js_ref="${js_ref%%\?*}"
js_ref="${js_ref%%#*}"

if [[ -z "$js_ref" || "$js_ref" == *..* ]]; then
  echo "home paint: built index.html has no JS entry" >&2
  exit 1
fi

case "$js_ref" in
  http://*|https://*)
    echo "home paint: JS entry is not a built file: $js_ref" >&2
    exit 1
    ;;
  /*) js_path="$WEB$js_ref" ;;
  *) js_path="$WEB/$js_ref" ;;
esac

if [[ ! -f "$js_path" ]]; then
  echo "home paint: JS entry missing: $js_path" >&2
  exit 1
fi

echo "home paint: js entry ${js_ref} ($(wc -c < "$js_path" | tr -d '[:space:]') bytes)"
