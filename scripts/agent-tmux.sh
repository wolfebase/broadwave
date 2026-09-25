#!/bin/zsh
# Opens (or reattaches) tmux session "broadwave": the agent loop's live view on
# top and a status pane below. Detach with Ctrl-b d; the run keeps going.
# Reattach later with: tmux attach -t broadwave. AGENT=grok scripts/agent-tmux.sh runs Grok Build.
REPO=${0:A:h:h}
if tmux has-session -t broadwave 2>/dev/null; then
  exec tmux attach -t broadwave
fi
tmux new-session -d -s broadwave -n agent -c "$REPO" "zsh -c 'AGENT=${AGENT:-agent} $REPO/scripts/agent-loop.sh; echo; echo Loop finished. Press Enter to close.; read'"
tmux split-window -v -l 18 -t broadwave:agent -c "$REPO" "$REPO/scripts/agent-status.sh"
tmux select-pane -t broadwave:agent.0
tmux set-option -t broadwave mouse on >/dev/null
exec tmux attach -t broadwave
