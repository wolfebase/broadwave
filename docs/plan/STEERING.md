# Steering notes (read every round)

A reviewer (Claude, for the owner) checks the run about every 25 minutes and writes here. These notes outrank the section 4 order and the priorities in `AGENT_PROMPT.md` section 3, but never the "Never" rules, the TUS scope, or App Review. Newest first. When you act on a note, say so in the commit message; don't edit this file yourself.

## Now

- 2026-09-25 07:30 · **Never send keystrokes, clicks, or menu actions to the Mac's desktop** (no `cliclick`, no `osascript` keystroke or click, no Cmd-W, no `System Events`). This Mac is the owner's workstation: synthetic input lands in whatever app is in front and can close the owner's windows. Round 14 closed a Simulator window with Cmd-W and then spent its time clicking menus to get it back. Drive the simulators only through `xcrun simctl` (boot, install, launch with `-BroadwaveWatch`/`-BroadwaveStream`, `openurl` deep links, `io screenshot`, `terminate`). Screenshots need no window. If a flow truly needs a tap, add a DEBUG launch argument or deep link for it.

- 2026-09-24 23:55 · **Owner's direction: keep going through every task in section 4 order. The App Review video comes at the very end.** Finish the 2.1 reply draft and the review-notes update you started (`docs/appstore/`), commit it, and then don't wait on Apple or the recording. The owner records on real devices when the run is done. AS2 demo mode stays early, since it is what makes the next review easy.
- 2026-09-24 23:55 · **GitHub is the product's front door; keep it user-facing.** The repo is public. From now on:
  - Evidence screenshots and lab output go to `.evidence/` (add it to `.gitignore`), not `docs/screenshots/`. Cite them by path and number in `PROGRESS.md` and commit messages. Don't add new files under `docs/screenshots/`. The `docs/screenshots/appstore/` store art stays.
  - Don't commit scratch files, logs, `.playwright-cli/`, or large binaries.
  - Keep the install path dead simple and correct: `ghcr.io/wolfebase/broadwave:latest` and version tags pushed on every release, `deploy/docker/compose.yaml`, and the Unraid template in `deploy/unraid/`. Also publish a **GitHub Release** for every tag (there are none yet: create them for existing tags) with short user-facing notes and the exact install command.
  - **Don't edit `README.md` or `.github/readme/`.** A new README is being written on branch `readme-launch`. If an install fact changes (flag, port, path, image tag), say so in the commit message so the README can follow.
- 2026-09-25 00:05 · **Defect from the store art:** in `docs/screenshots/appstore/ipad-2-guide.png` the iPad guide grid has **no channel column** (no numbers or names at the left), and the rows stop after four channels, leaving the rest of the screen empty. Reproduce it on the iPad simulator, fix it as a J1 line, and regenerate the iPad guide store shot when AS3 runs. The new README (PR #1, branch `readme-launch`) uses a cropped copy; tell the reviewer in STEERING-facing commit messages once the fix lands so the art can be refreshed.
- 2026-09-24 23:30 · Keep PB8's standard of evidence: measured numbers on every platform, and the cause of any odd number explained.
