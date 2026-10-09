# Share this server as a tuner

Plex, Jellyfin, Emby, and Channels can add Broadwave the way they add an HDHomeRun. They watch through this server. A channel that is already on does not take another tuner.

## Turn it on

Open Settings, then Share with other apps, and turn on Act as an HDHomeRun. The address is this server's host and port 8478. Include the port. Without it, an app assumes port 5004, and that port belongs to a tuner already on the network.

The switch applies within a minute. Broadwave does not answer HDHomeRun discovery, so a tuner already on the network stays the one those apps find on their own. Add this server by the address.

The playlist and the guide work with the switch off. They are on this server's own address:

- M3U playlist: `http://<this server>/export/lineup.m3u`
- XMLTV guide: `http://<this server>/export/guide.xml`

Settings lists a copyable row of each for Plex, Jellyfin, Emby, and Channels.

## What to give each app

Plex, Jellyfin, and Emby take the HDHomeRun address. Channels takes that address too, or the playlist and the guide together as a custom channel source. In the playlist, the channel id is the same id the guide uses. The channel number is the one on screen. When you open this server with https, the playlist and the guide use https too.

Jellyfin and Emby play the stream address in the lineup. Channels and Plex often ask for `/auto/v` and the channel number instead, and ignore the address written in the lineup. Both reach the channel.

Jellyfin checks a playlist stream before it plays. That check gets the stream type and does not take a tuner.

The guide gives each show a season and episode in the form Plex and Jellyfin read. A channel with a logo includes it, and so does a show that has artwork.

A virtual channel is on the HDHomeRun lineup. The playlist lists broadcast channels and multiview channels.

## What this tuner tells them

It introduces itself as a tuner that does not transcode. The stream is the broadcast, and it runs until the app disconnects. There is no DeviceAuth, and there is no SiliconDust guide. Use the XMLTV guide.

A channel scan finishes at once and does not take a tuner. The lineup stays the one Broadwave already shows. A hidden channel stays off it. An encrypted channel stays off it until you show that channel. The lineup then marks it protected, and a request to play it is refused.

The apps are told they may open several streams. The limit is still the tuners in the house. When those are busy, the stream says all tuners are in use.

Do not point an app at port 5004 for this server.

## When a stream does not start

An unknown channel is not found. A request for a transcode profile is refused, because this server does not transcode for these apps. A protected channel is refused. A tune that fails before any video is reported as a tune failure.
