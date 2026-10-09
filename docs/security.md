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

A browser page from another site cannot change the server. A request that changes something has to come from this server's own pages, or from an app that is not a browser. The host name on the request has to be this server.

The tuner interface this server offers to other apps uses that same host check.

## Fetches

Playlists, guides, artwork, and tuner requests use http or https. Other schemes are refused. Link-local addresses, and the well-known cloud metadata names and addresses, are refused. A tuner on a private network address still works. A tuner that answers only on a link-local address does not.

When ffmpeg or ffprobe opens a URL, the input is limited to http and https. A playlist cannot hand either program a file on this machine. A recording on disk is still opened as a file.

## Files

A channel number from a playlist is cleaned before it becomes a recording's file name, so the file stays in the recordings folder. Show and season folders are cleaned the same way. Artwork, backups, and the library are served from ids and from names that stay inside their own folders.

## Size

A playlist upload, a picture, and a guide each stop at a fixed size. An app copying the broadcast stops at 32 streams at once. A recording you start is set for at most a day. A picture whose header claims a huge width or height is not decoded. A compressed guide that expands past the cap is refused, and xz is held to a memory limit while it expands. An event socket keeps its message cap. The server holds a fixed number of those sockets and closes the next one.

Playlist text, guide XML, and the tables inside a broadcast are read with their own caps.

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

ffmpeg looks up a name on its own after Broadwave has checked the first address. A name can change where it points.

A stream that starts on an allowed address can later ask for a segment or a key on a link-local address.

A tuner that uses only a link-local address is refused.

Recordings stay the original broadcast. Transcoding is only for playback.
## Away from home

Use a VPN that puts your phone on the home network, and open the server's address on that VPN. Do not forward port 8477 from the public internet.

The apps do not install tuner firmware. Diagnostics says "This app does not install firmware."
