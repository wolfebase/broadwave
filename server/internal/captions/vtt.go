package captions

import (
	"fmt"
	"strings"
)

// Segment is the WebVTT for one video segment starting at start (its first
// presentation time, 90 kHz) and lasting dur ticks. It holds every cue on
// screen in that span, clipped to it; a cue that runs across segments is
// repeated in each one. A span with no captions is just the header, which
// players need to see.
func Segment(start, dur int64, cues []Cue) []byte {
	var b strings.Builder
	// The map ties cue time 0 to the segment's own timestamp, so players
	// place the cues on the picture's timeline, not on the playlist's.
	fmt.Fprintf(&b, "WEBVTT\nX-TIMESTAMP-MAP=LOCAL:00:00:00.000,MPEGTS:%d\n", start&(1<<33-1))
	for _, c := range cues {
		from := max(ptsDelta(c.Start, start), 0)
		to := min(ptsDelta(c.End, start), dur)
		if to <= from {
			continue
		}
		b.WriteString("\n")
		b.WriteString(stamp(from) + " --> " + stamp(to))
		if c.Top {
			b.WriteString(" line:10%")
		}
		b.WriteString("\n" + escape(c.Text) + "\n")
	}
	return []byte(b.String())
}

func stamp(ticks int64) string {
	ms := ticks / 90
	return fmt.Sprintf("%02d:%02d:%02d.%03d", ms/3_600_000, ms/60_000%60, ms/1000%60, ms%1000)
}

var escaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func escape(s string) string {
	return escaper.Replace(s)
}
