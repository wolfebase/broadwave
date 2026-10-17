package live

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// A sound lead starts an ATSC 3.0 picture before its sound can play.
//
// An AC-4 decoder needs one of the sound's I-frames, which a station sends
// every two seconds, and a cold 3.0 tune sends its first sound about two
// seconds after its first picture. ffmpeg writes nothing until every track it
// encodes has a frame, so an encode that converts AC-4 waits 2 to 4 s past
// its input, and the picture with it. The lead is a second ffmpeg on the same
// input that copies the picture and carries silence, with the same tracks and
// the same sample entries, from the first picture on. The packager plays it
// until the real encode has written, then cuts the lead's input before the
// next keyframe and continues on the real encode from that keyframe, so the
// playlist has one header, one timeline, and no break.

// leadProbe is the lead's probe. Its picture opens on a keyframe with its
// parameter sets; the AC-4 tracks it does not map would hold a longer probe
// to its ceiling.
const leadProbe = "300000"

// leadMax is how long a lead may play without the real encode taking over.
// Past it the lead is cut and the playlist waits for the real encode.
const leadMax = 20 * time.Second

// leadQueue bounds the input waiting for the lead to start or to read.
const leadQueue = 8 << 20

// leadSound is one silent track: its layout, as the AC-4 decoder reports the
// width, and the language the real track's tag gives it.
type leadSound struct {
	layout string
	lang   string
}

// soundLayout is ffmpeg's layout for an AC-4 track of that many channels.
func soundLayout(channels int) (string, bool) {
	switch channels {
	case 1:
		return "mono", true
	case 2:
		return "stereo", true
	case 6:
		return "5.1(side)", true
	}
	return "", false
}

// leadArgs turns a rendition's command line into its lead's: each sound map
// takes silence that starts at start (seconds on the broadcast clock, the
// first picture's), the probe is short, and the encode ends with its picture.
func leadArgs(args []string, start float64, sounds []leadSound) ([]string, bool) {
	at := strconv.FormatFloat(start, 'f', 6, 64)
	var out []string
	input, audio := false, 0
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-analyzeduration" && i+1 < len(args):
			out = append(out, a, leadProbe)
			i++
		case a == "-i" && i+1 < len(args) && args[i+1] == "pipe:0" && !input:
			input = true
			out = append(out, a, args[i+1])
			for _, s := range sounds {
				out = append(out, "-itsoffset", at, "-f", "lavfi", "-i", "anullsrc=r=48000:cl="+s.layout)
			}
			i++
		case a == "-map" && i+1 < len(args) && !strings.Contains(args[i+1], ":v:"):
			if audio >= len(sounds) {
				return nil, false
			}
			out = append(out, a, fmt.Sprintf("%d:a:0", audio+1))
			audio++
			i++
		case a == "-f" && i+1 < len(args) && args[i+1] == "mp4":
			for k, s := range sounds {
				if s.lang != "" {
					out = append(out, fmt.Sprintf("-metadata:s:a:%d", k), "language="+s.lang)
				}
			}
			out = append(out, "-shortest", a, args[i+1])
			i++
		default:
			out = append(out, a)
		}
	}
	if !input || audio != len(sounds) || !slices.Contains(out, "-shortest") {
		return nil, false
	}
	return out, true
}

// leadTee feeds the real encode and the lead the same program. Its writes
// come from the rendition's subscriber, in whole packets from the program
// pipe; the lead's share goes through a queue, so a lead that is slow to
// start never holds up the real encode.
type leadTee struct {
	w    io.WriteCloser
	pipe *programPipe
	// onPicture is told the first picture's presentation time once, from
	// the writing goroutine. It starts the lead or gives it up.
	onPicture func(pts int64)

	mu     sync.Mutex
	cond   *sync.Cond
	queue  [][]byte
	queued int
	lead   io.WriteCloser
	closed bool // nothing more goes to the lead
	gone   atomic.Bool
	// started is set once the first picture was seen; only the writer reads it.
	started bool

	// out is the parent's copy of the lead's stdout. Closing it lets the
	// packager move on once no lead holds it: when the tee gives the lead
	// up, and after the lead starts.
	out     *os.File
	outOnce sync.Once

	cut atomic.Bool // cut the lead before the next keyframe
	// pts and at are the first picture's presentation time and its arrival,
	// for dating: at is zero until it is seen.
	pts atomic.Int64
	at  atomic.Int64
}

func newLeadTee(w io.WriteCloser) *leadTee {
	t := &leadTee{w: w}
	t.cond = sync.NewCond(&t.mu)
	return t
}

// arrival is the first picture's presentation time and when it reached the
// encode.
func (t *leadTee) arrival() (int64, time.Time, bool) {
	if t == nil {
		return 0, time.Time{}, false
	}
	at := t.at.Load()
	if at == 0 {
		return 0, time.Time{}, false
	}
	return t.pts.Load(), time.Unix(0, at), true
}

func (t *leadTee) Write(b []byte) (int, error) {
	n, err := t.w.Write(b)
	t.feed(b)
	return n, err
}

func (t *leadTee) Close() error {
	t.drop()
	return t.w.Close()
}

// feed notes the first picture's arrival and passes b to the lead, up to
// the cut.
func (t *leadTee) feed(b []byte) {
	if t.started && t.gone.Load() {
		return
	}
	video := 0
	if t.pipe != nil {
		video = t.pipe.video
	}
	if video == 0 || len(b)%188 != 0 {
		// Not the program pipe's packets: nothing to date or cut on.
		t.started = true
		t.drop()
		return
	}
	end := len(b)
	for off := 0; off+188 <= len(b) && (!t.started || t.cut.Load()); off += 188 {
		pkt := b[off : off+188]
		if pkt[0] != 0x47 || int(pkt[1]&0x1f)<<8|int(pkt[2]) != video || pkt[1]&0x40 == 0 {
			continue
		}
		pay := tsPayload(pkt)
		if !t.started {
			pts, ok := pesPTS(pay)
			if !ok {
				continue
			}
			t.started = true
			t.pts.Store(pts)
			t.at.Store(time.Now().UnixNano())
			if t.onPicture != nil {
				t.onPicture(pts)
			}
			continue
		}
		if sequenceStart(pay, t.pipe.kind) {
			end = off
			break
		}
	}
	if !t.started || t.gone.Load() {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return
	}
	if end > 0 {
		if t.queued+end > leadQueue {
			t.closeLocked()
			return
		}
		t.queue = append(t.queue, append([]byte(nil), b[:end]...))
		t.queued += end
	}
	if end < len(b) {
		t.closeLocked()
	}
	t.cond.Broadcast()
}

// pesPTS is the presentation time of a PES that starts in this payload.
func pesPTS(b []byte) (int64, bool) {
	if len(b) < 14 || b[0] != 0 || b[1] != 0 || b[2] != 1 || b[7]&0x80 == 0 {
		return 0, false
	}
	return ptsField(b[9:]), true
}

// attach starts writing the queued input to the lead's stdin.
func (t *leadTee) attach(stdin io.WriteCloser) {
	t.mu.Lock()
	if t.lead != nil || (t.closed && len(t.queue) == 0) {
		t.mu.Unlock()
		_ = stdin.Close()
		return
	}
	t.lead = stdin
	t.mu.Unlock()
	go func() {
		for {
			t.mu.Lock()
			for len(t.queue) == 0 && !t.closed {
				t.cond.Wait()
			}
			if len(t.queue) == 0 {
				t.mu.Unlock()
				_ = stdin.Close()
				return
			}
			chunk := t.queue[0]
			t.queue = t.queue[1:]
			t.queued -= len(chunk)
			t.mu.Unlock()
			if _, err := stdin.Write(chunk); err != nil {
				t.drop()
				_ = stdin.Close()
				return
			}
		}
	}()
}

// drop ends the lead's input now, without what is still queued.
func (t *leadTee) drop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.queue, t.queued = nil, 0
	t.closeLocked()
}

func (t *leadTee) closeLocked() {
	t.closed = true
	t.gone.Store(true)
	t.cond.Broadcast()
	t.closeOut()
}

func (t *leadTee) closeOut() {
	if t.out != nil {
		t.outOnce.Do(func() { _ = t.out.Close() })
	}
}

// leadEncode is the lead's process and the pipe the packager reads first.
// Its fields are the hub's, under h.mu, except tee.
type leadEncode struct {
	tee    *leadTee
	sounds []leadSound
	// out is the read end the packager starts on; w is its write end, the
	// lead's stdout, closed once the lead starts or never will.
	out *os.File
	w   *os.File
	cmd *exec.Cmd
	// begun is set when the lead process started; ended once it ended or
	// was given up.
	begun bool
	ended bool
	timer *time.Timer
}

// datingOnly is a tee that notes the first picture's arrival and runs no
// lead: a converted picture of an AC-4 sound waits on that sound too.
func datingOnly(w io.WriteCloser) *leadEncode {
	t := newLeadTee(w)
	t.closed = true
	t.gone.Store(true)
	return &leadEncode{tee: t, begun: true, ended: true}
}

// arrival is the first picture's presentation time and its arrival, false
// without a lead.
func (le *leadEncode) arrival() (int64, time.Time, bool) {
	if le == nil {
		return 0, time.Time{}, false
	}
	return le.tee.arrival()
}

func newLeadEncode(w io.WriteCloser, sounds []leadSound) (*leadEncode, error) {
	out, pw, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	t := newLeadTee(w)
	t.out = pw
	return &leadEncode{tee: t, sounds: sounds, out: out, w: pw}, nil
}

// leadSoundsLocked is the silence a lead for spec carries, one track for
// each sound the rendition encodes. False when the rendition gets no lead:
// only a copied picture of a converted AC-4 sound waits on that sound, and
// a picture fed from the buffer has its sound at once.
func (h *Hub) leadSoundsLocked(f *feed, spec Rendition, extras []AudioTrack, input string) ([]leadSound, bool) {
	if h.NoLead || input != "pipe:0" || spec.ABR || spec.Video != "copy" {
		return nil, false
	}
	if spec.Audio != "ac3" && spec.Audio != "aac2" && spec.Audio != "aac6" {
		return nil, false
	}
	src := f.sourceFor(spec)
	if codecName(src.AudioCodec) != "ac4" {
		return nil, false
	}
	if m := muxOf(h, f); m == nil || m.input != "" {
		return nil, false
	}
	tracks := f.tracks
	if len(tracks) == 0 {
		tracks = f.stored
	}
	var lang string
	if i := slices.IndexFunc(tracks, func(t AudioTrack) bool { return t.PID == src.AudioPID }); src.AudioPID > 0 && i >= 0 {
		lang = tracks[i].Language
	} else if src.AudioPID == 0 && len(tracks) > 0 {
		lang = tracks[0].Language
	}
	// A conversion to stereo sounds the same from any width.
	layout, ok := "stereo", true
	if spec.Audio != "aac2" || src.AudioChannels > 0 {
		layout, ok = soundLayout(src.AudioChannels)
	}
	if !ok {
		return nil, false
	}
	sounds := []leadSound{{layout: layout, lang: lang}}
	for _, t := range extras {
		l, ok := soundLayout(t.Channels)
		if !ok || codecName(t.Codec) != "ac4" {
			return nil, false
		}
		sounds = append(sounds, leadSound{layout: l, lang: t.Language})
	}
	return sounds, true
}

// startLead starts r's lead on its first picture, at pts.
func (h *Hub) startLead(f *feed, r *rendition, le *leadEncode, pts int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if r.lead != le || le.ended || f.renditions[r.spec.Key()] != r || !r.ringFrom.IsZero() {
		h.endLeadLocked(r, le)
		return
	}
	what := renditionTag(f, r.spec.Key()) + " lead"
	args, ok := leadArgs(r.args, float64(pts)/90000, le.sounds)
	if !ok {
		slog.Warn(fmt.Sprintf("%s: no command line for its silence", what))
		h.endLeadLocked(r, le)
		return
	}
	cmd := exec.Command(h.FFmpeg, args...)
	cmd.Dir = r.dir
	stdin, err := cmd.StdinPipe()
	if err != nil {
		h.endLeadLocked(r, le)
		return
	}
	cmd.Stdout = le.w
	logCommand(cmd, what)
	if err := cmd.Start(); err != nil {
		slog.Warn(fmt.Sprintf("%s: %v", what, err))
		_ = stdin.Close()
		h.endLeadLocked(r, le)
		return
	}
	le.tee.closeOut()
	le.cmd, le.begun = cmd, true
	NotePID(h.Dir, cmd.Process.Pid)
	le.tee.attach(stdin)
	le.timer = time.AfterFunc(leadMax, func() {
		if le.tee.cut.CompareAndSwap(false, true) {
			slog.Warn(fmt.Sprintf("%s: no sound after %s; the picture waits for it", what, leadMax))
		}
	})
	go func() {
		_ = cmd.Wait()
		ForgetPID(h.Dir, cmd.Process.Pid)
		h.mu.Lock()
		le.ended = true
		h.mu.Unlock()
	}()
}

// endLeadLocked gives up le, r's lead, now: its input ends, its process is
// killed, and the packager moves on to the real encode.
func (h *Hub) endLeadLocked(r *rendition, le *leadEncode) {
	if le == nil {
		return
	}
	le.tee.drop()
	if le.timer != nil {
		le.timer.Stop()
	}
	le.begun = true
	if le.cmd != nil && !le.ended && le.cmd.Process != nil {
		_ = le.cmd.Process.Kill()
	}
	le.ended = true
}
