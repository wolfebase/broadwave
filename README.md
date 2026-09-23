# OTA Viewer

A local over-the-air DVR. The server finds an HDHomeRun, fills the guide from SiliconDust listings, plays channels through ffmpeg, and records the original broadcast as MPEG-TS.

## Run

Go, Node, and ffmpeg are required. On the Windows PC, Go lives at `%USERPROFILE%\sdk\go\bin` when it is not on PATH.

```powershell
cd web
npm.cmd install
npm.cmd run build
cd ..
go run ./cmd/ota-viewer
```

On a Mac, install Go, Node, and ffmpeg (`brew install go node ffmpeg`), then:

```bash
cd web
npm install
npm run build
cd ..
go run ./cmd/ota-viewer
```

Open http://127.0.0.1:8477

The process listens on `:8477` and keeps its catalog in `./data`. On startup it finds the HDHomeRun, reads the lineup, and asks SiliconDust for listings. Open the guide and press Watch. A recording is the original broadcast, as an MPEG-TS file under `data/work/recordings`. Library channels (numbers from 900) play those files and do not take a tuner.

On the Unraid server the same app is the container `OTA-Viewer` at http://192.168.1.2:8477. Config is `/mnt/cache/appdata/ota-viewer/config`. Recordings are `/mnt/user/media/ota-recordings`. Autostart is off.

Docker:

```powershell
docker build -f deploy/docker/Dockerfile -t ota-viewer .
```

Flags:

- `-addr :8477`
- `-config data`
- `-dev` allows a Vite dev server on localhost to call the API

## Tests

```powershell
go test ./...
```

A live read of the tuner is not part of `go test`. Discovery on the LAN happens when the server starts.
