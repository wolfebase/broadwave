package live

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"broadwave/internal/store"
)

func TestALeadCarriesSilenceForEachSoundAndEndsWithItsPicture(t *testing.T) {
	src := Source{VideoCodec: "HEVC", AudioCodec: "AC4", Progressive: true, AudioChannels: 6,
		Extras: []AudioTrack{{PID: 0x33, Codec: "ac4", Channels: 2, Language: "spa"}}}
	args := RenditionArgs(0, src, Rendition{Video: "copy", Audio: "ac3"}, "", "")
	lead, ok := leadArgs(args, 9411.1425, []leadSound{{layout: "5.1(side)", lang: "eng"}, {layout: "stereo", lang: "spa"}})
	if !ok {
		t.Fatalf("no lead for %v", args)
	}
	line := strings.Join(lead, " ")
	for _, want := range []string{
		"-analyzeduration 300000",
		"-i pipe:0 -itsoffset 9411.142500 -f lavfi -i anullsrc=r=48000:cl=5.1(side) -itsoffset 9411.142500 -f lavfi -i anullsrc=r=48000:cl=stereo",
		"-map 0:v:0 -map 1:a:0 -map 2:a:0",
		"-c:a ac3 -b:a 448k -b:a:1 192k",
		"-metadata:s:a:0 language=eng -metadata:s:a:1 language=spa -shortest -f mp4",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("lead args lack %q:\n%s", want, line)
		}
	}
	if strings.Contains(line, "0:i:51") || strings.Contains(line, "0:a:0") {
		t.Errorf("the lead still maps the broadcast's sound:\n%s", line)
	}
	// One silence short of the encode's sounds is no lead.
	if _, ok := leadArgs(args, 1, []leadSound{{layout: "5.1(side)"}}); ok {
		t.Fatal("a lead with a sound missing")
	}
}

func TestOnlyACopiedPictureOfConvertedAC4GetsALead(t *testing.T) {
	h := New(nil, t.TempDir(), "ffmpeg", "libx264")
	m := &mux{freq: 539000000, tuner: -1, feeds: map[string]*feed{}, cancel: func() {}}
	h.muxes[m.freq] = m
	f := h.addFeedLocked(m, store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "104.1", VideoCodec: "HEVC", AudioCodec: "AC4"}})
	f.source.AudioChannels = 6
	f.tracks = []AudioTrack{{PID: 0x32, Codec: "ac4", Channels: 6, Language: "eng", Role: "main"}}
	cases := []struct {
		spec Rendition
		want bool
	}{
		{Rendition{Video: "copy", Audio: "ac3"}, true},
		{Rendition{Video: "copy", Audio: "aac2"}, true},
		{Rendition{Video: "copy", Audio: "copy"}, false},
		{Rendition{Video: "copy", Audio: "none"}, false},
		{Rendition{Video: "1080", Audio: "aac2"}, false},
	}
	for _, c := range cases {
		sounds, ok := h.leadSoundsLocked(f, c.spec, nil, "pipe:0")
		if ok != c.want {
			t.Errorf("%s: lead %v, want %v", c.spec.Key(), ok, c.want)
		}
		if ok && (len(sounds) != 1 || sounds[0].lang != "eng") {
			t.Errorf("%s: sounds %+v", c.spec.Key(), sounds)
		}
	}
	if _, ok := h.leadSoundsLocked(f, Rendition{Video: "copy", Audio: "ac3"}, nil, "http://x/stream"); ok {
		t.Error("a link got a lead")
	}
	// A second sound of unknown width cannot be matched by silence.
	if _, ok := h.leadSoundsLocked(f, Rendition{Video: "copy", Audio: "ac3"}, []AudioTrack{{PID: 0x33, Codec: "ac4"}}, "pipe:0"); ok {
		t.Error("an unmeasured extra got a lead")
	}
	h.NoLead = true
	if _, ok := h.leadSoundsLocked(f, Rendition{Video: "copy", Audio: "ac3"}, nil, "pipe:0"); ok {
		t.Error("NoLead still leads")
	}
	ac3 := *f
	ac3.source.AudioCodec, ac3.tracks = "AC3", nil
	h.NoLead = false
	if _, ok := h.leadSoundsLocked(&ac3, Rendition{Video: "copy", Audio: "aac2"}, nil, "pipe:0"); ok {
		t.Error("an AC-3 channel got a lead")
	}
}

// leadPacket is one 188-byte packet on pid; a PES start carries pts.
func leadPacket(pid int, start bool, pts int64, payload []byte) []byte {
	pkt := make([]byte, 188)
	pkt[0] = 0x47
	pkt[1] = byte(pid>>8) & 0x1f
	if start {
		pkt[1] |= 0x40
	}
	pkt[2] = byte(pid)
	pkt[3] = 0x10
	body := pkt[4:4]
	if start {
		body = append(body, 0, 0, 1, 0xe0, 0, 0, 0x80, 0x80, 5)
		body = append(body, byte(0x21|(pts>>29)&0x0e), byte(pts>>22), byte(0x01|(pts>>14)&0xfe), byte(pts>>7), byte(0x01|(pts<<1)&0xfe))
	}
	body = append(body, payload...)
	for len(body) < 184 {
		body = append(body, 0xff)
	}
	return pkt
}

type bufWriter struct {
	bytes.Buffer
	closed bool
}

func (w *bufWriter) Close() error { w.closed = true; return nil }

type lockedBuf struct {
	mu     sync.Mutex
	b      bytes.Buffer
	closed chan struct{}
}

func (w *lockedBuf) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func (w *lockedBuf) Close() error { close(w.closed); return nil }

func TestTheTeeStartsTheLeadOnTheFirstPictureAndCutsItBeforeAKeyframe(t *testing.T) {
	real := &bufWriter{}
	tee := newLeadTee(real)
	tee.pipe = &programPipe{video: 0x31, kind: streamHEVC}
	var got int64 = -1
	tee.onPicture = func(pts int64) { got = pts }
	sps := []byte{0, 0, 1, 0x42, 1} // an HEVC SPS NAL header (type 33)
	plain := []byte{0, 0, 1, 0x02, 1}
	head := leadPacket(0, false, 0, nil)
	first := leadPacket(0x31, true, 9000, sps)
	if _, err := tee.Write(append(append([]byte{}, head...), first...)); err != nil {
		t.Fatal(err)
	}
	if got != 9000 {
		t.Fatalf("first picture %d", got)
	}
	if pts, at, ok := tee.arrival(); !ok || pts != 9000 || time.Since(at) > time.Second {
		t.Fatalf("arrival %d %v %v", pts, at, ok)
	}
	lead := &lockedBuf{closed: make(chan struct{})}
	tee.attach(lead)
	tee.Write(leadPacket(0x31, true, 12000, plain))
	tee.cut.Store(true)
	// The cut waits for a picture that starts a group.
	tee.Write(leadPacket(0x31, true, 15000, plain))
	tee.Write(append(leadPacket(0x34, false, 0, nil), leadPacket(0x31, true, 18000, sps)...))
	tee.Write(leadPacket(0x31, true, 21000, plain))
	select {
	case <-lead.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("the lead's input never ended")
	}
	lead.mu.Lock()
	n := lead.b.Len() / 188
	lead.mu.Unlock()
	// head, first, 12000, 15000, and the packet in front of the keyframe.
	if n != 5 {
		t.Fatalf("lead got %d packets", n)
	}
	if real.Len()/188 != 7 {
		t.Fatalf("the real encode got %d packets", real.Len()/188)
	}
}

func TestATeeThatCannotReadPacketsLetsThePackagerMoveOn(t *testing.T) {
	le, err := newLeadEncode(&nopWriter{}, []leadSound{{layout: "stereo"}})
	if err != nil {
		t.Fatal(err)
	}
	le.tee.pipe = &programPipe{}
	le.tee.Write([]byte("not a packet"))
	done := make(chan error, 1)
	go func() {
		_, err := io.ReadAll(le.out)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the lead's pipe stayed open")
	}
}

// The packager plays a lead's silent picture and continues on the real
// encode at the lead's next keyframe, with one header and no break.
func TestTheRealEncodeContinuesTheLeadWithoutABreak(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	dir := t.TempDir()
	sample := filepath.Join(dir, "sample.ts")
	// B-frames reorder pictures across the cut; a keyframe every half second.
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=30",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-t", "4", "-c:v", "libx264", "-preset", "veryfast", "-bf", "2", "-g", "15", "-keyint_min", "15", "-sc_threshold", "0", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-ac", "2", "-output_ts_offset", "1000", "-f", "mpegts", sample)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sample: %v %s", err, out)
	}
	data, err := os.ReadFile(sample)
	if err != nil {
		t.Fatal(err)
	}
	args := renditionArgs(0, Source{VideoCodec: "h264", AudioCodec: "aac", Progressive: true}, Rendition{Video: "copy", Audio: "aac2"}, "", "", "pipe:0")
	real := exec.Command("ffmpeg", args...)
	realIn, err := real.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	realOut, gate, done, err := packOutput(real)
	if err != nil {
		t.Fatal(err)
	}
	if err := real.Start(); err != nil {
		t.Fatal(err)
	}
	le, err := newLeadEncode(realIn, []leadSound{{layout: "stereo"}})
	if err != nil {
		t.Fatal(err)
	}
	pack := filepath.Join(dir, "pack")
	if err := os.MkdirAll(pack, 0o755); err != nil {
		t.Fatal(err)
	}
	in := startPackOn(pack, le.out, gate, done, nil, true)
	in.onTakeover = func() { le.tee.cut.Store(true) }
	in.followTakeover(realOut)
	le.tee.pipe = &programPipe{video: 0x100, kind: streamH264}
	var lead *exec.Cmd
	le.tee.onPicture = func(pts int64) {
		largs, ok := leadArgs(args, float64(pts)/90000, le.sounds)
		if !ok {
			t.Errorf("no lead args")
			return
		}
		lead = exec.Command("ffmpeg", largs...)
		lead.Stdout = le.w
		stdin, err := lead.StdinPipe()
		if err != nil {
			t.Error(err)
			return
		}
		if err := lead.Start(); err != nil {
			t.Error(err)
			return
		}
		le.tee.closeOut()
		le.tee.attach(stdin)
	}
	// The lead has a second of the program before the real encode reads
	// any, as when AC-4 cannot decode yet; then both read on together.
	quarter := len(data) / 4 / 188 * 188
	le.tee.feed(data[:quarter])
	if _, err := realIn.Write(data[:quarter]); err != nil {
		t.Fatal(err)
	}
	for off := quarter; off < len(data); off += 188 * 64 {
		if _, err := le.tee.Write(data[off:min(off+188*64, len(data))]); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	_ = le.tee.Close()
	if err := real.Wait(); err != nil {
		t.Fatal(err)
	}
	if lead != nil {
		_ = lead.Wait()
	}
	in.end()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the packager did not finish")
	}
	playlist, err := os.ReadFile(filepath.Join(pack, "index.m3u8"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(playlist, []byte("DISCONTINUITY")) {
		t.Fatalf("the takeover broke the playlist:\n%s", playlist)
	}
	init, err := os.ReadFile(filepath.Join(pack, "init.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	video, _, _ := videoTrack(init)
	// Every picture once, in decode order with no gap.
	var pictures int
	var next int64 = -1
	for _, line := range strings.Split(string(playlist), "\n") {
		if !strings.HasPrefix(line, "seg") {
			continue
		}
		seg, err := os.ReadFile(filepath.Join(pack, line))
		if err != nil {
			t.Fatal(err)
		}
		for len(seg) >= 8 {
			size := int(be32(seg))
			if size < 8 || size > len(seg) {
				break
			}
			if string(seg[4:8]) == "moof" {
				for _, tr := range boxes(seg[8:size]) {
					if tr.kind != "traf" {
						continue
					}
					run, ok := runTiming(tr.body)
					if !ok || run.id != video {
						continue
					}
					if next >= 0 && run.decodeTime() != next {
						t.Fatalf("%s: decode time %d after %d", line, run.decodeTime(), next)
					}
					d := run.decodeTime()
					for _, s := range run.samples {
						d += run.sampleDur(s)
						pictures++
					}
					next = d
				}
			}
			seg = seg[size:]
		}
	}
	if pictures != 120 {
		t.Fatalf("%d pictures in the playlist, want 120:\n%s", pictures, playlist)
	}
	if !slices.ContainsFunc(strings.Split(string(playlist), "\n"), func(s string) bool { return strings.HasPrefix(s, "seg") }) {
		t.Fatal("no segments")
	}
}

func be32(b []byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

func TestACopied3Point0PictureStartsWithALeadThatStopsWithIt(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec cat >/dev/null\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := New(nil, dir, script, "libx264")
	defer lockHub(h)()
	m := &mux{freq: 539000000, tuner: -1, feeds: map[string]*feed{}, cancel: func() {}}
	h.muxes[m.freq] = m
	f := h.addFeedLocked(m, store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "104.1", VideoCodec: "HEVC", AudioCodec: "AC4"}, FieldOrder: "progressive"})
	f.source.AudioChannels = 6
	r, err := h.ensureRenditionLocked(f, Rendition{Video: "copy", Audio: "ac3"})
	if err != nil {
		t.Fatal(err)
	}
	if r.lead == nil || r.lead.out == nil || r.lead.tee.pipe == nil {
		t.Fatalf("lead %+v", r.lead)
	}
	web, err := h.ensureRenditionLocked(f, Rendition{Video: "1080", Audio: "aac2"})
	if err != nil {
		t.Fatal(err)
	}
	if web.lead == nil || web.lead.out != nil {
		t.Fatalf("a transcode should only date by arrival: %+v", web.lead)
	}
	le := r.lead
	h.stopRenditionLocked(f, r.spec.Key())
	if !le.ended || !le.tee.gone.Load() {
		t.Fatal("the lead outlived its rendition")
	}
	h.stopRenditionLocked(f, web.spec.Key())
}

func TestAnAC4PictureIsDatedByItsFirstPicturesArrival(t *testing.T) {
	le := datingOnly(&bufWriter{})
	arrived := time.Now().Add(-3 * time.Second)
	le.tee.pts.Store(900000 + 90000)
	le.tee.at.Store(arrived.UnixNano())
	in := &packInput{}
	in.noteEncodeStart(10, true) // fragment time zero is 900000 on the broadcast clock
	r := &rendition{lead: le, input: in, clock: NewTimeline()}
	f := &feed{renditions: map[string]*rendition{"copy.ac3": r}}
	seedClockLocked(f, r)
	got := r.clock.Wall(90000)
	if want := arrived.Add(-ringDelay); !got.Equal(want) {
		t.Fatalf("first picture dated %v, want %v", got, want)
	}
}
