# Broadwave agent prompt (read every round)

`scripts/agent-loop.sh` passes this file to the Cursor agent at the start of every round. Edit it to steer the next round; changes take effect when the current round ends.

---

You are continuing **Broadwave** in `/Users/tyler/Projects/active/broadwave` (git repo, branch `main`). Broadwave is live TV and DVR for antenna users. It has a Go server (Docker/Unraid), a web app, and native iPhone, iPad, and Apple TV apps. Strangers install it, so the bar is category-best.

## 1. Where you are

1. Read `AGENTS.md`, then `docs/plan/PROGRESS.md` from the top ("Resume here", then the Review 2 and Review 3 blocks), then `docs/plan/MASTER_PLAN.md` sections 0.1, 0.1a, 0.1b, and 0.1c. Read the section 3 text for your current task, plus section 4 (order).
2. Run `git status` and `git log --oneline -5`. If the tree has uncommitted work, it belongs to the task in "Resume here": finish it, verify it, and commit it first. Do not throw it away.
3. The next task is the first unticked line in section 4 order that is not blocked. Update "Resume here" before you start it.

## 2. How to work (these rules exist because the last run broke them)

- **Never send a message without a tool call.** A text-only message ends your turn, and the last run stopped 9 times after writing "I'll check X next." Put status in `PROGRESS.md` and in commit messages, not in chat. If you want to say what you'll do next, do it instead.
- **Delegate.** The last run made about 3,300 tool calls and used subagents twice. It ran one 23-hour conversation until its context was huge and the connection dropped. For every task:
  - `explore` subagents map the code you're about to change (in parallel when there are several areas).
  - `docs-researcher` handles anything version-sensitive (ffmpeg, AVKit, hls.js, jellyfin-ffmpeg, HDHomeRun, Unraid).
  - A `generalPurpose` subagent does the browser verification with `playwright-cli open --browser=chrome` against staging at 390×844, 1440×900, and 1920×1080. It returns screenshots, `getVideoPlaybackQuality()` numbers, and console errors.
  - A `generalPurpose` subagent does Apple verification on the "WG Staging iPhone" and "WG Staging TV" simulators. It returns screenshot paths and defects.
  - A `code-reviewer` runs before committing any task over about 200 changed lines.
  - `best-of-n-runner` (its own git worktree) takes independent tasks in parallel, for example server work and Apple work that share no files. Only you commit to `main`. Merge a worktree's result, run `make check`, and then commit.
  - Give each subagent a self-contained prompt: paths, commands, acceptance criteria, and exactly what to return.
- **Run things in parallel and in the background.** Batch independent reads and greps in one step. Run `make check`, CI watches, Docker pulls, and long lab runs as background shells, and keep working while they run.
- **Verify like a person on staging** (MASTER_PLAN 0.1b): deploy the branch binary to `Broadwave-Staging` (:8490, iGPU), click through it in Chrome and the simulators, measure, and read the console. Production `Broadwave` (:8477) changes only in a phase deploy or a hotfix task. Check both servers' `/api/v1/tuners` and production's `/api/v1/schedule` before tuning. Never tune within 20 minutes of a scheduled recording. Never touch other containers on TUS (`channelsdvr_intel`, Plex, and the rest).
- **One task, one commit, with evidence.** `make check` must be green. The commit includes the `PROGRESS.md` tick, and the tick lists the proof for every Accept bullet: numbers, test names, screenshot paths. Anything not met becomes a new task line. Push, then watch CI (`gh run watch`); red CI stops everything until it's fixed. CI also builds the Docker image, which `make check` does not; if you touch `deploy/` or add files the web build imports, run `docker build -f deploy/docker/Dockerfile --target web .` locally.
- **Round length.** Inside `scripts/agent-loop.sh`, end your turn only after a task is committed, pushed, and green, with "Resume here" updated. The loop restarts you with a fresh context. End the round after about three tasks, or sooner if your context feels heavy, so each round stays fast and the connection stays healthy. Never end mid-task.
- **When stuck.** Try two approaches. If it truly needs the owner's hands, money, or an account, write it in `BLOCKERS.md` with what was tried and what unblocks it, mark the line `Blocked`, and move to the next task.

## 3. Current priorities (from the owner, 2026-09-24)

1. **Playback beats everything** (Phase PB): true 60-frame motion from field-rate deinterlacing on the Unraid iGPU; 720p stays at 60; GPU decode; audio passthrough and a track picker; measured on staging with the picture lab.
2. **The house sets itself up** (Phase HOME): one scan finds tuners, servers, and screens, and setup finishes itself in under 90 s.
3. **Multiview done right on web, iPhone, iPad, and Apple TV** (Phase MV).
4. **Every device, not just the owner's one CONNECT DUO** (Phase HW): fake fleets, pools, server and client matrices.

Then continue through section 4 to the end. The run is finished only when every line in `PROGRESS.md` is ticked with evidence or blocked with a reason.
