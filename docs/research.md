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

- The SiliconDust XMLTV feed (`api.hdhomerun.com/api/xmltv`) no longer needs their DVR subscription as of May 2026. Each request authenticates with the tuner's `DeviceAuth`, which rotates every 16 to 24 hours, so it is read fresh per request and never stored.
- Schedules Direct is the fallback, with a paid account.

## Competitors

- **Channels DVR:** best-in-class OTA scheduling and commercial skip. Multiview (up to 4) on Apple TV 4K and iPad with no buffer in multiview, Personal Sections, TV Everywhere, intro and credits detection (preview). Commercial skip modes: skip automatically, show a Skip button, or leave it manual, and a double seek-forward inside a break jumps past it. The UI is widely described as dated. Costs $80 a year.
- **NextPVR** shares one tuner across channels on the same frequency. **Jellyfin** shares one MPEG-TS body and counts viewers. Our relay does both, and adds a shared rewind buffer that follows the tune.
- **Tablo 4th gen:** plug and play, but a limited guide and all content transcoded to H.264.
- **Plex:** the widest client reach, but live TV has become a lower priority.

## Apple platforms (2026)

- iOS 27 and tvOS 27 refine Liquid Glass (better diffusion, a user transparency slider, darker edges). Standard components pick this up automatically. tvOS applies glass to focused standard controls on Apple TV 4K (2nd generation) and later.
- WWDC25 introduced multiview sync via `AVPlaybackCoordinationMedium` and AirPlay routing with `AVRoutingPlaybackArbiter`.
- WWDC26 introduced the Now Playing framework, remote media sessions, and CarPlay video apps (iOS 27).
