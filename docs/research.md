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

- The SiliconDust XMLTV feed (`api.hdhomerun.com/api/xmltv`) gives **2 days to everyone and 14 days with an HDHomeRun DVR subscription** (correction, verified 2026-09-23). Each request authenticates with the tuner's `DeviceAuth` (rotates, valid 16-24 h, read fresh per request, never stored), must accept gzip, and SiliconDust asks for the next download at a **random 20-28 h** after the last. It includes `<icon src>` channel logos and program images. On the user's lineup it lists only 9 of 27 channels. Checked again 2026-09-23 against `lineup.json`: both XMLTV and `api/guide` publish the same nine (4.1, 5.1, 9.1, 9.4, 29.1, 38.1, 39.7, 41.1, 62.1). 14.1–14.16, 43.3, and 46.7 are not in either feed under any call sign, so a matcher cannot invent their listings.
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

## Sources and discovery (2026-09-23; plan Phase S, re-verify in S0)

**What Channels DVR supports.** HDHomeRun is the only official tuner (auto-found or added by IP). Custom Channels are M3U playlists of MPEG-TS (http, https, rtsp, satip) or HLS, up to 750 channels each; free services arrive only through M3U generators. TV Everywhere uses a cable login in Chromium (`fancybits/channels-dvr:tve` image). Tablo Gen 3 beta was dropped, Gen 4 unsupported; AirTV unsupported. Parity target: HDHomeRun, M3U + XMLTV, source priority with rollover, stream limits, M3U/XMLTV export. ([dvr-server](https://getchannels.com/dvr-server/), [custom channels](https://getchannels.com/docs/channels-dvr-server/how-to/custom-channels/), [Tablo beta](https://community.getchannels.com/t/beta-support-for-tablo-tuners-gen-3/33846))

**HDHomeRun.**
- Models: FLEX DUO, FLEX QUATRO, FLEX 4K (4 tuners, 2 ATSC 3.0, 3.0 channels numbered 100+), PRIME (CableCARD). EXTEND and SCRIBE discontinued. SCRIBE/SERVIO are storage devices listing recordings at `recorded_files.json`. DRM (Widevine) ATSC 3.0 channels play nowhere; the FLEX 4K falls back to the 1.0 simulcast.
- Discovery: UDP 65001 broadcast (and per-subnet broadcast). Packet `u16 type, u16 len, TLVs, CRC32 (LE)`. Types `0x0002` request, `0x0003` reply. Tags `0x01` device type (1 tuner, 5 storage), `0x02` id, `0x10` tuner count, `0x27` lineup URL, `0x28` storage URL, `0x2A` base URL, `0x2B` DeviceAuth (never store), `0x2C` storage id. mDNS hostnames `hdhomerun.local`, `hdhr-<id>.local`. SSDP answers `upnp:rootdevice` with `Server: HDHomeRun/1.0`. `api.hdhomerun.com/discover` lists devices behind the same public IP but SiliconDust calls it unsupported and asks apps not to use it; behind CGNAT it returns strangers' devices. ([discovery API](https://info.hdhomerun.com/info/discovery_api), [libhdhomerun](https://github.com/Silicondust/libhdhomerun/blob/master/hdhomerun_discover_example.c), [reddit](https://www.reddit.com/r/hdhomerun/comments/1oq2o67/http_api_discover/))
- HTTP: `discover.json`, `lineup.json|.xml|.m3u` (Tags `favorite`, `drm`), `lineup_status.json`, `lineup.post?scan=start`, `status.json`. Streams on 5004: `/auto/v5.1` (any tuner), `/tuner1/v5.1`, `/auto/ch473000000` (**unfiltered full mux**), `/auto/ch473000000-3` (one program), `?duration=`, `?transcode=` (EXTEND only). 503 busy or DRM with `X-HDHomeRun-Error`, 404 unknown. A program-filtered stream gets a device-built PAT/PMT and probably no PSIP; PID filters exist only on the control protocol ("not available through HTTP"); some stations carry EIT on non-standard PIDs, so follow the MGT. ([HTTP API](https://info.hdhomerun.com/info/http/_api), [sample docs](https://info.hdhomerun.com/info/troubleshooting:creating_a_sample), [SD forum](https://forum.silicondust.com/forum/viewtopic.php?t=80073))

**Tablo.** Gen 1-3: discovery via `api.tablotv.com/assocserver/getipinfo/` or UDP 8881 → 8882; REST on 8885 (`/server/info`, `/guide/channels`, `POST …/watch` returns an HLS `playlist_url`). Gen 4 needs Tablo's cloud login and HMAC-signed local calls with keys from the iOS app: unsupported. Channels users rely on `tmm1/tablo-for-channels`. ([unofficial docs](https://jessedp.github.io/tablo-api-docs/), [tablo-api](https://github.com/trevor-viljoen/tablo-api))

**Other tuners and middleware.**

| Product | Way in | Notes |
| --- | --- | --- |
| AirTV 2 / Anywhere | none (Sling only) | unsupported |
| Fire TV Recast | none | unsupported |
| Ceton InfiniTV/ETTU | `cetonproxy` HDHomeRun emulation on :5004 | legacy CableCARD |
| tvheadend | web 9981, HTSP 9982; mDNS `_htsp._tcp`, `_http._tcp` | `/playlist/channels.m3u` (`/playlist/auth/channels` for persistent auth; tickets expire in 5 min), `/xmltv/channels`, `/stream/channelid/<id>?profile=pass`; HDHomeRun/SAT>IP modes need host networking |
| Antennas | HDHomeRun emulation for tvheadend on :5004 | Plex discovery breaks in Docker |
| Threadfin / xTeVe | :34400, UPnP announce | `/m3u/threadfin.m3u`, `/xmltv/threadfin.xml` |
| ErsatzTV | :8409 | HDHomeRun emulation, `/iptv/channels.m3u`, `/iptv/xmltv.xml` |
| Dispatcharr | :9191 | `/hdhr/discover.json`, `/hdhr/lineup.json`, `/output/m3u`, `/output/epg`, Xtream `get.php`/`xmltv.php`; EPG rebuilds can stall it |
| Hauppauge USB | through tvheadend | |

Plex does not auto-detect emulators on non-standard ports.

**IPTV formats.**
- M3U (Kodi pvr.iptvsimple reference): header `x-tvg-url`/`url-tvg`, `tvg-shift`, `catchup-correction`; entries `tvg-id`, `tvg-name`, `tvg-logo`, `tvg-chno`, `group-title` (`;`-separated), `radio`, `catchup` (`default|append|shift|flussonic|xc|vod`), `catchup-source` (`{utc}`, `{duration}`, `{Y}`…), `catchup-days`; `#EXTVLCOPT`, `#KODIPROP`; XMLTV gzip or xz. ([pvr.iptvsimple](https://github.com/kodi-pvr/pvr.iptvsimple/blob/Omega/README.md))
- Channels tags: `channel-id` (required), `channel-number`, `tvg-name`, `tvg-logo`, `tvc-guide-stationid` (Gracenote id, auto guide), fallback `tvc-guide-title/-description/-art/-tags/-genres/-categories/-placeholders`, `tvc-stream-vcodec/-acodec`. Without a station id, `channel-id` must match the XMLTV channel id.
- Xtream Codes: `player_api.php?username&password` with `action=get_live_categories|get_live_streams[&category_id]|get_short_epg&stream_id&limit|get_simple_data_table`; streams `/live/u/p/<id>.ts|.m3u8`; `get.php?…&type=m3u_plus&output=ts`; `xmltv.php`; catch-up `/timeshift/u/p/<mins>/<YYYY-MM-DD:HH-MM>/<id>.ts`. Stalker portals use MAC auth (out of scope).

**Free-channel generators** (none announce themselves; hosted i.mjh.nz lists were withdrawn in August 2024; Pluto now needs a login and one stream per device id): FastChannels (:5523, 25+ services, `/feeds/<name>/m3u`, `/feeds/<name>/epg.xml`, `/feeds/<name>/m3u/gracenote`), Pluto for Channels (kineticman :7777 with a session pool; maddox `8080:80`, `/playlist.m3u`, `/epg.xml`), Samsung TV Plus for Channels (`8182:80`, `/playlist.m3u8?regions=us`), Plex/Tubi for Channels (jgomez177). Roku and Pluto carry the most DRM channels. ([FastChannels](https://github.com/kineticman/FastChannels), [i.mjh.nz #127](https://github.com/matthuisman/i.mjh.nz/issues/127))

**Discovery a server can run.** HDHomeRun UDP 65001 (a few emulators answer too); SSDP `M-SEARCH` to `239.255.255.250:1900` with `ST: upnp:rootdevice` or `urn:schemas-upnp-org:device:MediaServer:1`, then `Location` → `device.xml` → base URL → `/discover.json`; mDNS `_channels_dvr._tcp` (8089, TXT version/arch/os), `_htsp._tcp`; Jellyfin answers `who is JellyfinServer?` on UDP 7359; Plex GDM on UDP 32414 (unverified); port probes on 80/5004, 9981, 34400, 8409, 9191, 8089, 5523, 7777/8182, 8885, 32400, 8096.

**Channels DVR as a source.** `/devices` lists its tuners; `/devices/ANY/channels.m3u?format=ts&codec=copy&filter=favorites|hd&bitrate=`; `/devices/ANY/guide/xmltv`; streams `/devices/<id>/channels/<n>/stream.mpg` or `/hls/master.m3u8`. No login on the LAN; requests that look external get 403 (bridged Docker can trigger it).

**Channels "Add Source" UX to match.** Nickname; stream format (HLS/MPEG-TS); URL or pasted text; refresh URL daily; prefer channel numbers from M3U; prefer logos from M3U; stream limit; XMLTV URL with refresh (3 h, 6 h, daily); guide optional when every channel has a station id. Also Scan Network, manage lineup (hide, set station id), source priority with rollover, duplicate stacking, and a limit of 1 stream to model a tuner.
