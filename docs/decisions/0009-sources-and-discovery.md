# 0009: Sources and discovery

Accepted, 2026-09-23.

## Goal

Every source a Channels DVR user already has can be added here, and an HDHomeRun on the same network shows up without anyone typing an address.

## Design

**One list of sources.** A source has a kind, a stable id, a display name, an enabled flag, a priority, and a health. Kinds that take an antenna tuner go through the tuner pool and reuse a frequency that is already tuned. Kinds that do not (a playlist, Xtream, a Channels DVR server, a folder) count their own stream limit instead. Refreshing a source keeps channel ids, favorites, hidden flags, passes, and recordings.

**Find HDHomeRun first.** On a new install, a tuner found by the UDP 65001 broadcast or by an HDHomeRun SSDP reply is added. `hdhomerun.local` is looked up only when both of those come back empty. A device-id hostname is not looked up. Everything else that discovery sees waits for one tap. The cloud list at `https://ipv4-api.hdhomerun.com/discover` is a last resort: SiliconDust does not support it, and behind CGNAT it can name someone else's device. A port probe of the LAN runs only when someone asks, and only on local subnets.

**Follow the device, not the address.** A tuner or a server is remembered by its id. When DHCP moves it, the address updates and the channels stay put.

**The full mux is the tune.** A known frequency opens `/tunerN/ch<frequency>`. An ATSC 3.0 channel, or a tune that cannot open that multiplex, uses `/auto/v<guide number>`. SiliconDust's docs also describe `/auto/ch<rf>`. A program-filtered stream rebuilds the PAT and drops PSIP.

**Playlists stay with their guide.** An M3U's `url-tvg` or an XMLTV URL fills only that source. The stored guide key is `tvg-id` when the line has one, and the station id otherwise. Matching tries that key, then the channel number, then the name, then the call sign. Xtream is the same idea with `player_api.php`, `/live/…`, and `xmltv.php`. Passwords and tokens are stored apart from the URL. The settings answer leaves them out, and the log replaces them with `••••`.

**What we will not pretend to support.** AirTV, Fire TV Recast, and TV Everywhere have no open API. Tablo Gen 4 needs Tablo's cloud. Pluto has no static playlist. Legacy Tablo (Gen 1–3) has an unofficial local API. Broadwave does not add one. Look harder only notices a page on port 8885 that says tablo. The matrix is `docs/sources.md`.

## Consequences

Setup for an antenna is a tuner appearing, then a channel playing. A playlist or another server is one confirmation. A source we cannot speak to is named in the matrix with the reason. Look harder can still show Add for one, including a page on port 8885 that says tablo, and that Add leaves the lineup unchanged.
