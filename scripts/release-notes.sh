#!/usr/bin/env bash
# Prints the CHANGELOG section for one version (the lines under "## X.Y.Z"),
# for the GitHub release. Fails when the version has no section.
# Usage: scripts/release-notes.sh 0.15.0
set -euo pipefail
version="${1:?usage: release-notes.sh VERSION}"
file="${2:-CHANGELOG.md}"
notes="$(awk -v v="$version" '
  /^## / {
    if (found) exit
    split($0, f, " ")
    if (f[2] == v) { found = 1; next }
  }
  found { print }
' "$file")"
# Trim blank lines at both ends.
notes="$(printf '%s\n' "$notes" | sed -e '/./,$!d' | sed -e ':a' -e '/^\n*$/{$d;N;ba' -e '}')"
if [ -z "$notes" ]; then
  echo "release-notes: no CHANGELOG section for $version" >&2
  exit 1
fi
printf '%s\n\nDocker image: `ghcr.io/wolfebase/broadwave:%s`\n' "$notes" "$version"
