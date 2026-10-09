# 0006: Broadcast guide

Accepted, 2026-09-24.

## Goal

Fill listings the tuner guide does not have, using the guide carried in the broadcast itself.

## Design

**Read the full mux.** The relay already tunes `/tunerN/ch<freq>`. That stream includes the PSIP tables. `/auto/v<channel>` is one program and does not.

**Harvest while tuned.** Guide packets are parsed off the side of the live read. They never start a second tune and never start a transcode.

**Fill gaps only.** SiliconDust, Schedules Direct, and a user XMLTV file win. A broadcast listing is stored only when that channel and time are empty.

**Scan when idle.** If no one is watching or recording, and nothing is scheduled in the next 30 minutes, the server dwells on each frequency that still needs listings, then releases the tuner. A watch or a recording cancels that scan before the next tune.

## Consequences

An antenna scan on 2026-09-24 found the channels the tuner could receive then. Channels that were only in an older lineup were not found again, so they stay without a broadcast listing.
