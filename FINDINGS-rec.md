# rec lane

## A library channel still ends when its file ends

`streamVirtual` plays one recording, then the response ends. A file shorter than its guide slot drops the client when the file ends. A file longer than the slot keeps playing into the next program. Carrying the next recording in the same response means a loop that stops when the client disconnects. The export tests use an ffmpeg stand-in that exits at once, so the loop has to watch the request context or those tests spin.

## A tune with no ffmpeg sends the file from the first byte

`TestAVirtualTuneWithoutFFmpegSendsTheFile` locks that in. The schedule offset needs ffmpeg, or a walk of the transport packets. The fallback is unchanged.
