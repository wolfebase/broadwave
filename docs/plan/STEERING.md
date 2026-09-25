# Steering notes (read every round)

A reviewer (Claude, for the owner) checks the run about every 25 minutes and writes here. These notes outrank the section 4 order and the priorities in `AGENT_PROMPT.md` section 3, but never the "Never" rules, the TUS scope, or App Review. Newest first. When you act on a note, say so in the commit message; don't edit this file yourself.

## Now

- 2026-09-24 23:55 · **Owner's direction: keep going through every task in section 4 order. The App Review video comes at the very end.** Finish the 2.1 reply draft and the review-notes update you started (`docs/appstore/`), commit it, and then don't wait on Apple or the recording. The owner records on real devices when the run is done. AS2 demo mode stays early, since it is what makes the next review easy.
- 2026-09-24 23:55 · **GitHub is the product's front door; keep it user-facing.** The repo is public. From now on:
  - Evidence screenshots and lab output go to `.evidence/` (add it to `.gitignore`), not `docs/screenshots/`. Cite them by path and number in `PROGRESS.md` and commit messages. Don't add new files under `docs/screenshots/`. The `docs/screenshots/appstore/` store art stays.
  - Don't commit scratch files, logs, `.playwright-cli/`, or large binaries.
  - Keep the install path dead simple and correct: `ghcr.io/wolfebase/broadwave:latest` and version tags pushed on every release, `deploy/docker/compose.yaml`, and the Unraid template in `deploy/unraid/`. Also publish a **GitHub Release** for every tag (there are none yet: create them for existing tags) with short user-facing notes and the exact install command.
  - **Don't edit `README.md` or `.github/readme/`.** A new README is being written on branch `readme-launch`. If an install fact changes (flag, port, path, image tag), say so in the commit message so the README can follow.
- 2026-09-24 23:30 · Keep PB8's standard of evidence: measured numbers on every platform, and the cause of any odd number explained.
