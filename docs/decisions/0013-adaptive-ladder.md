# 0013: Adaptive ladder from one decode

Accepted, 2026-10-11.

## Context

Each watch plays one rendition. Browsers and Apple players can switch picture size themselves when a master playlist lists several variants. A separate ffmpeg per size would decode the broadcast once per viewer.

## Decision

- The setting `adaptive` is "0" or "1". Missing means off. Off leaves every watch on the single rendition it has today.
- On, a browser or an Apple device (web, iOS, iPadOS, tvOS, macOS, visionOS) gets one ladder: 1920×1080, 1280×720, and 848×480 H.264, plus audio only. The server's picture-height budget can drop the 1080 rung, or the 1080 and 720 rungs. The tallest remaining picture is the one the session names. Tiles, an explicit rendition, a pinned tile or 360 or focus quality, a second language, described video, even volume, silent audio, and any other platform stay on one rendition.
- One ffmpeg decodes the channel in software (no hardware decode on the ladder input), deinterlaces or pulls film cadence once, then splits. Each rung scales with min(width, source) so a 720p broadcast is not upscaled; the 1080 rung can then have the same pixels as the 720 rung at a higher bitrate. The scale then crops to an even width and height: 848 wide against 16:9 is 477 tall, and libx264 rejects an odd side. All rungs share the frame rate. 480p is not cut to half rate. Audio is AAC stereo at 160 kb/s, copied to each video rung. The audio-only variant is not a fourth encode: it is the primary rung's existing `audio-2.m3u8` view, so its segments are the same fragments. Apple can switch to it. hls.js removes a variant that has no picture when the master also has picture variants, so a browser stays on the shortest picture.
- The master is `ladder.m3u8` in the primary rendition directory. `mainPlaylist` points at it. `playlist` stays that rung's `index.m3u8`. Variants are muxed picture-and-sound, highest bitrate first, then audio only. Each video line has BANDWIDTH, AVERAGE-BANDWIDTH, CODECS, and RESOLUTION. BANDWIDTH is twice the encoder target plus 160 kb/s of audio, the same headroom the other masters use. Audio only has CODECS and bandwidth and no resolution. Captions, when the channel has them, are a subtitles group on the video variants.
- Segment boundaries match across the rungs because the frames are split before scale, keyframes follow the source (film cadence uses one shared keyframe expression), and libx264 scenecut is off. NVIDIA and Quick Sync force those keyframes to be IDRs, and Quick Sync does not insert its own. A player fetches every rung at once, so those playlists take the primary rung's program dates: one frame has one date on every rung. That alignment is across the ladder variants. It is not a promise against a copy of the same channel.
- The ladder counts as one picture in the transcode budget. A tile beside it can still decode the channel again. The ladder is always H.264 and AAC stereo, including on a server whose normal encoder is HEVC or AC-3 passthrough. Smooth's motion-compensated deinterlacer is not used; broadcast and smooth both use one software bwdif. Film uses one pullup. VAAPI and the other hardware encoders, when they work, encode only; a ladder that dies at startup falls back to libx264. A channel-change guess does not start a ladder.
- Apple's player loads `mainPlaylist`, so it switches rungs without a change to the sync engine. The web player loads `ladder.m3u8` the same way and primes the tallest video rung so a room can still start on the shared frame.

## Consequences

The ladder is off until someone turns it on. One watched channel then spends the cost of one decode and three encodes.

One run of `TestLadderEncodeCost` (`LADDER_COST=1`) measured process CPU time (user plus system), not wall clock. The sample was 4 seconds of 1280×720 progressive MPEG-2 at 30 frames a second, encoded with libx264. The 1080p rung does not upscale that sample, so it encodes 1280×720 at the higher bitrate.

- Ladder, one decode and three encodes: 2.37 s
- Separate 1080p encode: 1.02 s
- Separate 720p encode: 0.85 s
- Separate 480p encode: 0.58 s
- The three separate encodes added up: 2.46 s

The sample is synthetic and progressive, so the shared decode saves little beside the encodes. An interlaced broadcast or a hardware encoder will not match these times.

## What it does not change

Roku and other shared clients keep the single rendition. Recordings stay the original broadcast. Whole-home sync still reads program date-times; the ladder does not change the room protocol.
