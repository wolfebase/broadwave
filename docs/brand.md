# Brand

## Name: Waveguide

Chosen 2026-09-23, replacing the working name "OTA Viewer".

A waveguide is the structure that carries a radio signal to where it's needed. That is the product: it takes the broadcast off your antenna and carries it to every screen in the house. It also reads as "the guide for the airwaves", and the guide is the heart of the app.

- One word, easy to say and spell, no "TV", "OTA", or "Air" prefix to blend in with.
- No live TV or DVR product uses it. The closest match is a dormant open-source WebRTC server.

### Names considered

| Name | Why not |
| --- | --- |
| Towerlight | Strong runner-up (the red beacon on broadcast towers, matching our tally-red light). |
| Rooftop | Too common a word to own in search. |
| Openair | Clear, but less distinctive. |
| Tally / TallyTV | Taken: a tracker app and a tvOS scoreboard app. |
| Rabbit Ears | RabbitEars.info is the well-known OTA reference site. |
| Hearth | Several TV launcher projects use it. |
| Airloom | An heirloom-photo app. |
| Air-anything | Crowded: AirTV, Aerial TV, IPTV Air, AIRDVR (a competitor with multiview and sports scores). |

## Identifiers

| Where | Value |
| --- | --- |
| Display name | Waveguide |
| Go module and binary | `waveguide` (`server/cmd/waveguide`) |
| Database | `waveguide.db` (the server adopts a pre-rename `ota-viewer.db` on start; migration 0004 renames a default "OTA Viewer on host" server name) |
| Bonjour | `_waveguide._tcp` |
| URL scheme | `waveguide://watch/<id>`, `waveguide://connect?url=` |
| Bundle ID | `com.wolfeup.waveguide` (iOS and tvOS) |
| Container image | `ghcr.io/twolfekc/waveguide` |
| GitHub | `twolfekc/waveguide` |
| Unraid | container `Waveguide`, appdata `/mnt/user/appdata/waveguide` |

The Swift packages `OTAKit` and `OTAUI` keep their names: "OTA" (over the air) describes the domain, not the brand.

## Visual identity

The signature is the tally-red "live" light (`--color-tally`), shown next to the wordmark on web and Apple. Logo, app icon, and marketing site are plan task J7.
