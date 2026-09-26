package live

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
)

// Export writes a channel's original broadcast as MPEG-TS to w until ctx ends.
// It rides the shared tune, so other apps can watch through this server
// without taking another tuner.
func (h *Hub) Export(ctx context.Context, channelID int64, w io.Writer) error {
	ch, err := h.Store.SourceChannel(ctx, channelID)
	if err != nil {
		return err
	}
	var res *http.Response
	if ch.TunerCount == 0 && ch.StreamURL != "" {
		h.mu.Lock()
		_, tuned := h.channels[channelID]
		h.mu.Unlock()
		if !tuned {
			if res, err = openStream(ch.StreamURL, ch.UserAgent, ch.Referrer); err != nil {
				return err
			}
		}
	}
	h.mu.Lock()
	f, err := h.ensureFeedLocked(ctx, ch, res)
	if err != nil {
		h.mu.Unlock()
		return err
	}
	cmd := exec.CommandContext(ctx, h.FFmpeg, exportCopyArgs(f.program)...)
	cmd.Stdout = w
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		h.dropIfUnusedLocked(f)
		h.mu.Unlock()
		return err
	}
	if err := cmd.Start(); err != nil {
		h.dropIfUnusedLocked(f)
		h.mu.Unlock()
		return err
	}
	sub := h.attachPipeLocked(muxOf(h, f), stdin)
	f.exports++
	h.mu.Unlock()

	err = cmd.Wait()

	h.mu.Lock()
	defer h.mu.Unlock()
	if m := muxOf(h, f); m != nil {
		m.detach(sub)
	}
	f.exports--
	h.dropIfUnusedLocked(f)
	if ctx.Err() != nil {
		return nil
	}
	return err
}

// exportCopyArgs copies one program out of the shared tune. The probe ceiling
// matches a live rendition: a one- or two-megabyte cap ends before the
// sequence header on a full multiplex, and the copy then has no video.
func exportCopyArgs(program int) []string {
	args := []string{"-hide_banner", "-loglevel", "error", "-fflags", "+genpts+discardcorrupt", "-copyts",
		"-probesize", "8000000", "-analyzeduration", "1000000", "-i", "pipe:0"}
	if program > 0 {
		args = append(args, "-map", fmt.Sprintf("0:p:%d", program))
	} else {
		args = append(args, "-map", "0")
	}
	return append(args, "-c", "copy", "-f", "mpegts", "pipe:1")
}
