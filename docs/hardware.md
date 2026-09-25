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

Untested, on a fake and on real hardware: first-generation HDHR-US, DVB and ISDB variants, CONNECT 4K (HDHR5-4K), FLEX 4K development edition, SCRIBE QUATRO, SCRIBE 4K, and the TECH rack models.
