package live

import (
	"bytes"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"broadwave/internal/logbuf"
)

func TestFFmpegProgressDropsAPassword(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "ffmpeg")
	script := "#!/bin/sh\necho 'Error opening input http://user:secretpass@playlist.example/a.ts' >&2\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	prevLog := log.Writer()
	prevSlog := slog.Default()
	t.Cleanup(func() {
		log.SetOutput(prevLog)
		slog.SetDefault(prevSlog)
	})
	var buf bytes.Buffer
	logbuf.Install(&buf)
	cmd := exec.Command(bin)
	logCommand(cmd, "test")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "secretpass") || !strings.Contains(buf.String(), "playlist.example") {
		t.Fatalf("log %s", buf.String())
	}
}

// The lines ffmpeg writes when a copied AC-3 track has no rate are a sound
// failure, not a GPU one.
var soundHeaderDeath = []string{
	"[mpegts @ 0x5573] Could not find codec parameters for stream 1 (Audio: ac3 (AC-3 / 0x332D4341), 0 channels, fltp): unspecified sample rate",
	"Consider increasing the value for the 'analyzeduration' (500000) and 'probesize' (8000000) options",
	"[mp4 @ 0x5573] sample rate not set",
	"[out#0/mp4 @ 0x5573] Could not write header (incorrect codec parameters ?): Invalid argument",
	"[vf#0:0 @ 0x5573] Error sending frames to consumers: Invalid argument",
	"[vf#0:0 @ 0x5573] Task finished with error code: -22 (Invalid argument)",
}

func TestCauseOfAnEncodeDeath(t *testing.T) {
	cases := []struct {
		name string
		tail []string
		want exitCause
	}{
		{"sound header", soundHeaderDeath, causeParams},
		{"vaapi open", []string{"[AVHWDeviceContext @ 0x1] Failed to initialise VAAPI connection: -1 (unknown libva error).", "Device creation failed: -5."}, causeGPU},
		{"vaapi encoder", []string{"[h264_vaapi @ 0x1] Failed to create surface: 2 (resource allocation failed)."}, causeGPU},
		{"nvenc", []string{"[h264_nvenc @ 0x1] OpenEncodeSessionEx failed: out of memory (10)"}, causeGPU},
		{"decoder chatter is not the GPU", []string{"[h264 @ 0x1] mmco: unref short failure", "[hevc_vaapi @ 0x2] Driver does not support some wanted packed headers"}, causeUnknown},
		{"nothing said", nil, causeUnknown},
		{"a copy's usual warning", []string{"[mp4 @ 0x1] track 1: codec frame size is not set"}, causeUnknown},
	}
	for _, c := range cases {
		if got := causeOf(c.tail); got != c.want {
			t.Errorf("%s: cause %s, want %s", c.name, got, c.want)
		}
	}
}

func TestFFmpegLinesGetALevel(t *testing.T) {
	for line, want := range map[string]slog.Level{
		"[h264 @ 0x1] mmco: unref short failure":                                                         slog.LevelDebug,
		"    Last message repeated 1 times":                                                              slog.LevelDebug,
		"x265 [info]: HEVC encoder version 3.5":                                                          slog.LevelDebug,
		"[mpegts @ 0x1] PES packet size mismatch":                                                        slog.LevelDebug,
		"av_interleaved_write_frame(): Broken pipe":                                                      slog.LevelDebug,
		"[mp4 @ 0x1] sample rate not set":                                                                slog.LevelWarn,
		"[out#0/mp4 @ 0x1] Could not write header (incorrect codec parameters ?)":                        slog.LevelWarn,
		"[AVHWDeviceContext @ 0x1] Failed to initialise VAAPI connection: -1":                            slog.LevelWarn,
		"Error opening input http://example/a.ts":                                                        slog.LevelWarn,
		"[hevc_vaapi @ 0x2] Driver does not support some wanted packed headers (wanted 0xd, found 0x1).": slog.LevelDebug,
	} {
		if got := ffmpegLevel(line); got != want {
			t.Errorf("%q: %s, want %s", line, got, want)
		}
	}
}

// A decoder on a weak signal complains many times a second. One process
// gets a few warnings each window, tagged with what it is for; the rest are
// counted, and every line stays in its tail.
func TestProcessLogIsTaggedAndRateLimited(t *testing.T) {
	prevSlog := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prevSlog) })
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	l := &procLog{tag: "ffmpeg 9.4 1080.copy.broadcast.hevc"}
	for i := range 40 {
		fmt.Fprintf(l, "[mp4 @ 0x1] sample rate not set %d\n", i)
	}
	fmt.Fprint(l, "[h264 @ 0x1] mmco: unref short failure\n")
	out := buf.String()
	if n := strings.Count(out, "sample rate not set"); n != procLogBurst {
		t.Fatalf("%d warnings logged, want %d:\n%s", n, procLogBurst, out)
	}
	if !strings.Contains(out, "ffmpeg 9.4 1080.copy.broadcast.hevc: [mp4") || !strings.Contains(out, "level=WARN") {
		t.Fatalf("lines are not tagged warnings:\n%s", out)
	}
	if strings.Contains(out, "mmco") {
		t.Fatalf("decoder chatter reached the default log:\n%s", out)
	}
	tail := l.Tail()
	if len(tail) != procTail || !strings.Contains(tail[len(tail)-1], "mmco") {
		t.Fatalf("tail %d lines, last %q", len(tail), tail[len(tail)-1])
	}
	// A line split across writes is one line.
	fmt.Fprint(l, "[out#0/mp4 @ 0x1] Could not write ")
	fmt.Fprint(l, "header\n")
	if got := l.Tail()[procTail-1]; got != "[out#0/mp4 @ 0x1] Could not write header" {
		t.Fatalf("split line %q", got)
	}
}
