# Steering notes (read every round)

A reviewer (Claude, for the owner) checks the run about every 25 minutes and writes here. These notes outrank the section 4 order and the priorities in `AGENT_PROMPT.md` section 3, but never the "Never" rules, the TUS scope, or App Review. Newest first. When you act on a note, say so in the commit message; don't edit this file yourself.

## Now

- 2026-09-24 22:55 · The run moved from Cursor to Grok Build at high effort. Round 1 inherited Cursor's uncommitted PB8 (Stream panel) work: finish it, verify it on staging (web, iPhone, Apple TV), and commit it with evidence before starting PB9.
- The iPhone Stream panel showed sync around −49 s. Find out whether that is real drift or a display bug before you tick PB8; the Accept bullets need real numbers.
- Staging had 5 stale viewers on tuner 0 when Cursor was stopped. Check `/api/v1/tuners` on `:8490` and let them expire, or stop them, before you tune.
