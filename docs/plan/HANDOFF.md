# Handoff: continuing the Broadwave run with any coding agent

Written 2026-09-24 when the Cursor loop was paused to move the run to another agent (Grok Build). Everything the run needs is in this repo; nothing depends on Cursor except `scripts/agent-loop.sh`.

## Read in this order

1. `AGENTS.md` (project guide and lessons; `CLAUDE.md` links to it).
2. `docs/plan/PROGRESS.md`, from "Resume here". It says the current task and anything half done.
3. `docs/plan/AGENT_PROMPT.md`: the operating rules every round follows. It names Cursor subagent types (`explore`, `generalPurpose`, `docs-researcher`, `code-reviewer`, `security-review`, `best-of-n-runner`); use your agent's equivalents, or do the work inline if it has none.
4. `docs/plan/MASTER_PLAN.md`: sections 0.1–0.1d (how to work), section 3 (each task's Do and Accept), section 4 (the order).
5. `docs/plan/BLOCKERS.md` and `docs/plan/UNRAID_LOG.md`.
6. Project skills in `.cursor/skills/broadwave-*` (media pipeline, Apple, dev loop, multiview, sources) and the personal skill `~/.cursor/skills/broadwave-unraid`. They are plain Markdown and work for any agent.

## State when paused

- 111 open tasks. Done through PB7b (playback: staging scripts, picture lab, scan type on first tune, film cadence, GPU decode and HEVC, AC-3 passthrough, 30→60 blend dropped, player buffers and Apple TV frame-rate matching, hour-long stall count). The next task is **PB8** (Stream panel on web, iPhone, and Apple TV). Round 12 may have committed part of it; check `git log` and "Resume here".
- App Store: Broadwave 1.0 (app `6815795649`, iOS and tvOS, build 2) is WAITING_FOR_REVIEW. Check it with `~/.blitz/bin/asc versions list --app 6815795649` at the start of every session (task AS1).
- Production: TUS `Broadwave` on `:8477`, v0.6.0, healthy. Staging: `Broadwave-Staging` on `:8490`. Next recording: Jeopardy, 2026-09-25 20:00 UTC.
- CI passes on `main`.

## Clean up before starting

The paused round may have left viewers running. Run these on the Mac:

```bash
curl -s http://192.168.1.2:8490/api/v1/tuners          # staging: stop any channel it still holds
curl -s -X POST http://192.168.1.2:8490/api/v1/watch/1/stop
playwright-cli close-all 2>/dev/null
xcrun simctl terminate "Broadwave Staging iPhone" com.wolfeup.broadwave 2>/dev/null
xcrun simctl terminate "Broadwave Staging TV" com.wolfeup.broadwave 2>/dev/null
git status   # uncommitted work belongs to the "Resume here" task: finish, verify, commit it
```

## Rules that matter most (details in AGENT_PROMPT.md)

- Every message you send includes a tool call. Status goes in `PROGRESS.md` "Resume here" and in commit messages, not in chat.
- One task, one commit, with evidence for every Accept bullet. `make check` must be green (it mirrors CI: gofmt, vet, Go tests, web typecheck and lint, SwiftLint, SwiftFormat, API drift, relay smoke, retired-name guard). Push, then watch CI (`gh run watch`).
- No single wait longer than 10 minutes; start long measurements detached and collect them later.
- Verify like a person on staging (Chrome through `playwright-cli --browser=chrome`, the "Broadwave Staging" simulators); production changes only in phase deploys. Check `/api/v1/tuners` on both servers and production's `/api/v1/schedule` before tuning; never tune within 20 minutes of a recording.
- LAN and SSH commands to TUS (`192.168.1.2`) need network access outside any sandbox. TUS has no Python.
- Never delete user data, reply to Apple in the Resolution Center, buy anything, or reintroduce retired product names.

## Running it nonstop

`scripts/agent-loop.sh` runs Cursor's `agent -p` headless, one fresh session per round, with `docs/plan/AGENT_PROMPT.md` as the prompt. For another agent, either:

- run that agent interactively in `~/Projects/active/broadwave` with the prompt below and let it keep going, or
- adapt the loop: set `AGENT` to the other CLI and change the `args` line in `scripts/agent-loop.sh` to that CLI's non-interactive flags (auto-approve, workspace, and a streaming JSON output if it has one; `scripts/agent-view.py` expects Cursor's stream format, so plain text output may need its own viewer).

Remove the pause first: `rm -f ~/.broadwave-agent-stop`.
