package live

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"broadwave/internal/store"
)

func TestPictureFacts(t *testing.T) {
	// 1280×720, 16:9, frame_rate_code 7 (59.94).
	seq := []byte{0x00, 0x00, 0x01, 0xB3, 0x50, 0x02, 0xD0, 0x37}
	got, ok := pictureFacts(programTS(1, streamMPEG2, 0x100, seq), 1)
	if !ok || got.Width != 1280 || got.Height != 720 || got.FPS != "59.94" {
		t.Fatalf("mpeg2 picture: %+v ok=%v", got, ok)
	}
	if _, ok := pictureFacts(programTS(1, streamMPEG2, 0x100, seq), 2); ok {
		t.Fatal("wrong program")
	}
}

func TestPictureFactsFFmpeg(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	path := t.TempDir() + "/s.ts"
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=30",
		"-t", "0.4", "-c:v", "libx264", "-pix_fmt", "yuv420p",
		"-f", "mpegts", path,
	}
	if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v %s", err, out)
	}
	raw, err := exec.Command("cat", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	got, ok := pictureFacts(raw, 1)
	if !ok || got.Width != 320 || got.Height != 240 {
		t.Fatalf("h264 picture: %+v ok=%v", got, ok)
	}

	// 1080 lines are stored as 1088 and cropped. The panel must show 1080.
	hd := t.TempDir() + "/hd.ts"
	args = []string{
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=1920x1080:rate=30",
		"-t", "0.2", "-c:v", "libx264", "-pix_fmt", "yuv420p",
		"-f", "mpegts", hd,
	}
	if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v %s", err, out)
	}
	raw, err = os.ReadFile(hd)
	if err != nil {
		t.Fatal(err)
	}
	got, ok = pictureFacts(raw, 1)
	if !ok || got.Width != 1920 || got.Height != 1080 {
		t.Fatalf("cropped h264 picture: %+v ok=%v", got, ok)
	}
}

func TestFilmDoesNotStick(t *testing.T) {
	if storedFieldOrder("film") != "tt" {
		t.Fatal("film must be stored as interlaced")
	}
	if storedFieldOrder("progressive") != "progressive" || storedFieldOrder("bb") != "bb" {
		t.Fatal("a stable scan type is stored as itself")
	}
	src := sourceOf(store.SourceChannel{FieldOrder: "film"})
	if src.Film || src.Progressive {
		t.Fatalf("a stored film flag must not lock the channel: %+v", src)
	}
	prog := sourceOf(store.SourceChannel{FieldOrder: "progressive"})
	if !prog.Progressive || prog.Film {
		t.Fatalf("progressive stays progressive: %+v", prog)
	}
}

func TestScanTypeCapturedHeaders(t *testing.T) {
	prog := mpeg2TS(1, true)
	if order, ok := scanType(prog, 1); !ok || order != "progressive" {
		t.Fatalf("progressive sequence: got %q %v", order, ok)
	}
	lace := mpeg2TS(1, false)
	if order, ok := scanType(lace, 1); !ok || order != "tt" {
		t.Fatalf("interlaced sequence: got %q %v", order, ok)
	}
	// progressive_frame=1 alone is not enough: film sets it in an interlaced sequence.
	film := programTS(1, streamMPEG2, 0x100, []byte{0x00, 0x00, 0x01, 0xB5, 0x80, 0x00, 0x00, 0x00, 0x80})
	if _, ok := scanType(film, 1); ok {
		t.Fatal("progressive picture header must not decide the sequence")
	}
	if order, ok := scanType(h264TS(1, true), 1); !ok || order != "progressive" {
		t.Fatalf("h264 frames: got %q %v", order, ok)
	}
	if order, ok := scanType(filmTS(1), 1); !ok || order != "film" {
		t.Fatalf("soft 3:2: got %q %v", order, ok)
	}
	if order, ok := scanType(h264TS(1, false), 1); !ok || order != "tt" {
		t.Fatalf("h264 fields: got %q %v", order, ok)
	}
	if _, ok := scanType(prog, 99); ok {
		t.Fatal("wrong program should not match")
	}
}

func TestScanTypeFFmpegHeaders(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"mpeg2 progressive", []string{"-c:v", "mpeg2video", "-b:v", "2M"}, "progressive"},
		{"mpeg2 interlaced", []string{"-c:v", "mpeg2video", "-b:v", "2M", "-flags", "+ilme+ildct", "-top", "1"}, "tt"},
		{"h264 frames", []string{"-c:v", "libx264", "-pix_fmt", "yuv420p"}, "progressive"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := t.TempDir() + "/s.ts"
			args := []string{"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=30", "-t", "0.4", "-f", "mpegts"}
			args = append(args, tc.args...)
			args = append(args, path)
			if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
				t.Fatalf("ffmpeg: %v %s", err, out)
			}
			raw, err := exec.Command("cat", path).Output()
			if err != nil {
				t.Fatal(err)
			}
			order, ok := scanType(raw, 1)
			if !ok || order != tc.want {
				t.Fatalf("got %q ok=%v, want %s", order, ok, tc.want)
			}
		})
	}
}

func TestStoredProgressiveStillScansForFilm(t *testing.T) {
	h, m := testHub(t)
	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()
	m.body = pr
	go h.readLoop(ctx, m)
	defer func() {
		cancel()
		_ = pw.Close()
	}()
	f := &feed{
		channel:    store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "5.1", VideoCodec: "MPEG2"}, FrequencyHz: m.freq, FieldOrder: "progressive"},
		source:     Source{VideoCodec: "MPEG2", Progressive: true},
		program:    1,
		tracks:     []AudioTrack{{PID: 0x101, Role: "main", Codec: "ac3"}},
		renditions: map[string]*rendition{},
	}
	m.feeds["5.1"] = f
	h.channels[1] = f
	go writeUntil(ctx, pw, filmTS(1))
	h.learnScanLocked(m, f)
	if !f.source.Film || f.source.Progressive {
		t.Fatalf("stored progressive skipped film: %+v", f.source)
	}
	if f.channel.FieldOrder != "tt" {
		t.Fatalf("film stored as %q", f.channel.FieldOrder)
	}
	if f.headerOrder != "film" {
		t.Fatalf("header %q", f.headerOrder)
	}
}

func TestLateHeaderAppliesAfterTheWindow(t *testing.T) {
	h, m := testHub(t)
	f := &feed{
		channel:    store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "4.1", VideoCodec: "MPEG2"}, FrequencyHz: m.freq},
		source:     Source{VideoCodec: "MPEG2"},
		program:    1,
		renditions: map[string]*rendition{},
	}
	m.feeds["4.1"] = f
	h.channels[1] = f
	buf := &scanBuf{wake: make(chan struct{}, 1), b: mpeg2TS(1, true)}
	h.finishScan(m, f, buf, nil)
	if !f.source.Progressive || f.source.Film || f.headerOrder != "progressive" {
		t.Fatalf("late progressive: %+v %q", f.source, f.headerOrder)
	}
	if f.channel.FieldOrder != "progressive" {
		t.Fatalf("stored %q", f.channel.FieldOrder)
	}

	film := &feed{
		channel:    store.SourceChannel{Channel: store.Channel{ID: 2, GuideNumber: "5.1", VideoCodec: "MPEG2"}, FrequencyHz: m.freq, FieldOrder: "progressive"},
		source:     Source{VideoCodec: "MPEG2", Progressive: true},
		program:    1,
		renditions: map[string]*rendition{},
	}
	h.channels[2] = film
	buf = &scanBuf{wake: make(chan struct{}, 1), b: filmTS(1)}
	h.finishScan(m, film, buf, nil)
	if !film.source.Film || film.source.Progressive || film.channel.FieldOrder != "tt" || film.headerOrder != "film" {
		t.Fatalf("late film: %+v stored %q header %q", film.source, film.channel.FieldOrder, film.headerOrder)
	}
}

func TestProbeDoesNotOverrideTheHeader(t *testing.T) {
	h, _ := testHub(t)
	f := &feed{
		channel:     store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "5.1", VideoCodec: "MPEG2"}, FieldOrder: "tt"},
		source:      Source{VideoCodec: "MPEG2", Film: true},
		headerOrder: "film",
		renditions:  map[string]*rendition{},
	}
	h.applyProbeLocked(f, "progressive")
	if !f.source.Film || f.source.Progressive || f.channel.FieldOrder != "tt" {
		t.Fatalf("probe overrode film: %+v stored %q", f.source, f.channel.FieldOrder)
	}
}

func TestProbeRebuildsTheRunningGraph(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	h, m := testHub(t)
	h.FFmpeg = "ffmpeg"
	h.Encoder = "libx264"
	f := &feed{
		channel:    store.SourceChannel{Channel: store.Channel{ID: 1, GuideNumber: "4.1", VideoCodec: "MPEG2"}, FrequencyHz: m.freq},
		source:     Source{VideoCodec: "MPEG2"},
		program:    1,
		renditions: map[string]*rendition{},
		timeline:   NewTimeline(),
	}
	m.feeds["4.1"] = f
	h.channels[1] = f
	t.Cleanup(func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.stopFeedLocked(f)
	})
	spec, ok := ParseRenditionKey("1080.aac2.broadcast")
	if !ok {
		t.Fatal("rendition key")
	}
	r, err := h.ensureRenditionLocked(f, spec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(r.args, " "), "bwdif") {
		t.Fatalf("started without field deinterlace: %s", strings.Join(r.args, " "))
	}
	seen := time.Now().Add(-time.Second)
	r.viewers = 2
	r.seen = seen
	h.applyProbeLocked(f, "progressive")
	nr := f.renditions[spec.Key()]
	if nr == nil || nr == r {
		t.Fatal("probe did not rebuild the rendition")
	}
	if nr.viewers != 2 || !nr.seen.Equal(seen) {
		t.Fatalf("viewers %d", nr.viewers)
	}
	line := strings.Join(nr.args, " ")
	if strings.Contains(line, "bwdif") || strings.Contains(line, "60000/1001") {
		t.Fatalf("rebuilt graph still field-deinterlaces: %s", line)
	}
	if !f.source.Progressive || f.channel.FieldOrder != "progressive" {
		t.Fatalf("stored %+v %q", f.source, f.channel.FieldOrder)
	}

	// A stored progressive rendition is rebuilt when the late header is film.
	r2, err := h.ensureRenditionLocked(f, spec)
	if err != nil {
		t.Fatal(err)
	}
	if r2 != nr {
		t.Fatal("same rendition should still be running")
	}
	buf := &scanBuf{wake: make(chan struct{}, 1), b: filmTS(1)}
	h.finishScan(m, f, buf, nil)
	filmR := f.renditions[spec.Key()]
	if filmR == nil || filmR == nr {
		t.Fatal("film header did not rebuild the rendition")
	}
	if filmR.viewers != 2 {
		t.Fatalf("viewers %d", filmR.viewers)
	}
	filmLine := strings.Join(filmR.args, " ")
	if !strings.Contains(filmLine, "pullup") || strings.Contains(filmLine, "bwdif") {
		t.Fatalf("film graph: %s", filmLine)
	}
	if !f.source.Film || f.source.Progressive || f.channel.FieldOrder != "tt" {
		t.Fatalf("film stored %+v %q", f.source, f.channel.FieldOrder)
	}
}

func writeUntil(ctx context.Context, w io.Writer, payload []byte) {
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if _, err := w.Write(payload); err != nil {
				return
			}
		}
	}
}

func mpeg2TS(program int, progressive bool) []byte {
	ext := []byte{0x00, 0x00, 0x01, 0xB5, 0x10, 0x00}
	if progressive {
		ext[5] = 0x08
	} else {
		// picture_coding_extension, progressive_frame clear (MSB of the 5th payload byte).
		pic := []byte{0x00, 0x00, 0x01, 0xB5, 0x80, 0x00, 0x00, 0x00, 0x00}
		ext = append(ext, pic...)
	}
	return programTS(program, streamMPEG2, 0x100, ext)
}

func filmTS(program int) []byte {
	// Interlaced sequence, then four progressive pictures with repeat_first_field on two of them.
	es := []byte{0x00, 0x00, 0x01, 0xB5, 0x10, 0x00}
	for i := 0; i < 4; i++ {
		pic := []byte{0x00, 0x00, 0x01, 0xB5, 0x80, 0x00, 0x00, 0x00, 0x80}
		if i%2 == 0 {
			pic[7] = 0x02
		}
		es = append(es, pic...)
	}
	return programTS(program, streamMPEG2, 0x100, es)
}

func h264TS(program int, frames bool) []byte {
	// Main profile SPS: frame_mbs_only_flag is the last bit written below.
	sps := []byte{0x4d, 0x00, 0x28, 0xfb, 0x00}
	if frames {
		sps[4] = 0x80
	}
	nal := append([]byte{0x00, 0x00, 0x01, 0x67}, sps...)
	return programTS(program, streamH264, 0x100, nal)
}

func programTS(program, streamType, videoPID int, es []byte) []byte {
	const pmtPID = 0x1000
	pat := psiSection(0x00, append([]byte{
		0x00, 0x01, 0xc1, 0x00, 0x00,
	}, progPID(program, pmtPID)...))
	pmt := psiSection(0x02, pmtBody(program, streamType, videoPID))
	pes := pesPacket(es)
	var out []byte
	out = append(out, tsPacket(0, true, pat)...)
	out = append(out, tsPacket(pmtPID, true, pmt)...)
	out = append(out, tsPacket(videoPID, true, pes)...)
	return out
}

func progPID(program, pid int) []byte {
	return []byte{
		byte(program >> 8), byte(program),
		0xe0 | byte(pid>>8), byte(pid),
	}
}

func pmtBody(program, streamType, videoPID int) []byte {
	body := []byte{
		byte(program >> 8), byte(program),
		0xc1, 0x00, 0x00,
		0xe0, 0x00, // PCR
		0xf0, 0x00, // program info length
		byte(streamType),
		0xe0 | byte(videoPID>>8), byte(videoPID),
		0xf0, 0x00,
	}
	return body
}

func psiSection(tableID byte, body []byte) []byte {
	// length covers body + CRC.
	n := len(body) + 4
	sec := []byte{tableID, 0xb0 | byte(n>>8), byte(n)}
	sec = append(sec, body...)
	sec = append(sec, 0, 0, 0, 0)
	pkt := []byte{0x00} // pointer
	return append(pkt, sec...)
}

func pesPacket(es []byte) []byte {
	pes := []byte{0x00, 0x00, 0x01, 0xe0, 0x00, 0x00, 0x80, 0x00, 0x00}
	return append(pes, es...)
}

func tsPacket(pid int, start bool, payload []byte) []byte {
	pkt := make([]byte, 188)
	pkt[0] = 0x47
	pkt[1] = byte(pid >> 8)
	if start {
		pkt[1] |= 0x40
	}
	pkt[2] = byte(pid)
	pkt[3] = 0x10
	if len(payload) > 184 {
		payload = payload[:184]
	}
	copy(pkt[4:], payload)
	return pkt
}
