# Architecture

```mermaid
flowchart LR
  subgraph home [Home network]
    HDHR[HDHomeRun tuners]
    subgraph server [OTA Viewer server]
      TunerPool[Tuner pool]
      Relay[Broadcast relay]
      Renditions[Renditions]
      Sync[Sync clock and rooms]
      DVR[DVR]
      Guide[Guide and metadata]
      API[API v1, events, Bonjour]
      Export[HDHR emulation, M3U, XMLTV]
    end
  end
  HDHR --> TunerPool --> Relay --> Renditions
  Relay --> DVR
  Renditions --> API
  Sync --> API
  Guide --> API
  Relay --> Export
  API --> TV[Apple TV app]
  API --> Phone[iPhone app]
  API --> Web[Web app]
  Export --> Others[Plex, Jellyfin, Channels]
```

## Server (`server/`)

One Go binary. It embeds the web app, keeps its catalog in SQLite under `-config` (default `./data`), and writes live buffers and recordings under `<config>/work`.

### Tuning and the relay (`internal/hdhr`, `internal/live`)

- `hdhr` speaks the HDHomeRun protocols: UDP discovery on 65001, the TCP control channel (`/tunerN/vchannel`, `/tunerN/status`, `/tunerN/streaminfo`), and HTTP `discover.json`, `lineup.json`, `status.json`.
- `live.Hub.Watch` tunes a whole RF **frequency** (`/tunerN/ch<freq>` on port 5004), not a single subchannel. A `mux` reads that transport stream once and fans the bytes out to subscribers. Each subchannel on the frequency becomes a `feed` with its own ffmpeg process, so 14.1 through 14.16 can all play from one tuner.
- Viewers of the same channel share the same feed and HLS playlist. Recordings attach another subscriber that copies the original MPEG-TS to disk.
- Tuners are released 20 seconds after the last viewer leaves, or after 45 seconds without segment requests (a closed tab or a sleeping phone).
- Live HLS uses 2-second segments with `EXT-X-PROGRAM-DATE-TIME`, keeping about 90 minutes for rewind.

**Next (relay v2):** a per-frequency ring buffer that live, recordings, and exports read from; a rendition ladder (direct remux with AC-3 passthrough for Apple devices, plus HEVC and H.264 transcodes) served from a master playlist, so a quality change never restarts anyone else; and a per-client stream decision. See `docs/decisions/0002-playback-pipeline.md`.

### Whole-Home Sync (`internal/sync`, planned)

A room per channel holds a target presentation time derived from segment program date-times. Clients measure their clock offset against the server over the events WebSocket and align playback to the room's target. See `docs/decisions/0003-whole-home-sync.md`.

### DVR (`internal/dvr`)

- `Plan` expands passes against the guide for 14 days and resolves conflicts by priority against the tuner count. Two airings on one channel need only one tuner.
- `Tick` runs every 20 seconds and starts recordings with padding.
- `OnSaved` runs commercial detection (comskip when installed, otherwise ffmpeg blackdetect) and writes EDL and JSON sidecars.
- Virtual (library) channels schedule recordings as a 24-hour channel without using a tuner.

### Guide (`internal/guide`)

The main source is the SiliconDust XMLTV feed, authenticated per request with the tuner's current `DeviceAuth`, which is never stored. Schedules Direct fills channels the feed misses, and M3U sources can bring their own XMLTV. Listings refresh at startup and every 4 hours.

### Store (`internal/store`)

SQLite (WAL) through `modernc.org/sqlite`. Tables: devices, channels, airings, recordings, passes, markers, virtual channels, progress, settings, events, sources.

### HTTP (`internal/httpapi`)

JSON under `/api`, HLS under `/media`, the SPA at `/`. The optional HDHomeRun emulator on `:8478` exposes channels to other apps. The contract lives in `api/openapi.yaml`.

## Web app (`web/`)

React and Vite, built into `server/cmd/ota-viewer/assets/web` and embedded with `go:embed`. It plays HLS through hls.js and doubles as the setup and admin console.

## Apple apps (`apple/`)

SwiftUI apps for iOS and tvOS on shared packages: `OTAKit` (API client, discovery, pairing, sync engine) and `OTAUI` (theme and components). Playback runs through `AVPlayerViewController`. See `.cursor/rules/apple-swift.mdc`.

## Design tokens (`design/`)

`tokens.json` generates CSS variables for the web app and a Swift theme for the Apple apps, so both clients share one visual language.
