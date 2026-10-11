# Security

Broadwave is a server you run at home. It has no accounts. Anything that can open port 8477 can use the web app and the API, so keep that port on your own network.

## Names the server answers to

A request is refused when its name is a public DNS name the server was not given. That stops a web page from calling Broadwave by pointing the page's own name at your server's address.

These names work without any extra setting:

- An address, such as the one your router gave the computer.
- A one-word name.
- A name ending in `.local` or `.lan`, and the home names `.home.arpa`, `.fritz.box`, and `.attlocal.net`.

Set `BROADWAVE_HOSTS` to any other name you use, comma-separated. A leading dot covers the subdomains of that name, such as `.example.com`. A browser that reaches the server by HTTPS on port 443 is allowed. Any other public name or port needs `BROADWAVE_HOSTS`. The README lists the same rule under image options.

A refused request answers with the name it saw, and tells you to set `BROADWAVE_HOSTS` to that name.

## Requests

A page on another site cannot change the server. A request that changes something has to come from this server's own pages, or from an app that is not a browser. The live socket and the stream exports refuse a page on another site too. Plex, Jellyfin, Channels, and curl send no browser site header and are not affected.

The tuner interface this server offers to other apps on port 8478 uses the same name check as the API.

Responses carry `X-Content-Type-Options: nosniff`. The web app can still be framed, so a home dashboard can show it.

## Fetches

Playlists, guides, artwork, scoreboards, and the release check use http or https. Other schemes are refused. Cloud metadata names and addresses are refused, however the address is written: decimal, hex, octal, IPv4-mapped, or NAT64. Redirects are checked the same way.

Link-local addresses are refused for playlists, guides, and artwork. A tuner is allowed one, because a tuner with no DHCP server gives itself a 169.254 address. Private addresses (`10/8`, `172.16/12`, `192.168/16`, IPv6 ULA), loopback, and `.local` names work everywhere.

When ffmpeg or ffprobe opens a URL, the input is limited to http and https. A remote playlist cannot hand either program a file on this machine. A recording on disk is still opened as a file. A playlist you add by its path on this machine is still read.

## Files

A channel number from a playlist is cleaned before it becomes a recording's file name, so the file stays in the recordings folder. Show and season folders are cleaned the same way. Artwork, backups, and the library are served from ids and from names that stay inside their own folders.

## Size

Each cap is far past real use and stops a file that never ends or a compression bomb:

- A playlist, read or uploaded: 128 MB and 250,000 channels. A 30,000-channel playlist is about 15 MB.
- A guide after it is unpacked: 512 MB. xz may use 256 MB while it unpacks.
- A picture: 32 megapixels, and no side past 16,384.
- A folder of videos: 200,000 files.
- Event sockets: 128 at once.
- Apps copying the broadcast through the exports or the tuner interface: 32 at once. People watching in Broadwave are not counted.
- A recording you start: a day.

Playlist text, guide XML, the tables inside a broadcast, and fMP4 fragments are read with their own caps.

## Secrets

A password, token, secret, or key in a playlist or guide address, and a tuner `DeviceAuth`, are masked before they are shown or written to diagnostics, the device list, and the support bundle. The field name is matched in any case, and a password in an Xtream-style stream path (`/live/<user>/<password>/<id>`) is masked too. A source saved before a field was masked still fetches with its login. Settings shows the guide address with its password removed, and saving that page keeps the stored password. ffmpeg's messages go through the same log filter. A failed tuner or artwork request does not repeat its address.

## What is stored

Channels, the guide, recordings, and watch history stay in the database on the machine that runs Broadwave.

The tuner's `DeviceAuth` value is read when a guide request needs it and is not written to the database or the log. A Schedules Direct password, a playlist password, and a guide or artwork key are stored so the server can refresh those sources. Settings does not show a saved password. The Schedules Direct password and the artwork and scores key fields stay blank and say Saved. The log keeps those values out.

## Support bundle

Settings calls the download "Download a support bundle". The hint under the button is "Logs, versions, and settings. Passwords are left out." The same zip is at `http://<your-server>:8477/api/v1/support`.

The zip holds `versions.json`, `doctor.json`, `config.json`, and `logs.txt`. Passwords, tuner credentials, and `DeviceAuth` are removed before the file is saved.

## What the server contacts

The server talks to your tuner, to guide providers you turn on, to scoreboards while live scores are on, and to any playlist you add. Once a day it can ask GitHub whether a newer release exists. That request carries nothing about what you watch. Settings, Check for updates, turns it off. Settings, Live scores, stops the scoreboard requests. Look harder asks SiliconDust's device list when nothing on your network answers. An artwork or scores key you add sends requests to that service.

None of those calls go to the maker of Broadwave. The privacy policy is [PRIVACY.md](https://github.com/wolfebase/broadwave/blob/main/PRIVACY.md).

## Limits to know

Anyone on the network can use the server until accounts exist.

ffmpeg looks up a stream's name again after Broadwave has checked it, and a later segment or key can name another address. A name can change where it points between the two lookups.

On plain http, a page on another site can still open an export or a tuner URL in a way that sends no site header.

The encoder's command line still shows the stream address it was given. Its log output does not.

Recordings stay the original broadcast. Transcoding is only for playback.

## Away from home

Use a VPN that puts your phone on the home network, and open the server's address on that VPN. Do not forward port 8477 from the public internet.

The apps do not install tuner firmware. Diagnostics says "This app does not install firmware."
