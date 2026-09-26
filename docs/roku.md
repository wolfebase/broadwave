# Roku

Design notes for a future Roku channel. This file does not build one. There is no SceneGraph package, no BrightScript, and no server change in this task.

Checked against Roku's streaming specifications and Video node reference as published on developer.roku.com on 2026-09-25, and against the live renditions this server already writes. Anything those pages do not say is marked unknown. This note does not state channel-store certification rules.

## What a channel would be

A Roku channel is a SceneGraph app. The package is a manifest, a BrightScript `main` that shows one scene, and XML components with BrightScript attached. The scene draws the screens. Video plays in the platform Video node. The app does not ship a decoder, and it does not run hls.js. Pointing that node at an HLS URL is the whole playback stack.

The first thing worth building, later, is one full-screen player for one live channel. Home, the guide, recordings, and settings wait until that player holds a picture. How the channel finds a Broadwave server is unknown. The other clients use Bonjour (`_broadwave._tcp`) and, after a few seconds, a UDP probe `BWDP?` plus a 16-byte nonce on port 8479. The server answers `BWDP!` plus its id, name, URL, and a signature. Whether a SceneGraph app can browse Bonjour, or send that probe, is not established here.

## What the player would be handed

Live playback is the rendition `Decide` already picks from the client's capability profile. Each rendition is one HLS media playlist: CMAF fMP4, 2-second segments, an init file, and `EXT-X-PROGRAM-DATE-TIME` from the shared broadcast timeline. Video and audio are both mapped into that playlist, so each media segment carries both. Segment 0 is never served. Apple and the web play this same playlist. A first Roku try uses that URL. It does not get its own packager.

A reasonable first profile, until a device says otherwise, is H.264, AAC stereo, and a max height of 720. That asks `Decide` for a transcode rather than original AC-3, and it stays off 1080p60. Put HEVC in the profile only when that device reports it can decode HEVC. The streaming spec's 4K example checks this with `roDeviceInfo.CanDecodeVideo`. This note has not run that check.

## HLS constraints

The Video node plays the stream. Content metadata carries the URL and `streamformat` `hls`. There is no JavaScript player to configure, so nothing about hls.js (buffer caps, transmuxing, the fragment table) applies. The reason the shared renditions are fMP4 is that MPEG-TS segments made hls.js corrupt that table mid-stream. That failure is not a Roku failure. Do not move Apple and the web back to MPEG-TS to suit this client.

The specifications table lists HLS video chunks as MPEG-TS or CMAF, and it says muxing audio and video is not supported for CMAF. Audio chunks in that row are AAC, AC-3, and E-AC-3, separate from the video. Our playlist is the other shape: one fMP4 init and media segments that contain both audio and video. On that wording, the playlist is not a supported CMAF layout. Whether a current player accepts it anyway is unknown. It has not been played on a Roku. If a device rejects it, the later fix is a Roku-only rendition, either demuxed CMAF or MPEG-TS, that does not replace the fMP4 the other clients use. That work is not this task.

Published HLS video codecs are AVC (H.264) and HEVC. AV1 is listed for DASH only, which we do not emit. MPEG-2 is not in the list. A profile that does not claim MPEG-2 already gets a transcode from `Decide`, which is what a Roku profile should do. H.264 in the table is profiles main and high, levels 4.1 and 4.2, up to 1920×1080. Our transcodes ask for high profile. The level written into the bitstream is not checked here. The published AVC video bitrate cap is 10 Mbps, peak 1.5 times the average. A 1080p field-rate transcode is 14 Mbps, over that cap. A 1080p progressive transcode is 10 Mbps, on the cap. 720p is 8 Mbps when deinterlaced to field rate and 5 Mbps when the source is progressive. A copied broadcast H.264 stream is whatever the station sent, which can also exceed 10 Mbps. Unknown whether the player refuses a stream over the cap or only plays it badly. HEVC is listed up to 40 Mbps, so our HEVC rates sit under that cap, but the same page limits HEVC to 4K-capable devices. It also says not every device plays 1080p60, and that a stream should include 720p60 or 1080p24/30 as well. That is why the first profile caps height at 720.

Published input frame rates are 24p, 25p, 30p, 50p, and 60p. Field-rate output here is 60000/1001, film is 24000/1001, and small pictures are 30000/1001. A progressive source keeps its own rate. 59.94, 23.976, and 29.97 are not named. Whether the player treats them as 60p, 24p, and 30p is unknown.

Published HLS audio includes AAC, MP3, DTS, Dolby Digital, and Dolby Digital Plus. AC-3 and E-AC-3 are passthrough and the page says that path is device-specific. The same page says an app must always include an AAC stereo track beside any Dolby track, because some devices do not decode AC-3. A Broadwave rendition carries one audio codec. An AC-3-only playlist does not meet that sentence, so the first profile should not claim AC-3. The AAC stereo rendition is 160 kbps, inside the published 32–256 kbps range. The AAC column lists 2.0 only, and the page says multichannel AAC is not on every model, so do not ask for the 5.1 AAC rendition. The sample rate is not pinned in the rendition arguments. Unknown whether it is always 48 kHz, one of the two published rates (44.1 and 48).

Two further notes on the segments we already write. The page recommends HLS chunks of 4 to 6 seconds. A footnote says live segments should be under 5 seconds, constant, and start on an IDR, and that matching media in other variants should line up. Ours are a constant 2 seconds and open on a keyframe. That matches the footnote and is shorter than the 4-to-6 recommendation. Unknown whether 2-second segments cause trouble. For live streams the page also says the app must stay at least 30 seconds away from the live edge. Seeking at live, for trick play, is described as a seek to 999999 seconds that the player clips to the current window. Our room targets are 6, 10, and 20 seconds behind real time (`lowest`, `balanced`, `stable`). All three are closer than 30 seconds. Unknown whether that line is enforced.

Captions are not part of a first player. The HLS subtitle row does list WebVTT, which is the format the later caption work plans to add. A Roku player should not depend on captions until that exists.

## Whole-Home Sync

A sync room has one target. At server time T the screen should show anchor media time plus (T minus anchor server time) times the rate. Media time is the program date-time of the frame. The default room sits 10 seconds behind real time. Clients measure their offset from the server on the events WebSocket. The web and Apple engines nudge rate by up to 3 percent when drift is under a few hundred milliseconds. Past that, a client that is ahead pauses for the drift, and one that is behind seeks forward. They do not seek backward in a live buffer. The bar on a home network is about 50 milliseconds.

A Roku channel should not promise that.

If the Video node plays the shared playlist, the Roku is on the same segments as the other screens, with the same program date-times. That is same-channel playback, not a lock. The published controls do not provide the lock.

What is documented on the Video node: `control` can pause and resume. `position` is read-only, and it is either UTC or elapsed since the start, depending on the content. `positionInfo` carries the last rendered video and audio sample, and its `epoch` field is 0 when those times are relative to the start and 1 when they are UTC. `pauseBufferEpochOffset` is described as converting pause-buffer times to UTC from the wall clock in a live playlist. `seek` is seconds from the beginning of the stream. Default `seekMode` lands on the earlier sync frame, meaning a segment or an I-frame inside one. `accurate` seeks to the requested time only when that platform can. `notificationInterval` defaults to 0.5 seconds. `streamingSegment.latency` is the milliseconds between the live edge and the segment currently playing, which is a segment measurement, not a frame measurement.

What blocks a lock:

- Every current room target is inside the published 30-second gap from the live edge. A player that holds the Roku 30 seconds back cannot show the frame a screen at 10 seconds is showing. Pulling the whole room back to suit one Roku would make every other screen later. Do not do that without a measurement.
- The Video node reference does not document a playback-rate field. The few-percent trim the other engines use has no published equivalent. Do not invent one.
- A forward seek in the default mode snaps to the previous keyframe. Keyframes are 2 seconds apart, so the miss can be almost 2 seconds. That cannot hold 50 milliseconds. Whether `accurate` works on this live HLS is unknown.
- The "ahead" correction on the other clients is a pause, not a backward seek. Pause and resume exist. Whether a short pause on a live stream holds the timeline, jumps toward live, or drops media is unknown. The pause-buffer fields include an overflow flag, so a live pause is not guaranteed to keep what it had.
- Whether `position` or `positionInfo` equals our program date-time is unknown. If the number is seconds from the start of the current window, the app cannot compute drift. The live-edge seek of 999999 seconds is a third clock. How to turn a program date-time into a `seek` value is unknown.
- A half-second position tick cannot steer a 50-millisecond lock. How small `notificationInterval` can be, and whether the sample time is accurate when it arrives, is unknown.
- The clock exchange needs the events WebSocket. Current docs describe `roWebSocket`. The `roUrlTransfer` page and a developer blog post about OS 16.0 say WebSocket support arrived in that release. Which players in a home still lack it is unknown. This note does not set a minimum OS. The documented URL example is `wss`. Our event socket is plain `ws` on the server port. Whether `roWebSocket` opens that is unknown.

Until a device plays the playlist, reports a position that matches a program date-time, and can correct forward without a rate field, treat frame sync as not feasible. Same-channel playback is the milestone, and only if the playlist plays. Follow mode, group pause, and group rewind wait on that. Group rewind seeks backward into the live buffer, which stays off until a device shows the seek is safe.

## Leave out

Leave multiview out, including several Video nodes and a mosaic rendition, until one full-screen player works. The Video node says only one stream may be prebuffering at a time. Whether two Video nodes can decode together is unknown. A mosaic is also a new server rendition, and this client does not need one to answer the playlist question.

Leave game alerts out. They need a player that is already watching, and a way to raise them. This note does not look up a notification API.

Also leave recordings, group mode, captions, trick-play thumbnails, and channel-store certification until a live picture exists.

## Open points

Unknown, and not to be filled in by guessing:

- Whether the current muxed fMP4 playlist plays at all.
- Whether 2-second segments, or 59.94, 29.97, and 23.976, are accepted.
- Whether an encode at or above the published 10 Mbps AVC cap is rejected.
- The H.264 level our encodes write, and the AAC sample rate they write.
- Whether AC-3 in this fMP4 plays on a device that can passthrough Dolby, given the playlist has no second AAC track.
- Whether the 30-second live-edge line is enforced.
- Whether position tracks program date-time, and how to seek to one.
- Whether a short live pause holds position, and whether accurate seek exists for this HLS.
- Whether any published field can change playback rate by a few percent.
- Whether `roWebSocket` can open the in-home `ws` event socket, and on which OS releases.
- How a channel discovers the server.
- Whether more than one Video node can play.
