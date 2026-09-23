# OTA Viewer — guide for agents and contributors

OTA Viewer is live TV and DVR for people with an antenna. A self-hosted server (Docker, Unraid, or a Mac) talks to HDHomeRun tuners, builds the guide, records, and relays every broadcast to native apps on iPhone and Apple TV and to a web app. It is a public product: strangers install it, so setup, polish, and reliability matter as much as features.

Read this file first, then the scoped rules in `.cursor/rules/` for the area you are touching, then `docs/architecture.md` and `docs/roadmap.md`.

## What we are building

The bar is "better than paying for YouTube TV to get ABC, CBS, FOX, and NBC", with the best experience on Apple devices of any live TV app, Channels DVR included.

Signature features. Protect them in every change:

1. **Whole-Home Sync.** Every screen on the same broadcast plays the same frame. No echo between rooms, no neighbor cheering first. Optional group mode where pause and rewind move everyone.
2. **Unlimited screens from one tuner.** Only the server talks to the tuner. One tune per RF frequency feeds every viewer, every recording, and every export (HDHomeRun emulation, M3U, XMLTV) for Plex, Jellyfin, and Channels.
3. **Game-aware DVR.** Team passes ("every Chiefs game") and recordings that keep going until the game is final.
4. **The guide is the centerpiece.** Fast, beautiful, with a live preview while browsing and a mini-guide over the player.
5. **Auto everything, override anything.** The server picks direct play, bitrate, audio passthrough, and frame rate per device. Every choice can be pinned per device or per channel.

## Product principles

- **Apple-first, native-first.** The iPhone and Apple TV apps are the flagship clients. Use system frameworks (SwiftUI, AVKit, Liquid Glass, Live Activities, App Intents) instead of reinventing them. The web app is a full player and the setup and admin console.
- **Content first.** Video and artwork fill the screen; chrome floats on glass. Information appears where it helps a decision (what's on, is it recording, is it live, how far behind live) and hides otherwise.
- **Zero-config by default.** Discovery over Bonjour and HDHomeRun broadcast, auto stream decisions, sensible DVR defaults. Settings exist for people who want them, not as a prerequisite.
- **Never waste a tuner.** Reuse a frequency that is already tuned before taking a new tuner. Release tuners promptly when nobody is watching or recording.
- **Original quality is kept.** Recordings are the untouched broadcast MPEG-TS. Transcoding is for delivery only.
- **Honest, short copy.** See `.cursor/rules/copy-voice.mdc`.

## Repo map

| Path | What lives there |
| --- | --- |
| `server/` | Go module `ota-viewer`. `cmd/ota-viewer` is the binary; `internal/` holds `hdhr` (tuner protocol), `live` (relay, ffmpeg, HLS), `dvr` (planning, passes, virtual channels), `guide` (XMLTV, Schedules Direct), `store` (SQLite), `httpapi` (HTTP API, HDHomeRun emulation), `source` (discovery, M3U, folders), `disk`. |
| `web/` | React 19 + Vite + TypeScript web app. Builds into `server/cmd/ota-viewer/assets/web`, which is embedded into the binary. |
| `apple/` | Xcode workspace for the iOS and tvOS apps and the shared Swift packages (`OTAKit`, `OTAUI`). |
| `design/` | Design tokens (`tokens.json`) shared by the web and Apple apps. |
| `api/openapi.yaml` | The HTTP contract. Source of truth for generated Swift and TypeScript clients. |
| `deploy/` | Dockerfile, compose, Unraid template. |
| `docs/` | Architecture, roadmap, decisions (ADRs), research, hardware notes. |

## Commands

Run from the repo root unless noted.

```bash
make run        # build web, run server on :8477 with ./data
make dev        # server with -dev (CORS for Vite); in another shell: cd web && npm run dev
make test       # go test ./server/... and web typecheck
make build      # bin/ota-viewer with the site embedded
make docker     # container image
make apple      # xcodegen + build the iOS and tvOS apps (Xcode 26.1+, tvOS platform installed)
make apple-test # OTAKit unit tests
make tokens     # regenerate web CSS and Swift theme from design/tokens.json
```

Apple apps: `apple/project.yml` is the source of truth (XcodeGen); the `.xcodeproj` is generated and ignored. For simulator testing, launch with `-OTAWatch <channel id>` (debug builds) to start a channel without tapping; deep links are `otaviewer://watch/<id>`, `otaviewer://guide`, `otaviewer://sports`.

Tools: Go 1.25+, Node 22+, ffmpeg (with ffprobe) on PATH, Xcode 26+ for `apple/`. `go.work` at the root makes `go` commands work from here.

Tests never open a real tuner. Live tuner checks happen by running the server on the LAN.

## Definition of done

- `make test` passes. New server behavior has a Go test; bugs get a regression test.
- API changes update `api/openapi.yaml` in the same change, and generated clients are regenerated.
- Schema changes are new numbered migrations, never edits to old ones.
- UI changes work at phone, desktop, and 10-foot (TV) sizes, with keyboard or remote focus, VoiceOver or screen reader labels, and Reduce Motion respected.
- User-facing strings follow the copy voice.
- If you change architecture, update `docs/architecture.md` or add an ADR in `docs/decisions/`.

## Working agreements

- Match the style of the surrounding code: small packages, plain names, few comments. Comments state constraints the code cannot show.
- Prefer deleting or replacing weak code over layering on top of it. Nothing here is frozen; improve it when it's in the way.
- Don't hardcode anyone's network (IPs, paths, timezones). Defaults must work for a stranger.
- Secrets and tuner `DeviceAuth` are never written to the database or logs.
- Don't install tuner firmware from the app.
