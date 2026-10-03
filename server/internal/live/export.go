package live

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"
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
	cmd := exec.CommandContext(ctx, h.FFmpeg, exportCopyArgs(f.program, ch.AudioCodec)...)
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
	sub := h.attachExportLocked(muxOf(h, f), stdin)
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

// exportHead is how much of the ring a new export starts with, so the app on
// the other end has its probe and a keyframe at once instead of waiting for
// the tuner to send them.
const exportHead = 3 * time.Second

// attachExportLocked subscribes an export from exportHead back when the ring
// holds it, else at the live edge. The caller holds h.mu; the ring's files
// are read on another goroutine.
func (h *Hub) attachExportLocked(m *mux, w io.WriteCloser) *pipeSub {
	if m != nil && m.ring != nil && m.input == "" {
		if pos, ok := m.ring.At(time.Now().Add(-exportHead)); ok {
			if oldest, _, ok := m.ring.Position(time.Time{}); ok && packetStart(pos) < oldest {
				pos += 188
			}
			sub := newPipeSub(w)
			go m.joinLive(sub, packetStart(pos))
			return sub
		}
	}
	return h.attachPipeLocked(m, w)
}

// exportCopyArgs copies one program out of the shared tune. The probe ceiling
// matches a live rendition: a one- or two-megabyte cap ends before the
// sequence header on a full multiplex, and the copy then has no video.
func exportCopyArgs(program int, audio string) []string {
	args := []string{"-hide_banner", "-loglevel", "error", "-fflags", "+genpts+discardcorrupt", "-copyts",
		"-probesize", "8000000", "-analyzeduration", "1000000"}
	args = append(append(args, captionProbe(audio)...), "-i", "pipe:0")
	if program > 0 {
		args = append(args, "-map", fmt.Sprintf("0:p:%d", program))
	} else {
		args = append(args, "-map", "0")
	}
	return append(args, "-c", "copy", "-f", "mpegts", "pipe:1")
}
