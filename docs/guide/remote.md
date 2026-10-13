# Away from home

There are two ways to watch from somewhere else.

- **A Cloudflare tunnel.** Broadwave gets a public `https` address, such as `tv.example.com`. You invite family, and only the phones, TVs, and browsers you invited can open it. Nothing on your router changes. This is the way to share with people outside the house.
- **A VPN only your devices join**, such as [Tailscale](https://tailscale.com/). See [Tailscale](#tailscale) below.

Either way, the tuner stays home. Your phone talks to Broadwave, and Broadwave talks to the tuner. One tune of a channel still feeds every screen and every recording. Do not forward port 8477 or 8478 on your router.

## Cloudflare tunnel

You need a free Cloudflare account and a domain whose DNS is on Cloudflare. A tunnel makes an outbound connection from your server to Cloudflare, so no port is opened at home.

Everything that comes in through the tunnel needs a paired device. That includes the guide, the video, recordings, the playlist (`/export/lineup.m3u`), and the stream URLs. The home network does not change. Phones and TVs in the house keep working with no code.

### 1. Make the tunnel

1. Sign in to the Cloudflare dashboard and open **Zero Trust**. The first time, it asks you to pick a team name and the free plan.
2. Go to **Networks**, then **Tunnels**, then **Create a tunnel**.
3. Choose **Cloudflared**, name it `broadwave`, and save it.
4. Cloudflare shows install commands. Copy any one of them. They all contain the same token, a long string that starts with `eyJ`. Keep it private. Anyone with it can run your tunnel.
5. Go on to **Public hostnames** (on newer dashboards, **Published application routes**). Add one:
   - **Subdomain:** `tv`, or any name you like.
   - **Domain:** your domain.
   - **Service type:** `HTTP`.
   - **URL:** `localhost:8477`.
6. Save. Cloudflare adds the DNS record for you.

### 2. Give Broadwave the token

The Docker image includes cloudflared, so Broadwave runs the tunnel itself.

1. Open Broadwave on the home network. Go to **Settings**, then **Away from home**.
2. Paste what you copied into **Tunnel token** and choose **Save**. The whole install command is fine. Broadwave keeps only the token.
3. Within a few seconds it says **Connected to Cloudflare** and shows the address, such as `https://tv.example.com`. Broadwave reads the address from the tunnel. If it does not appear, type it into **Public address**.

The token is never shown again, and it is left out of the support bundle. **Remove token** stops the tunnel.

### If you already run cloudflared

Leave **Tunnel token** empty. Point your tunnel's public hostname at `http://<the server's address>:8477`, then type the hostname into **Public address**, such as `tv.example.com`. Broadwave then answers to that name and asks for a paired device on every request through it. Requests carrying Cloudflare's `Cf-Connecting-Ip` header count as outside, whatever name they use.

### 3. Invite family

1. On the home network, open **Settings**, **Away from home**, **Invite someone**.
2. Type who it is for, choose what they may do, and choose **Make an invite**.
   - **Watch** plays live TV and recordings.
   - **Record** also records and edits passes.
   - **Admin** also changes settings. Give it only to someone who runs the server with you.
3. Send them the code, such as `K7QM-4XPA-9RTW`, and the address. An invite pairs one device and lasts 7 days. Make one per device.

On their side:

- **Apple TV, iPhone, or iPad.** Install Broadwave. On the first screen, choose **Join a home server**. Enter the address and the code. On iPhone and iPad, the app link from Settings does the same in one tap.
- **A browser.** Open the browser link from Settings, name the browser, and choose **Pair**.

Devices you invited are listed under **Settings**, **Devices**. **Revoke** stops one at once. **Remove** on a waiting invite stops it before anyone uses it.

### Good to know

- A live channel away from home uses one of your tuners, like a TV in the house. Recordings come first.
- Video goes through Cloudflare. Cloudflare's terms limit serving mostly video through its network on some plans. Read the current terms for your plan before you rely on it.
- Device sign-in at home (**Settings**, **Devices**, **Require a device to sign in**) is separate. Leave it off if everyone in the house should keep watching without a code.
- The HDHomeRun port, 8478, is never on the tunnel.

## What you open

The server listens on port 8477 on every network interface, unless you started it on another address. That port is the web app, the API, and the shared playlist.

The apps find a server on the home network. Do not count on them finding it over Tailscale. Type the address.

- Bonjour, `_broadwave._tcp`, can advertise a Tailscale address. The UDP probe on port 8479 answers a private home address. It does not answer a Tailscale address, and it does not answer on a tunnel.
- Port 8478 is the HDHomeRun address for Plex, Jellyfin, Emby, and Channels. It listens on every interface once Settings, Share with other apps, Act as an HDHomeRun is on. The change applies within about 20 seconds. A Tailscale address can open it. It does not answer tuner discovery, so those apps need the address with `:8478`. It never asks for a device code.

On Docker Desktop, the published-port install maps only 8477. Port 8478 is reachable when the container uses host networking, which is the Linux and Unraid install, and only while Act as an HDHomeRun is on.

## What someone on that network can do

Device sign-in is off until you turn it on. Settings, Devices, Require a device to sign in. While it is off, anyone who can open port 8477 can watch, record, change settings, delete recordings, and download the support bundle. The bundle leaves passwords out. It still has the logs and the settings.

The tailnet is everyone you invited to it. A tailnet you share is a shared server.

Turn sign-in on when other people can reach the server and you watch in a browser. The hint under the switch, once it is on, is "A new device needs a code. This browser stays signed in. Health checks, playlists, and the HDHomeRun port stay open. The guide needs a signed-in device."

A code is six digits and lasts 10 minutes. On a browser that is already signed in, Settings, Devices, choose Watch, Record, or Admin, then Pair a device. The hint is "Enter this code on the other device. It lasts 10 minutes." That page also has Approve a code. A browser that is not signed in says "Pair this device" and asks for the code from Settings. Show a code instead shows one there, and the hint is "Enter this code in Settings. It lasts 10 minutes." Pair this browser, on the signed-in page, opens that screen for the browser you are using. Watch plays and reads the guide. Record also records and edits passes. Admin also changes settings. Revoke stops one device. The last admin cannot be revoked while sign-in is on. The message is "Pair another admin device first. This is the last one that can change settings."

The iPhone, iPad, and Apple TV apps pair with a code too: **Join a home server** takes a code from Settings, Devices, Pair a device, or an invite.

These stay open with no code, even after sign-in is on:

| Path | What it does |
| --- | --- |
| The site itself | Loads the web app. The guide inside it asks for a code. |
| `/export/lineup.m3u` | The channel list other apps play. |
| `/export/guide.xml` | The listings the server has, with no code. |
| `/export/stream/…` and `/export/mosaic/…` | Copies the broadcast. Opening one in an app, or directly in a browser, starts a tune. A page on another site is refused: "This stream is for apps on your network, not a page on another site." |
| Port 8478 | The same kind of stream, while Act as an HDHomeRun is on. |

Health checks and the server's name and version are open too. Settings and the support bundle need an admin device while sign-in is on. Reading and playing a recording needs Watch. Adding or deleting one needs Record. Turning Act as an HDHomeRun off closes port 8478. Nothing closes the playlist.

## Tailscale

Install Tailscale on the computer that runs Broadwave, and on the phone, tablet, or laptop you watch from. On Unraid, or on Docker with host networking, install it on the host. The Broadwave container already uses the host's ports. Do not run a second copy inside the container.

Join both machines to the same tailnet. Then copy the server's Tailscale address. It is the address Tailscale shows for that machine.

1. In a browser, open `http://` that address, then `:8477`. An address needs no extra setting. `100.64.0.0/10` addresses and Tailscale's IPv6 addresses are addresses, so the name check lets them through.
2. On iPhone or iPad, the connect screen says "Or enter an address". On Apple TV, choose Enter an address. Type the Tailscale address and `:8477`, then Connect. If you leave the scheme and the port off, the app uses `http` and port 8477.

Do not count on the apps listing the server. The UDP probe stays on the home network. Bonjour can advertise a Tailscale address.

Leave Act as an HDHomeRun off unless a media server in the house needs it. On the web, turn it on when the tailnet is not only you, and remember that a playlist or a stream URL still plays with no code.

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

- Reach the server through a Cloudflare tunnel or Tailscale. Do not forward 8477 or 8478, and do not put Broadwave on any other proxy the public internet can open.
- Use the Tailscale address. Add a public name to `BROADWAVE_HOSTS` only when you actually open that name.
- Leave Act as an HDHomeRun off when nothing in the house needs port 8478. That port has no device code.
- The playlist and `/export/stream/` are open on port 8477 whenever the server is. A stream opened in an app, or directly in a browser, starts a tune and uses a tuner. A page on another site is refused.
- Device sign-in covers the guide, recordings, and settings in Broadwave. It does not cover the playlist, the stream URLs, or port 8478.
- Invite only the people who should be able to watch. Everyone on the tailnet can open the server.
