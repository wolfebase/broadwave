#!/bin/sh
# Fails when a retired product name comes back into the repo. The product is Broadwave.
# BLOCKERS.md may name old App Store records the owner still has to delete.
pattern='wave''guide|ota[ -]?viewer|ota''kit|ota''ui'
if git grep -n -I -i -E "$pattern" -- . ':!scripts/check-names.sh' ':!docs/plan/BLOCKERS.md'; then
  echo "Retired product names found above. Use Broadwave (docs/brand.md)." >&2
  exit 1
fi
