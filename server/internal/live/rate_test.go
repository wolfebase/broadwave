package live

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"broadwave/internal/store"
)

// A 25 Hz broadcast (DVB, most of the world) keeps its clock: 50 fields
// stretched to 59.94 repeat every fifth picture.
func TestFiftyHzKeepsItsOwnClock(t *testing.T) {
	src := Source{VideoCodec: "MPEG2", AudioCodec: "MPEG", Rate: "25"}
	full := strings.Join(RenditionArgs(1, src, Rendition{Video: "1080", Audio: "aac2", Mode: "broadcast"}, "libx264", ""), " ")
	if !strings.Contains(full, "bwdif=mode=send_field") || !strings.Contains(full, "fps=50,") || strings.Contains(full, "60000/1001") {
		t.Fatalf("1080 of 25i: %s", full)
	}
	small := strings.Join(RenditionArgs(1, src, Rendition{Video: "540", Audio: "aac2", Mode: "broadcast"}, "libx264", ""), " ")
	if !strings.Contains(small, "fps=25") || strings.Contains(small, "30000/1001") {
		t.Fatalf("540 of 25i: %s", small)
	}
	film := strings.Join(RenditionArgs(1, src, Rendition{Video: "1080", Audio: "aac2", Mode: "film"}, "libx264", ""), " ")
	if strings.Contains(film, "24000/1001") {
		t.Fatalf("film on 25i drops pictures: %s", film)
	}
	p50 := Source{VideoCodec: "H264", AudioCodec: "AAC", Progressive: true, Rate: "50"}
	if line := strings.Join(RenditionArgs(1, p50, Rendition{Video: "720", Audio: "aac2", Mode: "broadcast"}, "libx264", ""), " "); strings.Contains(line, "fps=") {
		t.Fatalf("720p50 keeps every frame: %s", line)
	}
	if line := strings.Join(RenditionArgs(1, p50, Rendition{Video: "360", Audio: "aac2", Mode: "broadcast"}, "libx264", ""), " "); !strings.Contains(line, "fps=25") {
		t.Fatalf("a 50p tile halves to 25: %s", line)
	}
	us := strings.Join(RenditionArgs(1, Source{VideoCodec: "MPEG2", AudioCodec: "AC3", Rate: "29.97"}, Rendition{Video: "1080", Audio: "aac2", Mode: "broadcast"}, "libx264", ""), " ")
	if !strings.Contains(us, "fps=60000/1001") {
		t.Fatalf("29.97i stays 59.94: %s", us)
	}
	unknown := strings.Join(RenditionArgs(1, Source{VideoCodec: "MPEG2", AudioCodec: "AC3"}, Rendition{Video: "1080", Audio: "aac2", Mode: "broadcast"}, "libx264", ""), " ")
	if !strings.Contains(unknown, "fps=60000/1001") {
		t.Fatalf("an unread rate stays 59.94: %s", unknown)
	}
	if facts := streamFacts(src, "tt", Rendition{Video: "1080", Audio: "aac2", Mode: "broadcast"}, "libx264", "", false, notedPicture{Width: 720, Height: 576, FPS: "25"}); facts.OutputFPS != "50" {
		t.Fatalf("stream panel output %q", facts.OutputFPS)
	}
	if line := strings.Join(PictureArgs(Graph{VideoCodec: "MPEG2", Encoder: "libx264", Rate: "25"}), " "); !strings.Contains(line, "fps=50,") {
		t.Fatalf("recording playback of 25i: %s", line)
	}
}

func TestFiftyHzLadderAndMosaic(t *testing.T) {
	src := Source{VideoCodec: "MPEG2", AudioCodec: "MPEG", Rate: "25"}
	line := strings.Join(LadderArgs(1, src, nil, "broadcast", "libx264", ""), " ")
	if !strings.Contains(line, "fps=50") || strings.Contains(line, "60000/1001") {
		t.Fatalf("ladder of 25i: %s", line)
	}
	if got := ladderFrameRate(src, "broadcast"); got != "50" {
		t.Fatalf("ladder FRAME-RATE %q", got)
	}
	if got := ladderFrameRate(Source{VideoCodec: "MPEG2"}, "broadcast"); got != "59.94" {
		t.Fatalf("ladder FRAME-RATE %q", got)
	}
	both := strings.Join(mosaicArgs([]mosaicInput{{input: "pipe:3", rate: "25"}, {input: "pipe:4", rate: "50"}}, "libx264"), " ")
	if !strings.Contains(both, "fps=50,") || strings.Contains(both, "60000/1001") {
		t.Fatalf("mosaic of 50 Hz tiles: %s", both)
	}
	mixed := strings.Join(mosaicArgs([]mosaicInput{{input: "pipe:3", rate: "25"}, {input: "pipe:4"}}, "libx264"), " ")
	if !strings.Contains(mixed, "fps=60000/1001") {
		t.Fatalf("mosaic with an unknown tile: %s", mixed)
	}
}

// The H.264 frame rate is read from the SPS timing: 1080i25 is the usual DVB
// HD picture, and its SPS counts 50 ticks a second.
func TestH264PictureRate(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	dir := t.TempDir()
	for _, c := range []struct {
		name, rate, want string
		lace             bool
	}{
		{"i25", "50", "25", true},
		{"p50", "50", "50", false},
		{"p2997", "30000/1001", "29.97", false},
	} {
		out := filepath.Join(dir, c.name+".ts")
		vf := "format=yuv420p"
		x264 := "keyint=15"
		if c.lace {
			vf = "tinterlace=interleave_top,fieldorder=tff,format=yuv420p"
			x264 += ":tff=1"
		}
		gen := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=320x240:rate="+c.rate,
			"-t", "1", "-vf", vf, "-c:v", "libx264", "-preset", "ultrafast", "-x264-params", x264, "-f", "mpegts", out)
		if b, err := gen.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v %s", c.name, err, b)
		}
		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		p, ok := pictureFacts(data, 0)
		if !ok || p.FPS != c.want {
			t.Fatalf("%s: %+v %v, want %s", c.name, p, ok, c.want)
		}
		if got := fileRate(out); got != c.want {
			t.Fatalf("%s: file rate %q", c.name, got)
		}
	}
}

// The HEVC frame rate is read from the SPS timing (or the VPS when the SPS
// has none). DVB-T2 sends HEVC at 50 and 25; ATSC 3.0 at 59.94.
func TestHEVCPictureRate(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	if out, err := exec.Command("ffmpeg", "-hide_banner", "-encoders").Output(); err != nil || !strings.Contains(string(out), "libx265") {
		t.Skip("ffmpeg has no libx265")
	}
	dir := t.TempDir()
	for _, c := range []struct {
		name, rate, want string
		lace             bool
	}{
		{"p50", "50", "50", false},
		{"p25", "25", "25", false},
		{"p2997", "30000/1001", "29.97", false},
		{"p5994", "60000/1001", "59.94", false},
		{"i25", "50", "25", true},
		{"untimed", "50", "", false},
	} {
		out := filepath.Join(dir, c.name+".ts")
		vf := "format=yuv420p"
		x265 := "keyint=15:log-level=error"
		if c.name == "untimed" {
			x265 += ":vui-timing-info=0"
		}
		if c.lace {
			// Interlaced HEVC codes each field as a picture, 50 a second.
			vf = "tinterlace=interleave_top,fieldorder=tff,separatefields,format=yuv420p"
			x265 += ":interlace=tff"
		}
		gen := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=320x240:rate="+c.rate,
			"-t", "1", "-vf", vf, "-c:v", "libx265", "-preset", "ultrafast", "-x265-params", x265, "-f", "mpegts", out)
		if b, err := gen.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v %s", c.name, err, b)
		}
		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		p, ok := pictureFacts(data, 0)
		if !ok || p.FPS != c.want || p.Width != 320 {
			t.Fatalf("%s: %+v %v, want %s", c.name, p, ok, c.want)
		}
		if got := fileRate(out); got != c.want {
			t.Fatalf("%s: file rate %q", c.name, got)
		}
	}
}

// x265 writes its timing in the SPS only. Other encoders put it in the VPS.
func TestHEVCVPSTiming(t *testing.T) {
	var w testBits
	w.put(0x4001, 16) // NAL header, VPS
	w.put(0, 4)       // id
	w.put(3, 2)       // base layer internal and available
	w.put(0, 6)       // max_layers_minus1
	w.put(0, 3)       // max_sub_layers_minus1
	w.put(1, 1)       // temporal_id_nesting
	w.put(0xffff, 16)
	w.put(0, 96)  // profile, tier, level
	w.put(1, 1)   // sub_layer_ordering_info_present
	w.putUE(4)    // max_dec_pic_buffering
	w.putUE(2)    // num_reorder
	w.putUE(0)    // max_latency
	w.put(0, 6)   // max_layer_id
	w.putUE(0)    // num_layer_sets_minus1
	w.put(1, 1)   // timing present
	w.put(1, 32)  // num_units_in_tick
	w.put(50, 32) // time_scale
	w.put(1, 8)
	es := append([]byte{0, 0, 1}, w.bytes()...)
	if units, scale := hevcVPSTiming(es); units != 1 || scale != 50 {
		t.Fatalf("vps timing %d/%d", units, scale)
	}
	if units, _ := hevcVPSTiming(es[:12]); units != 0 {
		t.Fatalf("a cut VPS has timing %d", units)
	}
}

type testBits struct {
	b []byte
	n int
}

func (w *testBits) put(v uint64, n int) {
	for i := n - 1; i >= 0; i-- {
		if w.n%8 == 0 {
			w.b = append(w.b, 0)
		}
		if v>>uint(i)&1 == 1 {
			w.b[len(w.b)-1] |= 0x80 >> uint(w.n%8)
		}
		w.n++
	}
}

func (w *testBits) putUE(v uint64) {
	v++
	n := 0
	for x := v; x > 1; x >>= 1 {
		n++
	}
	w.put(0, n)
	w.put(v, n+1)
}

func (w *testBits) bytes() []byte { return w.b }

// Timing that names no known rate leaves the rate unknown rather than guess.
func TestHEVCRateUnknownWithoutTiming(t *testing.T) {
	if got := hevcRate(0, 50, false); got != "" {
		t.Fatalf("no ticks: %q", got)
	}
	if got := hevcRate(1, 7, false); got != "" {
		t.Fatalf("odd rate: %q", got)
	}
	if got := hevcRate(1, 50, true); got != "25" {
		t.Fatalf("fields: %q", got)
	}
	if got := hevcRate(1001, 60000, false); got != "59.94" {
		t.Fatalf("59.94: %q", got)
	}
}

// ATSC 3.0 sends HEVC at 59.94 or 60. Reading its rate must not change one
// argument of any encode: the args before the rate was read are the args now.
func TestATSC3RateKeepsItsArgs(t *testing.T) {
	base := Source{VideoCodec: "HEVC", AudioCodec: "AC4", Progressive: true, AudioChannels: 6, Height: 1080}
	for _, rate := range []string{"59.94", "60", "29.97", "30"} {
		src := base
		src.Rate = rate
		for _, enc := range []string{"libx264", "h264_vaapi", "hevc_vaapi", "h264_videotoolbox"} {
			for _, video := range []string{"copy", "1080", "720", "540", "360"} {
				for _, mode := range []string{"broadcast", "smooth", "film"} {
					for _, codec := range []string{"", "hevc"} {
						spec := Rendition{Video: video, Audio: "aac2", Mode: mode, Codec: codec}
						was := RenditionArgs(1, base, spec, enc, "")
						now := RenditionArgs(1, src, spec, enc, "")
						if !slices.Equal(was, now) {
							t.Fatalf("%s %s %s: %v\n%v", rate, enc, spec.Key(), was, now)
						}
					}
				}
			}
			for _, mode := range []string{"broadcast", "film"} {
				if was, now := LadderArgs(1, base, nil, mode, enc, "pipe:0"), LadderArgs(1, src, nil, mode, enc, "pipe:0"); !slices.Equal(was, now) {
					t.Fatalf("ladder %s %s: %v\n%v", rate, enc, was, now)
				}
				if ladderFrameRate(base, mode) != ladderFrameRate(src, mode) {
					t.Fatalf("ladder FRAME-RATE %s", rate)
				}
			}
			g := Graph{VideoCodec: "HEVC", Encoder: enc, Progressive: true}
			was := PictureArgs(g)
			g.Rate = rate
			if now := PictureArgs(g); !slices.Equal(was, now) {
				t.Fatalf("recording %s %s: %v\n%v", rate, enc, was, now)
			}
		}
		was := mosaicArgs([]mosaicInput{{input: "pipe:3"}, {input: "pipe:4"}}, "libx264")
		if now := mosaicArgs([]mosaicInput{{input: "pipe:3", rate: rate}, {input: "pipe:4", rate: rate}}, "libx264"); !slices.Equal(was, now) {
			t.Fatalf("mosaic %s: %v\n%v", rate, was, now)
		}
	}
}

// A 59.94 rate read on a channel that had none is the clock it already ran
// on: nothing is rebuilt. A 25 Hz one is a new clock.
func TestRateChangeIsAClockChange(t *testing.T) {
	h, m := testHub(t)
	f := &feed{channel: store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "5.1"}}, source: Source{VideoCodec: "HEVC"}, renditions: map[string]*rendition{}}
	m.feeds["5.1"] = f
	h.channels[1] = f
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.takeRateLocked(f, "59.94") || f.source.Rate != "59.94" || f.channel.PictureRate != "59.94" {
		t.Fatalf("59.94 on an unread channel: %+v", f.source)
	}
	if h.takeRateLocked(f, "60") {
		t.Fatal("60 after 59.94 is the same clock")
	}
	if !h.takeRateLocked(f, "50") || !h.takeRateLocked(f, "29.97") {
		t.Fatal("50 Hz in or out is a new clock")
	}
}

// mpeg2RateTS is an interlaced MPEG-2 program whose sequence header names
// frame_rate_code code (3 is 25, 4 is 29.97).
func mpeg2RateTS(program int, code byte) []byte {
	es := []byte{0x00, 0x00, 0x01, 0xB3, 0x2D, 0x02, 0x40, 0x30 | code, 0xff, 0xff, 0xe0, 0x00}
	es = append(es, 0x00, 0x00, 0x01, 0xB5, 0x10, 0x00)
	es = append(es, 0x00, 0x00, 0x01, 0xB5, 0x80, 0x00, 0x00, 0x00, 0x00)
	return programTS(program, streamMPEG2, 0x100, es)
}

func ladderHub(t *testing.T) (*Hub, *mux, string) {
	t.Helper()
	bin, mark := writeFakeFFmpeg(t, "hold")
	h, m := testHub(t)
	h.FFmpeg = bin
	h.Encoder = "libx264"
	t.Cleanup(func() {
		h.mu.Lock()
		if live := h.channels[1]; live != nil {
			h.stopFeedLocked(live)
		}
		h.mu.Unlock()
	})
	return h, m, mark
}

// The first tune of a 25 Hz channel reads the rate with the scan, so the
// ladder it starts is already at 50.
func TestFirstTuneLadderTakesTheRate(t *testing.T) {
	h, m, _ := ladderHub(t)
	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()
	m.body = pr
	go h.readLoop(ctx, m)
	defer func() {
		cancel()
		_ = pw.Close()
	}()
	go writeUntil(ctx, pw, mpeg2RateTS(1, 3))
	h.mu.Lock()
	defer h.mu.Unlock()
	f := h.addFeedLocked(m, store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "7.1", VideoCodec: "MPEG2"}, ProgramNum: 1})
	if f.source.Rate != "25" || f.channel.PictureRate != "25" {
		t.Fatalf("first tune rate %q stored %q", f.source.Rate, f.channel.PictureRate)
	}
	r, err := h.ensureLadderLocked(f, ladderRung("broadcast", "1080"), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if line := strings.Join(r.args, " "); !strings.Contains(line, "fps=50") || strings.Contains(line, "60000/1001") {
		t.Fatalf("first tune ladder: %s", line)
	}
}

// A ladder that started before the header restarts once at the rate the
// header names, with its viewers. One header that changes the scan and the
// rate is one restart.
func TestLateRateRestartsTheLadderOnce(t *testing.T) {
	h, m, mark := ladderHub(t)
	h.mu.Lock()
	f := h.addFeedLocked(m, store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "7.1", VideoCodec: "MPEG2"}, ProgramNum: 1, FieldOrder: "tt"})
	r, err := h.ensureLadderLocked(f, ladderRung("broadcast", "1080"), time.Time{})
	if err != nil {
		h.mu.Unlock()
		t.Fatal(err)
	}
	r.viewers = 2
	h.mu.Unlock()
	waitMark(t, mark, ".count")
	h.finishScan(m, f, &scanBuf{wake: make(chan struct{}, 1), b: mpeg2RateTS(1, 3)}, nil)
	h.mu.Lock()
	now := f.ladderPrimary("broadcast")
	h.mu.Unlock()
	if now == nil || now == r || now.viewers != 2 {
		t.Fatalf("ladder after the rate: %+v", now)
	}
	if line := strings.Join(now.args, " "); !strings.Contains(line, "fps=50") {
		t.Fatalf("restarted ladder: %s", line)
	}
	deadline := time.Now().Add(3 * time.Second)
	for markCount(t, mark) < 2 && time.Now().Before(deadline) {
		time.Sleep(15 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	if n := markCount(t, mark); n != 2 {
		t.Fatalf("ladder started %d times, want 2", n)
	}
}

// A US station's 29.97 is the clock the ladder already runs on: reading it
// restarts nothing.
func TestUSRateKeepsTheLadder(t *testing.T) {
	h, m, _ := ladderHub(t)
	h.mu.Lock()
	f := h.addFeedLocked(m, store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "7.1", VideoCodec: "MPEG2"}, ProgramNum: 1, FieldOrder: "tt"})
	r, err := h.ensureLadderLocked(f, ladderRung("broadcast", "1080"), time.Time{})
	if err != nil {
		h.mu.Unlock()
		t.Fatal(err)
	}
	r.viewers = 1
	h.mu.Unlock()
	h.finishScan(m, f, &scanBuf{wake: make(chan struct{}, 1), b: mpeg2RateTS(1, 4)}, nil)
	h.mu.Lock()
	defer h.mu.Unlock()
	if f.source.Rate != "29.97" {
		t.Fatalf("rate %q", f.source.Rate)
	}
	if now := f.ladderPrimary("broadcast"); now != r {
		t.Fatal("a 29.97 rate restarted the ladder")
	}
}
