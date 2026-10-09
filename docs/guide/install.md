# Install

You need an always-on computer that runs Docker, and a network TV tuner. The image runs on x86-64 and ARM64. It includes ffmpeg, so nothing else has to be installed for the picture.

The tag `latest` is the newest release. It moves when a version is published. A full version tag from the [releases](https://github.com/wolfebase/broadwave/releases) page stays on that build.

## Docker on Linux

`--network host` lets the server hear your tuner, and lets phones and TVs find Broadwave.

```bash
docker run -d --name broadwave --network host --restart unless-stopped \
  -e TZ=America/Chicago \
  -v ~/broadwave:/config \
  -v ~/broadwave/recordings:/config/work/recordings \
  ghcr.io/wolfebase/broadwave:latest
```

Then open `http://<your-server>:8477`.

- `TZ` is your time zone, so the guide lines up. Without it the server runs on UTC.
- The two folders keep the database and the recordings. An update replaces the container and leaves the folders.
- Add `--device /dev/dri` when that path exists (Intel or AMD graphics). Docker refuses to start if the path is missing. The picture still plays from the processor without it.
- When the tuner does not appear, set `HDHR_HOST` to its address. Add `:port` only when the port is not 80.

`docker ps` shows `healthy` once the server answers. The image runs that check every 30 seconds.

## Docker Desktop on Mac or Windows

The container's host network is a virtual machine, so discovery cannot see the tuner. Publish the port and set `HDHR_HOST`.

```bash
docker run -d --name broadwave --restart unless-stopped \
  -e TZ=America/Chicago \
  -e HDHR_HOST=<tuner address> \
  -p 8477:8477 \
  -v ~/broadwave:/config \
  -v ~/broadwave/recordings:/config/work/recordings \
  ghcr.io/wolfebase/broadwave:latest
```

Then open `http://localhost:8477`.

## Docker Compose

```bash
curl -fsSLO https://raw.githubusercontent.com/wolfebase/broadwave/main/deploy/docker/compose.yaml
docker compose pull && docker compose up -d
```

Set `TZ` in `compose.yaml` first. That file uses host networking, which is right on Linux. On Docker Desktop, use the published-port command above. Remove the `devices` lines when `/dev/dri` is missing. On Unraid, leave `PUID` at 99 and `PGID` at 100.

## Unraid

Add the template [`deploy/unraid/broadwave.xml`](https://github.com/wolfebase/broadwave/blob/main/deploy/unraid/broadwave.xml). It uses host networking and port 8477.

- Leave PUID at 99 and PGID at 100 so recordings are not owned by root. A plain `docker run` on Unraid needs `-e PUID=99 -e PGID=100` as well.
- Set Config and Recordings to folders on your server. Recordings are mounted at `/config/work/recordings`.
- Remove `/dev/dri` from Extra Parameters when the server has no Intel or AMD graphics.

To update by hand, pull the new image and recreate the container with the same folders.

## Other settings

| Setting | What it does |
| --- | --- |
| `PUID`, `PGID` | The user and group that own the config folder and recordings. Leave them out to run as root. |
| `UMASK` | Permissions for new files. Default `022`. |
| `BROADWAVE_RECORDINGS` | The recordings folder, when it is not `/config/work/recordings`. |
| `BROADWAVE_ENCODER` | `gpu` or `software`, to skip the automatic choice. |
| `BROADWAVE_HOSTS` | Public names the server answers to. See [Security](security.md). |
| `BROADWAVE_ALTERNATES` | `0` carries only the chosen sound track. By default a full-size encode also carries the channel's other languages. |

NVIDIA graphics need the NVIDIA Container Toolkit and `--gpus all`. That path has not been tested on real hardware yet.

An optional updater can move the image forward at night. The commands are in the README, under automatic updates. Without an updater, the web app says when a new release is out.

To build from source you need Go 1.26 or newer, Node 22 or newer, and ffmpeg. From a checkout, `make run` builds the web app and serves it on port 8477.
