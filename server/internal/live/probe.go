package live

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

type probeReport struct {
	Programs []struct {
		ProgramNum int           `json:"program_num"`
		Streams    []probeStream `json:"streams"`
	} `json:"programs"`
	Streams []probeStream `json:"streams"`
}

type probeStream struct {
	CodecType  string `json:"codec_type"`
	FieldOrder string `json:"field_order"`
}

// fieldOrderFrom picks the video field order for one program from ffprobe JSON.
func fieldOrderFrom(raw []byte, program int) string {
	var rep probeReport
	if json.Unmarshal(raw, &rep) != nil {
		return ""
	}
	pick := func(streams []probeStream) string {
		for _, s := range streams {
			if s.CodecType == "video" && s.FieldOrder != "" && s.FieldOrder != "unknown" {
				return s.FieldOrder
			}
		}
		return ""
	}
	for _, p := range rep.Programs {
		if program == 0 || p.ProgramNum == program {
			if fo := pick(p.Streams); fo != "" {
				return fo
			}
		}
	}
	if program == 0 {
		return pick(rep.Streams)
	}
	return ""
}

// probeFieldOrderLocked learns whether a channel is progressive, which decides
// whether its picture can be delivered untouched. It reads from the shared tune.
func (h *Hub) probeFieldOrderLocked(m *mux, f *feed) {
	tool := FFProbePath(h.FFmpeg)
	if tool == "" || h.Store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	cmd := exec.CommandContext(ctx, tool, "-v", "error", "-probesize", "4000000", "-analyzeduration", "3000000",
		"-show_entries", "program=program_num:program_stream=codec_type,field_order:stream=codec_type,field_order",
		"-of", "json", "-i", "pipe:0")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return
	}
	var out strings.Builder
	cmd.Stdout = &out
	if err := cmd.Start(); err != nil {
		cancel()
		return
	}
	sub := h.attachPipeLocked(m, stdin)
	f.probing = true
	channelID, program := f.channel.ID, f.program
	go func() {
		defer cancel()
		_ = cmd.Wait()
		order := fieldOrderFrom([]byte(out.String()), program)
		if order != "" {
			_ = h.Store.SetFieldOrder(context.Background(), channelID, order)
		}
		h.mu.Lock()
		defer h.mu.Unlock()
		m.detach(sub)
		f.probing = false
		if order != "" {
			f.channel.FieldOrder = order
			f.source.Progressive = order == "progressive"
			f.source.Film = order == "film"
		}
		if h.channels[channelID] == f {
			h.dropIfUnusedLocked(f)
		}
	}()
}
