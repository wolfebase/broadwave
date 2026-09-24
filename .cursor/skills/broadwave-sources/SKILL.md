---
name: broadwave-sources
description: Rules and test recipes for Broadwave channel sources and auto-discovery — HDHomeRun devices (broadcast, unicast, SiliconDust cloud discovery, channel scan, multi-device pool), HDHomeRun emulators (tvheadend, Threadfin, xTeVe, ErsatzTV, Dispatcharr, Antennas), Channels DVR as a source, M3U/XMLTV playlists, Xtream Codes, free-channel presets, the setup wizard, and the setup doctor. Use when adding or changing a source kind, discovery, M3U parsing, guide attachment for playlists, setup, or Diagnostics checks (plan Phase S).
---

# Broadwave sources and discovery

Goal: anything Channels DVR accepts as a source works in Broadwave, and Broadwave finds most of it without the user typing an address. Facts and endpoints live in `docs/research.md` (S0) and the support matrix in `docs/sources.md`; re-verify version-sensitive ones.

## Where code lives

- `server/internal/hdhr`: device protocol (UDP 65001 discovery, HTTP `discover.json`/`lineup.json`/`lineup_status.json`, control protocol, stream info).
- `server/internal/discovery`: Bonjour advertising today; the S2 auto-find engine (broadcast, unicast subnet, cloud, SSDP, mDNS, opt-in port probe) goes here.
- `server/internal/source`: M3U parsing and install, media folders, sync. S1 turns this into one source model with kinds.
- `server/internal/guide`: XMLTV / Schedules Direct / SiliconDust parsing and merge (C2); PSIP goes in `internal/psip` (C7).
- HTTP: `server/internal/httpapi/sources.go`. Contract: `api/openapi.yaml` (drift test).

## Rules

- **Never waste a tuner.** Sources that need a tuner go through the tuner pool (reuse a tuned frequency first). Sources that don't (M3U, Xtream, Channels DVR, free channels) still honor their stream limit, counted per source.
- **Stable channel ids.** Refreshing a source must keep ids, favorites, hidden flags, passes, and recordings. Key HDHomeRun channels by device id + guide number, playlist channels by `tvg-id`, then stream URL, then name + number.
- **Credentials.** Xtream passwords, tvheadend auth, and tokens in URLs are stored apart from the URL, masked (`••••`) in every API response, log line, Diagnostics view, and error. Never write tuner `DeviceAuth` anywhere.
- **Guide attachment is per source.** A playlist's XMLTV (explicit or `url-tvg`) only fills that source's channels, matched by `tvg-id`, then `tvc-guide-stationid`, then name.
- **Auto-add only HDHomeRun on a fresh install.** Everything else found by discovery waits for one tap. The "Look harder" port probe runs only when the user asks, only on local subnets, rate-limited.
- **Follow devices, not addresses.** Track by device or server id; when DHCP moves a device, update the address silently.
- **Errors say what failed and what to try** (copy voice): "That playlist has no channels. Check the link opens in a browser." "Broadwave can't see your tuner from inside Docker. Switch the container to host networking."
- **HLS inputs keep sync.** When an input is HLS, use its program date-times for the feed timeline when present; otherwise stamp from arrival time. TS inputs use the normal mux path.

## Test recipes

- Fakes, not real networks: fake HDHomeRun (K1), a fake Xtream server (`httptest`), fake SSDP and UDP responders bound to loopback, recorded M3U/XMLTV samples (anonymized: strip tokens and hostnames) in `testdata/`.
- Real checks on this network: the DUO at `192.168.1.252` (broadcast and cloud discovery), a bridged container on the Mac (`docker run` without `--network host`) to prove the cloud/unicast fallback, and a local tvheadend or ErsatzTV container for emulator sources. Check Unraid's schedule before tuning (skill `broadwave-dev-loop`, "Shared tuner").
- Big playlists: generate a 5,000-entry M3U in the test; import must stay under 3 s and the guide must not render all of them unasked (picker).
- Setup: `FRESH=1 CONFIG=/tmp/broadwave-fresh scripts/dev-server.sh`, then walk the wizard at three sizes and on the tvOS and iPhone simulators; time from first load to a playing channel.

## Done means

Support matrix row updated in `docs/sources.md`, OpenAPI updated, Go tests with fakes, one real verification logged (or the reason it can't be), setup and Diagnostics show the source with health, and the apps (P5) can add it too.
