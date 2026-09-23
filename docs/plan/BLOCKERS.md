# Blockers

Only things that truly need the user's hands, money, hardware, or a third party's approval belong here. For each: what is blocked, why, what was tried, what would unblock it, and what was stubbed so work could continue.

**Not blockers (the executing agent handles these itself):**
- Apple team ID, bundle IDs, profiles, and `scripts/testflight.sh` (plan A7). The one remaining Apple step is an Apple ID login, listed below, because the API key cannot create the app record and the web session has expired.
- GitHub: creating the repo, pushing, CI, GHCR images, releases (plan A6, L2). The token authenticates as `wolfebase` (see the entry below).
- Unraid Community Apps: research the current submission process and submit (plan L3).

## Known before the run starts

- **APNs key (.p8) for broadcast Live Activities** — create it in the Apple developer portal if the agent has access; otherwise ship local (in-app) Live Activity updates and leave server push behind a setting.
- **Paid/optional data accounts** — Schedules Direct (14-day guide) and TMDB (artwork fallback) need the user's accounts or API keys. Implement settings fields and test with fixtures.
- **ATSC 3.0 hardware** — the user's tuner is a CONNECT DUO (ATSC 1.0 only). Implement ATSC 3.0 paths against sample files; hardware verification later.
- **Third-party review time** — Community Apps moderation and TestFlight external review are asynchronous; submit, record status here, and keep working.

## Encountered during the run

- **App Store Connect app record for `com.wolfeup.waveguide`.** Bundle IDs are registered, App Store profiles exist, and `scripts/testflight.sh` signs an iOS archive (team `D4MC63SS36`). Upload stops with "App record with bundle identifier com.wolfeup.waveguide not found." The API key in `~/.blitz` can read apps and create bundle IDs and profiles, and Apple returns 403 on `POST /v1/apps` (CREATE is not allowed for that key). The iris web session in `~/.blitz/asc-agent/web-session.json` last succeeded on 2026-05-04 and now returns 401. There is no `asc_web_auth` tool in this session. Unblock by signing in once: `asc web auth login --apple-id <apple id>` (password prompt and a 2FA code), then `asc web apps create --name Waveguide --bundle-id com.wolfeup.waveguide --sku waveguide --primary-locale en-US` for iOS and the tvOS platform, then `scripts/testflight.sh`. The App Group capability is on the bundle IDs; the group identifier itself is not attached (the profile's group list is empty), so the app is not signed with `group.com.wolfeup.waveguide` yet. The entitlement file is `apple/App/Waveguide.entitlements`.
- **GitHub username `twolfekc` does not exist.** `gh auth status` labels the keychain item `twolfekc`, but `gh api user` is `wolfebase` (id 78864560) and `GET /users/twolfekc` is 404. The public REST API cannot create a user or organization. The repo and images are `wolfebase/waveguide` and `ghcr.io/wolfebase/waveguide` so CI and releases can proceed. Unblock the original name by creating the GitHub user `twolfekc` (or an org) and transferring the repo.
