# Picture lab

`scripts/picture-lab.sh` captures a sample from staging and encodes each `scripts/picture-lab/*.args` file in a `bw-lab-*` container on TUS at `-re`. Stills and the sample stay out of git.

The 0.6.0 image ffmpeg has no `libvmaf` (only `vmafmotion`), so the VMAF column is `n/a` until PB5.

Channel 1 (4.1), 12 s capture, 8 s encode, 2026-09-24:

| candidate | fps | frames | size | decode errors | speed | cpu | vmaf |
| --- | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| field-deint | 119.88 | 959 | 1280x720 | 0 | 0.993x | 21.34% | n/a |
| progressive-scale | 59.94 | 479 | 1280x720 | 0 | 0.993x | 19.96% | n/a |

Stills: `docs/lab/runs/latest/stills/field-deint.jpg`, `progressive-scale.jpg`.
