package live

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/store"
)

var chromeCaps = Caps{Platform: "web", Video: []string{"h264"}, Audio: []string{"aac"}}

func TestUnknownCodecsAreNotCopied(t *testing.T) {
	safari := Caps{Platform: "web", Video: []string{"h264"}, Audio: []string{"aac", "ac3"}}
	for _, caps := range []Caps{chromeCaps, safari} {
		for _, audio := range []string{"", "surround"} {
			d := DecideFor(Source{Progressive: true}, caps, Prefs{Audio: audio}, "", Host{})
			if d.Rendition.Video == "copy" || d.Rendition.Audio == "copy" {
				t.Fatalf("%v audio %q: unknown codecs copied as %s", caps.Audio, audio, d.Rendition.Key())
			}
		}
	}
}

func TestMPEG2AndAC3AreConvertedForABrowser(t *testing.T) {
	src := Source{VideoCodec: "MPEG2", AudioCodec: "AC3", Progressive: true}
	d := DecideFor(src, chromeCaps, Prefs{}, "", Host{})
	if d.Rendition.Video != "1080" || d.Rendition.Audio != "aac2" {
		t.Fatalf("rendition %s", d.Rendition.Key())
	}
	args := RenditionArgs(0, src, d.Rendition, "libx264", "")
	for _, a := range args {
		if a == "aac_adtstoasc" {
			t.Fatalf("AC-3 got the AAC filter: %v", args)
		}
	}
}

// videoTS is audioTS with the program's video stream type swapped.
func videoTS(program, kind int, audios []esAudio) []byte {
	const pmtPID = 0x1000
	pat := psiSection(0x00, append([]byte{0x00, 0x01, 0xc1, 0x00, 0x00}, progPID(program, pmtPID)...))
	body := audioPMT(program, audios)
	body[9] = byte(kind)
	var out []byte
	out = append(out, tsPacket(0, true, pat)...)
	out = append(out, tsPacket(pmtPID, true, psiSection(0x02, body))...)
	return out
}

func TestVideoCodecOfReadsThePMT(t *testing.T) {
	audio := []esAudio{{pid: 0x101, lang: "eng", audioType: 0, bsmod: 0}}
	for _, c := range []struct {
		kind int
		want string
	}{{streamMPEG2, "MPEG2"}, {streamH264, "H264"}, {streamHEVC, "HEVC"}, {0x06, ""}} {
		if got := VideoCodecOf(videoTS(1, c.kind, audio), 1); got != c.want {
			t.Fatalf("stream type %#x: %q, want %q", c.kind, got, c.want)
		}
	}
	raw := videoTS(1, streamMPEG2, audio)
	if got := VideoCodecOf(raw, 2); got != "" {
		t.Fatalf("wrong program: %q", got)
	}
	if tracks := AudioTracks(raw, 1); mainCodec(tracks) != "ac3" {
		t.Fatalf("main audio %+v", tracks)
	}
}

func TestLineupCodecWords(t *testing.T) {
	for raw, want := range map[string]string{
		"mpeg2video": "MPEG2", "h264": "H264", "hevc": "HEVC", "ac3": "AC3",
		"eac3": "EAC3", "aac": "AAC", "mp2": "MP2", "ac4": "AC4", "mp3": "", "": "",
	} {
		if got := lineupCodec(raw); got != want {
			t.Fatalf("%q: %q, want %q", raw, got, want)
		}
	}
}

func codecStore(t *testing.T, deviceID string, ch hdhr.Channel) (*store.Store, int64) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	dev := hdhr.Device{DeviceID: deviceID, FriendlyName: "Test", BaseURL: "source"}
	if err := st.UpsertDevice(ctx, dev, []hdhr.Channel{ch}); err != nil {
		t.Fatal(err)
	}
	channels, err := st.Channels(ctx, false)
	if err != nil || len(channels) != 1 {
		t.Fatalf("%+v %v", channels, err)
	}
	return st, channels[0].ID
}

func waitCodecs(t *testing.T, st *store.Store, id int64, video, audio string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var got store.SourceChannel
	for time.Now().Before(deadline) {
		var err error
		got, err = st.SourceChannel(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if got.VideoCodec == video && got.AudioCodec == audio {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("stored %q %q, want %q %q", got.VideoCodec, got.AudioCodec, video, audio)
}

func pipeHub(t *testing.T, st *store.Store) (*Hub, *mux, *io.PipeWriter, context.Context) {
	t.Helper()
	h, m := testHub(t)
	h.Store = st
	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()
	m.body = pr
	go h.readLoop(ctx, m)
	t.Cleanup(func() {
		cancel()
		_ = pw.Close()
	})
	return h, m, pw, ctx
}

func TestFirstTuneLearnsLinkCodecsBeforeTheEncode(t *testing.T) {
	st, id := codecStore(t, "src-1", hdhr.Channel{GuideNumber: "801", GuideName: "Link", StreamURL: "http://example/live.ts"})
	h, m, pw, ctx := pipeHub(t, st)
	go writeUntil(ctx, pw, videoTS(1, streamMPEG2, []esAudio{{pid: 0x101, lang: "eng", audioType: 0, bsmod: 0}}))
	ch, err := st.SourceChannel(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	f := h.addFeedLocked(m, ch)
	src := f.source
	h.mu.Unlock()
	if src.VideoCodec != "MPEG2" || src.AudioCodec != "AC3" {
		t.Fatalf("feed source %+v", src)
	}
	waitCodecs(t, st, id, "MPEG2", "AC3")
}

func TestDeferredScanCorrectsLinkCodecs(t *testing.T) {
	// A row from before codecs were learned says H.264 and AAC.
	st, id := codecStore(t, "src-1", hdhr.Channel{GuideNumber: "801", GuideName: "Link", StreamURL: "http://example/live.ts", VideoCodec: "H264", AudioCodec: "AAC"})
	if err := st.SetFieldOrder(context.Background(), id, "progressive"); err != nil {
		t.Fatal(err)
	}
	h, m, pw, ctx := pipeHub(t, st)
	ch, err := st.SourceChannel(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	f := h.addFeedLocked(m, ch)
	h.mu.Unlock()
	go writeUntil(ctx, pw, videoTS(1, streamMPEG2, []esAudio{{pid: 0x101, lang: "eng", audioType: 0, bsmod: 0}}))
	waitCodecs(t, st, id, "MPEG2", "AC3")
	h.mu.Lock()
	src := f.source
	h.mu.Unlock()
	if src.VideoCodec != "MPEG2" || src.AudioCodec != "AC3" {
		t.Fatalf("feed source %+v", src)
	}
}

func TestTunerLineupCodecsAreNotRewritten(t *testing.T) {
	st, id := codecStore(t, "1050ABCD", hdhr.Channel{GuideNumber: "5.1", GuideName: "KCTV", VideoCodec: "H264", AudioCodec: "AC3"})
	h, m, pw, ctx := pipeHub(t, st)
	go writeUntil(ctx, pw, videoTS(1, streamMPEG2, []esAudio{{pid: 0x101, lang: "eng", audioType: 0, bsmod: 0}}))
	ch, err := st.SourceChannel(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	f := h.addFeedLocked(m, ch)
	video := f.source.VideoCodec
	h.mu.Unlock()
	if video != "H264" {
		t.Fatalf("tuner feed video %q", video)
	}
	time.Sleep(100 * time.Millisecond)
	got, err := st.SourceChannel(ctx, id)
	if err != nil || got.VideoCodec != "H264" || got.AudioCodec != "AC3" {
		t.Fatalf("stored %q %q %v", got.VideoCodec, got.AudioCodec, err)
	}
}

func TestLinkTuneStoresTheStreamCodecs(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	sample := t.TempDir() + "/mpeg2.ts"
	gen := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=30000/1001",
		"-f", "lavfi", "-i", "sine=frequency=1000:sample_rate=48000",
		"-t", "4", "-c:v", "mpeg2video", "-b:v", "1M", "-c:a", "ac3", "-f", "mpegts", sample)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("sample: %v %s", err, out)
	}
	body, err := os.ReadFile(sample)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp2t")
		// About real time, so the tune stays open while the scan reads it.
		for off := 0; off < len(body); off += 188 * 200 {
			end := min(off+188*200, len(body))
			if _, err := w.Write(body[off:end]); err != nil {
				return
			}
			if fl, ok := w.(http.Flusher); ok {
				fl.Flush()
			}
			select {
			case <-r.Context().Done():
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
	}))
	defer srv.Close()
	st, id := codecStore(t, "src-1", hdhr.Channel{GuideNumber: "801", GuideName: "Link", StreamURL: srv.URL + "/live.ts"})
	ctx := context.Background()
	src, err := (&Hub{Store: st}).SourceOf(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	first := DecideFor(src, chromeCaps, Prefs{}, "", Host{})
	if first.Rendition.Video == "copy" || first.Rendition.Audio == "copy" {
		t.Fatalf("first watch %s", first.Rendition.Key())
	}
	h := New(st, t.TempDir(), ffmpeg, "libx264")
	defer h.Shutdown()
	session, err := h.Watch(ctx, id, first.Rendition)
	if err != nil {
		t.Fatal(err)
	}
	waitCodecs(t, st, id, "MPEG2", "AC3")
	h.Release(id, session.Rendition)
	src, err = h.SourceOf(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	next := DecideFor(src, chromeCaps, Prefs{}, "", Host{})
	if next.Rendition.Video == "copy" || next.Rendition.Audio != "aac2" {
		t.Fatalf("next watch %s", next.Rendition.Key())
	}
}
