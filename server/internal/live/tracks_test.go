package live

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/store"
)

var (
	engMain  = AudioTrack{PID: 0x101, Language: "eng", Role: "main", Codec: "ac3", Label: "English", Channels: 6, Measured: true}
	spaExtra = AudioTrack{PID: 0x102, Language: "spa", Role: "language", Codec: "ac3", Label: "Spanish", Channels: 2, Measured: true}
)

// holdFFmpeg stays up and records each start's arguments.
func holdFFmpeg(t *testing.T) (*Hub, *feed, string) {
	t.Helper()
	return quietFFmpeg(t, "none")
}

// quietFFmpeg exits at once when it is asked to map pid, as ffmpeg does for a
// listed stream it never found packets for. Without that map it stays up.
// "any" exits on every start. The feed reads a tuner's pipe, as in production.
func quietFFmpeg(t *testing.T, pid string) (*Hub, *feed, string) {
	t.Helper()
	dir := t.TempDir()
	mark := filepath.Join(dir, "mark")
	h, m := testHub(t)
	h.FFmpeg = filepath.Join(dir, "ffmpeg")
	h.Encoder = "h264_vaapi"
	h.FallbackWindow = time.Hour
	h.Alternates = true
	h.mu.Lock()
	f := h.addFeedLocked(m, store.SourceChannel{
		Channel:    store.Channel{ID: 1, GuideNumber: "4.1"},
		FieldOrder: "progressive",
	})
	f.source = Source{VideoCodec: "MPEG2", Progressive: true}
	h.mu.Unlock()
	t.Cleanup(func() {
		h.mu.Lock()
		if live := h.channels[1]; live != nil {
			h.stopFeedLocked(live)
		}
		h.mu.Unlock()
	})
	script := "#!/bin/sh\nexec >/dev/null 2>&1\nmark=" + `"` + mark + `"` + `
n=0
if [ -f "$mark.count" ]; then n=$(cat "$mark.count"); fi
n=$((n+1))
echo "$n" > "$mark.count"
printf '%s\n' "$@" > "$mark.args.$n"
for a in "$@"; do
	if [ "$a" = "` + pid + `" ] || [ "` + pid + `" = any ]; then exit 1; fi
done
exec sleep 60
`
	if err := os.WriteFile(h.FFmpeg, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return h, f, mark
}

func settledCount(t *testing.T, mark string) int {
	t.Helper()
	time.Sleep(300 * time.Millisecond)
	return markCount(t, mark)
}

func TestLaterTracksDoNotRestartTheEncode(t *testing.T) {
	h, f, mark := holdFFmpeg(t)
	startRendition(t, h, f, "1080.aac2.broadcast")
	first := waitMark(t, mark, ".args.1")
	if strings.Contains(first, "0:i:") {
		t.Fatalf("no tracks are known yet:\n%s", first)
	}
	h.applyDeferredAudio(f, 1, []AudioTrack{engMain, spaExtra})
	if n := settledCount(t, mark); n != 1 {
		t.Fatalf("a new sound track restarted the encode: %d starts", n)
	}
}

func TestAMainSecondRebuildsOnceWithTheOtherTrack(t *testing.T) {
	h, f, mark := holdFFmpeg(t)
	startRendition(t, h, f, "1080.aac2.broadcast")
	waitMark(t, mark, ".args.1")
	spa := spaExtra
	h.applyDeferredAudio(f, 1, []AudioTrack{spa, engMain})
	args := waitMark(t, mark, ".args.2")
	if !strings.Contains(args, "-map\n0:i:257\n-map\n0:i:258\n") {
		t.Fatalf("rebuild should map the main then the other track:\n%s", args)
	}
	if n := settledCount(t, mark); n != 2 {
		t.Fatalf("starts = %d, want the one rebuild", n)
	}
}

func TestStoredTracksGoInTheFirstEncode(t *testing.T) {
	h, f, mark := holdFFmpeg(t)
	h.mu.Lock()
	f.stored = []AudioTrack{engMain, spaExtra}
	h.mu.Unlock()
	startRendition(t, h, f, "1080.aac2.broadcast")
	args := waitMark(t, mark, ".args.1")
	if !strings.Contains(args, "-map\n0:a:0\n-map\n0:i:258\n") {
		t.Fatalf("stored second track should follow the main:\n%s", args)
	}
	// The PMT agrees with what was stored, so nothing restarts.
	h.applyDeferredAudio(f, 1, []AudioTrack{engMain, spaExtra})
	if n := settledCount(t, mark); n != 1 {
		t.Fatalf("starts = %d", n)
	}
}

func TestAQuietTrackRestartsWithoutTheExtras(t *testing.T) {
	h, f, mark := quietFFmpeg(t, "0:i:258")
	h.mu.Lock()
	f.stored = []AudioTrack{engMain, spaExtra}
	h.mu.Unlock()
	startRendition(t, h, f, "1080.aac2.broadcast")
	args := waitMark(t, mark, ".args.2")
	if strings.Contains(args, "0:i:258") {
		t.Fatalf("the restart still maps the quiet track:\n%s", args)
	}
	if !strings.Contains(args, "h264_vaapi") {
		t.Fatalf("dropping a track is not a GPU failure:\n%s", args)
	}
	if n := settledCount(t, mark); n != 2 {
		t.Fatalf("starts = %d", n)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	r := f.renditions["1080.aac2.broadcast"]
	if r == nil || h.channels[1] == nil {
		t.Fatal("the picture should keep playing")
	}
	if len(r.extras) != 0 || r.restarted || r.fallback {
		t.Fatalf("extras=%v restarted=%v fallback=%v", r.extras, r.restarted, r.fallback)
	}
	// The next encode on this tune does not try the track again.
	next, _ := ParseRenditionKey("720.aac2.broadcast")
	if got := h.extrasLocked(f, next); len(got) != 0 {
		t.Fatalf("a second encode carries %v", got)
	}
}

func TestAPlainStartThatDiesStillFallsBackToSoftware(t *testing.T) {
	h, f, mark := quietFFmpeg(t, "any")
	h.mu.Lock()
	f.stored = []AudioTrack{engMain, spaExtra}
	h.mu.Unlock()
	startRendition(t, h, f, "1080.aac2.broadcast")
	args := waitMark(t, mark, ".args.3")
	if !strings.Contains(args, "libx264") || strings.Contains(args, "0:i:258") {
		t.Fatalf("third start should be software without the track:\n%s", args)
	}
	if n := settledCount(t, mark); n != 3 {
		t.Fatalf("starts = %d", n)
	}
}

func TestASoftwareEncodeDropsTheTrackWithoutAnExtraStart(t *testing.T) {
	h, f, mark := quietFFmpeg(t, "any")
	h.mu.Lock()
	h.Encoder = "libx264"
	f.stored = []AudioTrack{engMain, spaExtra}
	h.mu.Unlock()
	startRendition(t, h, f, "1080.aac2.broadcast")
	waitMark(t, mark, ".args.2")
	if n := settledCount(t, mark); n != 2 {
		t.Fatalf("starts = %d, want with the track then without it", n)
	}
}

func TestARenumberedTrackRebuildsOnThePMT(t *testing.T) {
	h, f, mark := holdFFmpeg(t)
	h.mu.Lock()
	f.stored = []AudioTrack{engMain, spaExtra}
	h.mu.Unlock()
	startRendition(t, h, f, "1080.aac2.broadcast")
	waitMark(t, mark, ".args.1")
	eng, spa := engMain, spaExtra
	eng.PID, spa.PID = 0x102, 0x101
	h.applyDeferredAudio(f, 1, []AudioTrack{eng, spa})
	args := waitMark(t, mark, ".args.2")
	if !strings.Contains(args, "-map\n0:a:0\n-map\n0:i:257\n") {
		t.Fatalf("rebuild should carry Spanish at its new pid:\n%s", args)
	}
	if n := settledCount(t, mark); n != 2 {
		t.Fatalf("starts = %d", n)
	}
}

func TestAlternatesOffKeepsOneSound(t *testing.T) {
	h, f, mark := holdFFmpeg(t)
	h.mu.Lock()
	h.Alternates = false
	f.stored = []AudioTrack{engMain, spaExtra}
	h.mu.Unlock()
	startRendition(t, h, f, "1080.aac2.broadcast")
	if args := waitMark(t, mark, ".args.1"); strings.Contains(args, "0:i:") || strings.Contains(args, "max_interleave") {
		t.Fatalf("alternates are off:\n%s", args)
	}
}

func TestExtrasOnlyInFullSizeEncodes(t *testing.T) {
	h, m := testHub(t)
	h.Alternates = true
	h.mu.Lock()
	defer h.mu.Unlock()
	f := &feed{channel: store.SourceChannel{Channel: store.Channel{GuideNumber: "4.1"}, FrequencyHz: m.freq}, tracks: []AudioTrack{engMain, spaExtra}, renditions: map[string]*rendition{}}
	m.feeds["4.1"] = f
	h.channels[1] = f
	for _, tc := range []struct {
		key  string
		want int
	}{
		{"1080.aac2.broadcast", 1},
		{"copy.copy", 1},
		{"720.aac6.broadcast", 1},
		{"540.aac2.broadcast", 0},
		{"360.aac2.broadcast.60", 0},
		{"540.none.broadcast", 0},
	} {
		spec, _ := ParseRenditionKey(tc.key)
		if got := h.extrasLocked(f, spec); len(got) != tc.want {
			t.Errorf("%s: extras %v", tc.key, got)
		}
	}
	lang, _ := ParseRenditionKey("1080.aac2.broadcast.lang")
	if got := h.extrasLocked(f, lang); len(got) != 1 || got[0].PID != engMain.PID {
		t.Errorf("a Spanish watch should carry the English main: %v", got)
	}
	unmeasured := spaExtra
	unmeasured.Measured = false
	f.tracks = []AudioTrack{engMain, unmeasured}
	spec, _ := ParseRenditionKey("1080.aac2.broadcast")
	if got := h.extrasLocked(f, spec); len(got) != 0 {
		t.Errorf("an unmeasured track went in: %v", got)
	}
	f.stored = []AudioTrack{engMain, spaExtra}
	if got := h.extrasLocked(f, spec); len(got) != 1 || got[0].Channels != 2 {
		t.Errorf("the last tune's measure should vouch for it: %v", got)
	}
	m.input = "http://example/live.m3u8"
	if got := h.extrasLocked(f, spec); len(got) != 0 {
		t.Errorf("a URL source has no PIDs to map: %v", got)
	}
	m.input = ""
	h.Alternates = false
	if got := h.extrasLocked(f, spec); len(got) != 0 {
		t.Errorf("alternates are off: %v", got)
	}
}

func tracksStore(t *testing.T) (*store.Store, int64) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	dev := hdhr.Device{DeviceID: "dev-1", FriendlyName: "Test", BaseURL: "http://tuner"}
	if err := st.UpsertDevice(ctx, dev, []hdhr.Channel{{GuideNumber: "4.1", GuideName: "KBWV"}}); err != nil {
		t.Fatal(err)
	}
	channels, err := st.Channels(ctx, false)
	if err != nil || len(channels) != 1 {
		t.Fatalf("%+v %v", channels, err)
	}
	return st, channels[0].ID
}

// tuneAndStore adds a feed whose mux carries raw and waits for what the tune
// stores. It returns once n tracks are stored or the time runs out.
func tuneAndStore(t *testing.T, raw []byte, n int) []AudioTrack {
	t.Helper()
	ctx := context.Background()
	h, m := testHub(t)
	st, id := tracksStore(t)
	h.Store = st
	m.picMu.Lock()
	m.picBuf = raw
	m.picMu.Unlock()
	h.mu.Lock()
	f := h.addFeedLocked(m, store.SourceChannel{
		Channel:    store.Channel{ID: id, GuideNumber: "4.1"},
		FieldOrder: "progressive",
		ProgramNum: 1,
	})
	h.mu.Unlock()
	t.Cleanup(func() {
		h.mu.Lock()
		h.stopFeedLocked(f)
		h.mu.Unlock()
	})
	var got []AudioTrack
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		ch, err := st.SourceChannel(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if got = loadTracks(ch.AudioTracks); len(got) == n {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	return got
}

// nextTuneExtras is what a tune's first surround encode carries when only
// stored is known.
func nextTuneExtras(t *testing.T, stored []AudioTrack) []AudioTrack {
	t.Helper()
	h, m := testHub(t)
	h.Alternates = true
	f := &feed{channel: store.SourceChannel{FrequencyHz: m.freq}, stored: stored, renditions: map[string]*rendition{}}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.extrasLocked(f, Rendition{Video: "1080", Audio: "aac6"})
}

func TestATuneStoresItsMeasuredAudio(t *testing.T) {
	raw := audioTS(1, []esAudio{
		{pid: 0x101, lang: "eng", audioType: 0, bsmod: 0},
		{pid: 0x102, lang: "spa", audioType: 0, bsmod: 0},
	})
	raw = append(raw, ac3Packets(0x101, ac3Header(8, 0, 7, 1), 4)...)
	raw = append(raw, ac3Packets(0x102, ac3Header(8, 0, 2, 0), 4)...)
	got := tuneAndStore(t, raw, 2)
	if len(got) != 2 || got[0].PID != 0x101 || got[0].Channels != 6 || got[1].PID != 0x102 || got[1].Channels != 2 || !got[1].Measured {
		t.Fatalf("stored %+v", got)
	}
	if extras := nextTuneExtras(t, got); len(extras) != 1 || extras[0].PID != 0x102 {
		t.Fatalf("the next tune should carry the stored track: %v", extras)
	}
}

func TestAQuietTrackIsStoredUnmeasured(t *testing.T) {
	window := trackWindow
	trackWindow = 300 * time.Millisecond
	t.Cleanup(func() { trackWindow = window })
	raw := audioTS(1, []esAudio{
		{pid: 0x101, lang: "eng", audioType: 0, bsmod: 0},
		{pid: 0x102, lang: "spa", audioType: 0, bsmod: 0},
	})
	raw = append(raw, ac3Packets(0x101, ac3Header(8, 0, 7, 1), 4)...)
	got := tuneAndStore(t, raw, 2)
	if len(got) != 2 || !got[0].Measured || got[1].Measured {
		t.Fatalf("stored %+v", got)
	}
	if extras := nextTuneExtras(t, got); len(extras) != 0 {
		t.Fatalf("a track that sent nothing still goes in: %v", extras)
	}
}
