# Blockers

Only things that truly need the user's hands, money, hardware, or a third party's approval belong here. For each: what is blocked, why, what was tried, what would unblock it, and what was stubbed so work could continue.

**Not blockers (the executing agent handles these itself):**
- Apple: bundle IDs, profiles, uploads, metadata, screenshots, and review submission all work with the API key in `~/.blitz` (`asc` CLI). Creating a new app record, registering an App Group, and first-time availability need the App Store Connect website; the owner signs in to Chrome and the agent drives it with Claude in Chrome.
- GitHub: creating the repo, pushing, CI, GHCR images, releases (plan A6, L2). The token authenticates as `wolfebase` (see the entry below).
- Unraid Community Apps: research the current submission process and submit (plan L3).

## Known before the run starts

- **APNs key (.p8) for broadcast Live Activities** — create it in the Apple developer portal if the agent has access; otherwise ship local (in-app) Live Activity updates and leave server push behind a setting.
- **Paid/optional data accounts** — Schedules Direct (14-day guide) and TMDB (artwork fallback) need the user's accounts or API keys. Implement settings fields and test with fixtures.
- **ATSC 3.0 hardware** — the user's tuner is a CONNECT DUO (ATSC 1.0 only). Implement ATSC 3.0 paths against sample files; hardware verification later.
- **Third-party review time** — Community Apps moderation and TestFlight external review are asynchronous; submit, record status here, and keep working.

## Encountered during the run

- **App Review (asynchronous).** Broadwave 1.0 (iOS and tvOS, app `6815795649`, build 2) was submitted on 2026-09-24 with a demo lineup, two demo videos, and review notes explaining the home server. Check with `asc review status --app 6815795649`. If Apple asks for a live server, the fastest answer is the Docker command in the review notes; a reachable demo server would need the owner's approval.
- **Old records to delete (owner only; deletion is permanent).** App Store Connect app `6815790773` ("WolfeUp Retired App", bundle `com.wolfeup.waveguide`), the unused bundle IDs `com.wolfeup.waveguide`, `.widgets`, and `.topshelf`, and the GHCR package `ghcr.io/wolfebase/waveguide`. None blocks anything.
- **GitHub username `twolfekc` does not exist.** `gh auth status` labels the keychain item `twolfekc`, but `gh api user` is `wolfebase` (id 78864560) and `GET /users/twolfekc` is 404. The public REST API cannot create a user or organization. The repo and images are `wolfebase/broadwave` and `ghcr.io/wolfebase/broadwave` so CI and releases can proceed. Unblock the original name by creating the GitHub user `twolfekc` (or an org) and transferring the repo.
