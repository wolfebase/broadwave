# Steering notes (read every round)

A reviewer (Claude, for the owner) checks the run about every 25 minutes and writes here. These notes outrank the section 4 order and the priorities in `AGENT_PROMPT.md` section 3, but never the "Never" rules, the TUS scope, or App Review. Newest first. When you act on a note, say so in the commit message; don't edit this file yourself.

## Now

- 2026-09-25 15:55 · **Sprint until 17:30 at `xhigh` effort: run 4 lanes instead of 2.** The owner wants maximum throughput for the next 90 minutes.
  - Keep **4 background lanes** busy at all times, plus Lane A. At most **one** lane may run `xcodebuild` at a time; the other three take server, web, or docs tasks.
  - Good picks now: OPS5's Pages part (only if PR #1 is merged), P4, P6, K2, K5, K6, J4, J6, N3, E1, E2, E3, E8, G8, F3, I1, I2, D7, D8, HW2, HW3's paper work.
  - Every lane result still gets its own diff check, a review, your own test run, and one commit per task.
  - The disk has 52 GB free. Follow the cleanup rule so worktrees don't pile up.
  - This round was restarted to raise effort, so uncommitted AP2 work is in the tree: finish it first.
  - After 17:30, go back to 2 lanes.

- 2026-09-25 15:10 · **Clean up lanes at the start of every round, before starting new ones.** At 15:09 there were 8 lane worktrees: three were 1–2 hours old (created 13:26, 14:04, 14:14), and two pairs were duplicates (13:26 ×2, 15:04 ×2, with near-identical diffs). For each worktree in `~/.grok/worktrees/active-broadwave/`:
  - Match it to a ledger line in "Resume here".
  - If it's merged, rejected, or a duplicate, remove it.
  - If it's finished but unmerged, merge it now (with review and your own test run) or reject it.
  - Keep at most 2 running at once.
  - Before spawning a lane, check the ledger and the existing worktrees so a task never gets two lanes.
  - Lane prompts must say "don't ask questions, you're working in a git worktree off `main`; if something is unclear, make the safe choice and note it in your summary".

- 2026-09-25 12:30 · **Keep both lanes busy, and don't sit in CI waits.** At 12:29 there were 0 lanes running while Lane A sat in a 5-minute `gh run watch`. Instead:
  - Start a CI watch in the background, and meanwhile start or collect lanes or begin the next Lane A task.
  - Only block on CI when you are about to tag or deploy.
  - Before any wait longer than a minute, check that two lanes are running; if not, start them first. Candidates: OPS2, P3, P4, P6, K2, K3, K5, J4, J6, N3, E1–E3, E8, I1, I2, AP1, AP2, F3.
  - Disk is fine now (63 GB free).

- 2026-09-25 12:10 · **Never trust a lane's reported evidence. Re-run it.** A round-22 lane wrote diffs and "Test output" as text (with fake `<tool_call>` tags) instead of running anything. Before merging any lane:
  1. Diff its worktree yourself (`git -C <worktree> diff`); if the change isn't actually in the files, the lane did nothing.
  2. Run the task's tests and `make check` yourself on `main` after applying it.
  3. Put only output you produced in `PROGRESS.md` and commit messages, never numbers a lane reported.
  4. If a lane's reply shows invented tool calls or output, discard it, delete its worktree, and redo the task in a fresh lane (or in Lane A).

- 2026-09-25 11:45 · **AS6 answered (the owner delegated the call to the reviewer).** Keep ESPN's public scoreboard as the default provider, with these conditions, then unblock AS6 and record the decision in ADR 0011:
  1. Only the user's own server fetches scores (never the apps, never Wolfe Up), at the current polite refresh rates, and it caches responses.
  2. Settings > Sports shows "Scores from ESPN's public scoreboard" and a switch to turn live scores off (on by default); the switch stops all scoreboard requests.
  3. TheSportsDB is an optional provider in the registry, used only when the user enters their own key; no key is bundled and nothing is bought.
  4. No ESPN, league, or team logos in store art, marketing, or the README; the apps show team names and colors, not fetched logos, unless the user's own server provides them.
  5. PRIVACY.md and the App Privacy answers stay accurate: this is a request from the user's server, and no data goes to Wolfe Up.

- 2026-09-25 11:10 · **The Mac's disk is 97% full (12 GB free). A full disk will fail builds and stall the run. Clean up after yourself, starting this round:**
  - Remove every lane worktree whose task is merged or dead with `git worktree remove --force` (P2b, LEGAL1, OPS3, HW1 and OPS1 are merged; the two `…01a0d8db…` multiview lanes are done). Remove each future lane's worktree as soon as its result is merged or rejected. There are 8 on disk now (1.2 GB).
  - Delete any simulator you cloned for a lane (`xcrun simctl delete <udid>`) when its lane ends. Never delete the "Broadwave Staging" or "Broadwave Shots" simulators, or any simulator you didn't create.
  - **Every round**, at the start and before any `xcodebuild`, check `df -h ~`. If less than 15 GB is free, clear regenerable caches you own: `go clean -cache`, `rm -rf apple/build`, and Broadwave's own folders under `~/Library/Developer/Xcode/DerivedData/Broadwave-*`. Never clear other projects' data. If less than 5 GB is free after that, stop starting lanes, write it in `BLOCKERS.md`, and notify the owner once.

- 2026-09-25 09:10 · **Lanes are running, but their work isn't landing. Fix this before starting any new lane.**
  1. Integrate the two finished lane results still sitting in worktrees:
     - P2b: `~/.grok/worktrees/active-broadwave/subagent-01a0d8af-b202-7711-a2e0-bafd5006f4ed` (contract tests and fixtures);
     - LEGAL1: `…/subagent-01a0d8af-b202-7711-a2e0-bb0358833876` (NOTICE, `docs/legal.md`, About, license files in the image).
     For each one: bring over only the task's files. Drop its `PROGRESS.md` edits, drop its deletion of `docs/lab/pb7b/*.log`, and drop `.build/`. Then review, run `make check`, and commit it as its own task with evidence.
  2. Remove the dead worktrees with `git worktree remove` once you have confirmed nothing in them is needed: `…bb170514cf69` and `…bb215ecf6174` (no task changes at all), and one of the two identical multiview lanes (`…568371cff638` / `…5692aafd57e5`).
  3. **Lane rules, tightened.** A lane never edits `PROGRESS.md`, never deletes files its task doesn't own, and never commits. Never start two lanes on the same task. Keep a lane ledger in "Resume here" (task → worktree path → running / ready / merged), so a new round collects finished lanes before it starts new ones. Reject a lane result that contains placeholder or stub code (`placeholder`, `// ...`, `TODO: implement`, empty handlers); send it back or do the task in Lane A.
  4. Add `.build/` to `.gitignore`.

- 2026-09-25 07:45 · **Owner's direction: go faster without lowering the bar. Run three lanes in parallel from now on.**
  - **Lane A (you, serial):** the next task in section 4 order, plus anything that needs a shared resource: staging (`:8490`), tuners, production, App Store Connect or TestFlight, or the two "Broadwave Staging" simulators. You are also the only one who commits, edits `PROGRESS.md`, or deploys.
  - **Lanes B and C (background subagents, `isolation: worktree`):** keep two running at all times. Each takes one open task that meets all of these:
    - its dependencies are ticked;
    - it needs no staging, tuner, or production (verify with unit tests, `FAKE=1` and the fake HDHomeRun, a local server on its own port, or a Vite dev server on its own port);
    - it touches a different area from Lane A and from the other lane (server package vs web feature vs Apple target), so the merges don't conflict.
    - Good lane tasks now: OPS1–OPS3, LEGAL1, P2b, P3, P4, P6, K2, K3, K5, K6, HW1, G8, G10, J4, J6, N3, E1–E3, E8, I1, I2, AP1, AP2, F3.
    - Skip OPS5's README part: the new README is in PR #1. Do the GitHub Pages part only after PR #1 is merged.
  - **Lane prompts** must be self-contained: the task's full Do and Accept text from MASTER_PLAN, file paths, the rules (no staging, tuners, or production; no keystrokes or clicks; no commits; no `PROGRESS.md` edits), the exact verification commands, and a return format: summary, diff stat, test output, and evidence paths under `.evidence/`.
  - **Apple-lane rule:** at most one lane runs `xcodebuild` at a time (the Mac has 24 GB). An Apple lane that needs a simulator makes its own with `xcrun simctl clone` ("Broadwave Lane B iPhone", and so on), never the staging ones, and deletes it when done.
  - **Integrating a lane:**
    1. Read its diff.
    2. Run a `code-reviewer` pass on it (every lane result, whatever its size).
    3. Apply it to `main` and run `make check`.
    4. If the Accept bullets need a real-hardware or staging check, do that check yourself in Lane A.
    5. Commit it as its own task with its evidence, and push.
    6. Start the next lane task immediately, so two lanes are always busy.
  - **Waits:** don't wait on CI. Push, keep working, and check the run before the next push; red CI still stops new lane starts until it's fixed.
  - **Round length:** don't end a round while a lane is running. Collect or hand off its result first, and note any in-flight lane in "Resume here". Aim for about six tasks per round instead of three.
  - **Quality stays the same:** one task per commit, evidence for every Accept bullet, `make check` green, a review before commit, and the same Never rules and TUS scope.

- 2026-09-25 07:30 · **Never send keystrokes, clicks, or menu actions to the Mac's desktop** (no `cliclick`, no `osascript` keystroke or click, no Cmd-W, no `System Events`). This Mac is the owner's workstation: synthetic input lands in whatever app is in front and can close the owner's windows. Round 14 closed a Simulator window with Cmd-W and then spent its time clicking menus to get it back. Drive the simulators only through `xcrun simctl` (boot, install, launch with `-BroadwaveWatch`/`-BroadwaveStream`, `openurl` deep links, `io screenshot`, `terminate`). Screenshots need no window. If a flow truly needs a tap, add a DEBUG launch argument or deep link for it.

- 2026-09-24 23:55 · **Owner's direction: keep going through every task in section 4 order. The App Review video comes at the very end.** Finish the 2.1 reply draft and the review-notes update you started (`docs/appstore/`), commit it, and then don't wait on Apple or the recording. The owner records on real devices when the run is done. AS2 demo mode stays early, since it is what makes the next review easy.
- 2026-09-24 23:55 · **GitHub is the product's front door; keep it user-facing.** The repo is public. From now on:
  - Evidence screenshots and lab output go to `.evidence/` (add it to `.gitignore`), not `docs/screenshots/`. Cite them by path and number in `PROGRESS.md` and commit messages. Don't add new files under `docs/screenshots/`. The `docs/screenshots/appstore/` store art stays.
  - Don't commit scratch files, logs, `.playwright-cli/`, or large binaries.
  - Keep the install path dead simple and correct: `ghcr.io/wolfebase/broadwave:latest` and version tags pushed on every release, `deploy/docker/compose.yaml`, and the Unraid template in `deploy/unraid/`. Also publish a **GitHub Release** for every tag (there are none yet: create them for existing tags) with short user-facing notes and the exact install command.
  - **Don't edit `README.md` or `.github/readme/`.** A new README is being written on branch `readme-launch`. If an install fact changes (flag, port, path, image tag), say so in the commit message so the README can follow.
- 2026-09-25 00:05 · **Defect from the store art:** in `docs/screenshots/appstore/ipad-2-guide.png` the iPad guide grid has **no channel column** (no numbers or names at the left), and the rows stop after four channels, leaving the rest of the screen empty. Reproduce it on the iPad simulator, fix it as a J1 line, and regenerate the iPad guide store shot when AS3 runs. The new README (PR #1, branch `readme-launch`) uses a cropped copy; tell the reviewer in STEERING-facing commit messages once the fix lands so the art can be refreshed.
- 2026-09-24 23:30 · Keep PB8's standard of evidence: measured numbers on every platform, and the cause of any odd number explained.
