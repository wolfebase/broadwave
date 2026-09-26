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

CONNECT DUO is the only model this project has already read on a real device: discover, lineup, and tuner status are in `docs/dev-lab.md`. This table does not add a real-hardware run.

A pool keeps ATSC 3.0 tuners for HEVC + AC-4 (a guide number alone does not count). A 1.0 channel uses a tuner that cannot do 3.0 when one is free. The picker test covers two DUOs plus a FLEX 4K, eight tuners in all. A device that disappears mid-stream opens that same frequency on the next device. This house has one CONNECT DUO, so that path is verified on fakes only.

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
