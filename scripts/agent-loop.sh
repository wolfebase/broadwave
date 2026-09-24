#!/bin/zsh
# Runs the Cursor agent through docs/plan/PROGRESS.md without stopping: one
# fresh headless session per round, each started with docs/plan/AGENT_PROMPT.md.
# A crash, a dropped stream, or a turn that ends early costs one round, not the run.
#
#   scripts/agent-loop.sh              # run until every line is ticked or blocked
#   touch ~/.broadwave-agent-stop      # stop after the current round
#   tail -f ~/Library/Logs/broadwave-agent/loop.log
#   scripts/agent-tmux.sh              # the loop plus a status pane in tmux
#
# Environment: AGENT (default: agent), MODEL (default: the CLI's selected model),
# MAX_IDLE (rounds without a new commit before giving up, default 6).

set -u
REPO=${0:A:h:h}
AGENT=${AGENT:-agent}
MAX_IDLE=${MAX_IDLE:-6}
LOGS=$HOME/Library/Logs/broadwave-agent
STOP=$HOME/.broadwave-agent-stop
LOCK=$LOGS/lock
mkdir -p "$LOGS"
cd "$REPO" || exit 1

if ! mkdir "$LOCK" 2>/dev/null; then
  echo "Another agent loop is running (remove $LOCK if it is not)." >&2
  exit 1
fi
trap 'rmdir "$LOCK" 2>/dev/null' EXIT INT TERM
rm -f "$STOP"

note() { print -r -- "$(date '+%F %T') $*" | tee -a "$LOGS/loop.log"; }
notify() { osascript -e "display notification \"$1\" with title \"Broadwave agent\"" >/dev/null 2>&1; }

# Long HTTP/2 streams die through this Mac's VPN tunnel ("http/2 stream closed
# with error code CANCEL"); HTTP/1.1 does not. The CLI can rewrite its config, so set it every round.
force_http1() {
  python3 - <<'EOF'
import json, os
p = os.path.expanduser("~/.cursor/cli-config.json")
try:
    c = json.load(open(p))
except Exception:
    raise SystemExit
if c.get("network", {}).get("useHttp1ForAgent") is not True:
    c.setdefault("network", {})["useHttp1ForAgent"] = True
    json.dump(c, open(p, "w"), indent=2)
EOF
}

open_tasks() {
  grep -E '^[[:space:]]*- \[ \]' docs/plan/PROGRESS.md | grep -viE 'blocked' | wc -l | tr -d ' '
}

if pgrep -fl "cursor-agent/versions" | grep -v worker >/dev/null; then
  note "warning: another Cursor agent session is running; close it so two agents do not edit the repo at once"
fi

round=0
idle=0
while true; do
  if [[ -f $STOP ]]; then note "stop file found; stopping"; break; fi
  left=$(open_tasks)
  if [[ $left -eq 0 ]]; then note "every task is ticked or blocked"; notify "Plan complete"; break; fi
  round=$((round + 1))
  before=$(git rev-parse HEAD)
  force_http1
  note "round $round: $left open tasks, HEAD ${before:0:7}"
  args=(-p --force --trust --approve-mcps --workspace "$REPO" --output-format stream-json)
  [[ -n ${MODEL:-} ]] && args+=(--model "$MODEL")
  log="$LOGS/round-$(printf %04d $round).jsonl"
  # The raw stream is kept for later; the terminal shows a readable view of it.
  "$AGENT" "${args[@]}" "$(cat docs/plan/AGENT_PROMPT.md)" 2>&1 | tee "$log" | python3 "$REPO/scripts/agent-view.py"
  code=${pipestatus[1]}
  session=$(grep -m1 -o '"session_id":"[^"]*"' "$log" | cut -d'"' -f4)
  [[ -n $session ]] && note "round $round session $session (open it with: agent --resume $session)"
  after=$(git rev-parse HEAD)
  if [[ $before == "$after" ]]; then
    idle=$((idle + 1))
    note "round $round: exit $code, no new commit ($idle in a row)"
  else
    idle=0
    note "round $round: exit $code, $(git rev-list --count "$before..$after") new commit(s): $(git log -1 --format=%s)"
  fi
  if [[ $idle -ge $MAX_IDLE ]]; then
    note "no commits in $idle rounds; stopping so a person can look"
    notify "Stuck: no commits in $idle rounds"
    break
  fi
  # A round that ends fast with an error is usually the network; wait longer each time.
  [[ $idle -gt 0 ]] && sleep $((idle * 60)) || sleep 5
done
