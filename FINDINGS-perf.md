# Perf lane findings

## J0.74 field-doubled encodes write two keyframes per group

Fixed on the software field-rate path (`weaveFieldKey` in `server/internal/live/picture.go`). `bwdif=mode=send_field` still copies the source key flag onto both fields, and no stock filter clears that flag. The live graph keeps a second branch that drops the odd field of each key pair, fills the hole with `minterpolate=mi_mode=dup` (the filler does not inherit the flag), and overlays the real field pictures back on. `-force_key_frames source` then writes one IDR per source group. Pictures and timestamps stay on the field clock. The remaining flag sits one field (16.7 ms) before the source key. A plain `fps` filter is not appended: it clones the last key to finish a finite input, and the pair comes back at the tail.

Not changed: VAAPI `deinterlace_vaapi=rate=field` (no device here to measure it), and `PictureArgs`, which forces keyframes on a clock (`expr:gte(t,n_forced*2)`) instead of `source`. A finite encode ends about two frames short because the hole at the tail is not padded. The packager's 17 ms minimum part stays, because a VAAPI field deinterlace can still emit the pair.

### What was wrong

`bwdif=mode=send_field` (and `yadif`, `w3fdif`, and `estdif` the same way) copies `AV_FRAME_FLAG_KEY` onto both output fields. `return_frame` in `libavfilter/yadif_common.c` calls `av_frame_copy_props` for the second field and clears only `AV_FRAME_FLAG_INTERLACED`.

Live renditions pass `-force_key_frames source` (`openingKeyframes` in `server/internal/live/rendition.go`). In ffmpeg 8.0.1 `forced_kf_apply` sets `pict_type` to I whenever the flag is set, and sets every other frame's `pict_type` to none. libx264 turns pict_type I into an IDR. The second field is one field later (16.683 ms at 60000/1001).

Copy and transcode are meant to close on the same broadcast frames. A fixed interval (`expr:gte(t,n_forced*2)`) drifts off the source group, which is why the live path uses `source`. `-g 600` is only a ceiling. `-keyint_min` does not suppress an IDR that ffmpeg forced.

On a synthetic interlaced MPEG-2 (lavfi `testsrc2`, `tinterlace`, `-g 15`) the old graph

`bwdif=mode=send_field:parity=auto:deint=interlaced,fps=60000/1001,format=yuv420p`

with `-force_key_frames source -g 600 -sc_threshold 0` wrote pairs at 0 / 0.016683, 0.500500 / 0.517183, 1.001000 / 1.017683, 1.501500 / 1.518183, and 1.968633 / 1.985317.

### What the graph does

`renditionArgs` builds this when the filter is `bwdif=mode=send_field` and the encode uses `-force_key_frames source`:

```
[0:v:0]bwdif=mode=send_field:parity=auto:deint=interlaced,split[pix][kf];
[kf]select='not(eq(key,1)*eq(mod(n,2),1))',minterpolate=fps=60000/1001:mi_mode=dup:scd=none[flags];
[flags][pix]overlay=eof_action=pass:shortest=1,scale=…,setsar=1,format=yuv420p[v]
```

Commas inside the select expression are escaped (`\,`), the same way `halfRate` escapes its select. `send_field` emits the two fields as an even frame then an odd one, so the select drops the second key field. `minterpolate` fills that hole and does not copy the flag onto the filler, which moves the surviving flag one field earlier. `overlay` puts the real bwdif pictures back on those timestamps. showinfo against plain bwdif matched every checksum. Opening frame 0 is not flagged; libx264 still emits the opening IDR.

`scd_metadata` would tag one field, but ffmpeg 8.0.1 rejects it, and the release image is jellyfin-ffmpeg 7.1. Rewriting the second IDR in the bitstream is not safe (the slice still needs `idr_pic_id` removed, `frame_num` renumbered, and DPB refs fixed). The same graph covers VideoToolbox, which deinterlaces in software and then encodes with `h264_videotoolbox`.

### Still open

VAAPI field rate was not measured. Mosaic uses `yadif` with a fixed keyframe interval, not `source`. A progressive frame inside `deint=interlaced` flips field parity, so `mod(n,2)` can drop the other field of that pair; the group still has one key. The upstream fix is still one line in `return_frame`: when `is_second` is set, clear `AV_FRAME_FLAG_KEY`. That would let this graph go away.
