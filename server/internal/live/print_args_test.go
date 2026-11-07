package live

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestPrintPlaybackArgs writes the ffmpeg arguments of recording playback as a
// shell case, for a check script run where the real hardware is. Nothing is
// copied by hand: BROADWAVE_PRINT_ARGS=1 go test ./server/internal/live -run PrintPlaybackArgs -v
func TestPrintPlaybackArgs(t *testing.T) {
	if os.Getenv("BROADWAVE_PRINT_ARGS") == "" {
		t.Skip("set BROADWAVE_PRINT_ARGS=1 to print")
	}
	const seek, carry = 12345.678, 98765.432
	word := func(w string) string {
		switch w {
		case "12345.678":
			return `"$SEEK"`
		case "98765.432":
			return `"$CARRY"`
		case "INPUT":
			return `"$INPUT"`
		}
		return "'" + strings.ReplaceAll(w, "'", `'\''`) + "'"
	}
	type config struct {
		name, encoder, codec, mode, order, deint, smooth string
	}
	configs := []config{
		{"software-mpeg2p", "libx264", "MPEG2", "broadcast", "progressive", "", ""},
		{"software-mpeg2i", "libx264", "MPEG2", "broadcast", "", "", ""},
		{"vaapi-mpeg2p", "h264_vaapi", "MPEG2", "broadcast", "progressive", "motion_adaptive", "motion_compensated"},
		// The default: ProbeDeint found motion adaptive.
		{"vaapi-mpeg2i-adaptive", "h264_vaapi", "MPEG2", "broadcast", "", "motion_adaptive", "motion_compensated"},
		// The smooth picture mode, where motion compensated exists.
		{"vaapi-mpeg2i-compensated", "h264_vaapi", "MPEG2", "smooth", "", "motion_adaptive", "motion_compensated"},
		// ProbeDeint found neither.
		{"vaapi-mpeg2i-nodeint", "h264_vaapi", "MPEG2", "broadcast", "", "", ""},
		{"vaapi-h264i", "h264_vaapi", "H264", "broadcast", "tt", "motion_adaptive", "motion_compensated"},
	}
	line := func(name, variant string, g Graph) {
		words := PictureArgs(g)
		for i, w := range words {
			words[i] = word(w)
		}
		fmt.Printf("\t%s/%s) exec \"$FF\" %s ;;\n", name, variant, strings.Join(words, " "))
	}
	prev := lowPower
	defer func() { lowPower = prev }()
	for _, power := range []bool{false, true} {
		lowPower = map[string]bool{"h264_vaapi": power}
		for _, c := range configs {
			name := c.name
			if power {
				if c.encoder != "h264_vaapi" {
					continue
				}
				name += "-lowpower"
			}
			h := &Hub{Encoder: c.encoder, DeintBroadcast: c.deint, DeintSmooth: c.smooth}
			g := h.fileGraph(c.codec, c.mode, c.order)
			g.Live = false

			g.Input = "INPUT"
			line(name, "first", g)
			g.Start, g.Carry = seek, carry
			line(name, "carry", g)

			g.Start, g.Carry = 0, 0
			g.Input = "pipe:0"
			line(name, "pipefirst", g)
			g.Carry = carry
			line(name, "pipecarry", g)
		}
	}
}
