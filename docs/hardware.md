# Hardware notes

Read on 2026-09-21 from this PC. No tuner stream was opened and no lock key was taken.

| | |
| --- | --- |
| Device | HDHomeRun CONNECT DUO |
| Model | HDHR5-2US |
| Address | 192.168.1.252 |
| Device ID | 10611B4C |
| Firmware | hdhomerun5_atsc 20250815 |
| Published firmware | 20260326. Do not install it from this app. |
| Tuners | 2, ATSC 1.0 |
| Lineup | 27 channels. 12 MPEG-2, 15 H.264, all audio AC-3. 8 marked HD. No DRM tags. |
| Favorites on the device | 4.1 WDAF, 5.1 KCTV, 9.1 KMBC, 41.1 KSHB |

`hdhomerun.local` does not resolve. Discovery is UDP port 65001, then `http://192.168.1.252/discover.json` and `lineup.json`.

At the time of the read, both tuners were on 9.1 KMBC-HD (signal about 92%, quality 100%). Tuner 0 was streaming to 192.168.1.192. Tuner 1 was streaming to 192.168.1.2, the Unraid server. Session 1 does not interrupt those clients.

This PC has an RTX 5090 and Intel UHD 770. The Windows dev server uses NVENC. Unraid TUS is an i9-12900K with UHD 770. The OTA-Viewer container there encodes with VAAPI (`h264_vaapi`) because the bookworm MFX library cannot open a session on that iGPU.

Channel 14.1 through 14.16 is the mux-sharing candidate. That test waits for a free tuner.
