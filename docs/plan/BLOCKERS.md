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
- **tvOS 1.0, Guideline 2.1 Information Needed (2026-09-25).** tvOS 1.0 is `REJECTED` (version `af283a9e-7bae-4efa-8047-9c9e1da5924d`, submission `7946ed56-284d-4a5c-9fd3-3d8cfa421c96`). Apple asked for information because the account history is limited, not because of a defect: a screen recording on a physical device, plus purpose, setup, external services, regions, and third-party material. iOS 1.0 is still `WAITING_FOR_REVIEW`. The reply and the shot list are in `docs/appstore/review-2.1-reply.md`. App Review Notes were updated on both versions (tvOS `5636ffb0-5306-4e85-bc42-462a4ead469a`, iOS `137b9976-2f80-42bb-a6c4-15f5fc3efa8c`, 2196 characters). Unblock: the owner pastes the reply into the Resolution Center and attaches the two recordings. Do not reply from here. Nothing was resubmitted.
- **Old records to delete (owner only; deletion is permanent).** App Store Connect app `6815790773` ("WolfeUp Retired App", bundle `com.wolfeup.waveguide`), the unused bundle IDs `com.wolfeup.waveguide`, `.widgets`, and `.topshelf`, and the GHCR package `ghcr.io/wolfebase/waveguide`. None blocks anything.
- **Sports provider default (owner decision, AS6).** The scoreboard registry is in. `Open("")` and `Open("espn")` are the ESPN board and do not add a second client. ADR `docs/decisions/0011-sports-provider.md` records the terms read on 2026-09-25: ESPN publishes no developer license for `site.api.espn.com`; TheSportsDB's free tier cannot ship in an App Store app, and the paid tier needs the operator's key, attribution, and does not clear third-party artwork; NBA, MLB, and NHL public terms do not license this app; NFL official data stays with Genius Sports under contract. Question: keep ESPN as the default, use a paid TheSportsDB key, or wait for a licensed feed? Scores stay on ESPN until you answer. Unblock: say which one. Nothing else was stubbed.
- **GitHub username `twolfekc` does not exist.** `gh auth status` labels the keychain item `twolfekc`, but `gh api user` is `wolfebase` (id 78864560) and `GET /users/twolfekc` is 404. The public REST API cannot create a user or organization. The repo and images are `wolfebase/broadwave` and `ghcr.io/wolfebase/broadwave` so CI and releases can proceed. Unblock the original name by creating the GitHub user `twolfekc` (or an org) and transferring the repo.
