package live

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/hdhr/fake"
	"broadwave/internal/store"
)

// hevc10.ts is three libx265 Main 10 pictures, 1920x1080 coded as 1088 with a
// conformance window, as service 3 (the program an ATSC 3.0 tune carries).
func hevcFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/hevc10.ts")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestHEVCPictureFacts(t *testing.T) {
	data := hevcFixture(t)
	p, ok := pictureFacts(data, 3)
	if !ok || p.Width != 1920 || p.Height != 1080 {
		t.Fatalf("facts: %+v %v", p, ok)
	}
	if order, ok := scanType(data, 3); !ok || order != "progressive" {
		t.Fatalf("scan: %q %v", order, ok)
	}
	if VideoCodecOf(data, 3) != "HEVC" {
		t.Fatalf("codec: %q", VideoCodecOf(data, 3))
	}
}

func TestHEVCProgramStartsOnAParameterSet(t *testing.T) {
	data := hevcFixture(t)
	_, video, kind := programVideo(data, 3)
	if video == 0 || kind != streamHEVC {
		t.Fatalf("video pid %d kind %#x", video, kind)
	}
	starts := 0
	for off := 0; off+188 <= len(data); off += 188 {
		pkt := data[off : off+188]
		if int(pkt[1]&0x1f)<<8|int(pkt[2]) != video || pkt[1]&0x40 == 0 {
			continue
		}
		if sequenceStart(tsPayload(pkt), kind) {
			starts++
		}
	}
	// Only the first of the three pictures is a random access point.
	if starts != 1 {
		t.Fatalf("starts: %d", starts)
	}
}

// atsc3PMT is the PMT a FLEX 4K sends for an ATSC 3.0 service: HEVC, an
// AC-4 track registered as "AC-4" with its language, and STPP captions.
func atsc3PMT() []byte {
	const pmtPID = 0x30
	body := []byte{0x00, 0x03, 0xc1, 0x00, 0x00, 0xe0, 0x31, 0xf0, 0x00,
		streamHEVC, 0xe0, 0x31, 0xf0, 0x00,
		streamPriv, 0xe0, 0x32, 0xf0, 0x0c, 0x05, 0x04, 'A', 'C', '-', '4', 0x0a, 0x04, 'e', 'n', 'g', 0x00,
		// A DVB-style stream says AC-4 in an extension descriptor instead.
		streamPriv, 0xe0, 0x33, 0xf0, 0x09, 0x7f, 0x01, extAC4, 0x0a, 0x04, 's', 'p', 'a', 0x00,
		streamPriv, 0xe0, 0x39, 0xf0, 0x06, 0x05, 0x04, 'S', 'T', 'P', 'P',
	}
	pat := psiSection(0x00, append([]byte{0x00, 0x01, 0xc1, 0x00, 0x00}, progPID(3, pmtPID)...))
	out := tsPacket(0, true, pat)
	return append(out, tsPacket(pmtPID, true, psiSection(0x02, body))...)
}

func TestAC4TracksFromThePMT(t *testing.T) {
	tracks := AudioTracks(atsc3PMT(), 3)
	if len(tracks) != 2 {
		t.Fatalf("tracks: %+v", tracks)
	}
	if tracks[0].PID != 0x32 || tracks[0].Codec != "ac4" || tracks[0].Role != "main" || tracks[0].Label != "English" {
		t.Fatalf("main: %+v", tracks[0])
	}
	if tracks[1].PID != 0x33 || tracks[1].Codec != "ac4" || tracks[1].Role != "language" {
		t.Fatalf("second: %+v", tracks[1])
	}
}

func TestAC4WidthsFromFFprobe(t *testing.T) {
	raw := []byte(`{"streams":[{"id":"0x31","codec_name":"hevc"},{"id":"0x32","codec_name":"ac4","channels":6},{"id":"0x33","codec_name":"ac4","channels":2},{"id":"0x39","codec_name":"bin_data"}]}`)
	got := ac4WidthsFrom(raw)
	if len(got) != 2 || got[0x32] != 6 || got[0x33] != 2 {
		t.Fatalf("widths: %v", got)
	}
	// An ffmpeg without the decoder reports no width.
	if got := ac4WidthsFrom([]byte(`{"streams":[{"id":"0x32","codec_name":"ac4","channels":0}]}`)); len(got) != 0 {
		t.Fatalf("no decoder: %v", got)
	}
}

func TestATSC3Decisions(t *testing.T) {
	apple := Caps{Platform: "tvos", Video: []string{"h264", "hevc"}, Audio: []string{"aac", "ac3", "eac3"}}
	web := Caps{Platform: "web", Video: []string{"h264"}, Audio: []string{"aac"}}
	webHEVC := Caps{Platform: "web", Video: []string{"h264", "hevc"}, Audio: []string{"aac", "ac3"}}
	surround := Source{VideoCodec: "HEVC", AudioCodec: "AC4", Progressive: true, AudioChannels: 6}
	stereo := Source{VideoCodec: "HEVC", AudioCodec: "AC4", Progressive: true, AudioChannels: 2}
	unmeasured := Source{VideoCodec: "HEVC", AudioCodec: "AC4", Progressive: true}
	cases := []struct {
		name string
		src  Source
		caps Caps
		p    Prefs
		want string
	}{
		{"apple gets the picture as sent and 5.1 as AC-3", surround, apple, Prefs{}, "copy.ac3"},
		{"a stereo mix stays AAC", stereo, apple, Prefs{}, "copy.aac2"},
		{"an unmeasured mix is stereo until measured", unmeasured, apple, Prefs{}, "copy.aac2"},
		{"stereo asked for is stereo", surround, apple, Prefs{Audio: "stereo"}, "copy.aac2"},
		{"surround without AC-3 is 5.1 AAC", surround, Caps{Platform: "ios", Video: []string{"hevc"}, Audio: []string{"aac"}}, Prefs{Audio: "surround"}, "copy.aac6"},
		{"a browser without HEVC gets H.264", surround, web, Prefs{}, "1080.aac2.broadcast"},
		{"a browser with HEVC gets the picture as sent", surround, webHEVC, Prefs{}, "copy.ac3"},
		{"a browser with HEVC still gets H.264 conversions", Source{VideoCodec: "MPEG2", AudioCodec: "AC3"}, webHEVC, Prefs{}, "1080.copy.broadcast"},
	}
	for _, c := range cases {
		got := DecideFor(c.src, c.caps, c.p, "libx264", Host{}).Rendition.Key()
		if got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}
}

func TestATSC3RenditionArgs(t *testing.T) {
	src := Source{VideoCodec: "HEVC", AudioCodec: "AC4", Progressive: true, AudioChannels: 6, AudioPID: 0x32,
		Extras: []AudioTrack{{PID: 0x33, Codec: "ac4", Role: "language", Channels: 2, Measured: true}}}
	copyLine := strings.Join(RenditionArgs(3, src, Rendition{Video: "copy", Audio: "ac3"}, "libx264", ""), " ")
	for _, want := range []string{"-c:v copy -tag:v hvc1", "-map 0:i:50", "-map 0:i:51", "-c:a ac3 -b:a 448k", "-b:a:1 192k"} {
		if !strings.Contains(copyLine, want) {
			t.Fatalf("missing %q in %s", want, copyLine)
		}
	}
	if strings.Contains(copyLine, "-ac ") {
		t.Fatalf("AC-3 keeps each track's width: %s", copyLine)
	}
	// A converted AC-4 mix is 48 kHz; ffmpeg would pick 44.1.
	for _, audio := range []string{"ac3", "aac2", "aac6"} {
		if line := strings.Join(RenditionArgs(3, src, Rendition{Video: "copy", Audio: audio}, "libx264", ""), " "); !strings.Contains(line, "-ar 48000") {
			t.Fatalf("%s from AC-4 without -ar 48000: %s", audio, line)
		}
	}
	ac3 := Source{VideoCodec: "MPEG2", AudioCodec: "AC3"}
	if line := strings.Join(RenditionArgs(1, ac3, Rendition{Video: "1080", Audio: "aac2"}, "libx264", ""), " "); strings.Contains(line, "-ar ") {
		t.Fatalf("a 1.0 conversion changed its rate: %s", line)
	}
	h264 := Source{VideoCodec: "H264", AudioCodec: "AC3", Progressive: true}
	if line := strings.Join(RenditionArgs(1, h264, Rendition{Video: "copy", Audio: "copy"}, "libx264", ""), " "); strings.Contains(line, "hvc1") {
		t.Fatalf("H.264 copy tagged hvc1: %s", line)
	}
	// A 10-bit picture would otherwise become H.264 High 10.
	transcode := strings.Join(RenditionArgs(3, src, Rendition{Video: "1080", Audio: "aac2"}, "libx264", ""), " ")
	if !strings.Contains(transcode, ",format=yuv420p") {
		t.Fatalf("libx264 keeps 10-bit: %s", transcode)
	}
	gpu := strings.Join(RenditionArgs(3, src, Rendition{Video: "1080", Audio: "aac2"}, "h264_vaapi", ""), " ")
	if !strings.Contains(gpu, "force_original_aspect_ratio=decrease:format=nv12") {
		t.Fatalf("VAAPI keeps P010: %s", gpu)
	}
}

// hevc2160.ts is three libx265 Main 10 pictures at 3840x2160, service 3.
// No broadcast here sends 4K yet; the picture must reach the player as sent.
func TestA2160pHEVCPassesThroughAsSent(t *testing.T) {
	data, err := os.ReadFile("testdata/hevc2160.ts")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := pictureFacts(data, 3)
	if !ok || p.Width != 3840 || p.Height != 2160 {
		t.Fatalf("facts: %+v %v", p, ok)
	}
	if VideoCodecOf(data, 3) != "HEVC" {
		t.Fatalf("codec: %q", VideoCodecOf(data, 3))
	}
	src := Source{VideoCodec: "HEVC", AudioCodec: "AC4", Progressive: true, AudioChannels: 6}
	apple := Caps{Platform: "tvos", Video: []string{"h264", "hevc"}, Audio: []string{"aac", "ac3"}}
	r := DecideFor(src, apple, Prefs{}, "libx264", Host{}).Rendition
	if r.Video != "copy" {
		t.Fatalf("rendition %s", r.Key())
	}
	line := strings.Join(RenditionArgs(3, src, r, "libx264", ""), " ")
	if !strings.Contains(line, "-c:v copy -tag:v hvc1") || strings.Contains(line, "-vf ") || strings.Contains(line, "scale=") {
		t.Fatalf("copy args: %s", line)
	}
}

// A player that can't show the broadcast's size gets it scaled; the size is
// the one the last tune stored.
func TestATooTallPictureIsScaledToTheScreen(t *testing.T) {
	st, id := codecStore(t, "1010ABCD", hdhr.Channel{GuideNumber: "119.1", GuideName: "UHD"})
	h, m := testHub(t)
	h.Store = st
	// A bare feed: a real one starts scans that race these fields.
	f := &feed{channel: store.SourceChannel{Channel: store.Channel{ID: id, GuideNumber: "119.1"}}, program: 3}
	m.picMu.Lock()
	m.pictures = map[int]notedPicture{3: {Width: 3840, Height: 2160}}
	m.picMu.Unlock()
	h.savePictureHeight(m, f)
	src, err := h.SourceOf(context.Background(), id)
	if err != nil || src.Height != 2160 {
		t.Fatalf("stored height %d %v", src.Height, err)
	}
	src.VideoCodec, src.AudioCodec, src.Progressive = "HEVC", "AC4", true
	tv := Caps{Platform: "tvos", Video: []string{"h264", "hevc"}, Audio: []string{"aac", "ac3"}}
	for _, c := range []struct {
		max  int
		want string
	}{{0, "copy"}, {2160, "copy"}, {1080, "1080"}, {720, "720"}} {
		tv.MaxHeight = c.max
		d := DecideFor(src, tv, Prefs{}, "libx264", Host{})
		if d.Rendition.Video != c.want {
			t.Errorf("max %d: %s (%s)", c.max, d.Rendition.Key(), d.Reason)
		}
	}
	src.Height = 1080
	tv.MaxHeight = 1080
	if d := DecideFor(src, tv, Prefs{}, "libx264", Host{}); d.Rendition.Video != "copy" {
		t.Errorf("a 1080 picture on a 1080 screen: %s", d.Rendition.Key())
	}
}

func TestAn3Point0TileIsTheBroadcastAsSent(t *testing.T) {
	apple := Caps{Platform: "tvos", Video: []string{"h264", "hevc"}, Audio: []string{"ac3", "aac"}}
	noHEVC := Caps{Platform: "web", Video: []string{"h264"}, Audio: []string{"aac"}}
	hd := Source{VideoCodec: "HEVC", AudioCodec: "AC4", Progressive: true, AudioChannels: 6, Height: 1080}
	uhd := Source{VideoCodec: "HEVC", AudioCodec: "AC4", Progressive: true, AudioChannels: 6, Height: 2160}
	unknown := Source{VideoCodec: "HEVC", AudioCodec: "AC4", Progressive: true, AudioChannels: 6}
	gpu := Host{Focus: "720", Tiles: 4}
	for _, c := range []struct {
		name string
		src  Source
		caps Caps
		p    Prefs
		want string
	}{
		{"a 1080 HEVC tile is copied", hd, apple, Prefs{Quality: "tile"}, "copy.none"},
		{"the focused 1080 HEVC tile is copied with sound", hd, apple, Prefs{Quality: "focus", Audio: "stereo"}, "copy.aac2"},
		{"a 2160 tile is converted", uhd, apple, Prefs{Quality: "tile"}, "540.none.broadcast.hevc"},
		{"a tile of a picture not measured yet is converted", unknown, apple, Prefs{Quality: "tile"}, "540.none.broadcast.hevc"},
		{"a player without HEVC gets a converted tile", hd, noHEVC, Prefs{Quality: "tile"}, "540.none.broadcast"},
		{"the smallest tile is still converted", hd, apple, Prefs{Quality: "360", Audio: "none"}, "360.none.broadcast.hevc"},
	} {
		got := DecideFor(c.src, c.caps, c.p, "h264_vaapi", gpu)
		if got.Rendition.Key() != c.want || got.Reason == "" {
			t.Errorf("%s: got %s (%s), want %s", c.name, got.Rendition.Key(), got.Reason, c.want)
		}
	}
}

func TestAnAC4RecordingKeepsTheBroadcastPackets(t *testing.T) {
	if !rawRecording(store.SourceChannel{Channel: store.Channel{AudioCodec: "AC-4"}}) || rawRecording(store.SourceChannel{Channel: store.Channel{AudioCodec: "AC3"}}) {
		t.Fatal("only an AC-4 channel records its own packets")
	}
	const pmtPID, videoPID, audioPID = 0x1000, 0x100, 0x101
	pmt := []byte{0x00, 0x03, 0xc1, 0x00, 0x00, 0xe1, 0x00, 0xf0, 0x00,
		streamHEVC, 0xe1, 0x00, 0xf0, 0x00,
		streamPriv, 0xe1, 0x01, 0xf0, 0x0c, 0x05, 0x04, 'A', 'C', '-', '4', 0x0a, 0x04, 'e', 'n', 'g', 0x00,
	}
	in := tsPacket(0, true, psiSection(0x00, append([]byte{0x00, 0x01, 0xc1, 0x00, 0x00}, progPID(3, pmtPID)...)))
	in = append(in, tsPacket(pmtPID, true, psiSection(0x02, pmt))...)
	var pts int64 = -1
	sent := 0
	data := hevcFixture(t)
	for off := 0; off+188 <= len(data); off += 188 {
		pkt := data[off : off+188]
		pid := int(pkt[1]&0x1f)<<8 | int(pkt[2])
		if pid != videoPID {
			continue
		}
		in = append(in, pkt...)
		if pkt[1]&0x40 != 0 && pts < 0 {
			pts, _ = pesTime(tsPayload(pkt))
		}
		if pts >= 0 && off%(188*20) == 0 {
			at := pts + int64(sent)*3003
			pes := []byte{0x00, 0x00, 0x01, 0xbd, 0x00, 0x00, 0x80, 0x80, 0x05,
				byte(0x21 | (at>>29)&0x0e), byte(at >> 22), byte(0x01 | (at>>14)&0xfe), byte(at >> 7), byte(0x01 | (at<<1)&0xfe)}
			in = append(in, tsPacket(audioPID, true, append(pes, make([]byte, 100)...))...)
			sent++
		}
	}
	if pts < 0 || sent < 3 {
		t.Fatalf("fixture: pts %d, %d sound packets", pts, sent)
	}
	path := filepath.Join(t.TempDir(), "rec.ts")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := newProgramPipe(file, 3)
	for len(in) > 0 {
		n := min(len(in), 7*188)
		if _, err := w.Write(in[:n]); err != nil {
			t.Fatal(err)
		}
		in = in[n:]
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	count := map[int]int{}
	described := false
	for off := 0; off+188 <= len(out); off += 188 {
		pkt := out[off : off+188]
		pid := int(pkt[1]&0x1f)<<8 | int(pkt[2])
		count[pid]++
		if pid == pmtPID && bytes.Contains(pkt, []byte("AC-4")) {
			described = true
		}
	}
	if !described || count[videoPID] == 0 || count[audioPID] == 0 {
		t.Fatalf("recording: AC-4 in the map %v, packets per pid %v", described, count)
	}
}

func TestA3Point0WatchOpensTheAutoStreamWithoutAProbe(t *testing.T) {
	st := openStore(t)
	srv := &fake.Server{Profile: fake.ProfileFlex4K}
	base, control, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	t.Setenv("HDHR_CONTROL_PORT", control)
	ctx := context.Background()
	dev, err := (&hdhr.Client{}).FetchDevice(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	lineup, err := (&hdhr.Client{}).FetchLineup(ctx, dev.LineupURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, dev, lineup); err != nil {
		t.Fatal(err)
	}
	ch := sourceByNumber(t, st, "104.1")
	h := New(st, t.TempDir(), "ffmpeg", "libx264")
	t.Cleanup(h.Shutdown)
	began := time.Now()
	h.mu.Lock()
	f, err := h.ensureFeedLocked(ctx, ch, nil)
	h.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	took := time.Since(began)
	t.Cleanup(func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.stopFeedLocked(f)
	})
	reqs := srv.Requests()
	for _, r := range reqs {
		if strings.HasSuffix(r, "/vchannel") {
			t.Fatalf("a 3.0 watch probed the control port: %v", reqs)
		}
	}
	if !slices.Contains(reqs, "/auto/v104.1") || took > 2*time.Second {
		t.Fatalf("3.0 watch took %v; requests %v", took, reqs)
	}
}

func sourceByNumber(t *testing.T, st *store.Store, number string) store.SourceChannel {
	t.Helper()
	chs, err := st.Channels(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range chs {
		if c.GuideNumber == number {
			ch, err := st.SourceChannel(context.Background(), c.ID)
			if err != nil {
				t.Fatal(err)
			}
			return ch
		}
	}
	t.Fatalf("no %s", number)
	return store.SourceChannel{}
}

func TestAWarm3Point0StreamGivesItsTunerToANewChannel(t *testing.T) {
	st := openStore(t)
	srv := &fake.Server{Profile: fake.ProfileFlex4K}
	base, control, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	t.Setenv("HDHR_CONTROL_PORT", control)
	ctx := context.Background()
	dev, err := (&hdhr.Client{}).FetchDevice(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	lineup, err := (&hdhr.Client{}).FetchLineup(ctx, dev.LineupURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, dev, lineup); err != nil {
		t.Fatal(err)
	}
	h := New(st, t.TempDir(), "ffmpeg", "libx264")
	t.Cleanup(func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		for _, f := range h.feedsLocked() {
			h.stopFeedLocked(f)
		}
	})
	// 104.1 was watched and left: the device tuned it, and nobody is on it now.
	// Its first watch stored the scan, as on any channel watched before.
	c3 := sourceByNumber(t, st, "104.1")
	if err := st.SetFieldOrder(ctx, c3.ID, "progressive"); err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	_, err = h.ensureFeedLocked(ctx, sourceByNumber(t, st, "104.1"), nil)
	h.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	held := -1
	for i := 0; i < 4 && held < 0; i++ {
		waitTuned := time.Now().Add(2 * time.Second)
		for time.Now().Before(waitTuned) && guideOn(t, base, i) != "104.1" {
			if g := fetchGuides(t, base); slices.Contains(g, "104.1") {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if guideOn(t, base, i) == "104.1" {
			held = i
		}
	}
	if held < 0 {
		t.Fatalf("104.1 is on no tuner: %v", fetchGuides(t, base))
	}
	// Someone else has every other tuner.
	for i := 0; i < 4; i++ {
		if i != held {
			mustTune(t, fmt.Sprintf("%s/tuner%d/v4.1", base, i))
			waitGuide(t, base, i, "4.1")
		}
	}
	h.mu.Lock()
	_, err = h.ensureFeedLocked(ctx, sourceByNumber(t, st, "5.1"), nil)
	h.mu.Unlock()
	if err != nil {
		t.Fatalf("a new channel with a warm 3.0 stream on tuner %d: %v (%v)", held, err, fetchGuides(t, base))
	}
	waitGuide(t, base, held, "5.1")
}

// A guess at a 3.0 channel tunes it ahead of the click, without holding the
// hub while the device answers, only when another 3.0 tuner stays free, and
// only for a player that takes the picture as sent. The watch joins it.
func TestAGuessTunesA3Point0ChannelAhead(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	st := openStore(t)
	srv := &fake.Server{Profile: fake.ProfileFlex4K, TuneDelay: 800 * time.Millisecond, Channels: []fake.Channel{
		{Number: "4.1", Name: "KBWV", Freq: 593000000},
		{Number: "104.1", Name: "KBWV", Freq: 599000000, Video: "HEVC", Audio: "AC-4", ATSC3: true},
		{Number: "106.1", Name: "WTST", Freq: 611000000, Video: "HEVC", Audio: "AC-4", ATSC3: true},
	}}
	base, control, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	t.Setenv("HDHR_CONTROL_PORT", control)
	ctx := context.Background()
	dev, err := (&hdhr.Client{}).FetchDevice(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	lineup, err := (&hdhr.Client{}).FetchLineup(ctx, dev.LineupURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, dev, lineup); err != nil {
		t.Fatal(err)
	}
	h := New(st, t.TempDir(), "ffmpeg", "libx264")
	t.Cleanup(h.Shutdown)
	// Both were watched before, so a tune runs no field-order probe.
	for _, n := range []string{"104.1", "106.1"} {
		if err := st.SetFieldOrder(ctx, sourceByNumber(t, st, n).ID, "progressive"); err != nil {
			t.Fatal(err)
		}
	}
	c3 := sourceByNumber(t, st, "104.1")
	copied := Rendition{Video: "copy", Audio: "aac2"}
	opens := func(number ...string) int {
		want := "/auto/v104.1"
		if len(number) > 0 {
			want = "/auto/v" + number[0]
		}
		n := 0
		for _, r := range srv.Requests() {
			if r == want {
				n++
			}
		}
		return n
	}

	if ok, err := h.Warm(ctx, c3.ID, Rendition{Video: "1080", Audio: "aac2"}, false); err != nil || ok || opens() != 0 {
		t.Fatalf("a guess tuned for a transcode: ok %v err %v opens %d", ok, err, opens())
	}

	// Someone else has one of the two 3.0 tuners.
	other, err := http.Get(base + "/tuner0/v4.1")
	if err != nil {
		t.Fatal(err)
	}
	waitGuide(t, base, 0, "4.1")
	if ok, err := h.Warm(ctx, c3.ID, copied, false); err != nil || ok || opens() != 0 {
		t.Fatalf("a guess took the last 3.0 tuner: ok %v err %v opens %d", ok, err, opens())
	}
	other.Body.Close()
	waitGuide(t, base, 0, "")

	done := make(chan error, 1)
	go func() {
		ok, err := h.Warm(ctx, c3.ID, copied, false)
		if err == nil && !ok {
			err = errors.New("no guess with both 3.0 tuners free")
		}
		done <- err
	}()
	time.Sleep(300 * time.Millisecond)
	began := time.Now()
	h.Touch(c3.ID, copied.normalized().Key())
	if waited := time.Since(began); waited > 200*time.Millisecond {
		t.Fatalf("the hub was held %v while the device answered", waited)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	// Resting on another 3.0 row replaces the guess; it does not need a third tuner.
	next := sourceByNumber(t, st, "106.1")
	if ok, err := h.Warm(ctx, next.ID, copied, false); err != nil || !ok {
		t.Fatalf("a second guess was refused while the first held a tuner: ok %v err %v", ok, err)
	}
	h.mu.Lock()
	_, kept := h.channels[c3.ID]
	h.mu.Unlock()
	if kept {
		t.Fatal("the first guess kept its tuner")
	}
	if _, err := h.Watch(ctx, next.ID, copied, false); err != nil {
		t.Fatal(err)
	}
	if n := opens("106.1"); n != 1 {
		t.Fatalf("the watch tuned again: %d opens", n)
	}
}

// A 3.0 watch does not hold the hub while the device answers, and when no
// 3.0 tuner is free it still takes the one a channel left a moment ago holds.
func TestA3Point0WatchLeavesTheHubFreeWhileTheDeviceAnswers(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	st := openStore(t)
	srv := &fake.Server{Profile: fake.ProfileFlex4K, TuneDelay: 800 * time.Millisecond, Channels: []fake.Channel{
		{Number: "4.1", Name: "KBWV", Freq: 593000000},
		{Number: "104.1", Name: "KBWV", Freq: 599000000, Video: "HEVC", Audio: "AC-4", ATSC3: true},
		{Number: "106.1", Name: "WTST", Freq: 611000000, Video: "HEVC", Audio: "AC-4", ATSC3: true},
	}}
	base, control, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	t.Setenv("HDHR_CONTROL_PORT", control)
	ctx := context.Background()
	dev, err := (&hdhr.Client{}).FetchDevice(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	lineup, err := (&hdhr.Client{}).FetchLineup(ctx, dev.LineupURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, dev, lineup); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"104.1", "106.1"} {
		if err := st.SetFieldOrder(ctx, sourceByNumber(t, st, n).ID, "progressive"); err != nil {
			t.Fatal(err)
		}
	}
	h := New(st, t.TempDir(), "ffmpeg", "libx264")
	t.Cleanup(h.Shutdown)
	copied := Rendition{Video: "copy", Audio: "aac2"}
	first := sourceByNumber(t, st, "104.1")

	done := make(chan error, 1)
	var session Session
	asked := time.Now()
	go func() {
		var err error
		session, err = h.Watch(ctx, first.ID, copied, false)
		done <- err
	}()
	time.Sleep(300 * time.Millisecond)
	began := time.Now()
	h.Touch(first.ID, copied.normalized().Key())
	if waited := time.Since(began); waited > 200*time.Millisecond {
		t.Fatalf("the hub was held %v while the device answered", waited)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	// The start line names the device's answer, so a slow 3.0 start can be read.
	note := h.NoteStart(first.ID, session.Rendition, asked)
	var answered float64
	if _, err := fmt.Sscanf(note[strings.Index(note, "lock ")+5:], "%f", &answered); err != nil || answered < 0.7 {
		t.Fatalf("no tune steps in %q", note)
	}
	h.Release(first.ID, session.Rendition)

	// Someone else takes the other 3.0 tuner; 104.1 is kept for a flip back.
	var other int
	for i, g := range fetchGuides(t, base) {
		if g == "" && i < 2 {
			other = i
		}
	}
	res, err := http.Get(fmt.Sprintf("%s/tuner%d/v4.1", base, other))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	waitGuide(t, base, other, "4.1")
	if _, err := h.Watch(ctx, sourceByNumber(t, st, "106.1").ID, copied, false); err != nil {
		t.Fatalf("no 3.0 tuner was free and the one left a moment ago was kept: %v (%v)", err, fetchGuides(t, base))
	}
}

// Viewers who open 3.0 channels at the same moment share a tune per channel.
// Two watches of one channel each opened the device stream, so that channel
// held both 3.0 tuners until one let go, and a third viewer found every
// tuner busy.
func TestViewersOpening3Point0ChannelsTogetherShareATune(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	st := openStore(t)
	srv := &fake.Server{Profile: fake.ProfileFlex4K, TuneDelay: 800 * time.Millisecond, TunesFirst: true, Channels: []fake.Channel{
		{Number: "4.1", Name: "KBWV", Freq: 593000000},
		{Number: "104.1", Name: "KBWV", Freq: 599000000, Video: "HEVC", Audio: "AC-4", ATSC3: true},
		{Number: "106.1", Name: "WTST", Freq: 611000000, Video: "HEVC", Audio: "AC-4", ATSC3: true},
	}}
	base, control, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	t.Setenv("HDHR_CONTROL_PORT", control)
	ctx := context.Background()
	dev, err := (&hdhr.Client{}).FetchDevice(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	lineup, err := (&hdhr.Client{}).FetchLineup(ctx, dev.LineupURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, dev, lineup); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"104.1", "106.1"} {
		if err := st.SetFieldOrder(ctx, sourceByNumber(t, st, n).ID, "progressive"); err != nil {
			t.Fatal(err)
		}
	}
	h := New(st, t.TempDir(), "ffmpeg", "libx264")
	t.Cleanup(h.Shutdown)
	first, second := sourceByNumber(t, st, "104.1"), sourceByNumber(t, st, "106.1")
	asks := []struct {
		id   int64
		want Rendition
	}{
		{first.ID, Rendition{Video: "copy", Audio: "aac2"}},
		{first.ID, Rendition{Video: "copy", Audio: "none"}},
		{second.ID, Rendition{Video: "copy", Audio: "aac2"}},
	}
	errs := make(chan error, len(asks))
	for _, a := range asks {
		// The other channel's viewer comes while the device still answers.
		if a.id == second.ID {
			time.Sleep(300 * time.Millisecond)
		}
		go func() {
			_, err := h.Watch(ctx, a.id, a.want, false)
			errs <- err
		}()
	}
	for range asks {
		if err := <-errs; err != nil {
			t.Fatalf("a viewer was refused: %v (%v)", err, fetchGuides(t, base))
		}
	}
}

func TestListsDecoderReadsTheDecoderColumn(t *testing.T) {
	jellyfin := " A....D ac3                  ATSC A/52A (AC-3)\n A....D ac4                  AC-4\n"
	stock := " A....D ac3                  ATSC A/52A (AC-3)\n A....D eac3                 ATSC A/52B (AC-3, E-AC-3)\n V....D ac4like              not ac4\n"
	if !listsDecoder(jellyfin, "ac4") {
		t.Fatal("jellyfin-ffmpeg lists ac4")
	}
	if listsDecoder(stock, "ac4") {
		t.Fatal("a description that names ac4 is not the decoder")
	}
}

func TestMissingAC4OnlyJudgesAnFFmpegThatAnswers(t *testing.T) {
	dir := t.TempDir()
	script := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	stock := script("stock", "echo ' A....D aac                  AAC (Advanced Audio Coding)'; echo ' A....D ac3                  ATSC A/52A (AC-3)'")
	jellyfin := script("jellyfin", "echo ' A....D aac                  AAC (Advanced Audio Coding)'; echo ' A....D ac4                  AC-4'")
	broken := script("broken", "exit 1")
	if !MissingAC4(stock) {
		t.Fatal("a stock ffmpeg has no AC-4")
	}
	if MissingAC4(jellyfin) || MissingAC4(broken) || MissingAC4("") {
		t.Fatal("only an ffmpeg that lists its decoders without AC-4 is missing it")
	}
}

func TestUnvoicedLeavesOutOnlyAC4Sound(t *testing.T) {
	r := Unvoiced("AC4", Rendition{Video: "copy", Audio: "ac3", Track: "lang"})
	if r.Audio != "none" || r.Track != "" {
		t.Fatalf("AC-4 converted to AC-3 needs a decoder: %+v", r)
	}
	if r := Unvoiced("AC4", Rendition{Video: "copy", Audio: "copy"}); r.Audio != "copy" {
		t.Fatalf("copied AC-4 needs no decoder: %+v", r)
	}
	if r := Unvoiced("AC3", Rendition{Video: "1080", Audio: "aac2"}); r.Audio != "aac2" {
		t.Fatalf("AC-3 keeps its sound: %+v", r)
	}
}

// A hub whose ffmpeg cannot decode AC-4 starts a 3.0 channel's encode without
// sound, whatever the watch asked for, so ffmpeg does not exit at once.
func TestAnAC4EncodeIsSilentWithoutADecoder(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := New(nil, dir, script, "libx264")
	h.NoAC4 = true
	t.Cleanup(func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		for _, f := range h.channels {
			for _, r := range f.renditions {
				if r.cmd != nil && r.cmd.Process != nil {
					_ = r.cmd.Process.Kill()
				}
			}
		}
	})
	defer lockHub(h)()
	m := &mux{freq: 593000000, tuner: -1, input: "color", feeds: map[string]*feed{}, cancel: func() {}}
	h.muxes[m.freq] = m
	f := h.addFeedLocked(m, store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "104.1", VideoCodec: "HEVC", AudioCodec: "AC4"}, FieldOrder: "progressive"})
	r, err := h.ensureRenditionLocked(f, Rendition{Video: "copy", Audio: "ac3"})
	if err != nil {
		t.Fatal(err)
	}
	if r.spec.Audio != "none" {
		t.Fatalf("spec %+v", r.spec)
	}
	if r.cmd == nil || !slices.Contains(r.cmd.Args, "-an") || slices.Contains(r.cmd.Args, "ac3") {
		t.Fatalf("args %v", r.cmd)
	}
}

// A link that declares no codecs and turns out to carry AC-4 moves its encodes
// to one silent encode on an ffmpeg that cannot decode it. Their viewers stay,
// and the keys they joined under still find it.
func TestALinkFoundToCarryAC4PlaysSilentUnderItsOldKey(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := New(nil, dir, script, "libx264")
	h.NoAC4 = true
	t.Cleanup(func() { lockHub(h)() })
	h.mu.Lock()
	m := &mux{freq: 1, tuner: -1, input: "color", feeds: map[string]*feed{}, cancel: func() {}}
	h.muxes[m.freq] = m
	f := h.addFeedLocked(m, store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "801", DeviceID: "src-1"}, FieldOrder: "progressive"})
	r, err := h.ensureRenditionLocked(f, Rendition{Video: "1080", Audio: "aac2"})
	if err != nil {
		h.mu.Unlock()
		t.Fatal(err)
	}
	// A second sound choice of the same picture lands on the same silent encode.
	r6, err := h.ensureRenditionLocked(f, Rendition{Video: "1080", Audio: "aac6"})
	if err != nil {
		h.mu.Unlock()
		t.Fatal(err)
	}
	r.viewers, r6.viewers = 2, 1
	old := r.spec.Key()
	learned := h.learnCodecsLocked(f, "", "ac4")
	h.rebuildRenditionsLocked(f)
	count := len(f.renditions)
	h.mu.Unlock()

	now := h.Current(1, old)
	s, ok := h.Session(1, now)
	h.Release(1, old)
	h.mu.Lock()
	var spec Rendition
	var args []string
	viewers := -1
	if moved := f.renditions[now]; moved != nil {
		spec, viewers = moved.spec, moved.viewers
		if moved.cmd != nil {
			args = moved.cmd.Args
		}
	}
	h.mu.Unlock()
	if !learned || now == old || !ok || s.Rendition != now || count != 1 {
		t.Fatalf("learned %v old %s now %s session %v %s renditions %d", learned, old, now, ok, s.Rendition, count)
	}
	if spec.Audio != "none" || !slices.Contains(args, "-an") {
		t.Fatalf("spec %+v args %v", spec, args)
	}
	if viewers != 2 {
		t.Fatalf("3 viewers, 1 released by the old key: %d left", viewers)
	}
}

// The first scan of a link can end before its PMT: a 3.0 stream may send one
// every 2 s. The encode starts on unknown sound, and on an ffmpeg without an
// AC-4 decoder it dies twice in about 2 s. The late scan learns the codec from
// the PMT and moves the encode to a silent one first.
func TestALateScanLearnsALinksAC4(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := New(nil, dir, script, "libx264")
	h.NoAC4 = true
	t.Cleanup(func() { lockHub(h)() })
	h.mu.Lock()
	m := &mux{freq: 1, tuner: -1, input: "color", feeds: map[string]*feed{}, cancel: func() {}}
	h.muxes[m.freq] = m
	f := h.addFeedLocked(m, store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "801", DeviceID: "src-1"}})
	f.program = 3
	r, err := h.ensureRenditionLocked(f, Rendition{Video: "1080", Audio: "aac2"})
	h.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	old := r.spec.Key()
	body := append(pmtBody(3, streamMPEG2, 0x100), streamPriv, 0xe1, 0x01, 0xf0, 0x06, 0x05, 0x04, 'A', 'C', '-', '4')
	var ts []byte
	ts = append(ts, tsPacket(0, true, psiSection(0x00, append([]byte{0x00, 0x01, 0xc1, 0x00, 0x00}, progPID(3, 0x1000)...)))...)
	ts = append(ts, tsPacket(0x1000, true, psiSection(0x02, body))...)
	ts = append(ts, tsPacket(0x100, true, pesPacket([]byte{0x00, 0x00, 0x01, 0xB5, 0x10, 0x08}))...)
	h.finishScan(m, f, &scanBuf{wake: make(chan struct{}, 1), b: ts}, nil)

	now := h.Current(1, old)
	h.mu.Lock()
	codec := f.source.AudioCodec
	tracks := len(f.tracks)
	var spec Rendition
	if moved := f.renditions[now]; moved != nil {
		spec = moved.spec
	}
	h.mu.Unlock()
	if codecName(codec) != "ac4" || tracks != 1 || now == old || spec.Audio != "none" {
		t.Fatalf("codec %q tracks %d old %s now %s spec %+v", codec, tracks, old, now, spec)
	}
}

// A 3.0 recording plays its picture alone on an ffmpeg with no AC-4 decoder,
// and its playlist is made again once ffmpeg has one.
func TestASilentRecordingLeavesOutItsSound(t *testing.T) {
	dir := t.TempDir()
	ac4 := filepath.Join(dir, "ac4.ts")
	if err := os.WriteFile(ac4, atsc3PMT(), 0o644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "other.ts")
	if err := os.WriteFile(other, make([]byte, 188*4), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &Hub{Encoder: "libx264", NoAC4: true}
	g := h.fileGraphFor(ac4, "hevc", "broadcast", "progressive")
	if g.Audio != "none" {
		t.Fatalf("graph %+v", g)
	}
	list := PictureArgs(g)
	args := strings.Join(list, " ")
	if !slices.Contains(list, "-an") || strings.Contains(args, "0:a:0") || strings.Contains(args, "-c:a") {
		t.Fatalf("args %s", args)
	}
	if g := h.fileGraphFor(other, "mpeg2video", "broadcast", ""); g.Audio == "none" {
		t.Fatal("a 1.0 recording keeps its sound")
	}
	h.NoAC4 = false
	voiced := h.fileGraphFor(ac4, "hevc", "broadcast", "progressive")
	if voiced.Audio == "none" || graphStamp(voiced) == graphStamp(g) {
		t.Fatalf("with a decoder: %+v, stamps %q %q", voiced, graphStamp(voiced), graphStamp(g))
	}
}
