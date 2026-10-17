# Security

Broadwave is a server you run at home. It has no account password. With device sign-in off, which is the default, anything that can open port 8477 can use the web app and the API, so keep that port on your own network.

## Names the server answers to

A request is refused when its name is a public DNS name the server was not given. That stops a web page from calling Broadwave by pointing the page's own name at your server's address.

These names work without any extra setting:

- An address, such as the one your router gave the computer.
- A one-word name.
- A name ending in `.local` or `.lan`, and the home names `.home.arpa`, `.fritz.box`, and `.attlocal.net`.

Set `BROADWAVE_HOSTS` to any other name you use, comma-separated. A leading dot covers the subdomains of that name, such as `.example.com`. A browser that sends `Sec-Fetch-Site` is allowed on port 443, or when the name has no port. The check does not look at HTTPS. Any other public name needs `BROADWAVE_HOSTS`. The README lists the same rule under image options.

A refused request answers with the name it saw, and tells you to set `BROADWAVE_HOSTS` to that name.

## Requests

A page on another site cannot change the server. A request that changes something has to come from this server's own pages, or from an app that is not a browser. The live socket and the stream exports refuse a page on another site too. Plex, Jellyfin, Channels, and curl send no browser site header and are not affected.

The tuner interface this server offers to other apps on port 8478 uses the same name check as the API.

Responses carry `X-Content-Type-Options: nosniff`. The web app can still be framed, so a home dashboard can show it.

## Fetches

Playlists, guides, artwork, scoreboards, and the release check use http or https. Other schemes are refused. Cloud metadata names and addresses are refused, however the address is written: decimal, hex, octal, IPv4-mapped, or NAT64. Redirects are checked the same way.

Link-local addresses are refused for playlists, guides, and artwork. A tuner is allowed one, because a tuner with no DHCP server gives itself a 169.254 address. Private addresses (`10/8`, `172.16/12`, `192.168/16`, IPv6 ULA), loopback, and `.local` names work everywhere.

When ffmpeg or ffprobe opens a URL, the input is limited to http, https, tcp, tls, and crypto. A remote playlist cannot hand either program a file on this machine. A recording on disk is still opened as a file. A playlist you add by its path on this machine is still read.

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

The tuner's `DeviceAuth` value is read when a guide request needs it and is not written to the database or the log. A Schedules Direct password, a playlist password, and a guide or artwork key are stored so the server can refresh those sources. Settings does not show a saved password. The Schedules Direct password and the movie artwork key stay blank and say Saved, on the web and in the Apple apps. The TheSportsDB key field says Saved on the web and stays blank in the Apple apps. The log keeps those values out.

## Support bundle

Settings calls the download "Download a support bundle". The hint under the button is "Logs, versions, and settings. Passwords are left out." The same zip is at `http://<your-server>:8477/api/v1/support`.

The zip holds `versions.json`, `doctor.json`, `config.json`, and `logs.txt`. Passwords, tuner credentials, and `DeviceAuth` are removed before the file is saved.

## Secure name on the home network

The server gets its own HTTPS name, such as `lan.abcdefghij.direct.wolfeup.com`, with a Let's Encrypt certificate, and serves the same API and media on it with HTTP/2. Nothing is installed on the phone or the router. The apps use it when it answers and stay on the plain address when it does not. Over HTTP/2 live playlists carry a preload hint, so Apple players run low-latency HLS: on Apple TV about three seconds closer to live, and a channel starts sooner (`BROADWAVE_LL_HINTS=0` leaves the hint out).

- **The name.** On first start the server makes an Ed25519 key, `direct/server.key` in the config folder (mode 0600, outside the catalog and its backups), and registers it with the direct service at `api.direct.wolfeup.com`. The service returns a random ten-letter label. Every later call is signed with that key over the method, path, time, and body, and must be within five minutes of the service's clock, so only this server can change its name.
- **Addresses.** The server reports the home network address it routes from (and that interface's `fd00::/8` addresses). The service publishes only private ranges: `10/8`, `172.16/12`, `192.168/16`, `100.64/10`, and `fd00::/8`. A name under the service can never point at a public site. The record is DNS only, with a five-minute TTL. A new address is reported within five minutes, and the same one once a day.
- **The certificate.** The server asks Let's Encrypt for `lan.<label>.direct.wolfeup.com` over the ACME DNS-01 challenge. The service sets the `_acme-challenge` TXT record for the server; no port is opened. The certificate and its key are `direct/cert.pem` and `direct/cert.key` (0600). It is replaced 30 days before it expires. `BROADWAVE_DIRECT_ACME=staging` uses Let's Encrypt's staging directory, whose certificates no device trusts, for testing.
- **The port.** HTTPS listens on 8443 beside the default 8477, or the HTTP port plus one for any other port (staging on 8490 uses 8491). 8478 is the tuner emulator and 8479 the discovery probe. `BROADWAVE_SECURE_ADDR` picks another. When the port is taken the secure name stays off. Host networking needs no extra mapping.
- **What the name is.** Requests by this server's own secure name are home requests, like its address: the remote rules do not apply. Another server's name under the same domain is refused. `GET /api/v1/server` offers `secureUrl` only while a trusted, unexpired certificate for the current name is loaded. `GET /api/v1/direct` shows the state, `PUT /api/v1/direct` with `{"enabled": false}` turns it off (an admin call while device sign-in is on; there is no Settings switch yet), and `BROADWAVE_DIRECT=0` keeps it off.
- **Routers that refuse it.** Some routers drop a public name that answers with a private address (DNS rebinding protection). The apps then fail a three-second check against the name and stay on the plain address, and try again after ten minutes or when the network changes.
- **Device tokens.** A token belongs to the origin it was issued at. An app copies its token to the secure name only when all of these hold: the secure URL came from `GET /server` at the address the token was issued at, that answer named the server id the token is bound to, the URL has the direct service's shape (`https://lan.<label>.direct.wolfeup.com[:port]`, no path), and `GET /server` over TLS at the secure name answers with the same id and discovery key. Anyone able to rewrite the plain HTTP answer could already read the token sent over it, so this gives such a party nothing new. The web app stays on its plain address; it keeps its token in the page's own storage, and moving it to another origin is not done.
- **Rate limits.** The service allows five registrations an hour from one address (it keeps a hash of the address, not the address), 500 a day overall, 20 address changes an hour and ten challenge records a day per name, and 40 first certificates a week across all servers, under Let's Encrypt's 50 a week for the domain. Renewals do not count.

## What the server contacts

The server talks to your tuner, to guide providers you turn on, to scoreboards while live scores are on, and to any playlist you add. Once a day it can ask GitHub whether a newer release exists. That request carries nothing about what you watch. Settings, Check for updates, turns it off. Settings, Live scores, stops the scoreboard requests. Look harder asks SiliconDust's device list when nothing on your network answers. An artwork or scores key you add sends requests to that service. The secure name, above, calls the direct service at `api.direct.wolfeup.com`, which Wolfe Up runs, and Let's Encrypt. Those calls carry the server's public key, its home network addresses, and the challenge value; nothing about what you watch. `BROADWAVE_DIRECT=0` stops them.

Apart from the direct service, none of those calls go to the maker of Broadwave. The privacy policy is [PRIVACY.md](https://github.com/wolfebase/broadwave/blob/main/PRIVACY.md).

## Limits to know

While device sign-in is off, anyone on the network can use the server. Turning it on does not close the playlist or port 8478. Away from home, below, is the list.

ffmpeg looks up a stream's name again after Broadwave has checked it, and a later segment or key can name another address. A name can change where it points between the two lookups.

On plain http, a page on another site can still open an export or a tuner URL in a way that sends no site header.

The encoder's command line still shows the stream address it was given. Its log output does not.

Recordings stay the original broadcast. Transcoding is only for playback.

## Away from home

A Cloudflare tunnel publishes the server at an https address with no open port. Every request that arrives through Cloudflare (it carries `Cf-Connecting-Ip` or `Cf-Ray`) or by the saved public name needs a paired device, `/export` included, whatever device sign-in is set to. The name check lets a request with Cloudflare's headers through only from loopback (the built-in cloudflared), and never on port 8478; a cloudflared elsewhere needs the public address saved. Responses to outside requests are `Cache-Control: private`, so a cache rule on the zone cannot hand them to someone else, and the sign-in cookie is `Secure` when the browser came over https. Wrong codes from outside count against their own server-wide cap, so they cannot stop pairing at home. The pairing rate limits count Cloudflare's client address when the peer is on this machine or the home network, and the peer's own address otherwise. An invite is twelve characters from 32 (60 bits), lasts 7 days, pairs one device, and is stored as a hash. The tunnel token is a secret: it lives in `<config>/secrets/cloudflare.json` (0600), never in the catalog or a backup; Settings shows only whether one is saved, and the support bundle leaves it out. Sign in with Cloudflare is admin and home-network only; the sign-in certificate, which can change every tunnel in the account, stays in memory and is dropped once the tunnel is made. The handbook page [Away from home](guide/remote.md) has the steps.

Or use a VPN that only your devices can join, and open the server's address on that VPN. Tailscale is one. Do not forward port 8477 or 8478 from the public internet, and do not turn on Tailscale Funnel. The handbook page [Away from home](guide/remote.md) is the setup: the Tailscale address, a `ts.net` name, and device sign-in.

An address in the Host header is allowed with no `BROADWAVE_HOSTS`, including a Tailscale address in `100.64.0.0/10`. A MagicDNS name on port 8477 is a public name and is refused until `BROADWAVE_HOSTS` lists it. The port 443 exception needs a non-empty `Sec-Fetch-Site`. The Apple apps do not send that header, and their address field defaults to `http` and port 8477.

Device sign-in is off until Settings, Devices, Require a device to sign in. While it is off, anyone who can open port 8477 can use the API. While it is on, the guide, recordings, and settings need a paired device. These stay open with no code either way: `/export/lineup.m3u`, `/export/guide.xml`, `/export/stream/`, and `/export/mosaic/`. A stream opened in an app, or directly in a browser, starts a tune. A page on another site is refused: "This stream is for apps on your network, not a page on another site." Port 8478 does too, while Act as an HDHomeRun is on. It checks the name, and a page on another site gets that same refusal. Bonjour can advertise a Tailscale address. The UDP probe on 8479 answers a private home address, not a Tailscale address, and not a tunnel.

The sign-in cookie is not marked Secure, so a browser will store it on plain HTTP. Tailscale encrypts the path between your phone and the server. The cookie does not hide the server from anyone else on the tailnet.

The apps do not install tuner firmware. Diagnostics says "This app does not install firmware."
