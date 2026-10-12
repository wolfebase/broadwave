# Security

Broadwave has no account password. With device sign-in off, which is the default, anything that can reach the server can change channels, recordings, and settings. Keep port 8477 on your own network. [Away from home](remote.md) is how to reach it through Tailscale, and what stays open after you turn sign-in on.

## Public names

The server refuses a public name it was not given. Set `BROADWAVE_HOSTS` when you open it by a name such as `tv.example.com`. A leading dot covers the subdomains. Addresses, one-word names, and home names (`.local`, `.lan`, and a few others) work without that setting. A browser that reaches the server by HTTPS on port 443 is allowed. Any other public name or port needs `BROADWAVE_HOSTS`.

This stops a web page from calling your server by pointing the page's own name at your server's address.

## Secrets

The tuner's `DeviceAuth` value is not stored and is not written to the log. Passwords for a guide account or a playlist are stored so the server can refresh them, and Settings does not show a saved password. A guide password or key field stays blank and says Saved.

A support bundle leaves passwords and tuner credentials out. Settings calls the button "Download a support bundle". The hint under it is "Logs, versions, and settings. Passwords are left out."

## Away from home

Use Tailscale, or another VPN that only your devices can join, and open the server's address on that VPN. Do not forward port 8477 or 8478, and do not turn on Tailscale Funnel. [Away from home](remote.md) covers the address, MagicDNS names, device sign-in, and the playlist and port 8478, which stay open with no code.

The apps do not install tuner firmware.
