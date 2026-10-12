# Broadwave support thread (draft, not posted)

This is the text for an Unraid Community Apps support thread. It has not been posted. Do not submit the template until this thread exists and the template's Support address points at it. Until then, Support stays on the GitHub issues page.

Suggested title: Broadwave — live TV and DVR for an antenna

## Post

Broadwave is live TV and DVR for an antenna. You run the server on Unraid. It talks to an HDHomeRun on your network, fills the guide, records the original broadcast, and plays it in a browser on port 8477 and in the Broadwave apps for iPhone, iPad, and Apple TV.

One tune of a channel feeds every screen and every recording. The tuner stays on the home network. Only this server talks to it.

The template is not in Community Apps yet. You can add it from the repository file `deploy/unraid/broadwave.xml`. An optional second template, `deploy/unraid/broadwave-updater.xml`, checks nightly at 03:30 and updates only the Broadwave container, and it skips a night while someone is watching or a recording is due.

Project and issues: https://github.com/wolfebase/broadwave

Install notes: https://github.com/wolfebase/broadwave/blob/main/docs/guide/install.md

## What the template expects

- An HDHomeRun on the same network. Host networking is on so the server can hear it, and so phones and Apple TVs can find the server. Do not switch the container to bridge.
- Config is the database and the guide. Put it on a fast pool. The default is `/mnt/user/appdata/broadwave`.
- Recordings are a second folder, mounted inside the container at `/config/work/recordings`. They are the original broadcast files. Leave that mount where it is. A different recordings path would miss the folder you mapped.
- PUID 99 and PGID 100 are Unraid's nobody user and users group. The server starts as root, gives those folders to that user, and then drops root. Set both, or neither.
- TZ is your time zone, such as `America/Chicago`. The default in the template is UTC.
- Extra Parameters includes `--device=/dev/dri` for Intel or AMD graphics. If this server has no such device, delete that flag before you start the container. Docker will not start while the path is missing. The picture still plays from the processor.

Tuner address, public names, the encoder choice, and extra audio are under Show more settings. Leave the tuner address blank unless the tuner is on another subnet.

## First run

Open the WebUI, which is `http://<your-server>:8477`, and follow setup. It looks for the tuner on its own. When none appears, add it by address. Then open the Broadwave app on the same network. It looks for the server on its own. You can also type the address.

## Other apps

Settings, Share with other apps, can make this server look like an HDHomeRun on port 8478. Plex, Jellyfin, Emby, and Channels add that address with the port. It does not answer tuner discovery, so a tuner you already have is left as it is. Leave this off when you do not use those apps. The port has no device code.

## Away from home

Do not forward port 8477 or 8478 on your router. Use Tailscale, installed on the Unraid host rather than inside this container, and type the server's Tailscale address and port 8477. The apps will not find the server over Tailscale by themselves. There is no account password. Device sign-in covers the guide and settings in the browser, and the shared playlist can still start a tune. The Apple apps cannot sign in, so leave device sign-in off if you use them.

The handbook page is https://github.com/wolfebase/broadwave/blob/main/docs/guide/remote.md

## Updates

Install the Broadwave container first, then the updater if you want nightly updates. The updater reads labels on the Broadwave container. Keep "Only labeled containers" and "Busy check" turned on so it leaves every other container alone and skips a night when Broadwave is busy. To update by hand, pull the new image and recreate the container with the same folders.

## Where to look when something fails

- The container will not start, and the log names `/dev/dri`. Remove `--device=/dev/dri` from Extra Parameters.
- Setup says no tuner answered. Confirm host networking, and that the tuner is on this network. If it is on another subnet, set Tuner address.
- Phones do not list the server. Host networking has to be on, and the phone needs Local Network access for Broadwave. Or type the server's address.
- Diagnostics says recordings are owned by root. Set PUID and PGID.
- Diagnostics says less than 20 GB is free. Free space before a long recording. A recording that will not start because of the reserve names the free space and the reserve. The reserve is Keep this much free, in Settings. The default is 10 GB.

A support bundle from Settings leaves passwords out. Attach it to a GitHub issue. The same file is `http://<your-server>:8477/api/v1/support`.

## What this server does not do

It does not supply channels. The picture comes from your antenna. It does not install tuner firmware. It has no Roku app. It is free, with no subscription and no account.
