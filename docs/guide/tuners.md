# Tuners

Broadwave plays the channels on an HDHomeRun, or on a server that answers the same way. More than one can be in the lineup at once. The server is the only thing that tunes. Every screen and every recording of one channel shares that tune.

## Models

Any current HDHomeRun works. Broadwave finds it on the network.

| Tuner | What it receives |
| --- | --- |
| FLEX 4K | Four tuners. Two of them receive ATSC 3.0. A 3.0 channel will not tune on the other two. |
| FLEX DUO, FLEX QUATRO, CONNECT | The regular broadcast only. A model name that contains 4K marks the first two tuners as ATSC 3.0. |

A clear ATSC 3.0 channel is an HEVC picture and AC-4 sound. Phones, TVs, and browsers do not decode AC-4, so Auto sends Dolby Digital where the device plays it, and stereo AAC otherwise. Surround sends 5.1 AAC when the device cannot play Dolby Digital. Apple TV 4K, a recent iPhone or iPad, and Safari can take the HEVC picture as the station sent it. Apple TV HD and an older iPhone or iPad get H.264. A browser that cannot play HEVC gets H.264.

Some NextGen stations encrypt the broadcast. Broadwave cannot decrypt them. When that station also sends a regular broadcast, Broadwave plays and records the regular one, and the player says "The 3.0 version is encrypted. Showing the regular broadcast." A station with no regular broadcast stays in Settings and does not play.

[ATSC 3.0](../atsc3.md) is the longer page: what plays, the Show choice between 3.0 and the regular channel, and the tuners again.

## How a tuner is found

With no address, the server broadcasts for HDHomeRun devices and reads an HDHomeRun announcement, and it looks up `hdhomerun.local` only when both come back empty. A reply counts only when it comes from this network. The address inside the packet is ignored.

The first time the lineup is empty, that search adopts one tuner. About every five minutes it refreshes tuners already in the lineup. It does not add a second one on its own.

Settings, Tuners and channels, Search the network runs that search and adds every tuner that answers. The search reads the lineup. It does not tune a channel.

Add by address takes a host, or a host and a port. A port you type is the only one tried. Leave it off and Broadwave checks port 80 and the ports other compatible servers use: 5004, 34400, 8409, and 9191.

Look harder checks this network, then, if nothing answers, SiliconDust's discovery list. A device from that list is kept only when it answers here with the same id.

## Several tuners

Each tuner keeps its own channels. Subchannels of one station share a tune. KBWV 4.1 and 4.2 use one tuner. WTST 5.1 takes another when one is free.

When the same channel number is on two tuners, Broadwave plays it from the tuner that channel belongs to, and tries the others when that device does not answer or has no free tuner that can take the channel. Two tuners on one antenna list each channel once on the guide. The other row stays in Settings and plays the same way.

A regular channel uses a tuner that cannot receive ATSC 3.0 when one is free, so a NextGen tune stays available.

## Removing one

Settings, Tuners and channels, Remove forgets a tuner that is gone. Favorites and passes move to the same channel on another tuner when there is one. Recordings stay. Broadwave refuses while something is playing or recording from it. The message is "Something is playing or recording from it. Stop that first."

[Finding tuners](../tuners.md) is the same account with the confirm text and the 1.0 / 3.0 Show choice.
