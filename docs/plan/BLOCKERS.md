# Blockers

Only things that truly need the user's hands, money, hardware, or a third party's approval belong here. For each: what is blocked, why, what was tried, what would unblock it, and what was stubbed so work could continue.

**Not blockers (the executing agent handles these itself):**
- Apple developer team ID, signing, bundle IDs, App Store Connect app records, TestFlight uploads (plan A7, L4). Find the team in Xcode/keychain/other projects; use the `asc-*` skills in `~/.claude/skills/` for App Store Connect.
- GitHub: creating the repo, pushing, CI, GHCR images, releases (plan A6, L2). The token authenticates as `wolfebase` (see the entry below).
- Unraid Community Apps: research the current submission process and submit (plan L3).

## Known before the run starts

- **APNs key (.p8) for broadcast Live Activities** — create it in the Apple developer portal if the agent has access; otherwise ship local (in-app) Live Activity updates and leave server push behind a setting.
- **Paid/optional data accounts** — Schedules Direct (14-day guide) and TMDB (artwork fallback) need the user's accounts or API keys. Implement settings fields and test with fixtures.
- **ATSC 3.0 hardware** — the user's tuner is a CONNECT DUO (ATSC 1.0 only). Implement ATSC 3.0 paths against sample files; hardware verification later.
- **Third-party review time** — Community Apps moderation and TestFlight external review are asynchronous; submit, record status here, and keep working.

## Encountered during the run

- **GitHub username `twolfekc` does not exist.** `gh auth status` labels the keychain item `twolfekc`, but `gh api user` is `wolfebase` (id 78864560) and `GET /users/twolfekc` is 404. The public REST API cannot create a user or organization. The repo and images are `wolfebase/waveguide` and `ghcr.io/wolfebase/waveguide` so CI and releases can proceed. Unblock the original name by creating the GitHub user `twolfekc` (or an org) and transferring the repo.
