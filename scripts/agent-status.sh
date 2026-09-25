#!/bin/zsh
# Status pane for the agent run: where it is in the plan, recent commits, CI,
# App Review, and whether the household server has a tuner in use. Refreshes every 30 s.
REPO=${0:A:h:h}
cd "$REPO" || exit 1
while true; do
  clear
  print -P "%B── Broadwave agent · $(date '+%a %H:%M')%b"
  echo
  sed -n '/^## Resume here/,/^## /p' docs/plan/PROGRESS.md | sed '$d' | cut -c1-150
  echo "Open tasks: $(grep -E '^[[:space:]]*- \[ \]' docs/plan/PROGRESS.md | grep -vic blocked)"
  echo
  print -P "%BCommits%b"
  git log --oneline -8 --format='%h %cr  %s' | cut -c1-140
  echo
  print -P "%BCI%b"
  gh run list -L 3 --json status,conclusion,workflowName,displayTitle \
    -q '.[] | "\(.status) \(.conclusion // "") \(.workflowName): \(.displayTitle[0:80])"' 2>/dev/null
  echo
  print -P "%BApp Review%b"
  ~/.blitz/bin/asc versions list --app 6815795649 2>/dev/null | python3 -c "
import json,sys
try:
    for v in json.load(sys.stdin)['data']:
        a=v['attributes']; print(' ', a.get('platform'), a.get('versionString'), a.get('appStoreState') or a.get('appVersionState'))
except Exception: print('  (unavailable)')"
  echo
  print -P "%BHousehold server (TUS)%b"
  curl -s -m 4 http://192.168.1.2:8477/api/v1/tuners 2>/dev/null | python3 -c "
import json,sys
try:
    t=json.load(sys.stdin)['tuners']; print('  tuners in use:', sum(1 for x in t if x.get('ours')), 'of', len(t))
except Exception: print('  (unreachable)')"
  echo
  print -P "%BLoop%b"
  tail -4 ~/Library/Logs/broadwave-agent/loop.log 2>/dev/null | cut -c1-150
  guard=~/Library/Logs/broadwave-agent/guard.log
  [[ -s $guard ]] && echo "  TUS guard blocked $(wc -l < $guard | tr -d ' '): $(tail -1 $guard | python3 -c 'import json,sys; print(json.load(sys.stdin)["why"])' 2>/dev/null)"
  sleep 30
done
