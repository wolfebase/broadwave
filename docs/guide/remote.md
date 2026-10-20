# Away from home

There are two ways to watch from somewhere else.

- **Sign in with Cloudflare.** Broadwave gets a public `https` address, such as `tv.example.com`. You invite family, and only the phones, TVs, and browsers you invited can open it. Nothing on your router changes. This is the way to share with people outside the house.
- **A VPN only your devices join**, such as [Tailscale](https://tailscale.com/). See [Tailscale](#tailscale) below.
- **A reverse proxy you already run**, such as Nginx Proxy Manager, SWAG, Caddy, or Traefik. It works like the Cloudflare tunnel: everything that comes in through it needs a paired device. See [Your own reverse proxy](#your-own-reverse-proxy).

Either way, the tuner stays home. Your phone talks to Broadwave, and Broadwave talks to the tuner. One tune of a channel still feeds every screen and every recording. Do not forward port 8477 or 8478 on your router.

## Sign in with Cloudflare

You need a free Cloudflare account with a domain on it. If you do not have a domain on Cloudflare yet, add one first (Cloudflare sells them too), or use [Tailscale](#tailscale).

Broadwave makes a Cloudflare tunnel: an outbound connection from your server to Cloudflare, so no port is opened at home. Everything that comes in through the tunnel needs a paired device. That includes the guide, the video, recordings, the playlist (`/export/lineup.m3u`), and the stream URLs. The home network does not change. Phones and TVs in the house keep working with no code.

### 1. Turn it on

Do this on the home network, on a device that is an admin. It is refused from outside the house.

1. Open Broadwave. Go to **Settings**, then **Away from home**.
2. Choose **Sign in with Cloudflare**. A Cloudflare tab opens. Settings says **Waiting for Cloudflare…** with a countdown. The sign-in lasts about nine minutes.
3. In the Cloudflare tab, sign in, pick the domain to use, and choose **Authorize**. Then come back to Broadwave. The tab can be closed.
4. Settings says **Signed in · example.com**. Type a name for the address. `tv` makes `tv.example.com`.
5. Choose **Turn on**. Broadwave makes the tunnel, adds the name to your DNS, and connects. It takes a few seconds.
6. Settings says **Connected** and shows the address, such as `https://tv.example.com`. **Copy address** copies it. Now [invite family](#3-invite-family).

The tunnel is named `broadwave-` and the start of the server's id, so it is easy to find in the Cloudflare dashboard under **Zero Trust**, **Networks**, **Tunnels**.

What Broadwave keeps: only the tunnel's own token, which can run this one tunnel and nothing else. The sign-in itself can change every tunnel in the account, so Broadwave keeps it in memory only while you set up, and drops it when setup is done, when you choose **Cancel**, or about 15 minutes after you signed in. The token is in the config folder under `secrets/`, readable only by the server. It is not in the catalog, not in backups, and not in the support bundle.

If something goes wrong:

- **That Cloudflare account has no domain yet.** Add a domain to Cloudflare, then sign in again. Or use Tailscale.
- **The name is already in use.** Something else in your DNS has that name. Pick another name, or choose **Replace it** to point that name at Broadwave instead.
- **Sign-in timed out.** You have about nine minutes to authorize in the Cloudflare tab. Choose **Try again**.
- **Cloudflare did not let this sign-in make a tunnel.** Sign in with the account that owns the domain.
- **Set this up from the home network.** Signing in hands the server a key to your Cloudflare account, so it only starts at home.

### Turn it off

**Turn off** stops the tunnel here and forgets its token. The tunnel and the DNS name stay in Cloudflare, so turning it back on later is quick.

**Turn off and remove from Cloudflare** asks you to sign in once more, with the account that has the tunnel, then deletes the tunnel and its DNS name too. If Cloudflare does not let Broadwave delete the name, Settings says which name is left. Delete it in the dashboard under **DNS**.

### Restoring a backup

Backups never hold the tunnel token, so restoring one does not start a tunnel. A server that already runs one keeps running it. On a new server, sign in with Cloudflare again. When the catalog came from the same server, it finds the tunnel it made before by name and uses it again.

### Advanced: a token, or your own cloudflared

**Advanced**, at the bottom of **Away from home**, keeps the older ways.

**Paste a tunnel token.** Make a tunnel yourself in the Cloudflare dashboard (**Zero Trust**, **Networks**, **Tunnels**, **Create a tunnel**, **Cloudflared**). Give it a public hostname such as `tv.example.com` with service `HTTP` and URL `localhost:8477`. Copy any install command Cloudflare shows. They all contain the token, a long string that starts with `eyJ`. Paste it into **Tunnel token** and choose **Save**. The whole command is fine. Broadwave keeps only the token, in the same `secrets/` file. The token is never shown again. **Remove token** stops the tunnel.

**Your own cloudflared.** Leave **Tunnel token** empty. Point your tunnel's public hostname at `http://<the server's address>:8477`, then type the hostname into **Public address**, such as `tv.example.com`. Broadwave then answers to that name and asks for a paired device on every request through it. Requests carrying Cloudflare's `Cf-Connecting-Ip` header count as outside, whatever name they use.

A token saved by an older version moves out of the catalog into `secrets/` the first time the new version starts.

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

## Your own reverse proxy

A reverse proxy with its own certificate can publish Broadwave at a name such as `tv.example.com`. Point the name at `http://<the server's address>:8477` and turn on WebSockets for it (Nginx Proxy Manager calls it Websockets Support). Broadwave treats those requests like the Cloudflare tunnel: each one needs a paired device, whatever device sign-in is set to. That includes the guide, the video, recordings, settings, the support bundle, the playlist (`/export/lineup.m3u`), and the stream URLs. The home network does not change.

1. On the home network, open **Settings**, **Away from home**, **Advanced**, and type the name into **Public address**, such as `tv.example.com`. Broadwave then answers to that name from the apps too, and invite links use it.
2. [Invite family](#3-invite-family) the same way as with Cloudflare. A browser opens the invite link. The Apple apps choose **Join a home server** and type the address and the code.

A request counts as outside the house when any of these is true:

- The name it asks for is a public name, in `Host` or in `X-Forwarded-Host`. An address, a one-word name, a home name (`.local`, `.lan`, `.home.arpa`, and the others in [Security](security.md)), and the server's own secure name are not.
- An address in `Forwarded`, `X-Forwarded-For`, or `X-Real-IP` is not on the home network, or cannot be read. Home addresses are private ones (`10/8`, `172.16/12`, `192.168/16`, IPv6 ULA), loopback, link-local, Tailscale's `100.64.0.0/10`, and IPv6 addresses on the server's own networks.
- The connection comes straight from an address outside the home network, such as through a forwarded port.

These headers can only ask for more. A device in the house that sends them locks only itself out, and none of them opens anything that would otherwise need a code. Nginx Proxy Manager, SWAG, Caddy, and Traefik send the client's address. If your proxy rewrites the name to the server's address, it has to send `X-Forwarded-For` or `X-Real-IP`, or Broadwave cannot tell the request came from outside.

Opening the public name from inside the house asks for a paired device too. Use the server's address or its `.local` name at home to watch with no code.

Do not proxy port 8478. The HDHomeRun port never asks for a code.

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

A MagicDNS name ends in `ts.net`. That is a public name. `http://<the-name>:8477` is refused until you set `BROADWAVE_HOSTS` to that name, and once it works, every device that opens the name needs to be paired, like a request through [your own reverse proxy](#your-own-reverse-proxy). The Tailscale address needs no code. The page names it:

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

A request by the MagicDNS name is a public name, so a device needs to be paired. The Tailscale address needs no code.

If the page says the server does not answer to a name, set `BROADWAVE_HOSTS` to the name in that message. The Apple apps should keep using the Tailscale address and port 8477.

Do not enable Funnel.

## A home address over a subnet route

A Tailscale subnet router can let the phone reach the server's home address. Multicast still stops at the home network, so the apps still do not find the server. Type the address. The Tailscale address works without a subnet route.

## Keep it small

- Reach the server through a Cloudflare tunnel, Tailscale, or a reverse proxy with a certificate. Through the tunnel or a proxy, every request needs a paired device. Do not forward 8477 or 8478: a forwarded 8477 asks for a paired device too, but it is plain `http`, so the device's sign-in travels unencrypted.
- Use the Tailscale address. Add a public name to `BROADWAVE_HOSTS` only when you actually open that name.
- Leave Act as an HDHomeRun off when nothing in the house needs port 8478. That port has no device code.
- The playlist and `/export/stream/` are open on port 8477 whenever the server is. A stream opened in an app, or directly in a browser, starts a tune and uses a tuner. A page on another site is refused.
- Device sign-in covers the guide, recordings, and settings in Broadwave. It does not cover the playlist, the stream URLs, or port 8478.
- Invite only the people who should be able to watch. Everyone on the tailnet can open the server.

## Locked out

If **Require a device to sign in** is on and no admin device is left (a browser's data was cleared, a phone was reset), the server can be unlocked from the machine that runs it. Start it once with device sign-in turned off:

- **Docker, Compose, or Unraid.** Add the variable `BROADWAVE_RESET_SIGNIN=1` to the container and restart it. On Unraid, edit the container, choose **Add another Path, Port, Variable, Label or Device**, pick **Variable**, and set key `BROADWAVE_RESET_SIGNIN` to `1`.
- **Running the program yourself.** Add `-reset-signin`, such as `broadwave -config data -reset-signin`.

The log says `reset-signin: DEVICE SIGN-IN IS NOW OFF`. The home network is open again, the way it is before sign-in is ever turned on. Devices that were already paired keep working, and a request from outside the house still needs a paired device. Then:

1. On the home network, open **Settings**, **Devices**, and pair an admin device.
2. Turn **Require a device to sign in** back on, if you want it.
3. Remove `BROADWAVE_RESET_SIGNIN` or `-reset-signin`, and restart. While it is set, every start turns sign-in off again, and the log says so each time.
