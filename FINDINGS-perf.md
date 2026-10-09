# Perf lane findings

## J0.74 field-doubled encodes write two keyframes per group

Blocked on this lane. Stock ffmpeg cannot keep one IDR per source group of pictures on a field-doubled encode without drifting off the broadcast clock. The double keyframe is reproduced below. No Broadwave argv change fixes it.

### What happens

`bwdif=mode=send_field` (and `yadif`, `w3fdif`, and `estdif` the same way) copies `AV_FRAME_FLAG_KEY` onto both output fields. `return_frame` in `libavfilter/yadif_common.c` calls `av_frame_copy_props` for the second field and clears only `AV_FRAME_FLAG_INTERLACED`.

Live renditions then pass `-force_key_frames source` (`openingKeyframes` in `server/internal/live/rendition.go`). In ffmpeg 8.0.1 `forced_kf_apply` (`fftools/ffmpeg_enc.c`) that mode sets `pict_type` to I whenever the flag is set, and sets every other frame's `pict_type` to none. libx264 turns pict_type I into an IDR. The second field is one field later (16.683 ms at 60000/1001).

Copy and transcode are meant to close on the same broadcast frames. A fixed interval (`expr:gte(t,n_forced*2)` and the same family) drifts off the source group, which is why the live path uses `source`. That comment still holds. Do not switch field-doubled encodes to a fixed interval.

`-g 600` is only a ceiling. `-keyint_min` does not suppress an IDR that ffmpeg forced. Without `-force_key_frames`, the same graph writes a single opening I-frame in a 2 second clip, because `forced_kf_apply` clears pict_type when it is not forcing. The key flag is the whole signal.

VAAPI `deinterlace_vaapi=rate=field` was not measured here (no device). A software filter inserted only on the bwdif branch would not change that graph.

### Proof

ffmpeg 8.0.1, libx264, synthetic interlaced MPEG-2 (lavfi `testsrc2`, `tinterlace=mode=interleave_top`, `-g 15 -bf 2 -sc_threshold 1000000000 -flags +ildct+ilme`). After

`bwdif=mode=send_field:parity=auto:deint=interlaced,fps=60000/1001,format=yuv420p`

with `-force_key_frames source -g 600 -sc_threshold 0`, ffprobe keyframes land in pairs:

- 0.000000 and 0.016683
- 0.500500 and 0.517183
- 1.001000 and 1.017683
- 1.501500 and 1.518183
- 1.968633 and 1.985317

About 120 frames, `r_frame_rate` 60000/1001. The packager already holds a 17 ms keyframe fragment so a player does not see an empty audio part (`TestNoPartIsOneField`). That workaround is a symptom. Leave it in place.

### Approaches that do not work

No filter in current libavfilter clears `AV_FRAME_FLAG_KEY`. The flag is only read, or set by sources (`testsrc`, gradients). A sweep of stock filters (scale, fps, geq, lut, tblend, framerate, minterpolate, and others) kept the flag on a frame that arrived with it. `tblend` holds the first frame and copies props from the current frame; an `iskey:1` then `iskey:0` reading was a one-frame shift, not a cleared flag.

Dropping the second key field with `select='not(eq(key,1)*mod(n,2))'` removes it until `fps` or `framerate` runs. Those filters clone the remaining keyframe into the 16.7 ms hole, and the encode has the pairs again. Closing the hole with `setpts` runs the transcode clock fast against the copy rendition (one field per source group). `minterpolate=mi_mode=dup` either keeps a shifted pair or, after `send_frame`, clears every key flag and duplicates pictures. `send_frame` plus a rate doubler is 29.97 repeated, not a field-rate picture.

`-force_key_frames` expression variables are only `n`, `n_forced`, `prev_forced_n`, `prev_forced_t`, and `t`. There is no `key`, and source mode cannot be combined with a minimum gap. `scd_metadata` (force a key when `lavfi.scd.time` is set) is on ffmpeg master and would allow a split/select/metadata graph to tag only the first field. ffmpeg 8.0.1 rejects that mode (`Invalid keyframe time: scd_metadata`). The release image is jellyfin-ffmpeg 7.1, which is older. `metadata`'s `enable` expression cannot see `key`, so it cannot tag only the first field of a source key for any mode this ffmpeg implements.

A bitstream rewrite of the second IDR into a non-IDR slice would still be an intra frame, and it would be a different parser for libx264 and VideoToolbox. Not a fix.

### What would actually fix it

One line in ffmpeg, not in this repo: in `return_frame`, when `is_second` is set, clear `AV_FRAME_FLAG_KEY` on the output frame. `-force_key_frames source` would then IDR the first field of each source key and leave the second field unforced. Frame count and timestamps stay field-rate, and the cut still lands on the source key. The same clear belongs on any other field-rate deinterlacer that copies props onto both fields.
