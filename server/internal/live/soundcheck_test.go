package live

import (
	"context"
	"encoding/binary"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/store"
)

const synthRate = 48000

// speech is sound with speech's shape: syllable-long bursts at changing
// loudness, in phrases with pauses between them.
func speech(seed int64, seconds float64) []float64 {
	r := rand.New(rand.NewSource(seed))
	out := make([]float64, int(seconds*synthRate))
	at := 0
	for at < len(out) {
		for range 3 + r.Intn(6) {
			n := int((0.1 + 0.15*r.Float64()) * synthRate)
			amp := 0.3 + 0.7*r.Float64()
			for i := 0; i < n && at+i < len(out); i++ {
				shape := math.Sin(math.Pi * float64(i) / float64(n))
				out[at+i] = amp * shape * (2*r.Float64() - 1)
			}
			at += n + int((0.03+0.09*r.Float64())*synthRate)
		}
		at += int((0.3 + 0.5*r.Float64()) * synthRate)
	}
	return out
}

// crowd is a stadium under the commentary: steady noise that swells.
func crowd(seed int64, seconds float64) []float64 {
	r := rand.New(rand.NewSource(seed))
	out := make([]float64, int(seconds*synthRate))
	level := 0.1
	for i := range out {
		if i%synthRate == 0 {
			level = 0.05 + 0.1*r.Float64()
		}
		out[i] = level * (2*r.Float64() - 1)
	}
	return out
}

func mix(parts ...[]float64) []float64 {
	out := make([]float64, len(parts[0]))
	for _, p := range parts {
		for i := range out {
			if i < len(p) {
				out[i] += p[i]
			}
		}
	}
	return out
}

func scaled(p []float64, k float64) []float64 {
	out := make([]float64, len(p))
	for i, v := range p {
		out[i] = v * k
	}
	return out
}

func writePCM(t *testing.T, path string, samples []float64) {
	t.Helper()
	b := make([]byte, 2*len(samples))
	for i, v := range samples {
		binary.LittleEndian.PutUint16(b[2*i:], uint16(int16(max(-1, min(1, v))*30000)))
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeTS encodes each track to AC-3 on its PID with its language, as a
// station would send them.
func writeTS(t *testing.T, ffmpeg, path string, pids []int, langs []string, tracks ...[]float64) {
	t.Helper()
	dir := t.TempDir()
	var args []string
	for i, tr := range tracks {
		in := filepath.Join(dir, strconv.Itoa(i)+".pcm")
		writePCM(t, in, tr)
		args = append(args, "-f", "s16le", "-ar", strconv.Itoa(synthRate), "-ac", "1", "-i", in)
	}
	for i := range tracks {
		args = append(args, "-map", strconv.Itoa(i)+":a")
	}
	args = append(args, "-c:a", "ac3", "-b:a", "128k")
	for i := range tracks {
		args = append(args, "-metadata:s:a:"+strconv.Itoa(i), "language="+langs[i], "-streamid", strconv.Itoa(i)+":"+strconv.Itoa(pids[i]))
	}
	args = append(args, "-f", "mpegts", "-y", path)
	if out, err := exec.Command(ffmpeg, append([]string{"-hide_banner", "-loglevel", "error"}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v\n%s", err, out)
	}
}

// decodeTS runs the check's own decode on a file, as it runs on a tune.
func decodeTS(t *testing.T, ffmpeg, path string, pids []int, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	cmd := exec.Command(ffmpeg, soundArgs(pids, dir)...)
	cmd.Stdin = in
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
}

// soundScene is a game with English and Spanish commentary over one crowd.
type soundScene struct {
	eng, spa, crowd []float64
}

func newScene() soundScene {
	return soundScene{eng: speech(1, 60), spa: speech(2, 60), crowd: crowd(3, 60)}
}

func (s soundScene) english() []float64 { return mix(s.eng, s.crowd) }
func (s soundScene) spanish() []float64 { return mix(s.spa, s.crowd) }

// twinOf is the regular broadcast's main: the channel's sound from 25 s on
// for 15 s, as the channel's own tune hears it later, encoded again at
// another rate with a little noise of its own.
func twinOf(src []float64) []float64 {
	part := src[25*synthRate : 40*synthRate]
	return mix(part, scaled(crowd(9, 15), 0.1))
}

// judgeScene encodes the channel (PID 50 labelled eng, PID 51 labelled spa)
// and the twin (PID 52, eng), decodes them as a check does, and judges.
func judgeScene(t *testing.T, at50, at51, twin []float64) soundVerdict {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	near, far := filepath.Join(dir, "near.ts"), filepath.Join(dir, "far.ts")
	writeTS(t, ffmpeg, near, []int{50, 51}, []string{"eng", "spa"}, at50, at51)
	writeTS(t, ffmpeg, far, []int{52}, []string{"eng"}, twin)
	decodeTS(t, ffmpeg, near, []int{50, 51}, filepath.Join(dir, "n"))
	decodeTS(t, ffmpeg, far, []int{52}, filepath.Join(dir, "f"))
	v, err := judgeFiles(soundFile(filepath.Join(dir, "f"), 52), soundFile(filepath.Join(dir, "n"), 50), soundFile(filepath.Join(dir, "n"), 51))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("main %.2f second %.2f alike %.2f ok %v swapped %v %s", v.main, v.lang, v.same, v.ok, v.swapped, v.why)
	return v
}

func TestSoundCheckFindsTheTwinsLanguageInEitherLabelOrder(t *testing.T) {
	s := newScene()
	// Labels right: PID 50, marked English, is English.
	v := judgeScene(t, s.english(), s.spanish(), twinOf(s.english()))
	if !v.ok || v.swapped {
		t.Fatalf("labels right: %+v, want decided, not swapped", v)
	}
	// Labels wrong: the track marked English carries Spanish.
	v = judgeScene(t, s.spanish(), s.english(), twinOf(s.english()))
	if !v.ok || !v.swapped {
		t.Fatalf("labels wrong: %+v, want decided, swapped", v)
	}
}

func TestSoundCheckRefusesWhatDoesNotDecide(t *testing.T) {
	s := newScene()
	silent := make([]float64, 15*synthRate)
	if v := judgeScene(t, s.english(), s.spanish(), silent); v.ok {
		t.Fatalf("a silent twin decided: %+v", v)
	}
	if v := judgeScene(t, s.english(), s.english(), twinOf(s.english())); v.ok || v.same < soundSame {
		t.Fatalf("two tracks of one sound decided: %+v", v)
	}
	// A twin that is half of each language matches both about as well.
	half := mix(scaled(s.eng, 0.7), scaled(s.spa, 0.7), s.crowd)
	if v := judgeScene(t, s.english(), s.spanish(), twinOf(half)); v.ok {
		t.Fatalf("an even match decided: %+v", v)
	}
	// A twin of other words altogether matches neither.
	if v := judgeScene(t, s.english(), s.spanish(), twinOf(mix(speech(7, 60), s.crowd))); v.ok {
		t.Fatalf("an unrelated twin decided: %+v", v)
	}
}

func TestSoundEnvelopeOfSilenceIsNotSound(t *testing.T) {
	if _, ok := soundEnvelope(make([]byte, 2*soundRate*10)); ok {
		t.Fatal("ten seconds of silence counted as sound")
	}
	if _, ok := soundEnvelope(nil); ok {
		t.Fatal("nothing counted as sound")
	}
	if v := judgeSound(make([]float64, 100), make([]float64, 6000), make([]float64, 6000)); v.ok {
		t.Fatalf("a one-second twin decided: %+v", v)
	}
}

// The background gates. The still sweep needs the device idle; a sound
// check's twin may tune while a viewer watches, and shares every other gate.
func TestTunerGates(t *testing.T) {
	open := tunerGate{free: 2}
	cases := []struct {
		name         string
		g            tunerGate
		sweep, sound bool
	}{
		{"idle device", open, true, true},
		{"three free", tunerGate{free: 3}, true, true},
		{"last free tuner", tunerGate{free: 1}, false, false},
		{"no free tuner", tunerGate{free: 0}, false, false},
		{"still sweep off", tunerGate{free: 2, off: true}, false, true},
		{"scan, hold, or guess", tunerGate{free: 2, held: true}, false, false},
		{"tuner opened by number", tunerGate{free: 2, opened: true}, false, true},
		{"recording on the device", tunerGate{free: 2, recording: true}, false, false},
		{"recording soon", tunerGate{free: 2, soon: true}, false, false},
		{"viewer or export", tunerGate{free: 2, used: true}, false, true},
	}
	for _, c := range cases {
		if got := c.g.sweepOpen(); got != c.sweep {
			t.Errorf("%s: sweep %v, want %v", c.name, got, c.sweep)
		}
		if got := c.g.backgroundOpen(); got != c.sound {
			t.Errorf("%s: sound check %v, want %v", c.name, got, c.sound)
		}
	}
}

// A gate the hub reads: a recording on the device shuts the sound check's
// twin tune before the device is asked anything.
func TestSoundCheckGateShutByARecording(t *testing.T) {
	h, m := testHub(t)
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h.Store = st
	m.host = "http://tuner"
	f := addTestFeed(h, m, 1, "104.1")
	h.mu.Lock()
	f.recordings = append(f.recordings, &recording{})
	h.mu.Unlock()
	asked := false
	open := func(g tunerGate) bool {
		asked = asked || g.free != sweepFreeTuners
		return g.backgroundOpen()
	}
	if h.tunerGateOpen(context.Background(), store.SourceChannel{BaseURL: "http://tuner"}, open) {
		t.Fatal("the gate opened while a recording was on the device")
	}
	if asked {
		t.Fatal("the device was asked with the gate already shut")
	}
}

func TestSoundDue(t *testing.T) {
	now := time.Date(2026, 11, 15, 15, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *time.Time { v := now.Add(d); return &v }
	cases := []struct {
		name    string
		last    *time.Time
		program time.Time
		want    bool
	}{
		{"never checked", nil, time.Time{}, true},
		{"checked this program", at(-10 * time.Minute), now.Add(-time.Hour), false},
		{"program began after the check", at(-time.Hour), now.Add(-10 * time.Minute), true},
		{"program has not settled", at(-time.Hour), now.Add(-time.Minute), false},
		{"no guide, recent", at(-time.Hour), time.Time{}, false},
		{"no guide, stale", at(-soundRecheck), time.Time{}, true},
	}
	for _, c := range cases {
		if got := soundDue(c.last, c.program, now); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

var (
	ac4Eng = AudioTrack{PID: 50, Language: "eng", Role: "main", Codec: "ac4", Label: "English", Channels: 6, Measured: true}
	ac4Spa = AudioTrack{PID: 51, Language: "spa", Role: "language", Codec: "ac4", Label: "Spanish", Channels: 2, Measured: true}
)

func TestSoundPairNeedsTwoLanguages(t *testing.T) {
	if _, _, ok := soundPair([]AudioTrack{ac4Eng}, false); ok {
		t.Fatal("one track is a pair")
	}
	described := AudioTrack{PID: 52, Language: "eng", Role: "described", Codec: "ac4"}
	if _, _, ok := soundPair([]AudioTrack{ac4Eng, described}, false); ok {
		t.Fatal("main and described video are a pair")
	}
	main, lang, ok := soundPair(swapSounds([]AudioTrack{ac4Eng, ac4Spa}), true)
	if !ok || main.PID != 50 || main.Language != "eng" || lang.PID != 51 {
		t.Fatalf("a swapped channel's pair is %+v %+v, want the labels as sent", main, lang)
	}
}

func TestSoundCheckSkipsOneLanguageChannels(t *testing.T) {
	h, m := testHub(t)
	f := addTestFeed(h, m, 1, "104.1")
	h.mu.Lock()
	f.tracks = []AudioTrack{ac4Eng}
	f.renditions["x"] = &rendition{viewers: 1}
	h.mu.Unlock()
	// No store: a channel that qualified would read it and panic.
	if _, ok := h.soundJobDue(context.Background(), time.Now()); ok {
		t.Fatal("a one-language channel is due a check")
	}
}

// soundStore is a store with a station on 4.1 and on 104.1, paired as twins.
func soundStore(t *testing.T) (*store.Store, store.Channel, store.Channel) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	dev := hdhr.Device{DeviceID: "dev-1", FriendlyName: "Test", BaseURL: "http://tuner", TunerCount: 4}
	err = st.UpsertDevice(ctx, dev, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "WDAF", VideoCodec: "MPEG2", AudioCodec: "AC3"},
		{GuideNumber: "104.1", GuideName: "WDAF", VideoCodec: "HEVC", AudioCodec: "AC4"},
	})
	if err != nil {
		t.Fatal(err)
	}
	chs, err := st.Channels(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	var one, three store.Channel
	for _, ch := range chs {
		switch ch.GuideNumber {
		case "4.1":
			one = ch
		case "104.1":
			three = ch
		}
	}
	if three.TwinID != one.ID || three.Standard != store.TwinATSC3 {
		t.Fatalf("not twins: %+v %+v", one, three)
	}
	return st, one, three
}

func TestSoundCheckIsDueForAWatchedTwoLanguageChannel(t *testing.T) {
	st, one, three := soundStore(t)
	h, m := testHub(t)
	h.Store = st
	f := addTestFeed(h, m, three.ID, "104.1")
	h.mu.Lock()
	f.tracks = []AudioTrack{ac4Eng, ac4Spa}
	h.mu.Unlock()
	if _, ok := h.soundJobDue(context.Background(), time.Now()); ok {
		t.Fatal("a channel nobody watches or records is due")
	}
	h.mu.Lock()
	f.renditions["x"] = &rendition{viewers: 1}
	h.mu.Unlock()
	job, ok := h.soundJobDue(context.Background(), time.Now())
	if !ok || job.id != three.ID || job.twin.ID != one.ID {
		t.Fatalf("job %+v %v, want %d against %d", job, ok, three.ID, one.ID)
	}
	h.mu.Lock()
	h.soundTries = map[int64]soundTry{three.ID: {until: time.Now().Add(time.Minute)}}
	h.mu.Unlock()
	if _, ok := h.soundJobDue(context.Background(), time.Now()); ok {
		t.Fatal("a channel waiting out a retry is due")
	}
}

// A verdict that changes the main restarts the encode once, on the other
// PID; the same verdict again restarts nothing; the flag flips both ways.
func TestASoundVerdictRestartsOnceAndFlipsBothWays(t *testing.T) {
	h, f, mark := holdFFmpeg(t)
	h.mu.Lock()
	f.tracks = []AudioTrack{ac4Eng, ac4Spa}
	h.mu.Unlock()
	startRendition(t, h, f, "1080.aac2.broadcast")
	if first := waitMark(t, mark, ".args.1"); !strings.Contains(first, "-map\n0:i:50\n") {
		t.Fatalf("first encode should play PID 50:\n%s", first)
	}
	now := time.Now()
	h.ApplySound(1, true, SoundCheckedByTwin, now)
	if args := waitMark(t, mark, ".args.2"); !strings.Contains(args, "-map\n0:i:51\n") || !strings.Contains(args, "language=eng") {
		t.Fatalf("a swap should play PID 51 as English:\n%s", args)
	}
	h.ApplySound(1, true, SoundCheckedByTwin, now.Add(time.Minute))
	if n := settledCount(t, mark); n != 2 {
		t.Fatalf("starts = %d after the same verdict twice, want 2", n)
	}
	h.ApplySound(1, false, SoundCheckedByTwin, now.Add(2*time.Minute))
	if args := waitMark(t, mark, ".args.3"); !strings.Contains(args, "-map\n0:i:50\n") {
		t.Fatalf("swapping back should play PID 50:\n%s", args)
	}
	if n := settledCount(t, mark); n != 3 {
		t.Fatalf("starts = %d, want 3", n)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if f.channel.SoundSwapped || f.channel.SoundCheckedAt == nil || f.channel.SoundCheckedBy != SoundCheckedByTwin {
		t.Fatalf("channel = %+v", f.channel.Channel)
	}
}

// 2026-11-15: a FOX station's NFL game played its Spanish commentary as the
// main sound. Two Sundays before, the station had sent Spanish on the PID it
// labelled English, and the channel was set to swap its sounds; that day its
// labels were right again, and the stored swap played Spanish. A check whose
// sound says the track labelled English is English clears the swap, stores
// it, and the tune plays English as its main sound.
func TestIncidentAStaleSwapPlaysTheEnglishMainOnceChecked(t *testing.T) {
	st, one, three := soundStore(t)
	ctx := context.Background()
	if _, err := st.PatchChannel(ctx, three.ID, store.ChannelPatch{SoundSwapped: new(true)}); err != nil {
		t.Fatal(err)
	}
	h, m := testHub(t)
	h.Store = st
	src, err := st.SourceChannel(ctx, three.ID)
	if err != nil || !src.SoundSwapped {
		t.Fatalf("source %+v %v", src, err)
	}
	h.mu.Lock()
	f := h.addFeedLocked(m, src)
	// The PMT as the station sends it today, read through the stored swap.
	f.tracks = swapSounds([]AudioTrack{ac4Eng, ac4Spa})
	h.mu.Unlock()
	if got := f.sourceFor(Rendition{Video: "copy", Audio: "aac2"}); got.AudioPID != 51 {
		t.Fatalf("before the check the main is PID %d; the incident played 51", got.AudioPID)
	}
	// The sound decides it: the track marked English matched 4.1's English.
	v := judgeSound(twinEnvelope(), noiseEnvelope(1), noiseEnvelope(2))
	if !v.ok || v.swapped {
		t.Fatalf("verdict %+v", v)
	}
	h.settleSound(ctx, three.ID, v.swapped, SoundCheckedByTwin, time.Now(), one)
	src, err = st.SourceChannel(ctx, three.ID)
	if err != nil || src.SoundSwapped || src.SoundCheckedBy != SoundCheckedByTwin || src.SoundCheckedAt == nil {
		t.Fatalf("stored %+v %v", src.Channel, err)
	}
	h.mu.Lock()
	got := f.sourceFor(Rendition{Video: "copy", Audio: "aac2"})
	h.mu.Unlock()
	if got.AudioPID != 50 {
		t.Fatalf("after the check the main is PID %d, want 50 (English)", got.AudioPID)
	}
	events, _ := st.Events(ctx, 5)
	if len(events) == 0 || events[0].Kind != "sound" || !strings.Contains(events[0].Message, "English") {
		t.Fatalf("activity %+v", events)
	}
	// The same answer again notes nothing more.
	h.settleSound(ctx, three.ID, false, SoundCheckedByTwin, time.Now(), one)
	if again, _ := st.Events(ctx, 5); len(again) != len(events) {
		t.Fatalf("activity grew on the same answer: %+v", again)
	}
}

// noiseEnvelope is an envelope of words seeded by seed; twinEnvelope is 15 s
// of seed 1's, as the other broadcast hears it.
func noiseEnvelope(seed int64) []float64 {
	r := rand.New(rand.NewSource(seed))
	out := make([]float64, 6000)
	for i := range out {
		out[i] = r.NormFloat64()
	}
	return out
}

func twinEnvelope() []float64 {
	return append([]float64(nil), noiseEnvelope(1)[2500:4000]...)
}

// A recording of a two-language channel keeps the track the channel plays
// as its main, and playback maps it when the file lists it.
func TestRecordingKeepsTheChannelsMainSound(t *testing.T) {
	st, _, three := soundStore(t)
	ctx := context.Background()
	id, err := st.CreateRecording(ctx, store.Recording{ChannelID: three.ID, Title: "Game", Status: "recording", StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	h, m := testHub(t)
	h.Store = st
	f := addTestFeed(h, m, three.ID, "104.1")
	rec := &recording{id: id}
	h.mu.Lock()
	f.tracks = []AudioTrack{ac4Eng, ac4Spa}
	f.recordings = append(f.recordings, rec)
	h.noteRecordingSoundLocked(f)
	h.mu.Unlock()
	waitSound := func(pid int) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if got, _ := st.Recording(ctx, id); got.SoundPID == pid {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		got, _ := st.Recording(ctx, id)
		t.Fatalf("recording sound %d %q, want %d", got.SoundPID, got.SoundLanguage, pid)
	}
	waitSound(50)
	h.ApplySound(three.ID, true, SoundCheckedByTwin, time.Now())
	waitSound(51)
	if got, _ := st.Recording(ctx, id); got.SoundLanguage != "eng" {
		t.Fatalf("language %q", got.SoundLanguage)
	}

	args := strings.Join(PictureArgs(Graph{Input: "x.ts", AudioPID: 51, Encoder: "libx264"}), " ")
	if !strings.Contains(args, "-map 0:i:51") || strings.Contains(args, "0:a:0") {
		t.Fatalf("playback args %s", args)
	}
	if old := strings.Join(PictureArgs(Graph{Input: "x.ts", Encoder: "libx264"}), " "); !strings.Contains(old, "-map 0:a:0?") {
		t.Fatalf("a recording with no stored sound changed: %s", old)
	}
	if graphStamp(Graph{AudioPID: 51}) == graphStamp(Graph{}) || graphStamp(Graph{}) != graphStamp(Graph{AudioPID: 0}) {
		t.Fatal("the copy stamp should name a stored sound and only then")
	}
}

// Playback maps the stored track only when the file's PMT lists it.
func TestRecordingPlaysTheStoredTrackOnlyWhenTheFileHasIt(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "rec.ts")
	s := newScene()
	writeTS(t, ffmpeg, path, []int{50, 51}, []string{"eng", "spa"}, s.english()[:5*synthRate], s.spanish()[:5*synthRate])
	h, _ := testHub(t)
	h.FFmpeg = ffmpeg
	if g := h.fileGraphFor(path, "mpeg2video", "broadcast", "progressive", 51); g.AudioPID != 51 {
		t.Fatalf("stored 51: graph plays %d", g.AudioPID)
	}
	if g := h.fileGraphFor(path, "mpeg2video", "broadcast", "progressive", 77); g.AudioPID != 0 {
		t.Fatalf("a PID the file lacks is mapped: %d", g.AudioPID)
	}
}
