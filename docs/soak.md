# Soak

A long run of the server with simulated viewers. It records stalls, dropped segments, how far viewers drift apart, and whether goroutines, file descriptors, heap, or ffmpeg processes are still growing after everyone has left. The report is one HTML page.

The harness lives in `scripts/soak/`. It starts the built-in fake tuner and a staging server with a throwaway catalog, both on localhost. It never talks to a real tuner.

## Running it

Use ports 19180–19182. The harness refuses anything outside 19180–19189. It sets `BROADWAVE_E2E=1`, so discovery stays on localhost, and it marks setup finished for that throwaway catalog only.

```bash
node scripts/soak/run.mjs --minutes 60 --port 19180 --browsers 2 --hls 4 --quality 360
```

`--minutes` defaults to 1440 (24 hours). `--quality 360` keeps the encode small, and the browser is asked for that same picture so it shares the scripted viewers' encode. `--quality auto` leaves the browser's saved choice alone. Add `--build` to rebuild the web app and the two binaries; a run also builds when `scripts/soak/.run/` has no binaries yet. The first build needs Node, Go, ffmpeg, and Chrome.

The server listens on 19180. The fake tuner listens on 19181, and its control port is 19182. Both are localhost. Two headless Chrome windows change channels, pause, seek, record, and open a side-by-side multiview. Four scripted clients poll the live playlists and fetch each new segment.

When the viewers stop, the harness stops every recording that began during the run, then waits until the server counts no viewers and no ffmpeg processes, up to 90 seconds, and reads the process again. A recording that was already in progress is left alone. A closed viewer is dropped on a sweep about a minute after its last fetch, so a shorter wait would score a healthy server as a leak. The report is `scripts/soak/.run/report.html` (and a JSON file beside it). That directory is disposable: binaries, the sample, the catalog, and the report. Exit status is 0 when every check passed.

What fails a run:

- The server process exits.
- An ffmpeg process, a zombie, or a counted viewer is still there after the quiet window. A quiet reading that never arrives fails that check too.
- Goroutines, open files, or heap stay well above where they started (a spike that comes back does not). A missing baseline or quiet reading fails the check.
- The gap between two scripted viewers on the same channel is over 3 seconds at the 95th percentile.
- Any dropped segment. Stalls are counted too; the default allows 40, because a busy machine buffers.

A failed check names the counter that moved. Fix that, add a regression test, and run a shorter soak before the next full day.
