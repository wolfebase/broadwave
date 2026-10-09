# Hardware

What the fake HDHomeRun fleet covers. Set `Server.Profile`. Leave it empty for the original two-tuner fake. `TunerCount` overrides that profile from 1 through 8.

A closed tuner reports `TargetIP` `closed` so the next tune skips it. Codec bytes are a 188-byte MPEG-TS packet with a marker in the payload, not a real elementary stream. Nothing here is a house address, and this table is not a new run on a physical tuner.

| Model | Tuners | What the fake asserts | Status |
| --- | --- | --- | --- |
| CONNECT (HDHR4-2US), the default fake | 2 | Discover, lineup 4.1/4.2/5.1, control tune, busy 805, MPEG-TS stream. `DeviceAuth` is in discover.json and dropped by the client. | verified on fake |
| DUAL (HDHR3-US) | 2 | Discover, scan, tune, multiview plan, record plan, failover onto the other tuner | verified on fake |
| CONNECT DUO (HDHR5-2US) | 2 | Same path as the DUAL. `transcode=` is ignored. | verified on fake |
| CONNECT QUATRO (HDHR5-4US) | 4 | Same path, four tuners, busy when all four are held | verified on fake |
| FLEX DUO (HDFX-2US) | 2 | Same path. Model string does not contain "FLEX", so Sources has no ATSC 3.0 note. | verified on fake |
| FLEX QUATRO (HDFX-4US) | 4 | Same path, four tuners | verified on fake |
| FLEX 4K (HDFX-4K) | 4, two of them ATSC 3.0 | Channels 104.1 and 105.1. 104.1 is HEVC + AC-4 and will not tune on tuner 2 or 3. 105.1 is tagged DRM and returns 811. | verified on fake |
| PRIME (HDHR3-CC) | 3 | Cable numbers, `lock=qam256`, lineup tags `copy-once` and `copy-never`. Both return 811. The client marks both protected. | verified on fake |
| EXTEND (HDTC-2US) | 2 | `transcode=heavy`, `mobile`, `internet540`, `internet480`, `internet360`, and `internet240` stream an AVC + AAC marker. An unknown profile returns 802. | verified on fake |
| SCRIBE DUO (HDVR-2US-1TB) | 2 | Tuner path plus `recorded_files.json` (`Title`, `EpisodeTitle`, `Filename`) | verified on fake |
| SERVIO (HHDD-2TB) | 0 | Storage discover and `recorded_files.json`. Scan is refused. No tune and no failover. | verified on fake |
| DUAL, old firmware (HDHR3-US, version 20140301) | 2 | No `DeviceAuth`, lineup without codecs, upgrade route is 404. Tune, scan, and failover still run. | verified on fake |
| Tuner counts 1–8 | override on CONNECT DUO | Discover count, status length, busy at the cap. Failover when the count is 2 or more. One tuner fails the next tune after it is closed. | verified on fake |

CONNECT DUO is the only model this project has already read on a real device (discover, lineup, and tuner status). This table does not add a real-hardware run.

A pool keeps ATSC 3.0 tuners for HEVC + AC-4 (a guide number alone does not count). A 1.0 channel uses a tuner that cannot do 3.0 when one is free. The picker test covers two DUOs plus a FLEX 4K, eight tuners in all. A device that disappears mid-stream opens that same frequency on the next device. That path is verified on fakes only.

What a viewer sees is in [ATSC 3.0](atsc3.md). ATSC 3.0 channels arrive as HEVC Main 10 with AC-4 sound, sometimes with a second-language AC-4 track. Players that decode HEVC (Apple devices, Safari, Chrome with HEVC) get the picture as sent; other browsers get 8-bit H.264. No player decodes AC-4. The server reads each AC-4 track's width once per tune with ffprobe, which needs the AC-4 decoder that jellyfin-ffmpeg has. A 5.1 mix goes out as AC-3 5.1 to players that take AC-3, and everything else as stereo AAC. This was read on a real FLEX 4K.

Untested, on a fake and on real hardware: first-generation HDHR-US, DVB and ISDB variants, CONNECT 4K (HDHR5-4K), FLEX 4K development edition, SCRIBE QUATRO, SCRIBE 4K, and the TECH rack models.

## Clients

`TestClientMatrix` asks `Decide` for an interlaced MPEG-2 AC-3 broadcast and for progressive 720p H.264 AAC, with quality left on auto and the network on the LAN. A 720p H.264 stream is copied for every row below. The MPEG-2 stream is converted.

| Client | What it can play | MPEG-2 AC-3 rendition |
| --- | --- | --- |
| Apple TV HD (`AppleTV5,3`) | H.264, AC-3, 1080p, no HEVC | `1080.copy.broadcast` |
| Apple TV 4K, 1st (`AppleTV6,2`), 2nd (`AppleTV11,1`), 3rd (`AppleTV14,1`) | H.264, HEVC, AC-3 | `1080.copy.broadcast.hevc` |
| iPhone 6s (`iPhone8,1`, `iPhone8,2`) and iPad gen 6 and earlier | H.264, AC-3, no HEVC | `1080.copy.broadcast` |
| iPhone SE (2nd generation), iPhone 11, later iPhones, iPad gen 7 and later | H.264, HEVC, AC-3 | `1080.copy.broadcast.hevc` |
| Safari, when it reports AC-3 | H.264, AC-3, no HEVC | `1080.copy.broadcast` |
| Chrome, Edge, Firefox | H.264, AAC, no HEVC, no AC-3 | `1080.aac2.broadcast` |

The Apple app builds that row from the machine id (`Capabilities.forMachine`). A machine it does not recognize still asks for HEVC. The web app sends H.264 and adds AC-3 only when `MediaSource.isTypeSupported` says the browser can play it. The server's HEVC is 8-bit Main, not 10-bit.

## Servers

Startup probes the encoder in this order: NVENC, VideoToolbox, VAAPI, Quick Sync, then libx264. VAAPI is preferred over Quick Sync when both answer, because Quick Sync can open on a chip whose measured path is VAAPI. The PCI vendor distinguishes Intel VAAPI (`0x8086`) from AMD VAAPI (`0x1002`, `0x1022`). A VAAPI device with no vendor id stays unlabeled.

A GPU on a home server is often shared: Plex, Jellyfin, and Channels DVR transcode on the same chip, and under their load a 1080i channel fell to 0.75× real time. The normal VAAPI encoder runs on the shader cores those apps also scale and tone-map on. Intel's low-power encoder (VDEnc) is fixed-function, so when the driver has it, every VAAPI encode uses it (`-low_power 1`, no B-frames, which it does not make) and live TV stays on the GPU. On a UHD 770 beside three unthrottled 4K HEVC transcodes, a 1080i channel ran at 3.7× on low power, 1.4× on the normal encoder, and 1.4× on six CPU cores; the transcodes slowed by 4% beside four live encodes on low power and by 29% on the normal encoder. Picture quality at the same bitrate was within 0.05 dB.

Without a low-power encoder, when startup finds a GPU it first runs the encode a 1080i channel takes on the processor: four seconds of a test picture through field-rate `bwdif` and libx264 with the live settings, timed from the first frame out. At 2.5× or faster, live TV runs on the processor. A broadcast runs about 20% slower than the test picture, so that still holds two 1080i pictures. `BROADWAVE_ENCODER=gpu` or `software` skips the check. On a 12th-gen Core i9 the check read 3.5× to 3.9× while a real 1080i sample ran 3.1×. The 1080p60 speed below read anywhere from 2.3× to 3.9× on the same machine within minutes, so it is not used for this choice.

The same startup encodes a 1080p60 test picture. On a GPU it encodes three seconds and times the whole run; one second is mostly process startup, so a fast GPU looks too slow, and the GPU rows below were set on that figure. On the processor it encodes five seconds and times them from the first frame out: timed with startup, the same six cores read 1.6× right after a restart and 4.2× a second later, and the budget flipped between one picture and four. Timed from the first frame, it read 6.2× to 6.4× there, and 3.0× to 3.2× on three cores. The speed is how many times faster than real time that encode ran. It chooses the tallest transcode and how many new pictures can start. A broadcast the device can already play is still copied. A channel that is already being converted keeps a compatible picture instead of starting another: the same codec, and a watch with sound does not take a silent tile. A picture nobody is watching does not hold a slot. One more channel than the table allows is refused. A bench that does not finish within 20 seconds uses the under 0.5× row and does not invent a speed. A probe that fails before it encodes stays on the unmeasured row.

On a GPU:

| 1080p60 speed | Tallest transcode | Selected tile | Tiles at once |
| --- | --- | --- | --- |
| 4× and up | 1080p | 720p60 | 4 |
| 2× to 4× | 1080p | 720p60 | 2 |
| 1× to 2× | 720p | 540p60 | 2 |
| 0.5× to 1× | 540p | 360p60 | 1 |
| under 0.5× | 540p | 360p | 1 |
| not measured | 1080p | 720p60 | 2 |

On the processor:

| 1080p60 speed | Tallest transcode | Selected tile | Tiles at once |
| --- | --- | --- | --- |
| 5.5× and up | 1080p | 720p60 | 4 |
| 3.8× to 5.5× | 1080p | 720p60 | 2 |
| 2.7× to 3.8× | 720p | 540p60 | 2 |
| 1.9× to 2.7× | 720p | 360p60 | 1 |
| 1.4× to 1.9× | 540p | 360p60 | 1 |
| under 1.4× | 540p | 360p | 1 |
| not measured | 540p | 360p | 1 |

Intel VAAPI, Intel Quick Sync, AMD VAAPI, NVIDIA NVENC, and Apple VideoToolbox are the GPU rows. Software is libx264, which is what a Raspberry Pi 5 runs (it has no H.264 encoder in this image) and what a J4125-class board runs when it has no GPU device. Those two boards were not in the room: their rows are the rule, applied to their speed. Diagnostics shows the sentence for the machine that actually ran.

The processor rows leave each layout half again the processor it needs. Every encode decodes and deinterlaces its own 1080i feed. On 1080i MPEG-2 with the live settings, at three and at six cores, one encode at real time took this share of the bench speed: a 360p60 tile 0.79, 540p60 0.90, 720p60 1.27. A quad of 360p60 tiles ran at 2.0× each on six cores and 1.0× on three; two 720p60 pictures ran at 2.5× on six cores and 1.2× on three, too close for a broadcast, which runs about 20% slower than the test picture.
