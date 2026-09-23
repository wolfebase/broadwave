# Research notes

Facts gathered while building. Each should shape a decision or a test.

## HDHomeRun

- CONNECT (non-EXTEND) models ignore HTTP `transcode=` profiles. Delivery encoding has to happen in ffmpeg on the server.
- Busy tuners return HTTP 503 with `X-HDHomeRun-Error: 805`.
- Signal fields worth showing in diagnostics: strength (`ss`), signal-to-noise quality (`snq`), symbol quality (`seq`).
- A FLEX 4K has four tuners, and only two of them do ATSC 3.0 (HEVC video, AC-4 audio). Neither AVPlayer nor browsers decode AC-4, so the server must transcode ATSC 3.0 audio. Some ATSC 3.0 stations are DRM-protected and can't be played.
- `hdhomerun.local` often doesn't resolve. Use UDP discovery on 65001, then `discover.json` and `lineup.json`.
- Don't install firmware from the app.

## Guide data

- The SiliconDust XMLTV feed (`api.hdhomerun.com/api/xmltv`) gives **2 days to everyone and 14 days with an HDHomeRun DVR subscription** (correction, verified 2026-09-23). Each request authenticates with the tuner's `DeviceAuth` (rotates, valid 16-24 h, read fresh per request, never stored), must accept gzip, and SiliconDust asks for the next download at a **random 20-28 h** after the last. It includes `<icon src>` channel logos and program images. On the user's lineup it lists only 9 of 27 channels.
- SiliconDust also has a JSON guide endpoint (`api.hdhomerun.com/api/guide?DeviceAuth=...`, fields `ImageURL`, `EpisodeNumber`, `Synopsis`, paged by start time) — check coverage and terms before using it.
- Schedules Direct is the fallback, with a paid account.

## Competitors

- **Channels DVR:** best-in-class OTA scheduling and commercial skip. Multiview (up to 4) on Apple TV 4K and iPad with no buffer in multiview, Personal Sections, TV Everywhere, intro and credits detection (preview). Commercial skip modes: skip automatically, show a Skip button, or leave it manual, and a double seek-forward inside a break jumps past it. The UI is widely described as dated. Costs $80 a year.
- **NextPVR** shares one tuner across channels on the same frequency. **Jellyfin** shares one MPEG-TS body and counts viewers. Our relay does both, and adds a shared rewind buffer that follows the tune.
- **Tablo 4th gen:** plug and play, but a limited guide and all content transcoded to H.264.
- **Plex:** the widest client reach, but live TV has become a lower priority.
- **AIRDVR:** a newer HDHomeRun DVR with side-by-side multiview (one view per tuner) and live sports scores; its iOS app was still "coming soon" in 2026. Watch it closely: it targets the same sports-first, multiview niche.

## Apple platforms (2026)

- iOS 27 and tvOS 27 refine Liquid Glass (better diffusion, a user transparency slider, darker edges). Standard components pick this up automatically. tvOS applies glass to focused standard controls on Apple TV 4K (2nd generation) and later.
- WWDC25 introduced multiview sync via `AVPlaybackCoordinationMedium` and AirPlay routing with `AVRoutingPlaybackArbiter`.
- WWDC26 introduced the Now Playing framework, remote media sessions, and CarPlay video apps (iOS 27).

## Research for the master plan (2026-09-23)

- Multiview on Apple: `AVPlaybackCoordinationMedium`, `AVRoutingPlaybackArbiter`, `networkResourcePriority` — WWDC25 session 302 and the "Creating a seamless multiview playback experience" sample (tvOS/iOS 26+).
- hls.js multiview: ~3-4 players per page is reasonable; fMP4 cuts CPU; Chrome MSE limits ~150 MB video / 12 MB audio per SourceBuffer, so cap back buffer per tile and use `capLevelToPlayerSize`. iPhone Safari uses ManagedMediaSource (hls.js 1.6+).
- LL-HLS: ffmpeg's HLS muxer does not emit `EXT-X-PART`/`EXT-X-PRELOAD-HINT`; a custom packager with blocking playlist reload is required. hls.js needs `lowLatencyMode: true`; AVPlayer supports it natively.
- ATSC 3.0: AC-4 decode exists in jellyfin-ffmpeg (experimental, resample to 48 kHz); DRM (A3SA) stations can't be decrypted.
- Sports status: ESPN unofficial scoreboard `site.api.espn.com/apis/site/v2/sports/{sport}/{league}/scoreboard?dates=YYYYMMDD` with `status.type.state` pre/in/post, competitors, logos, colors, broadcasts.
- Live Activities: iOS 18+ broadcast push channels need the developer's APNs key; self-hosted servers can't push without it.
- Top Shelf: `TVTopShelfContentProvider` + `TVTopShelfCarouselContent`; Swift 6 needs `@preconcurrency import TVServices` or the completion-handler override.
- Unraid Community Apps: public repo, OSI license, `ca_profile.xml` with non-empty `<Profile>`, template XML, real icon; validate at ca.unraid.net/submit/new.
- Channels DVR 2026: Enhanced Commercial Detection (fingerprinting, re-fingerprint, idle backfill), season-aware intro detection, multiview up to 4 (live only, no buffer).
