# 0002: Playback pipeline and renditions

Accepted, 2026-09-22.

## Context

Every live channel currently gets one ffmpeg transcode to H.264 and AAC. When one viewer changes quality, the feed restarts for everyone. Apple devices can play H.264 video and AC-3 audio in HLS directly, but everything is re-encoded anyway, and 5.1 audio is converted. MPEG-2 channels (about half of a typical ATSC 1.0 lineup) can't be decoded by AVPlayer or browsers.

## Decision

- Clients use the platform players: AVPlayer on Apple devices, hls.js or native HLS on the web. We don't embed a software decoder in the apps. System PiP, AirPlay, SharePlay, captions, and battery life are worth more than client-side MPEG-2 decoding.
- Each feed publishes a **master playlist** of independent renditions, started lazily and stopped when unused:
  - `direct`: H.264 channels remuxed without transcoding, with AC-3 5.1 passed through. For Apple clients.
  - `hevc-1080`: hardware HEVC for MPEG-2 channels on Apple clients.
  - `h264-1080`, `h264-720`, `h264-540`: for browsers, cellular, and weak Wi-Fi.
- Deinterlacing modes (Broadcast 60p, Smooth, Film 24p) are part of the transcoded rendition key.
- Clients send a capability profile (codecs, audio, display, network) and the server chooses the starting rendition. It reports the reason in `streamInfo`, and a user override always wins.
- Segments keep `EXT-X-PROGRAM-DATE-TIME` and aligned keyframes across renditions so switching and syncing are seamless.
- Captions (CEA-608/708) are preserved in every rendition.

## Consequences

Direct play costs almost nothing on the server and gives Apple TV the original picture and surround sound. Transcoding is reserved for MPEG-2 and constrained networks, and one viewer's choice never disturbs another's.
