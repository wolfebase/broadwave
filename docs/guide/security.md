# Security

Broadwave has no accounts. Anything that can reach the server on your network can change channels, recordings, and settings. Keep port 8477 on that network.

## Public names

The server refuses a public name it was not given. Set `BROADWAVE_HOSTS` when you open it by a name such as `tv.example.com`. A leading dot covers the subdomains. Addresses, one-word names, and home names (`.local`, `.lan`, and a few others) work without that setting. A browser behind an HTTPS reverse proxy is allowed.

This stops a web page from calling your server by pointing the page's own name at your server's address.

## Secrets

The tuner's `DeviceAuth` value is not stored and is not written to the log. Passwords for a guide account or a playlist are stored so the server can refresh them, and Settings shows them masked.

A support bundle leaves passwords and tuner credentials out. Settings calls the button "Download a support bundle." The hint under it is "Logs, versions, and settings. Passwords are left out."

## Away from home

Use a VPN that puts your phone on the home network. Do not forward port 8477 from the public internet.

The apps do not install tuner firmware.
