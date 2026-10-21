# Install

You need an always-on computer that runs Docker, and a network TV tuner. The image runs on x86-64 and ARM64. It includes ffmpeg, so nothing else has to be installed for the picture.

The tag `latest` is the newest release. It moves when a version is published. A full version tag from the [releases](https://github.com/wolfebase/broadwave/releases) page stays on that build.

## Docker on Linux

`--network host` lets the server hear your tuner, and lets phones and TVs find Broadwave.

```bash
docker run -d --name broadwave --network host --restart unless-stopped \
  -e TZ=<your time zone> \
  -v ~/broadwave:/config \
  -v ~/broadwave/recordings:/config/work/recordings \
  ghcr.io/wolfebase/broadwave:latest
```

Then open `http://<your-server>:8477`.

- `TZ` is your time zone: replace `<your time zone>` with a name like `America/New_York`, `America/Chicago`, `America/Denver`, `America/Phoenix`, `America/Los_Angeles`, `America/Anchorage`, `Pacific/Honolulu`, `America/Toronto`, or `America/Vancouver`. Without it, a container runs on UTC, and Diagnostics says "The time zone is not set. Set TZ so the guide uses your local time." A mistyped name (`America/Denvr`) also runs on UTC; the log, Diagnostics, and the web app (for an admin) name it. The guide on a phone or in a browser uses that device's clock. Pass times and the server's own clock messages use `TZ`.
- The two folders keep the database and the recordings. An update replaces the container and leaves the folders.
- Add `--device /dev/dri` when that path exists (Intel or AMD graphics). Docker refuses to start if the path is missing. The picture still plays from the processor without it.
- When the tuner does not appear, set `HDHR_HOST` to its address. Add `:port` only when the port is not 80.

`docker ps` shows `healthy` once the server answers. The image runs that check every 30 seconds.

## Docker Desktop on Mac or Windows

The container's host network is a virtual machine, so discovery cannot see the tuner. Publish the port and set `HDHR_HOST`.

```bash
docker run -d --name broadwave --restart unless-stopped \
  -e TZ=<your time zone> \
  -e HDHR_HOST=<tuner address> \
  -p 8477:8477 \
  -v ~/broadwave:/config \
  -v ~/broadwave/recordings:/config/work/recordings \
  ghcr.io/wolfebase/broadwave:latest
```

Then open `http://localhost:8477`.

For the secure name, also publish `-p 8443:8443` and set `-e BROADWAVE_DIRECT_ADDRS=<this computer's address>`. Without them the apps use the plain address.

## Docker Compose

```bash
curl -fsSLO https://raw.githubusercontent.com/wolfebase/broadwave/main/deploy/docker/compose.yaml
docker compose pull && docker compose up -d
```

Put your time zone in a `.env` file next to `compose.yaml` first (such as `TZ=America/Denver`); Compose stops and says so when `TZ` is missing. That file uses host networking, which is right on Linux. On Docker Desktop, use the published-port command above. Graphics are off in it, so it starts on every machine: when `ls /dev/dri` lists `renderD128` (Intel or AMD graphics), remove the `#` in front of the two `devices` lines for hardware transcoding. `PUID` and `PGID` default to 1000, the first user on most Linux hosts; set them in `.env` to another user that exists there (99 and 100 on Unraid).

## Unraid

Add the template [`deploy/unraid/broadwave.xml`](https://github.com/wolfebase/broadwave/blob/main/deploy/unraid/broadwave.xml). It uses host networking and port 8477.

- Leave PUID at 99 and PGID at 100 so recordings are not owned by root. A plain `docker run` on Unraid needs `-e PUID=99 -e PGID=100` as well.
- Set Config and Recordings to folders on your server. Recordings are mounted at `/config/work/recordings`.
- Remove `/dev/dri` from Extra Parameters when the server has no Intel or AMD graphics. Docker will not start while that path is missing.
- Tuner address, public names, the encoder, and extra audio are under Show more settings. Leave the tuner address blank unless the tuner is on another subnet.

The optional updater is [`deploy/unraid/broadwave-updater.xml`](https://github.com/wolfebase/broadwave/blob/main/deploy/unraid/broadwave-updater.xml). It checks at 03:30 and updates only the Broadwave container. It skips a night while a tuner is in use, a recording is running, or a recording starts within two hours. To update by hand, pull the new image and recreate the container with the same folders.

## NAS boxes

Each of these runs the Compose file above. Make two folders first, one for the config and one for the recordings, on a volume that is always mounted. Find the numbers for `PUID` and `PGID` with `id <your user>` over SSH, and put them in `.env` with `TZ`. When a share that holds the recordings is not mounted, the server stops recording and says "The recordings folder isn't there. Is the drive mounted?" rather than filling the system disk.

### Synology

DSM 7.2: install **Container Manager**, then **Project › Create**, pick a folder such as `/volume1/docker/broadwave`, and paste the Compose file. Put your values in place of `${TZ...}`, `${PUID...}`, and `${PGID...}` in the environment lines, or add a `.env` file in the project folder. The first user made on a Synology is usually `1026`, and the `users` group is `100`: `PUID=1026`, `PGID=100`. Host networking works as written.

Intel "+" models with a Celeron or Pentium (DS920+, DS720+, DS1520+) have `/dev/dri`: uncomment the `devices` lines. ARM models and the AMD Ryzen models (DS923+, DS1522+) have none; check with `ls /dev/dri` over SSH when unsure.

### QNAP

Install **Container Station** (3.0 or newer), then **Applications › Create**, and paste the Compose file. Use folders under `/share/Container/broadwave`. QNAP users usually start at `500`, and the `everyone` group is `100`. Intel models have `/dev/dri`; ARM models do not.

### TrueNAS SCALE

24.10 (Electric Eel) or newer: **Apps › Discover Apps › Custom App › Install via YAML**, and paste the Compose file with `TZ`, `PUID`, and `PGID` filled in (TrueNAS does not read `.env`). Point the two volumes at datasets, such as `/mnt/tank/broadwave/config` and `/mnt/tank/broadwave/recordings`. The `apps` user is `568:568`; give it write access to both datasets, or use your own user's numbers. Uncomment `devices` when the machine has Intel or AMD graphics.

### Raspberry Pi and other ARM boards

Use a 64-bit OS (Raspberry Pi OS 64-bit, or Ubuntu Server for ARM64) and Docker Engine; the image is ARM64. Leave `devices` commented: the Pi 5 has no hardware H.264 encoder, and the image does not use the Pi 4's. Recordings save the broadcast as it is, with no encode, so they cost little. Watching is the limit: ATSC 1.0 channels are MPEG-2, which no player takes as it is, so every live picture is encoded on the processor. Expect one picture at a time, at 540p or 720p, and one multiview tile; Diagnostics shows what the board measured ([hardware](../hardware.md)). Keep the recordings on a USB SSD or a NAS share, not the SD card.

## Without Docker

Each [release](https://github.com/wolfebase/broadwave/releases) has the server as one file for Linux (x86-64 and ARM64), macOS (Apple silicon and Intel), and Windows (x86-64), with the web app built in, and a `SHA256SUMS` file to check the downloads against (`sha256sum -c SHA256SUMS --ignore-missing`).

```bash
tar xzf broadwave-<version>-linux-amd64.tar.gz
cd broadwave-<version>-linux-amd64
./broadwave -config ~/broadwave
```

Then open `http://<your-server>:8477`. `-config` is the folder for the database; recordings go in `work/recordings` inside it, or set `-recordings` (or `BROADWAVE_RECORDINGS`). `-addr :8477` changes the port. The settings in the table below work as environment variables here too; `PUID`, `PGID`, and `UMASK` are for containers and are ignored on Windows.

ffmpeg is not included. Install it so that `ffmpeg` and `ffprobe` are on the `PATH` before you start the server:

- **Version:** ffmpeg 6.1 or newer. The Docker image uses jellyfin-ffmpeg 7, and that is the one to pick when you can (Debian and Ubuntu packages are at repo.jellyfin.org).
- **Needed:** `libx264` (the picture when there is no graphics encoder), the built-in AAC encoder, and the MPEG-2, H.264, HEVC, AC-3, and E-AC-3 decoders. Distribution packages (`apt install ffmpeg`, Homebrew, the gyan.dev or BtbN builds on Windows) have all of these.
- **ATSC 3.0 sound** needs the AC-4 decoder, which only jellyfin-ffmpeg has. With another ffmpeg, ATSC 3.0 channels play without sound, and the server log says so.
- **Hardware encoding** uses what that ffmpeg has: VAAPI or QSV (Intel, AMD), NVENC (NVIDIA), or VideoToolbox (Mac). Settings › About shows the encoder in use.
- **Commercial detection** also uses [comskip](https://github.com/erikkaashoek/Comskip) when it is on the `PATH`; without it the server's own break finder runs.

To keep it running, start it from systemd, launchd, or a Windows service manager such as NSSM. To update, stop the server, replace the file, and start it again; the database is copied before a new version changes it.

## Other settings

| Setting | What it does |
| --- | --- |
| `PUID`, `PGID` | The user and group that own the config folder and recordings. The Compose file defaults to 1000 (99 and 100 on Unraid). Leave them out of a plain `docker run` to run as root. |
| `UMASK` | Permissions for new files. The Compose file and the Unraid template set `022`. |
| `BROADWAVE_RECORDINGS` | The recordings folder, when it is not `/config/work/recordings`. |
| `BROADWAVE_ENCODER` | `gpu` or `software`, to skip the automatic choice. |
| `BROADWAVE_HOSTS` | Public names the server answers to. See [Security](security.md). |
| `BROADWAVE_RESET_SIGNIN` | `1` turns device sign-in off when the server starts, for when every admin device is lost. Remove it afterwards. See [Away from home](remote.md#locked-out). |
| `BROADWAVE_DIRECT` | `0` turns off the secure name on the home network. See [Security](../security.md#secure-name-on-the-home-network). |
| `BROADWAVE_LL_HINTS` | `0` leaves the low-latency preload hint out of live playlists on the secure name, so Apple players play segments as they do over plain HTTP. |
| `BROADWAVE_SECURE_ADDR` | The HTTPS port for the secure name, such as `:9443`. By default 8443, or the HTTP port plus one when that is not 8477. |
| `BROADWAVE_DIRECT_ADDRS` | The home network addresses the secure name points at, comma-separated, when the server cannot tell (for example Docker with published ports). |
| `BROADWAVE_ALTERNATES` | `0` carries only the chosen sound track. By default a full-size encode also carries the channel's other languages. |

NVIDIA graphics need the NVIDIA Container Toolkit and `--gpus all`. That path has not been tested on real hardware yet.

An optional updater can move the image forward at night. The commands are in the README, under automatic updates. Without an updater, the web app says when a new release is out.

To build from source you need Go 1.26 or newer, Node 22 or newer, and ffmpeg. From a checkout, `make run` builds the web app and serves it on port 8477.
