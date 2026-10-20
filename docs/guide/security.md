# Security

Broadwave has no account password. With device sign-in off, which is the default, anything that can reach the server can change channels, recordings, and settings. Keep port 8477 on your own network. [Away from home](remote.md) is how to reach it through Tailscale, and what stays open after you turn sign-in on.

## Public names

The server refuses a public name it was not given. Set `BROADWAVE_HOSTS` when you open it by a name such as `tv.example.com`. A leading dot covers the subdomains. Addresses, one-word names, and home names (`.local`, `.lan`, and a few others) work without that setting. A browser that sends `Sec-Fetch-Site` is allowed on port 443, or when the name has no port. The check does not look at HTTPS. The Apple apps do not send that header, so a public name on port 8477 needs `BROADWAVE_HOSTS`.

This stops a web page from calling your server by pointing the page's own name at your server's address.

## Locked out

If device sign-in is on and every admin device is gone, start the server once with `BROADWAVE_RESET_SIGNIN=1` (Docker and Unraid) or `-reset-signin`. It turns sign-in off and says so in the log. Then pair an admin device and remove the setting. [Away from home](remote.md#locked-out) has the steps.

## Secrets

The tuner's `DeviceAuth` value is not stored and is not written to the log. Passwords for a guide account or a playlist are stored so the server can refresh them, and Settings does not show a saved password. The Schedules Direct password and the movie artwork key stay blank and say Saved, on the web and in the Apple apps. The TheSportsDB key field says Saved on the web and stays blank in the Apple apps.

The Cloudflare tunnel token is kept in the config folder under `secrets/cloudflare.json`, readable only by the server, never in the catalog, so a backup never holds it. Sign in with Cloudflare keeps the sign-in itself in memory only while you set up, then drops it.

A support bundle leaves passwords, tuner credentials, and the tunnel token out. Settings calls the button "Download a support bundle". The hint under it is "Logs, versions, and settings. Passwords are left out."

## Away from home

Use Sign in with Cloudflare, Tailscale or another VPN that only your devices can join, or a reverse proxy with a certificate. Sign in with Cloudflare only starts from the home network, on an admin device. A request through the tunnel, by a public name, or that a proxy says came from an address outside the home network always needs a paired device, whatever device sign-in is set to. Do not forward port 8477 or 8478, and do not turn on Tailscale Funnel. [Away from home](remote.md) covers the address, MagicDNS names, device sign-in, and the playlist and port 8478, which stay open with no code.

The apps do not install tuner firmware.
