# Demuxed alternates from one encode

Accepted, 2026-09-28.

## Context

A live rendition used to carry one sound track. Choosing another language, or described video, started a second encode and a new watch. A player that can switch sound in place needs those tracks as alternates of one playlist, not as a second copy of the picture.

This sits beside the playback pipeline decision. That decision still picks the picture and the audio codec. This one is only how the other measured mixes of the same program are carried.

## Options

1. **Keep one sound per encode and list the other encodes as extra variants.** AVPlayer builds its audio menu from audio alternates, not from variants, so it cannot pick one of those encodes. hls.js could change level without a gap, but every language is a full video encode, it has to be running already, and a late start has none of the media the room is already playing, so the switch stalls. Rejected.

2. **Run a separate audio-only encode for each extra track.** The main encode stays as it is, and a silent stream cannot take the picture down. Audio-only fragmented MP4 has no keyframes, so it does not cut on the picture's groups. The packager would have to cut that audio on another process's timestamps, keep the sequence numbers aligned, follow restarts and timestamp breaks, and have enough media to cover the room. Rejected for now.

3. **One encode carries every measured sound track, and the server cuts a view per track when a player asks.** The files on disk stay one fragmented MP4. Each view is the stamped index with its own media names, so sequence numbers, dates, parts, and blocking reload match. A switch does not rewrite the picture buffer.

## Decision

The third option.

- A full-size encode maps the main mix, then each other track that has sent a frame. The same audio treatment applies to each: a copy copies each track, and an AAC encode sets the channel count per stream so a stereo track is not widened. The muxer waits at most a second for a quiet track. A 540 or 360 picture, a silent rendition, and a playlist or URL source map one sound.
- `video.m3u8` and `audio-<id>.m3u8` are cut from the same init, segments, and parts on request. Decode times are left as written, so the views line up sample for sample. Cuts are shared across screens, and a file rewritten by a restart is a new cut. When an encode carries other tracks, the legacy names are the picture plus the main sound, so an older player never receives several.
- `master.m3u8` puts the picture and those tracks in one audio group, named from the program map. The default is that encode's main sound. A master is handed only to a watch that sends `caps.alternates`, and only for an encode that carries another sound and has no track of its own. A `.lang` or `.vi` encode would otherwise advertise the other mix as the default, so it keeps its one-sound playlist. A watch for another language or for described video that sends the cap joins the main encode when that encode carries the track, or when the channel has no such track.
- The web player fetches the master and its picture playlist before playback. hls.js loads no picture playlist from a master until it is told to start, and a join on the room's frame needs those fragments first. A playlist loader answers hls.js's first two requests with the same bodies. A later choice uses hls.js's audio option (`setAudioOption`). Assigning `hls.audioTrack` drops the buffered sound and is not used. The web player does not load the master's subtitle group. Captions stay on their own track, still derived from the stamped index.
- The iPhone and Apple TV apps send the cap for a full-screen watch and switch with AVPlayer's media selection, which keeps the picture and the buffer. A choice is kept for the next channel. A track the channel lacks plays the main mix.
- An export copies one program, every stream it carries. A recording copies that program's picture and its first sound track, still the original encoding. A clear ATSC 3.0 channel with AC-4 sound keeps the program's own packets, so every sound track and the captions stay. The multiplex kept on disk for the live buffer stays the unfiltered tune. Multiview tiles stay on the one-sound playlist.

## Consequences

Changing sound on the web, iPhone, or Apple TV does not start a new watch and does not stall the picture. A player that does not send `caps.alternates` still gets one sound, and a separate encode for another language.

The first tune of a channel has no stored track list. When the scan is already known, the encode starts at once with the main sound. Tracks measured beside it are stored, and the next tune's first encode carries them. A track learned after an encode has started does not restart it. When the scan is not known, that first tune waits, inside the same window as the picture header, for a frame from each AC-3 track before the encode starts, because nothing stored can vouch for a stream that has not sent a packet. A listed stream that never sends one is stored as unmeasured and is not mapped. An encode that dies early with extra tracks restarts once without them, and the rest of that tune stays on one sound.

An extra stereo AAC track was measured at about one percent of a core, and a 5.1 track at about three. Cutting a view is a copy of a fragment already on disk. A second language no longer takes a second picture slot.

A master here lists the sound tracks of one encode. It is not a choice of picture size, and it is not a surround-or-stereo toggle. Playing a recording builds a separate HLS playlist and does not rewrite the MPEG-TS file.
