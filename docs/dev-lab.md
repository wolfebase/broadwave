# Development lab hardware

The maintainer's test rig. Useful as a reference setup; nothing in the product depends on these values.

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

At the time of the read, both tuners were on 9.1 KMBC-HD (signal about 92%, quality 100%). Tuner 0 was streaming to 192.168.1.192. Tuner 1 was streaming to 192.168.1.2, the Unraid server.

This PC has an RTX 5090 and Intel UHD 770. The Windows dev server uses NVENC. Unraid TUS is an i9-12900K with UHD 770. The Broadwave container there encodes with VAAPI (`h264_vaapi`) because the bookworm MFX library cannot open a session on that iGPU.

Channel 14.1 through 14.16 is the mux-sharing candidate. That test waits for a free tuner.

## Scan-type matrix (PB3)

Each row is the live rendition graph (`RenditionArgs`) for a 1080p delivery. 480i uses the same field-rate graph as 1080i; `scale` never upscales (`min(1920,iw)`). Soft 3:2 (MPEG-2 `repeat_first_field` on progressive pictures inside an interlaced sequence) is stored as field order `film` and plays at 24p: `fieldmatch,decimate` on the CPU, then `scale_vaapi` when the encoder is VAAPI.

Measured on this Mac with `TestScanMatrixPicture` (1s moving bars, libx264 and VideoToolbox). `idet` is multi-frame TFF+BFF vs progressive. `mpdecimate` kept every 59.94 frame (drop of 4 or fewer).

| Scan | Encoder | Output | Frames | idet interlaced | idet progressive |
| --- | --- | --- | ---: | ---: | ---: |
| 720p | libx264 | 59.940 | 60 | 0 | 60 |
| 720p | VideoToolbox | 59.940 | 60 | 0 | 60 |
| 1080i (`tinterlace`) | libx264 | 59.940 | 60 | 0 | 60 |
| 1080i | VideoToolbox | 59.940 | 60 | 0 | 60 |
| 480i | libx264 | 59.940 | 60 | 0 | 60 |
| 480i | VideoToolbox | 59.940 | 60 | 0 | 60 |
| H.264 PAFF (`tff=1`) | libx264 | 59.940 | 60 | 0 | 60 |
| H.264 PAFF | VideoToolbox | 59.940 | 60 | 0 | 60 |
| H.264 MBAFF | libx264 | 59.940 | 60 | 0 | 60 |
| H.264 MBAFF | VideoToolbox | 59.940 | 60 | 0 | 60 |
| Hard telecine (`telecine=pattern=23`) | libx264 | 23.976 | 23 | 23 | 0 |
| Hard telecine | VideoToolbox | 23.976 | 23 | 23 | 0 |

Hard telecine has no `repeat_first_field`, so the header scan leaves it on the field-rate path unless the viewer picks film. Forcing film recovers 23.976 fps, and `idet` still sees comb on this 1s fixture (PB3b). Soft 3:2 is `TestScanTypeCapturedHeaders` (`filmTS`).

VAAPI on TUS (`scripts/picture-lab.sh run`, `deinterlace_vaapi=mode=motion_adaptive:rate=field`):

| Sample | fps | frames | size | decode errors | speed | cpu | idet |
| --- | ---: | ---: | --- | ---: | ---: | ---: | --- |
| 10s interlaced fixture | 59.94 | 479 | 640×360 | 0 | 0.993× | 4.51% | TFF 4 / progressive 476; mpdecimate kept 480 |
| Real 5.1 (1920×1080 tt, 30000/1001) | 59.94 | 311 | 1920×1080 | 0 | 0.999× | 13.71% | TFF 209 / progressive 103, same as `bwdif` on that capture |
| Real 4.1 progressive scale (1280×720, 60000/1001) | 59.94 | 306 | 1280×720 | 0 | 1× | 16.84% | kept 59.94, not doubled |

480i and the H.264 subchannels (14.x) are the fixtures above. Those channels are not in the current scan, so they were not tuned.
