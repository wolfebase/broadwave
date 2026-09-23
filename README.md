# Waveguide

Live TV and DVR for your antenna, built for Apple devices.

Waveguide runs on your server (Docker, Unraid, or a Mac), finds your HDHomeRun, and fills a guide with your local channels. It records the original broadcast and shares one tuner with every screen in the house, in sync. Watch on iPhone, Apple TV, or any browser.

- **One tuner, every screen.** Everyone watching the same channel shares one tune, and Plex, Jellyfin, or Channels can use it too.
- **Whole-Home Sync.** Every room plays the same frame, with no echo and no spoilers from the next room.
- **Guide first.** A fast, beautiful guide with 14 days of listings.
- **Game-aware DVR.** Record every game for your team, and keep recording until it's final.
- **Original quality.** Recordings are the untouched broadcast. Apple TV gets direct play with 5.1 surround.

## Install

### Docker

```bash
docker run -d --name waveguide --network host \
  -v /path/to/config:/config \
  -v /path/to/recordings:/config/work/recordings \
  --device /dev/dri \
  ghcr.io/twolfekc/waveguide:latest
```

Open `http://<server>:8477`. Host networking lets the server find your tuner and lets the apps find the server. See `deploy/docker/compose.yaml` for Compose.

### Unraid

Use the template in `deploy/unraid/waveguide.xml` (Community Apps listing coming).

## Develop

You need Go 1.25+, Node 22+, and ffmpeg. On a Mac: `brew install go node ffmpeg`.

```bash
make run      # build the web app and run the server on :8477 with ./data
make dev      # API with CORS for Vite; then: cd web && npm run dev
make test
```

Flags: `-addr :8477`, `-config data`, `-hdhr <tuner address>` (or `HDHR_HOST`), `-dev`.

See `AGENTS.md` for the project guide, `docs/architecture.md` for how it works, and `docs/roadmap.md` for what's next.

## License

Apache 2.0. ffmpeg and comskip run as separate programs under their own licenses. See `NOTICE`.
