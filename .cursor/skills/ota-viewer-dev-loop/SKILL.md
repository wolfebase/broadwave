---
name: ota-viewer-dev-loop
description: Build, run, restart, and verify the OTA Viewer server and web app against the real HDHomeRun, including relay smoke tests, browser checks at three layouts, and measuring Whole-Home Sync between two screens. Use when working in the OTA Viewer repo on server or web code, when starting/restarting the dev server, when verifying UI changes, or when asked to test playback or sync.
---

# OTA Viewer dev loop

Repo: `/Users/tyler/Projects/active/ota viewer` (note the space; always quote paths). Read `AGENTS.md` and `docs/plan/MASTER_PLAN.md` first.

## Start / restart the dev server

Run in the background (Shell tool `block_until_ms: 0`), then wait for "listening" in the log:

```bash
scripts/dev-server.sh                                  # real catalog copy at /tmp/otav-live, :18477
SKIP_WEB=1 scripts/dev-server.sh                       # server-only change
FRESH=1 CONFIG=/tmp/otav-fresh scripts/dev-server.sh   # empty catalog (setup wizard)
```

Log: `$CONFIG/stderr.log`. Startup takes ~5-10 s (encoder probes), discovery ~4 s, guide ~1 s.

Pitfalls that cost hours before:
- `pkill -f otav` kills the calling shell. Use `pkill -9 -x otav` (the script does).
- A killed server can hold :18477 briefly; the script waits for the port.
- Shell commands to 127.0.0.1 need `required_permissions: ["all"]` (the sandbox blocks localhost and ~/go writes).
- The browser caches old bundles from before `index.html` became `no-cache`. In a test tab run CDP `Network.setCacheDisabled {cacheDisabled:true}` and check loaded scripts: `performance.getEntriesByType('resource').filter(e=>e.name.endsWith('.js'))` must match `server/cmd/ota-viewer/assets/web/assets/index-*.js`.
- Navigating before the server listens leaves the tab on `chrome-error://`; open a new tab (`newTab: true`) after the server is up.
- Autoplay with sound is blocked in automation tabs: `v.muted=true; await v.play()` before measuring.

## Test gates

```bash
make test                  # go test ./server/... + web typecheck
scripts/relay-smoke.sh     # no-tuner end-to-end relay: renditions, CMAF, PDT, export (12 checks)
cd apple/Packages/OTAKit && swift test
make apple                 # iOS + tvOS builds
```

Any change under `server/internal/live`, `web/src/lib/sync.ts`, or `OTAKit/SyncEngine.swift` must also pass the two-screen sync measurement below.

## Real tuner facts

HDHomeRun CONNECT DUO `192.168.1.252`, 2 tuners, 27 channels (mostly MPEG-2 + AC-3; 4.1 WDAF FOX, 5.1 KCTV CBS, 9.1 KMBC ABC, 41.1 KSHB NBC; 14.x are H.264 subchannels on one frequency). Channel ids in the catalog: 1 = 4.1, 2 = 5.1, 3 = 9.1. Check tuners: `curl -s 127.0.0.1:18477/api/v1/tuners`. Everything about the server: `curl -s 127.0.0.1:18477/api/v1/diagnostics`.

## Browser verification (cursor-ide-browser MCP)

1. `browser_navigate` with `newTab: true` to `http://127.0.0.1:18477/...`
2. Desktop: CDP `Emulation.setDeviceMetricsOverride {width:1440,height:900,deviceScaleFactor:1,mobile:false}`; phone: `{width:390,height:844,mobile:true}`; TV: `?layout=tv` at 1920x1080.
3. `browser_take_screenshot` and look at it. Check console errors via CDP `Runtime.evaluate`.

## Measure Whole-Home Sync (two screens)

Open two tabs on `/watch?channel=1`, mute+play both, wait ~20 s, then in each tab sample:

```js
(async()=>{const v=document.querySelector('video');const o=[];for(let i=0;i<4;i++){o.push([v.dataset.syncDrift,v.dataset.syncOffset,document.querySelector('.sync-pill')?.className].join(':'));await new Promise(r=>setTimeout(r,500))}return o.join(' | ')})()
```

`data-sync-offset` is media time minus local wall clock (ms); the difference between tabs is the screen-to-screen offset (target < 50 ms; last measured 15 ms). `data-sync-drift` is drift from the room target. `data-hls-error` shows the last hls.js error; `video.hls` is the hls.js instance (check `hls.levels[0].details.fragments` for negative `start`/`duration`, which means hls.js timeline corruption).

## Commit

Small commits with what + why. Tick `docs/plan/PROGRESS.md`. Never commit `data/`, `server/cmd/ota-viewer/assets/web/`, or `apple/*.xcodeproj`.
