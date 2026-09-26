#!/bin/sh
# Fails when a retired product name comes back into the repo. The product is Broadwave.
pattern='wave''guide|ota[ -]?viewer|ota''kit|ota''ui'
if git grep -n -I -i -E "$pattern" -- . ':!scripts/check-names.sh'; then
  echo "Retired product names found above. Use Broadwave." >&2
  exit 1
fi
