# Away from home

Broadwave has no account password. The safe way to watch from somewhere else is a VPN that only your own devices can join. [Tailscale](https://tailscale.com/) is one. Do not forward port 8477 or 8478 on your router, and do not turn on Tailscale Funnel. Funnel publishes the server on the public internet.

The tuner stays home. Your phone talks to Broadwave, and Broadwave talks to the tuner. One tune of a channel still feeds every screen and every recording.

## What you open

The server listens on port 8477 on every network interface, unless you started it on another address. That port is the web app, the API, and the shared playlist.

Two more listeners stay on the home network and do not help a phone that is only on Tailscale:

- The apps find a server by Bonjour, as `_broadwave._tcp`, and by a UDP probe on port 8479. The probe answers a private home address. It does not answer a Tailscale address, and it does not answer on a tunnel. Type the address.
- Port 8478 is the HDHomeRun address for Plex, Jellyfin, Emby, and Channels. It listens only after Settings, Share with other apps, Act as an HDHomeRun is on, and the change applies within a minute. It does not answer tuner discovery, so those apps need the address with `:8478`. It never asks for a device code.

On Docker Desktop, the published-port install maps only 8477. Port 8478 is reachable when the container uses host networking, which is the Linux and Unraid install, and only while Act as an HDHomeRun is on.

## What someone on that network can do

Device sign-in is off until you turn it on. Settings, Devices, Require a device to sign in. While it is off, anyone who can open port 8477 can watch, record, change settings, delete recordings, and download the support bundle. The bundle leaves passwords out. It still has the logs and the settings.

The tailnet is everyone you invited to it. A tailnet you share is a shared server.

Turn sign-in on when other people can reach the server and you watch in a browser. The hint under the switch, once it is on, is "A new device needs a code. This browser stays signed in. Health checks, playlists, and the HDHomeRun port stay open. The guide needs a signed-in device."

A code is six digits and lasts 10 minutes. In Devices, choose Watch, Record, or Admin, then Pair a device and read the code out, or open Pair this browser on the new browser and approve the code it shows. Watch plays and reads the guide. Record also records and edits passes. Admin also changes settings. Revoke stops one device. The last admin cannot be revoked while sign-in is on. The message is "Pair another admin device first. This is the last one that can change settings."

The iPhone, iPad, and Apple TV apps have no place to enter a code. Leave sign-in off if you use them. With it on they stop at "Sign in from a paired device."

These stay open with no code, even after sign-in is on:

| Path | What it does |
| --- | --- |
| The site itself | Loads the web app. The guide inside it asks for a code. |
| `/export/lineup.m3u` | The channel list other apps play. |
| `/export/guide.xml` | The listings the server has, with no code. |
| `/export/stream/…` and `/export/mosaic/…` | Copies the broadcast. Opening one starts a tune. |
| Port 8478 | The same kind of stream, while Act as an HDHomeRun is on. |

Health checks and the server's name and version are open too. Settings, recordings, and the support bundle need an admin device while sign-in is on. There is no switch that closes the playlist or port 8478.

## Tailscale

Install Tailscale on the computer that runs Broadwave, and on the phone, tablet, or laptop you watch from. On Unraid, or on Docker with host networking, install it on the host. The Broadwave container already uses the host's ports. Do not run a second copy inside the container.

Join both machines to the same tailnet. Then copy the server's Tailscale address. It is the address Tailscale shows for that machine.

1. In a browser, open `http://` that address, then `:8477`. An address needs no extra setting. `100.64.0.0/10` addresses and Tailscale's IPv6 addresses are addresses, so the name check lets them through.
2. On iPhone or iPad, the connect screen says "Or enter an address". On Apple TV, choose Enter an address. Type the Tailscale address and `:8477`, then Connect. If you leave the scheme and the port off, the app uses `http` and port 8477.

The apps will not list the server on their own. Bonjour and the UDP probe stay on the home network.

Leave Act as an HDHomeRun off unless a media server in the house needs it. Leave device sign-in off while you use the Apple apps. On the web, turn it on when the tailnet is not only you, and remember that a playlist or a stream URL still plays with no code.

### A name instead of the address

A MagicDNS name ends in `ts.net`. That is a public name. `http://<the-name>:8477` is refused until you set `BROADWAVE_HOSTS` to that name. The page names it:

```text
This server does not answer to <the-name>. If you reach it by that name, set BROADWAVE_HOSTS=<the-name> on the server.
```

Set the one name you open. A leading dot covers the subdomains, so `.ts.net` would allow any Tailscale name that points at this server. Prefer the single name. Addresses, one-word names, `.local`, `.lan`, `.home`, `.internal`, `.home.arpa`, `.fritz.box`, and `.attlocal.net` work without the setting. [Security](security.md) is the same rule.

The Apple apps do not send the header a browser sends on HTTPS, so a MagicDNS name does not get the port 443 exception. Use the Tailscale address for the apps. For a name, set `BROADWAVE_HOSTS` and open it on port 8477.

### Tailscale Serve

Serve publishes the server to your tailnet with HTTPS. It does not publish it to the rest of the internet, and it does not add a password. Tailscale already encrypts the path between your phone and the server. Serve adds a certificate the browser can show.

Whether the name works depends on the Host header Serve sends, which Broadwave does not control:

- An address, including `127.0.0.1`, is allowed.
- The MagicDNS name on port 443, or with no port, is allowed when the browser's `Sec-Fetch-Site` header is still on the request.
- The same name on port 8477 is refused until `BROADWAVE_HOSTS` lists it.

If the page says the server does not answer to a name, set `BROADWAVE_HOSTS` to the name in that message. The Apple apps should keep using the Tailscale address and port 8477.

Do not enable Funnel.

## A home address over a subnet route

A Tailscale subnet router can let the phone reach the server's home address. Multicast still stops at the home network, so the apps still do not find the server. Type the address. The Tailscale address works without a subnet route.

## Keep it small

- Reach the server through Tailscale. Do not forward 8477 or 8478, and do not put Broadwave on a proxy the public internet can open.
- Use the Tailscale address. Add a public name to `BROADWAVE_HOSTS` only when you actually open that name.
- Leave Act as an HDHomeRun off when nothing in the house needs port 8478. That port has no device code.
- The playlist and `/export/stream/` are open on port 8477 whenever the server is. A stream starts a tune and uses a tuner.
- Device sign-in covers the guide, recordings, and settings in Broadwave. It does not cover the playlist, the stream URLs, or port 8478.
- Leave device sign-in off while you use the iPhone, iPad, or Apple TV app.
- Invite only the people who should be able to watch. Everyone on the tailnet can open the server.
