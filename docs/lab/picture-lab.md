# Picture lab

`scripts/picture-lab.sh` captures a sample from staging and encodes each `scripts/picture-lab/*.args` file in a `bw-lab-*` container on TUS at `-re`. Stills and the sample stay out of git.

jellyfin-ffmpeg 7.1.4, like the 0.6.0 Debian ffmpeg, is built without `libvmaf`. The VMAF column stays `n/a` until a binary that has the filter scores a capture (PB5b).

Channel 1 (4.1), 12 s capture, 8 s encode, 2026-09-24:

| candidate | fps | frames | size | decode errors | speed | cpu | vmaf |
| --- | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| field-deint | 119.88 | 959 | 1280x720 | 0 | 0.993x | 21.34% | n/a |
| progressive-scale | 59.94 | 479 | 1280x720 | 0 | 0.993x | 19.96% | n/a |

Stills: `docs/lab/runs/latest/stills/field-deint.jpg`, `progressive-scale.jpg`.

PB4, 2026-09-24. Channel 2 capture reduced to 1080p30 with `bwdif=mode=send_frame`, then 4 s at `-re` in `ghcr.io/wolfebase/broadwave:0.6.0`. Filter-only CPU is `-f null`. Ghosting stills are `docs/lab/pb4/`.

| method | fps target | cpu | speed | note |
| --- | --- | ---: | ---: | --- |
| fps duplicate, filter only | 59.94 | 9.83% | 0.883× | sharp; motion stays 30 |
| minterpolate blend, filter only | 59.94 | 25.43% | 0.881× | ghosted edges |
| framerate, filter only | 59.94 | 19.79% | 0.881× | same ghost |
| minterpolate mci + libx264 | 59.94 | 118.12% | 0.09× | not real time |
| vpp_qsv framerate | 60 | — | — | MFX session -3 |
| fps duplicate + h264_vaapi | 59.94 | 24.10% | 0.877× | |
| blend + h264_vaapi | 59.94 | 41.68% | 0.875× | fits one core; looks worse |
