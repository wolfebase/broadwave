# Sources

What Waveguide can take in, how it is found, and how it is played. Decision: [0009](decisions/0009-sources-and-discovery.md). Checked against vendor docs on 2026-09-23.

HDHomeRun on a new install is added when it is found. Everything else waits for one tap. A "Look harder" probe runs only when asked, and only on local subnets.

| Source | Found by | Stream | Guide | Status |
| --- | --- | --- | --- | --- |
| HDHomeRun (FLEX, CONNECT, PRIME) | UDP 65001, `hdhomerun.local`, `<deviceID>.local` | Full mux on port 5004. This server uses `/tunerN/ch<freq>`. Docs also describe `/auto/ch<rf>`. | Device XMLTV, then another guide for the gaps | Supported. Auto-added. |
| HDHomeRun-compatible (tvheadend, Antennas, Threadfin, xTeVe, ErsatzTV, Dispatcharr) | Address, or SSDP / the app's HDHomeRun announce | HDHomeRun URLs, or the app's M3U | The app's XMLTV, or none | Supported once confirmed. Not auto-added. |
| M3U playlist | Pasted URL or text. Header `url-tvg` / `x-tvg-url` | MPEG-TS or HLS. Attributes `tvg-id`, `tvg-chno`, `tvg-logo`, `group-title` | That playlist's XMLTV only, by `tvg-id`, then station id, then name | Supported. Stream limit per playlist. |
| Xtream Codes | Server, username, password | `/live/user/pass/id.ts` or `.m3u8`. Panel `player_api.php` | `xmltv.php`, or `get_short_epg` | Supported. Password stored apart from the URL and masked. |
| Channels DVR server | mDNS `_channels_dvr._tcp` port 8089 | `/devices/ANY/channels.m3u?format=ts&codec=copy` | `/devices/ANY/guide/xmltv` | Supported. Uses that server's tuners. Host networking, or the DVR answers 403. |
| tvheadend | mDNS `_htsp._tcp`, web 9981 | `/playlist/channels.m3u`, `/stream/channelid/<id>?profile=pass` | `/xmltv/channels` | Supported. Web calls need an account or they return 403. |
| Threadfin / xTeVe | Port 34400, UPnP | `/m3u/threadfin.m3u` (xTeVe equivalent) | `/xmltv/threadfin.xml` | Supported as HDHomeRun or M3U. |
| ErsatzTV | Port 8409 | `/iptv/channels.m3u` or its HDHomeRun emulation | `/iptv/xmltv.xml` | Supported as HDHomeRun or M3U. |
| Dispatcharr | Port 9191 | `/output/m3u`, `/hdhr/lineup.json` | `/output/epg` | Supported as HDHomeRun or M3U. |
| Media folder | A path on the server | Files already on disk | Sidecar metadata | Supported. No tuner. |
| Tablo Gen 1–3 | UDP 8881/8882 or `api.tablotv.com` | Unofficial REST on port 8885, HLS | Its own guide | Optional. Not an open API, not auto-added. |
| Tablo Gen 4 | — | Needs Tablo's cloud and a signed call | — | Unsupported. No local API. |
| AirTV, Fire TV Recast | — | Sling or Amazon only | — | Unsupported. No open API. |
| TV Everywhere | — | Cable login inside a browser | — | Unsupported. No open API. |
| Free channels (FastChannels, Pluto for Channels, Samsung TV Plus for Channels) | Ports 5523, 7777, and 8182 when the user asks | The generator's M3U. DRM streams are skipped. | The generator's XMLTV | Supported. One tap when a server is already running. Waveguide does not scrape those services itself. |
| Pluto and other free apps as themselves | — | Pluto's stitcher wants a session from `boot.pluto.tv` | — | Unsupported as a built-in source. A generated M3U can still be added as a playlist. |

The public `i.mjh.nz` playlists were removed in August 2024. They are not a source.
