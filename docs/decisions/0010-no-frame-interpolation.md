# 0010: No invented frames

Accepted, 2026-09-24.

## Goal

Smooth motion at 60 should be the real moments in the broadcast, not frames the server made up.

## What was measured

jellyfin-ffmpeg 7.1.4 on the Unraid UHD 770, fed at real time. The picture was 4 seconds of 1080p30 made with `bwdif=mode=send_frame` from a 12 second capture of channel 2 (MPEG-2 1920×1080, top field first, 30000/1001). A second clip was a white box moving across green, so a blended edge is obvious.

| method | what the new frames are | CPU | speed | keep |
| --- | --- | --- | ---: | --- |
| `fps=60000/1001` | duplicates | 9.83% filter, 24.10% with VAAPI encode | 0.88× | no; motion stays 30 |
| `minterpolate=mi_mode=blend` | average of the two neighbors | 25.43% filter, 41.68% with VAAPI encode | 0.88× | no; both edges ghost |
| `framerate=fps=60000/1001` | linear mix of the two neighbors | 19.79% filter | 0.88× | no; same ghost |
| `minterpolate=mi_mode=mci` | motion search | 118% and 0.09× with libx264 | 0.09× | no; not real time |
| `vpp_qsv=framerate=60` | drop or repeat, and only if QSV starts | device init failed (MFX session -3) | — | no |
| VAAPI | no interpolation filter in this ffmpeg | — | — | field deinterlace only |

`libx264` at 1080p60 was about 240% of one core for duplicate, blend, and framerate alike, so the encoder, not the blend, was the cost. Blend does fit in one core once encode is on the GPU. It still looks worse: the invented frame has a pale trail on each side of a moving edge (`docs/lab/pb4/blend-box.jpg`, `rate-box.jpg`). The duplicated frame stays sharp (`dup-box.jpg`).

FFmpeg 7.1 has no VAAPI filter that invents progressive frames. `deinterlace_vaapi=rate=field` rebuilds 59.94 from interlaced fields. That path stays.

## Decision

Smooth does not change the frame rate of a progressive source. It only selects motion-compensated deinterlace when the iGPU has that mode. The `minterpolate` blend path and its startup probe are gone.

Apple TV display matching at 59.94 is still PB7. The apps do not set `AVDisplayCriteria` yet.

## Consequences

A true 30p or 24p program plays at the rate it was broadcast. Film cadence still recovers 24p with `pullup`. Interlaced sports stay at field rate, which is where the 60 real moments are.
