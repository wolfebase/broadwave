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

## What is stored

Channels, the guide, recordings, and watch history stay in the database on the machine that runs Broadwave.

The tuner's `DeviceAuth` value is read when a guide request needs it and is not written to the database or the log. A Schedules Direct password, a playlist password, and a guide or artwork key are stored so the server can refresh those sources. Settings shows a password masked. The log keeps those values out.

## Support bundle

Settings calls the download "Download a support bundle". The hint under the button is "Logs, versions, and settings. Passwords are left out." The same zip is at `http://<your-server>:8477/api/v1/support`.

The zip holds `versions.json`, `doctor.json`, `config.json`, and `logs.txt`. Passwords, tuner credentials, and `DeviceAuth` are removed before the file is saved.

## What the server contacts

The server talks to your tuner, to guide providers you turn on, to scoreboards while live scores are on, and to any playlist you add. Once a day it can ask GitHub whether a newer release exists. That request carries nothing about what you watch. Settings, Check for updates, turns it off. Settings, Live scores, stops the scoreboard requests.

None of those calls go to the maker of Broadwave. The privacy policy is [PRIVACY.md](https://github.com/wolfebase/broadwave/blob/main/PRIVACY.md).

## Away from home

Use a VPN that puts your phone on the home network, and open the server's address on that VPN. Do not forward port 8477 from the public internet.

The apps do not install tuner firmware. Diagnostics says "This app does not install firmware."
