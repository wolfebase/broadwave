# Broadwave support thread (not posted yet)

The text for the Unraid forum support thread. Post it as written, in **Docker Containers** (https://forums.unraid.net/forum/47-docker-containers/), then follow `SUBMITTING.md`. Everything between the two lines below is the post.

Title: `[Support] Broadwave - live TV and DVR for an antenna`

---

**Broadwave** is live TV and DVR for an antenna. It runs on Unraid, talks to the HDHomeRun on your network, fills the guide, records the untouched broadcast, and plays it in a browser on port 8477 and in the Broadwave apps for iPhone, iPad, and Apple TV.

One tune of a channel feeds every screen and every recording, and every screen in the house plays the same frame. Only this server talks to the tuner.

- Project and issues: https://github.com/wolfebase/broadwave
- Install guide: https://github.com/wolfebase/broadwave/blob/main/docs/guide/install.md#unraid
- Handbook: https://wolfebase.github.io/broadwave/docs/
- Apple apps (public beta): https://testflight.apple.com/join/893B2HRj

**Install**

Search for Broadwave in the Apps tab. To add the template by hand instead, run this in the Unraid terminal, then pick Broadwave under Docker, Add Container, Template:

```
wget -O /boot/config/plugins/dockerMan/templates-user/my-Broadwave.xml https://raw.githubusercontent.com/wolfebase/broadwave/main/deploy/unraid/broadwave.xml
```

**What the template expects**

- An HDHomeRun or another network tuner on the same network. Host networking is on so the server can hear it, and so phones and Apple TVs can find the server. Keep it on host.
- **TZ** is required: your time zone, such as `America/New_York` or `America/Denver`. Without it, or with a mistyped name, the server runs on UTC and records at the wrong hour. Diagnostics says so.
- **Config** is the database and the guide. Put it on a fast pool. The default is `/mnt/user/appdata/broadwave`.
- **Recordings** is a second folder, mounted inside the container at `/config/work/recordings`. Recordings are the original broadcast files. Leave the container path as it is.
- **PUID 99 and PGID 100** are Unraid's nobody user and users group. The server starts as root, gives those folders to that user, and then drops root. Set both, or neither.
- **Intel or AMD graphics** passes `/dev/dri` for hardware transcoding. If this server has no such graphics, remove that entry before you start the container: Docker will not start while the path is missing. The picture still plays from the processor.

Tuner address, public names, the encoder choice, the secure name, and extra audio are under Show more settings. Leave the tuner address blank unless the tuner is on another subnet.

**First run**

Open the WebUI, `http://<your-server>:8477`, and follow setup. It looks for the tuner on its own; when none appears, add it by address. Then open the Broadwave app on the same network. It lists the servers it finds, or you type the address.

**Other apps**

Settings, Share with other apps, can make this server look like an HDHomeRun on port 8478. Plex, Jellyfin, Emby, and Channels add that address with the port. It does not answer tuner discovery, so type the address. Leave it off when you don't use those apps.

**Away from home**

Don't forward port 8477 or 8478 on your router. There is no account password: anything that can open port 8477 can use the server until you turn on device sign-in in Settings, Devices.

1. **Sign in with Cloudflare.** In Settings, Devices and away from home, choose Sign in with Cloudflare. You need a free Cloudflare account with a domain on it. Broadwave makes a tunnel and an address such as `tv.example.com`, and only phones, TVs, and browsers you pair or invite can open it. The iPhone, iPad, and Apple TV apps pair with a code.
2. **Tailscale**, when you have no domain. Install it on the Unraid host, not in this container, and type the server's Tailscale address with port 8477. The apps don't find the server over Tailscale by themselves.
3. **A reverse proxy you already run** (Nginx Proxy Manager, SWAG, Caddy, Traefik) works like the tunnel: every request through it needs a paired device.

Details: https://github.com/wolfebase/broadwave/blob/main/docs/guide/remote.md

**Updates**

Apply updates from the Docker tab like any other container. The database is copied before a new version changes it. For nightly updates that wait until Broadwave isn't busy, add the optional updater template by hand, as `my-Broadwave-Updater.xml`, from https://raw.githubusercontent.com/wolfebase/broadwave/main/deploy/unraid/broadwave-updater.xml. It checks at 03:30, updates only the Broadwave container, and skips a night while a tuner is in use, a recording is running, or a recording starts within two hours. Keep "Only labeled containers" and "Busy check" on.

**When something fails**

- The container won't start, and the log names `/dev/dri`. Remove the Intel or AMD graphics entry.
- Setup says no tuner answered. Check that the container is on host networking and the tuner is on this network. If it is on another subnet, set Tuner address.
- Phones don't list the server. Host networking has to be on, and the phone needs Local Network access for Broadwave. Or type the server's address.
- Diagnostics says recordings are owned by root. Set PUID and PGID.
- Diagnostics says less than 20 GB is free. Free space before a long recording. A recording that won't start because of the reserve names the free space and the reserve (Keep this much free, in Settings; 10 GB by default).

Please attach a support bundle to a GitHub issue: Settings, About, or `http://<your-server>:8477/api/v1/support`. It leaves passwords and keys out.

**What it doesn't do**

It doesn't supply channels: the picture comes from your antenna. It doesn't install tuner firmware. It has no Roku app. It is free, with no subscription and no account.

---
