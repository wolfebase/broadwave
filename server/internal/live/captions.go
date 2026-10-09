package live

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"broadwave/internal/captions"
)

// captionKeep covers the live window, so a viewer who rewinds still has
// captions. Cues are small; an hour of roll-up is under 20,000.
const captionKeep = 2 * 3600 * 90000

// captionTrack decodes one channel's CEA-608 captions from the tuner's
// multiplex. Cue times are broadcast timestamps. Every rendition keeps those
// timestamps (-copyts), offset by where its fMP4 starts.
type captionTrack struct {
	mu     sync.Mutex
	reader *captions.Reader
	dec    *captions.Decoder
	cues   []keptCue
	last   int64
	seen   bool
	// line counts the broadcast's timelines. A clock that goes back and stays
	// back is a new one, and its timestamps can repeat the old one's.
	line int
	// held is a run of pictures off the timeline, kept until pesAgree of them
	// agree on a new one (the program filter's rule) or the clock comes back.
	// Past breakLimit a minute the filter follows a break in the same encode,
	// whose segments stay on the old timeline and get no cues until the next.
	held []heldPicture
	// age is the media time read so far, in 90 kHz ticks, across timelines.
	age int64
	// encoders are the packagers reading this channel. Each one learns an
	// encode's caption line when that line is committed, which is later than
	// the filter's break: the reader holds pictures back for display order.
	encoders []*captionEncode
}

// captionLineKeep bounds lines waiting for an encode to take them.
const captionLineKeep = 64

// captionEncode is one packager's caption lines. base is the line when the
// packager was bound, and it belongs to the first encode. Later encodes take
// lines in the order the filter accepted their breaks.
type captionEncode struct {
	track     *captionTrack
	mu        sync.Mutex
	base      int
	took      bool
	strict    bool
	waiting   int
	unmatched int
	lines     []int
}

type keptCue struct {
	captions.Cue
	line int
	at   int64
}

type heldPicture struct {
	pts   int64
	pairs []byte
}

func newCaptionTrack(program int) *captionTrack {
	c := &captionTrack{dec: captions.NewDecoder()}
	c.reader = captions.NewReader(program, c.picture)
	return c
}

// Write never fails: the decoder reads broadcast bytes, and a fault in it
// must not end the relay. It drops the captions instead.
func (c *captionTrack) Write(p []byte) (n int, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	defer func() {
		if v := recover(); v != nil {
			slog.Warn(fmt.Sprintf("captions stopped: %v", v))
			c.reader = captions.NewReader(0, func(int64, []byte) {})
			n, err = len(p), nil
		}
	}()
	return c.reader.Write(p)
}

func (c *captionTrack) Close() error { return nil }

// picture runs under c.mu, from Write.
func (c *captionTrack) picture(pts int64, pairs []byte) {
	if !c.seen {
		c.last, c.seen = pts, true
		c.feed(pts, pairs)
		return
	}
	if d := ptsDelta(pts, c.last); d <= pesJump && d >= -pesBack {
		c.held = nil
		if d > 0 {
			c.age += d
		}
		c.last = pts
		c.feed(pts, pairs)
		return
	}
	if len(c.held) > 0 && ptsGap(pts, c.held[len(c.held)-1].pts) <= pesJump {
		c.held = append(c.held, heldPicture{pts, append([]byte(nil), pairs...)})
	} else {
		c.held = []heldPicture{{pts, append([]byte(nil), pairs...)}}
	}
	if len(c.held) < pesAgree {
		return
	}
	// A new timeline. The cue on screen ends where the old clock stopped.
	c.dec.Close(c.last)
	c.keep()
	if ptsDelta(c.held[0].pts, c.last) < 0 {
		c.line++
		c.noteLine(c.line)
	}
	c.dec = captions.NewDecoder()
	held := c.held
	c.held = nil
	c.last = held[0].pts
	for _, h := range held {
		if d := ptsDelta(h.pts, c.last); d > 0 {
			c.age += d
		}
		c.last = h.pts
		c.feed(h.pts, h.pairs)
	}
}

func (c *captionTrack) feed(pts int64, pairs []byte) {
	c.dec.Feed(pts, pairs)
	c.keep()
	drop := 0
	for drop < len(c.cues) && c.age-c.cues[drop].at > captionKeep {
		drop++
	}
	if drop > 0 {
		c.cues = append(c.cues[:0], c.cues[drop:]...)
	}
}

func (c *captionTrack) keep() {
	for _, cue := range c.dec.Take() {
		c.cues = append(c.cues, keptCue{Cue: cue, line: c.line, at: c.age})
	}
}

// noteLine runs under c.mu, from picture, after a backward break commits.
func (c *captionTrack) noteLine(line int) {
	for _, e := range c.encoders {
		e.commit(line)
	}
}

// bindEncoder registers a packager. next is each encode's line, in order.
// expect is called when the filter accepts a backward break, before the
// reader has emitted the new pictures. drop unregisters the packager.
func (c *captionTrack) bindEncoder() (next func() int, expect func(), drop func()) {
	c.mu.Lock()
	e := &captionEncode{track: c, base: c.line}
	c.encoders = append(c.encoders, e)
	c.mu.Unlock()
	return e.next, e.expect, func() { c.dropEncoder(e) }
}

func (c *captionTrack) dropEncoder(e *captionEncode) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, have := range c.encoders {
		if have == e {
			c.encoders = append(c.encoders[:i], c.encoders[i+1:]...)
			return
		}
	}
}

func (e *captionEncode) expect() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.strict = true
	if e.unmatched > 0 {
		e.unmatched--
		return
	}
	e.waiting++
}

func (e *captionEncode) commit(line int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	// A break the filter follows in the same encode has no encode waiting
	// for it. Once a real break has been reserved, those lines are not queued.
	if e.strict && e.waiting == 0 {
		return
	}
	if e.waiting > 0 {
		e.waiting--
	} else {
		e.unmatched++
	}
	if len(e.lines) >= captionLineKeep {
		return
	}
	e.lines = append(e.lines, line)
}

func (e *captionEncode) next() int {
	e.mu.Lock()
	if !e.took {
		e.took = true
		n := e.base
		e.mu.Unlock()
		return n
	}
	if len(e.lines) > 0 {
		n := e.lines[0]
		e.lines = e.lines[1:]
		e.mu.Unlock()
		return n
	}
	e.mu.Unlock()
	return e.track.timeline()
}

// timeline is the broadcast timeline the captions are on now.
func (c *captionTrack) timeline() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.line
}

// span returns the cues of one timeline on screen between from and
// from+dur, moved by -offset onto an encode's own clock. The cue still on
// screen runs to the end of the span.
func (c *captionTrack) span(line int, from, dur, offset int64) []captions.Cue {
	c.mu.Lock()
	defer c.mu.Unlock()
	all := c.cues
	if cue, ok := c.dec.Showing(); ok && line == c.line && ptsDiff(from+dur, cue.Start) > 0 {
		cue.End = wrapPTS(from + dur)
		all = append(all[:len(all):len(all)], keptCue{Cue: cue, line: line})
	}
	var out []captions.Cue
	for _, kept := range all {
		cue := kept.Cue
		if kept.line != line || ptsDiff(cue.End, from) <= 0 || ptsDiff(cue.Start, from+dur) >= 0 {
			continue
		}
		cue.Start = wrapPTS(cue.Start - offset)
		cue.End = wrapPTS(cue.End - offset)
		out = append(out, cue)
	}
	return out
}

func wrapPTS(v int64) int64 {
	return (v%ptsWrap + ptsWrap) % ptsWrap
}

// captionPlaylist is the WebVTT playlist for a stamped video playlist: the
// same sequence numbers, dates, durations, and breaks, one .vtt per segment.
// Parts and blocking reload stay with the video; players reload subtitles on
// their own schedule.
func captionPlaylist(video []byte) []byte {
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:6\n")
	var pending []string
	for line := range strings.SplitSeq(string(video), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
		case strings.HasPrefix(line, "#EXT-X-TARGETDURATION:"),
			strings.HasPrefix(line, "#EXT-X-MEDIA-SEQUENCE:"),
			strings.HasPrefix(line, "#EXT-X-DISCONTINUITY-SEQUENCE:"),
			strings.HasPrefix(line, "#EXT-X-PLAYLIST-TYPE:"):
			b.WriteString(line + "\n")
		case strings.HasPrefix(line, "#EXT-X-SERVER-CONTROL:") && strings.Contains(line, "CAN-BLOCK-RELOAD=YES"):
			// AVPlayer asks this playlist for the video's next segment too;
			// the server holds it until the video lists that segment.
			b.WriteString("#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES\n")
		case line == "#EXT-X-ENDLIST":
			pending = nil
			b.WriteString(line + "\n")
		case line == "#EXT-X-DISCONTINUITY",
			strings.HasPrefix(line, "#EXT-X-PROGRAM-DATE-TIME:"),
			strings.HasPrefix(line, "#EXTINF:"):
			// Held until the segment they belong to. A break after the last
			// segment belongs to the open parts, which this playlist leaves out.
			pending = append(pending, line)
		case strings.HasPrefix(line, "#"):
		default:
			name := filepath.Base(line)
			if !strings.HasSuffix(name, ".m4s") {
				pending = nil
				continue
			}
			for _, p := range pending {
				b.WriteString(p + "\n")
			}
			pending = nil
			b.WriteString(strings.TrimSuffix(name, ".m4s") + ".vtt\n")
		}
	}
	return []byte(b.String())
}

// mainPlaylist wraps one rendition's video playlist with its captions, so
// players offer the captions track. CLOSED-CAPTIONS=NONE keeps a player from
// also offering the 608 bytes some encodes carry in the picture.
func mainPlaylist(r Rendition) []byte {
	return fmt.Appendf(nil, "#EXTM3U\n#EXT-X-VERSION:6\n#EXT-X-INDEPENDENT-SEGMENTS\n"+captionMedia+
		"#EXT-X-STREAM-INF:%s,SUBTITLES=\"cc\",CLOSED-CAPTIONS=NONE\nindex.m3u8\n", streamRates(r, 0))
}

// captionMedia is the SUBTITLES group of every multivariant playlist.
const captionMedia = "#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID=\"cc\",NAME=\"English CC\",LANGUAGE=\"en\",DEFAULT=NO,AUTOSELECT=YES,FORCED=NO," +
	"CHARACTERISTICS=\"public.accessibility.transcribes-spoken-dialog,public.accessibility.describes-music-and-sound\",URI=\"captions.m3u8\"\n"

// startCaptionsLocked follows the feed's captions while it has a rendition.
func (h *Hub) startCaptionsLocked(f *feed) {
	if f.captions != nil {
		return
	}
	m := muxOf(h, f)
	if m == nil {
		return
	}
	f.captions = newCaptionTrack(f.program)
	f.captionSub = h.attachPipe(m, f.captions, true)
}

// bindCaption reserves caption lines for one packager. next is each encode's
// line. expect is told from the filter's break. drop unregisters the packager
// when it stops. The caller holds h.mu; the packager calls next later without.
func bindCaption(f *feed) (next func() int, expect func(), drop func()) {
	if f == nil || f.captions == nil {
		return nil, nil, nil
	}
	return f.captions.bindEncoder()
}

// captionLine is a packager's line function when the caller does not keep the
// registration. The hub uses bindCaption so the packager can drop it.
func captionLine(f *feed) func() int {
	next, _, _ := bindCaption(f)
	return next
}

// stopCaptionsLocked runs when the last rendition stops. A recording alone
// has no use for them.
func (h *Hub) stopCaptionsLocked(f *feed) {
	if f.captionSub != nil {
		muxOf(h, f).detach(f.captionSub)
	}
	f.captions, f.captionSub = nil, nil
}

// MainPlaylist is the multivariant playlist for a running rendition.
func (h *Hub) MainPlaylist(channelID int64, key string) ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	f := h.channels[channelID]
	if f == nil || f.renditions[key] == nil || f.captions == nil {
		return nil, os.ErrNotExist
	}
	return mainPlaylist(f.renditions[key].spec), nil
}

// CaptionPlaylist is the WebVTT playlist that follows a rendition's video.
func (h *Hub) CaptionPlaylist(channelID int64, key string) ([]byte, error) {
	video, err := h.stamped(channelID, key)
	if err != nil {
		return nil, err
	}
	return captionPlaylist(video), nil
}

// CaptionSegment is the WebVTT for one video segment, named segNNNNN.vtt.
// Its timestamp map is the segment's own start, so players put each cue on
// the picture it was sent with.
func (h *Hub) CaptionSegment(channelID int64, key, name string) ([]byte, error) {
	video := strings.TrimSuffix(name, ".vtt") + ".m4s"
	seq, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSuffix(name, ".vtt"), "seg"))
	if err != nil {
		return nil, os.ErrNotExist
	}
	h.mu.Lock()
	f := h.channels[channelID]
	var dir string
	var in *packInput
	var track *captionTrack
	if f != nil && f.renditions[key] != nil {
		dir, in, track = f.renditions[key].dir, f.renditions[key].input, f.captions
	}
	h.mu.Unlock()
	if dir == "" {
		return nil, os.ErrNotExist
	}
	start, dur, ok := segmentSpan(dir, video)
	if !ok {
		return nil, os.ErrNotExist
	}
	var cues []captions.Cue
	// After a timestamp break the next encode shares the playlist with its
	// own offset, from the first segment it wrote.
	if enc, ok := in.encodeAt(seq); ok && track != nil {
		cues = track.span(enc.line, wrapPTS(start+enc.offset), dur, enc.offset)
	}
	return captions.Segment(start, dur, cues), nil
}
