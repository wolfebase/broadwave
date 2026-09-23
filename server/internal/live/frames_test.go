package live

import (
	"strings"
	"testing"
	"time"
)

func TestFrameSchedule(t *testing.T) {
	now := time.Date(2026, 9, 23, 15, 0, 0, 0, time.UTC)
	if !FrameDue(time.Time{}, now) {
		t.Fatal("the first sample is due immediately")
	}
	if FrameDue(now.Add(-time.Minute), now.Add(-time.Second)) {
		t.Fatal("a fresh sample is not due again")
	}
	if !FrameDue(now.Add(-FrameEvery), now) {
		t.Fatal("a sample is due after a minute")
	}
	if !FrameStale(time.Time{}, now) {
		t.Fatal("a missing frame is stale")
	}
	if FrameStale(now.Add(-9*time.Minute), now) {
		t.Fatal("nine minutes is still current")
	}
	if !FrameStale(now.Add(-10*time.Minute-time.Second), now) {
		t.Fatal("older than ten minutes is stale")
	}
}

func TestFrameArgsSampleTheOpenMux(t *testing.T) {
	args := FrameArgs([]FrameJob{{ChannelID: 4, Program: 3}, {ChannelID: 5, Program: 4}}, "/work/frames")
	text := strings.Join(args, " ")
	if !strings.Contains(text, "-skip_frame nokey") || !strings.Contains(text, "-i pipe:0") {
		t.Fatalf("args = %s", text)
	}
	if strings.Contains(text, "http") || strings.Contains(text, "tuner") {
		t.Fatal("a preview must not open its own tune")
	}
	if !strings.Contains(text, "0:p:3:v:0") || !strings.Contains(text, "0:p:4:v:0") {
		t.Fatal("each program on the mux needs a frame")
	}
	if !strings.Contains(text, "scale=480:-2") || !strings.Contains(text, "scale=1280:-2") {
		t.Fatal("both sizes are written")
	}
	if !strings.Contains(text, "/work/frames/4.jpg.part") || !strings.Contains(text, "/work/frames/5-1280.jpg.part") {
		t.Fatalf("paths = %s", text)
	}
}
