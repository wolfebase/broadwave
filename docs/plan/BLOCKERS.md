# Blockers

Only things that need the user's hands or a decision belong here. For each: what is blocked, why, what was tried, what would unblock it, and what was stubbed so work could continue.

## Known before the run starts

- **Apple developer team** — device installs, TestFlight, App Groups (widgets/Live Activities sharing), push. Needs `DEVELOPMENT_TEAM` in `apple/project.yml`. Until then: simulator-only verification.
- **APNs key (.p8)** — broadcast Live Activity updates from the server. Until then: local Live Activities updated by the app.
- **Public GitHub repository** — GHCR publishing from CI and Unraid Community Apps submission. Until then: images built locally / on Unraid via `scripts/deploy-unraid.sh`.
- **Schedules Direct account / TMDB API key** — optional guide/artwork sources; implement with settings fields and test with fixtures.
- **ATSC 3.0 hardware** — the user's tuner is a CONNECT DUO (ATSC 1.0 only). Implement ATSC 3.0 paths against sample files; hardware verification later.
