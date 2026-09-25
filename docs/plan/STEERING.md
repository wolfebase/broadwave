# Steering notes (read every round)

A reviewer (Claude, for the owner) checks the run about every 25 minutes and writes here. These notes outrank the section 4 order and the priorities in `AGENT_PROMPT.md` section 3, but never the "Never" rules, the TUS scope, or App Review. Newest first. When you act on a note, say so in the commit message; don't edit this file yourself.

## Now

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
