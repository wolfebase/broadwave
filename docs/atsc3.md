# ATSC 3.0

NextGen TV, on a tuner that receives it. A clear station plays. An encrypted one plays as that station's regular broadcast.

A channel is 3.0 when the lineup says HEVC and AC-4. The channel number is not the sign: 100.1 can still be a regular broadcast. A station can show up twice, once around its usual number and once a hundred higher, such as KBWV on 4.1 and on 104.1.

## What plays

A clear 3.0 channel is an HEVC picture and AC-4 sound. A second sound track, often another language, can be picked the same way as on any other channel.

Apple TV 4K, an iPhone whose machine id is `iPhone9` or later, an iPad whose machine id is `iPad7` or later, and Safari or Chrome when it reports HEVC, get the HEVC picture as the station sent it, including a 2160p picture. Apple TV HD gets H.264 at 1080p. An older iPhone or iPad gets H.264. A browser that cannot play HEVC gets H.264. The picture is scaled down only when the player names a maximum height. Apple TV HD names 1080, and Apple TV 4K names 2160. iPhone and iPad do not, so a 2160p picture they can decode stays 2160p.

Phones, TVs, and browsers do not decode AC-4. Broadwave reads each track and sends it on:

- A 5.1 mix goes out as Dolby Digital where the device plays it.
- Auto sends stereo AAC otherwise. Surround sends 5.1 AAC when the device cannot play Dolby Digital.

A recording of a clear 3.0 channel keeps the broadcast's own packets. The Docker image includes the decoder this needs. A server built without it still shows the picture, with no sound.

## What does not play

Some NextGen stations encrypt the broadcast (A3SA). The tuner marks those channels and will not send the stream. The license stays in the tuner maker's app, so Broadwave cannot decrypt them and does not try.

When that station also sends a regular broadcast, Broadwave plays and records the regular one. The encrypted channel stays off the guide. Opening one of its shows from search, or the channel from a link, starts that broadcast, and the player says "The 3.0 version is encrypted. Showing the regular broadcast." On the web, Settings reads "Encrypted (ATSC 3.0 DRM). Plays and records 5.1, the same station in ATSC 1.0." On iPhone, iPad, and Apple TV the same row reads "Encrypted (ATSC 3.0 DRM). It plays and records the same station in ATSC 1.0." WTST on 115.1 is that kind of row: it plays and records as 5.1.

A scheduled recording on the encrypted channel records the regular broadcast too.

If the station has no regular broadcast to use, the channel stays in Settings and does not play. The note is "Encrypted (ATSC 3.0 DRM). Only the tuner maker's app can play it."

## Choosing 3.0 or 1.0

A clear 3.0 channel is paired with the same station's main regular channel, the lowest subchannel of that call sign. The two share one guide.

On the web, Settings lists the 3.0 row with Show:

- **3.0 only.** The usual choice. The regular channel stays off the guide.
- **1.0 only.** The regular channel stays, with its number in the label. Broadwave picks this on its own when that picture is known to be taller.
- **Both.** The guide lists each.

The favorite follows the channel that stays. On iPhone, iPad, and Apple TV the same three choices are on the channel's page. The note there is "This station broadcasts in ATSC 1.0 and 3.0. Both share one guide."

An encrypted station does not offer the choice. It always uses the regular broadcast.

## Tuners

| Tuner | What it receives |
| --- | --- |
| FLEX 4K | Four tuners. Two of them receive ATSC 3.0. A 3.0 channel will not tune on the other two. |
| FLEX DUO, FLEX QUATRO, CONNECT | The regular broadcast only. A model name that contains 4K marks the first two tuners as ATSC 3.0. |

A 3.0 channel takes a tuner that can receive it. A regular channel takes a tuner that cannot, when one is free, so a NextGen tune stays available. Everyone watching one channel still shares that one tune. A 3.0 channel is its own frequency. It does not ride the station's regular multiplex.
