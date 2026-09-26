package live

import (
	"os"
	"os/exec"
	"testing"

	"broadwave/internal/store"
)

func TestFeedStatsCountsViewersAndProcesses(t *testing.T) {
	h := New(nil, t.TempDir(), "ffmpeg", "libx264")
	running := &exec.Cmd{Process: &os.Process{Pid: os.Getpid()}}
	waited := &rendition{viewers: 4, cmd: running}
	waited.waited.Store(true)
	feed := &feed{
		channel: store.SourceChannel{Channel: store.Channel{ID: 7, GuideNumber: "4.1", DisplayName: "KMBC"}},
		renditions: map[string]*rendition{
			"a": {viewers: 2, cmd: running},
			"b": {viewers: 1},
			"c": waited,
		},
		recording: &recording{cmd: running},
		exports:   3,
	}
	h.channels[7] = feed
	h.channels[8] = feed
	got := h.FeedStats()
	if len(got) != 1 {
		t.Fatalf("feeds %d", len(got))
	}
	st := got[0]
	if st.ChannelID != 7 || st.GuideNumber != "4.1" || st.Name != "KMBC" || st.Viewers != 7 || st.FFmpeg != 2 || !st.Recording || st.Exports != 3 {
		t.Fatalf("%+v", st)
	}
	if len(h.FeedStats()) != 1 {
		t.Fatal("second read changed")
	}
	if stats := (*Hub)(nil).FeedStats(); len(stats) != 0 {
		t.Fatalf("%+v", stats)
	}
}
