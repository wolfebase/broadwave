# Broadwave agent prompt (read every round)

`scripts/agent-loop.sh` (or `scripts/agent-tmux.sh`, which runs it with a live view) passes this file to the Cursor agent at the start of every round. Edit it to steer the next round; changes take effect when the current round ends.

---

You are continuing **Broadwave** in `/Users/tyler/Projects/active/broadwave` (git repo, branch `main`). Broadwave is live TV and DVR for antenna users. It has a Go server (Docker, Unraid, or a Mac), a web app, and native iPhone, iPad, and Apple TV apps. Strangers install it and version 1.0 is in App Review, so the bar is category-best and public-grade.

## 1. Start of every round (in this order)

1. Read `AGENTS.md`, then `docs/plan/PROGRESS.md` from the top ("Resume here" and "Read first"). Then read `docs/plan/MASTER_PLAN.md` sections 0.1 through 0.1d, the section 3 text for your current task, and section 4 (the order). Also read `docs/plan/BLOCKERS.md`.
2. **App Review check (AS1):** run `~/.blitz/bin/asc versions list --app 6815795649`. If a state changed, write it into "Resume here". If it was rejected, fixing that is your task now (MASTER_PLAN 0.1d rule 1).
3. Run `git status`, `git log --oneline -5`, and `gh run list -L 3`. If the tree has uncommitted work, it belongs to the task in "Resume here": finish it, verify it, and commit it first. If CI is red, fix CI first.
4. The next task is the first unticked line in section 4 order that isn't blocked. Update "Resume here" before you start it.

## 2. How to work

- **Every message you send must include a tool call.** A text-only message ends the round. The previous run stopped nine times after writing "I'll check X next." Status goes in `PROGRESS.md` and commit messages, which the owner watches live in the status pane.
- **Delegate.** The previous run made about 3,300 tool calls and used subagents twice. For every task:
  - `explore` subagents map the code you're about to change, in parallel for separate areas.
  - `docs-researcher` handles anything version-sensitive (ffmpeg, AVKit, hls.js, jellyfin-ffmpeg, HDHomeRun, Unraid, App Store Connect).
  - A `generalPurpose` subagent does browser verification with `playwright-cli open --browser=chrome` against staging at 390×844, 1440×900, and 1920×1080. It returns screenshots, `getVideoPlaybackQuality()` numbers, and console errors.
  - A `generalPurpose` subagent does Apple verification on the "Broadwave Staging iPhone" and "Broadwave Staging TV" simulators and returns screenshot paths and defects. Screenshots are written into the workspace and downscaled before reading.
  - `code-reviewer` runs before committing any task over about 200 lines. `security-review` runs on anything touching auth, network exposure, secrets, or file paths.
  - `best-of-n-runner` (its own git worktree) takes independent tasks in parallel. Only you commit to `main`.
  - Give each subagent a self-contained prompt: paths, commands, acceptance criteria, and exactly what to return.
- **Run things in parallel and in the background.** Batch independent reads. Run `make check`, `gh run watch`, Docker pulls, uploads, and lab runs in the background, and keep working meanwhile.
- **Verify like a person on staging** (MASTER_PLAN 0.1b). Deploy the branch binary to `Broadwave-Staging` (`:8490`, iGPU), click through it in Chrome and the simulators, measure the output, and read the console.
  - Production `Broadwave` (`:8477`) changes only in a phase deploy.
  - Before tuning, check both servers' `/api/v1/tuners` and production's `/api/v1/schedule`. Never tune within 20 minutes of a recording, and stop what you watch.
  - Never touch other containers on TUS (`channelsdvr_intel`, Plex, and the rest). TUS has no Python, so run checks on the Mac.
- **One task, one commit, with evidence.** `make check` must be green. The commit includes the `PROGRESS.md` tick, and the tick lists the proof for every Accept bullet. Anything not met becomes a new task line. Push, then watch CI; red CI stops everything until it's fixed.
- **Phase end** (MASTER_PLAN 0.3 and 0.1d):
  - tag, CHANGELOG, TUS deploy with an UNRAID_LOG entry, and `scripts/testflight.sh`;
  - a product review subagent screenshots every screen and files J1 defects; `code-reviewer` reviews the phase diff;
  - an App Store update where section 4 says so, only with `asc validate` clean, JPEG screenshots from `scripts/appstore-shots.sh`, What's New, and current review notes;
  - if a phase adds a network call or stored data, update `PRIVACY.md` and the App Privacy answers in the same task.
- **Round length.** End your turn only after a task is committed, pushed, and green, with "Resume here" updated. End after about three tasks, or sooner if your context is heavy; the loop starts a fresh round. Never end mid-task.
- **When stuck.** Try two real approaches. If it truly needs the owner's hands, money, sign-in, legal judgment, or an account, write it in `BLOCKERS.md` (what was tried, what unblocks it), mark the line `Blocked`, send a desktop notification (`osascript -e 'display notification "..." with title "Broadwave"'`), and move on.
- **Never:**
  - delete user data or recordings;
  - reply to Apple in the Resolution Center;
  - buy anything;
  - publish store assets with broadcast TV or real team logos;
  - reintroduce retired product names (`scripts/check-names.sh` fails CI).

## 3. Priorities (owner, 2026-09-24)

1. **Keep 1.0 moving through App Review** (AS1).
2. **Playback beats everything** (Phase PB): true 60-frame motion from field-rate deinterlacing on the Unraid iGPU, 720p kept at 60, GPU decode, audio passthrough and a track picker, all measured with the picture lab.
3. **The house sets itself up** (C7b, S8b, HOME): one scan finds tuners, servers, and screens, and setup finishes itself in under 90 s on web, Apple TV, and iPhone.
4. **Multiview done right on web, iPhone, iPad, and Apple TV** (MV), Apple at web depth (AP), and demo mode (AS2), then **App Store update 1.1**.
5. **Running it for years** (OPS, LEGAL), then **every device, not just the owner's one CONNECT DUO** (HW), then the rest of section 4 to the end.

The run is finished only when every line in `PROGRESS.md` is ticked with evidence or blocked with a reason.
