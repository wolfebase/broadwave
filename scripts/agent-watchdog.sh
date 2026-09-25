#!/bin/zsh
# Ends an agent round that has gone silent, so an overnight run moves on to a
# fresh round instead of hanging. The loop itself keeps going.
#
#   scripts/agent-watchdog.sh            # SILENT_MIN (default 40) minutes without output ends the round
LOGS=$HOME/Library/Logs/broadwave-agent
SILENT_MIN=${SILENT_MIN:-40}
while true; do
  sleep 300
  log=$(ls -t "$LOGS"/round-*.jsonl 2>/dev/null | head -1)
  [[ -n $log ]] || continue
  age=$(( ($(date +%s) - $(stat -f %m "$log")) / 60 ))
  (( age >= SILENT_MIN )) || continue
  pids=$(pgrep -f -- "--output-format streaming-json" ; pgrep -f -- "--output-format stream-json")
  [[ -n $pids ]] || continue
  print -r -- "$(date '+%F %T') watchdog: no output for $age min in ${log:t}; ending the round" >> "$LOGS/loop.log"
  kill ${=pids} 2>/dev/null
done
