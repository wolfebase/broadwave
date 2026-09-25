package live

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"broadwave/internal/disk"
	"broadwave/internal/hdhr"
	"broadwave/internal/psip"
	"broadwave/internal/store"
)

type Tuner struct {
	Index    int    `json:"index"`
	Guide    string `json:"guide,omitempty"`
	Name     string `json:"name,omitempty"`
	Target   string `json:"target,omitempty"`
	Ours     bool   `json:"ours"`
	Strength int    `json:"strength,omitempty"`
	Quality  int    `json:"quality,omitempty"`
	Symbol   int    `json:"symbol,omitempty"`
	Shared   int    `json:"viewers,omitempty"`
}

// StreamInfo explains what a viewer is getting and why.
type StreamInfo struct {
	Rendition   string `json:"rendition"`
	Video       string `json:"video"`
	Audio       string `json:"audio"`
	Mode        string `json:"mode,omitempty"`
	Reason      string `json:"reason"`
	SourceVideo string `json:"sourceVideo,omitempty"`
	SourceAudio string `json:"sourceAudio,omitempty"`
	Encoder     string `json:"encoder,omitempty"`
}

type Session struct {
	ChannelID int64      `json:"channelId"`
	Playlist  string     `json:"playlist"`
	Rendition string     `json:"rendition"`
	Stream    StreamInfo `json:"stream"`
	Encoder   string     `json:"encoder"`
	Shared    bool       `json:"shared"`
	Viewers   int        `json:"viewers"`
	Frequency int        `json:"frequencyHz"`
	Program   int        `json:"program"`
	Tuners    []Tuner    `json:"tuners,omitempty"`

	// Fields the current web player reads.
	Profile   string   `json:"profile"`
	Audio     string   `json:"audio"`
	Picture   string   `json:"picture,omitempty"`
	VideoMode string   `json:"videoMode"`
	Hints     []string `json:"hints"`

	File string `json:"-"`
}

type BusyError struct {
	Tuners []Tuner
}

func (e *BusyError) Error() string {
	return "every tuner is busy"
}

// Hub owns the tuners. It tunes a whole frequency once, and every subchannel,
// rendition, and recording on that frequency reads from the same stream.
type Hub struct {
	Store          *store.Store
	Dir            string
	FFmpeg         string
	Encoder        string
	HEVC           bool
	DeintBroadcast string
	DeintSmooth    string
	OnSaved        func(store.Recording)

	// OnChange is called, outside the hub lock, when viewers, renditions,
	// recordings, or tuners change.
	OnChange func()

	// OnPSIP is called, outside the read loop, when a tuned mux yields a guide.
	OnPSIP func(freqHz int, guide psip.Guide)

	// RenditionIdle is how long a rendition with no viewers keeps running,
	// so flipping back to a channel is instant.
	RenditionIdle time.Duration

	mu         sync.Mutex
	muxes      map[int]*mux
	channels   map[int64]*feed
	reserved   map[int]bool
	hold       int
	next       int
	scanCancel context.CancelFunc
	scanToken  *struct{}
	playMu     sync.Mutex
	plays      map[int64]struct{}
}

type mux struct {
	freq     int
	tuner    int
	host     string
	device   string
	body     io.ReadCloser
	input    string
	cancel   context.CancelFunc
	feeds    map[string]*feed
	pipes    []*pipeSub
	programs []hdhr.Program
	pipeMu   sync.Mutex
	frames   sync.Once
	psip     psip.Harvester
}

// feed is one channel on a tuned frequency.
type feed struct {
	channel    store.SourceChannel
	program    int
	source     Source
	renditions map[string]*rendition
	recording  *recording
	timeline   *Timeline
	tracks     []AudioTrack
	probing    bool
	// exports counts raw MPEG-TS readers such as Plex or Jellyfin using the emulated tuner.
	exports int
}

// rendition is one ffmpeg process producing HLS for one delivery form.
type rendition struct {
	spec     Rendition
	dir      string
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	sub      *pipeSub
	viewers  int
	seen     time.Time
	idle     *time.Timer
	stamper  playlistStamper
	fallback bool
}

type recording struct {
	id    int64
	cmd   *exec.Cmd
	stdin io.WriteCloser
	sub   *pipeSub
	timer *time.Timer
}

type pipeSub struct {
	w    io.WriteCloser
	ch   chan []byte
	done chan struct{}
	once sync.Once
}

func (s *pipeSub) stop() {
	s.once.Do(func() { close(s.done) })
}

func New(st *store.Store, dir, ffmpeg, encoder string) *Hub {
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	if encoder == "" {
		encoder = DetectEncoder(ffmpeg)
	}
	broadcast, smooth := ProbeDeint(ffmpeg, encoder)
	return &Hub{
		Store: st, Dir: dir, FFmpeg: ffmpeg, Encoder: encoder, HEVC: ProbeHEVC(ffmpeg, encoder),
		DeintBroadcast: broadcast, DeintSmooth: smooth,
		RenditionIdle: 20 * time.Second,
		muxes:         map[int]*mux{}, channels: map[int64]*feed{}, reserved: map[int]bool{},
	}
}

func (h *Hub) deintFor(mode, codec string) string {
	if NormalizeMode(mode) == "film" {
		return ""
	}
	if !InterlacedCodec(codec) && codecName(codec) != "h264" {
		return ""
	}
	if NormalizeMode(mode) == "smooth" && h.DeintSmooth != "" {
		return h.DeintSmooth
	}
	return h.DeintBroadcast
}

// SourceOf describes a channel for the stream decision.
func (h *Hub) SourceOf(ctx context.Context, channelID int64) (Source, error) {
	ch, err := h.Store.SourceChannel(ctx, channelID)
	if err != nil {
		return Source{}, err
	}
	return sourceOf(ch), nil
}

func sourceOf(ch store.SourceChannel) Source {
	return Source{VideoCodec: ch.VideoCodec, AudioCodec: ch.AudioCodec, Progressive: ch.FieldOrder == "progressive", Film: ch.FieldOrder == "film", UserAgent: ch.UserAgent, Referrer: ch.Referrer}
}

// Watch starts or joins one rendition of a channel.
func (h *Hub) Watch(ctx context.Context, channelID int64, want Rendition) (Session, error) {
	h.preemptScan()
	want = want.normalized()
	ch, err := h.Store.SourceChannel(ctx, channelID)
	if err != nil {
		return Session{}, err
	}
	candidates := []store.SourceChannel{ch}
	if ids, err := h.Store.AlternateChannels(ctx, ch.GuideNumber, ch.ID); err == nil {
		for _, id := range ids {
			if alt, err := h.Store.SourceChannel(ctx, id); err == nil {
				candidates = append(candidates, alt)
			}
		}
	}
	var res *http.Response
	var last error
	chosen := -1
	for i, cand := range candidates {
		h.mu.Lock()
		_, tuned := h.channels[cand.ID]
		inUse := h.streamsForDeviceLocked(cand.DeviceID)
		h.mu.Unlock()
		if streamBusy(cand, inUse) && !tuned {
			last = fmt.Errorf("%s", StreamLimitMessage(cand.StreamLimit))
			continue
		}
		if cand.TunerCount == 0 && cand.StreamURL != "" && !hlsStream(cand) && !tuned {
			opened, err := openStream(cand.StreamURL, cand.UserAgent, cand.Referrer)
			if err != nil {
				last = err
				continue
			}
			res = opened
		}
		ch = cand
		chosen = i
		last = nil
		break
	}
	if chosen < 0 {
		if last == nil {
			last = fmt.Errorf("no source has this channel")
		}
		return Session{}, last
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	f, err := h.ensureFeedLocked(ctx, ch, res)
	if err != nil {
		return Session{}, err
	}
	r, err := h.ensureRenditionLocked(f, want)
	if err != nil {
		h.dropIfUnusedLocked(f)
		return Session{}, err
	}
	r.viewers++
	r.seen = time.Now()
	stopTimer(&r.idle)
	h.changed()
	return h.sessionLocked(f, r), nil
}

// ensureFeedLocked returns the tuned feed for a channel, tuning if needed. It does
// not start ffmpeg; renditions and recordings attach to the feed on demand.
func (h *Hub) ensureFeedLocked(ctx context.Context, ch store.SourceChannel, stream *http.Response) (*feed, error) {
	if f := h.channels[ch.ID]; f != nil {
		if stream != nil {
			stream.Body.Close()
		}
		return f, nil
	}
	if stream != nil {
		return h.addFeedLocked(h.streamMuxLocked(ch, stream.Body, "stream"), ch), nil
	}
	if hlsStream(ch) {
		return h.addFeedLocked(h.hlsMuxLocked(ch), ch), nil
	}
	host := hostOf(ch.BaseURL)
	if ch.FrequencyHz > 0 {
		if m := h.muxes[ch.FrequencyHz]; m != nil {
			return h.addFeedLocked(m, ch), nil
		}
	}
	bases := []string{ch.BaseURL}
	if h.Store != nil {
		if more, err := h.Store.OtherDevices(ctx, ch.GuideNumber, ch.DeviceID); err == nil {
			for _, base := range more {
				if base != "" && base != ch.BaseURL {
					bases = append(bases, base)
				}
			}
		}
	}
	var devices []DeviceTuners
	var last []Tuner
	for _, candidate := range bases {
		tuners, err := h.readTuners(ctx, candidate)
		if err != nil {
			continue
		}
		last = tuners
		devices = append(devices, DeviceTuners{Host: hostOf(candidate), Tuners: tuners})
	}
	if len(devices) == 0 {
		return nil, fmt.Errorf("the tuner did not answer")
	}
	held := h.reserved
	if h.hold > 0 && len(devices) > 0 {
		held = map[int]bool{}
		for k, v := range h.reserved {
			held[k] = v
		}
		for k, v := range HoldBack(devices[0].Tuners, h.hold) {
			held[k] = v
		}
	}
	host, tuner, ok := PickTuner(devices, h.usedTunersLocked(), held)
	if !ok {
		return nil, &BusyError{Tuners: last}
	}
	h.reserved[tuner] = true
	defer delete(h.reserved, tuner)
	freq, programs, err := probe(host, tuner, ch.GuideNumber)
	if err != nil {
		streamURL := streamRoot(ch) + "/auto/v" + ch.GuideNumber
		if h.Encoder == "" || h.Encoder == "libx264" {
			if q := hdhr.ExtendQuery(ch.ModelNumber); q != "" {
				streamURL += "?" + q
			}
		}
		res, err := openStream(streamURL, "", "")
		if err != nil {
			return nil, err
		}
		return h.addFeedLocked(h.streamMuxLocked(ch, res.Body, host), ch), nil
	}
	for _, p := range programs {
		_ = h.Store.RememberProgram(ctx, ch.DeviceID, p.GuideNumber, freq, p.Number)
	}
	ch.ProgramNum = programFor(programs, ch.GuideNumber)
	ch.FrequencyHz = freq
	body, err := openMux(streamRoot(ch), tuner, freq)
	if err != nil {
		_, _ = hdhr.Control{Addr: controlAddr(host)}.Set(fmt.Sprintf("/tuner%d/channel", tuner), "none")
		return nil, err
	}
	m := &mux{freq: freq, tuner: tuner, host: host, device: ch.DeviceID, body: body, feeds: map[string]*feed{}, programs: programs}
	runCtx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	h.muxes[freq] = m
	go h.readLoop(runCtx, m)
	h.startFrames(runCtx, m)
	return h.addFeedLocked(m, ch), nil
}

// streamMuxLocked wraps a single-program stream (IPTV, or the tuner's /auto URL).
func (h *Hub) streamMuxLocked(ch store.SourceChannel, body io.ReadCloser, host string) *mux {
	h.next--
	m := &mux{freq: h.next, tuner: -1, host: host, device: ch.DeviceID, body: body, feeds: map[string]*feed{}}
	runCtx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	h.muxes[m.freq] = m
	go h.readLoop(runCtx, m)
	h.startFrames(runCtx, m)
	return m
}

func (h *Hub) hlsMuxLocked(ch store.SourceChannel) *mux {
	h.next--
	m := &mux{freq: h.next, tuner: -1, host: "hls", device: ch.DeviceID, input: ch.StreamURL, feeds: map[string]*feed{}, cancel: func() {}}
	h.muxes[m.freq] = m
	return m
}

func hlsURL(raw string) bool {
	u := strings.ToLower(raw)
	return strings.Contains(u, ".m3u8")
}

func hlsStream(ch store.SourceChannel) bool {
	switch strings.ToLower(ch.StreamFormat) {
	case "hls":
		return true
	case "mpegts", "ts":
		return false
	default:
		return hlsURL(ch.StreamURL)
	}
}

func (h *Hub) addFeedLocked(m *mux, ch store.SourceChannel) *feed {
	if m.tuner < 0 {
		ch.FrequencyHz = m.freq
		ch.ProgramNum = 0
	} else {
		if ch.ProgramNum == 0 {
			ch.ProgramNum = programFor(m.programs, ch.GuideNumber)
		}
		ch.FrequencyHz = m.freq
	}
	if f := m.feeds[ch.GuideNumber]; f != nil {
		h.channels[ch.ID] = f
		return f
	}
	f := &feed{
		channel: ch, program: ch.ProgramNum, source: sourceOf(ch),
		renditions: map[string]*rendition{}, timeline: NewTimeline(),
	}
	m.feeds[ch.GuideNumber] = f
	h.channels[ch.ID] = f
	if m.input == "" && (ch.FieldOrder == "" || len(f.tracks) == 0) {
		h.learnScanLocked(m, f)
	}
	return f
}

func (h *Hub) ensureRenditionLocked(f *feed, want Rendition) (*rendition, error) {
	if want.Codec == "hevc" && !h.HEVC {
		want.Codec = ""
	}
	key := want.Key()
	if r := f.renditions[key]; r != nil {
		return r, nil
	}
	dir := filepath.Join(h.Dir, "live", fmt.Sprintf("%d", f.channel.ID), key)
	if err := os.RemoveAll(dir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	input := "pipe:0"
	if m := muxOf(h, f); m != nil && m.input != "" {
		input = m.input
	}
	args := renditionArgs(f.program, f.sourceFor(want), want, h.Encoder, h.deintFor(want.Mode, f.source.VideoCodec), input)
	cmd := exec.Command(h.FFmpeg, args...)
	cmd.Dir = dir
	var stdin io.WriteCloser
	if input == "pipe:0" {
		var err error
		stdin, err = cmd.StdinPipe()
		if err != nil {
			return nil, err
		}
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	pid := cmd.Process.Pid
	NotePID(h.Dir, pid)
	r := &rendition{spec: want, dir: dir, cmd: cmd, stdin: stdin, seen: time.Now()}
	if stdin != nil {
		r.sub = h.attachPipeLocked(muxOf(h, f), stdin)
	}
	f.renditions[key] = r
	go h.watchRendition(f, r, pid, encoderOf(h.Encoder, want))
	return r, nil
}

func encoderOf(base string, want Rendition) string {
	if want.Video == "copy" {
		return ""
	}
	return OutputEncoder(base, want.Codec)
}

// watchRendition restarts a GPU rendition on software decode when ffmpeg dies immediately.
func (h *Hub) watchRendition(f *feed, r *rendition, pid int, encoder string) {
	started := time.Now()
	err := r.cmd.Wait()
	ForgetPID(h.Dir, pid)
	if err == nil || time.Since(started) > 8*time.Second || !vaapiFamily(encoder) || r.fallback {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if f.renditions[r.spec.Key()] != r {
		return
	}
	r.fallback = true
	if m := muxOf(h, f); m != nil {
		m.detach(r.sub)
	} else if r.sub != nil {
		r.sub.stop()
	}
	r.sub = nil
	input := "pipe:0"
	if m := muxOf(h, f); m != nil && m.input != "" {
		input = m.input
	}
	args := renditionArgs(f.program, f.sourceFor(r.spec), r.spec, "libx264", "", input)
	cmd := exec.Command(h.FFmpeg, args...)
	cmd.Dir = r.dir
	var stdin io.WriteCloser
	if input == "pipe:0" {
		var pipeErr error
		stdin, pipeErr = cmd.StdinPipe()
		if pipeErr != nil {
			return
		}
	}
	cmd.Stderr = os.Stderr
	if cmd.Start() != nil {
		return
	}
	next := cmd.Process.Pid
	NotePID(h.Dir, next)
	r.cmd = cmd
	r.stdin = stdin
	if stdin != nil {
		r.sub = h.attachPipeLocked(muxOf(h, f), stdin)
	}
	go func() {
		_ = cmd.Wait()
		ForgetPID(h.Dir, next)
	}()
}

// Playlist returns a rendition's live playlist stamped with the channel timeline.
func (h *Hub) Playlist(channelID int64, key string) ([]byte, error) {
	h.mu.Lock()
	f := h.channels[channelID]
	var r *rendition
	if f != nil {
		r = f.renditions[key]
		if r != nil {
			r.seen = time.Now()
		}
	}
	h.mu.Unlock()
	if r == nil {
		return nil, os.ErrNotExist
	}
	raw, err := readPlaylist(filepath.Join(r.dir, "index.m3u8"))
	if err != nil {
		return nil, err
	}
	return r.stamper.stamp(r.dir, raw, f.timeline), nil
}

// Touch records that a viewer of a rendition is still fetching video.
func (h *Hub) Touch(channelID int64, key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if f := h.channels[channelID]; f != nil {
		if r := f.renditions[key]; r != nil {
			r.seen = time.Now()
		}
	}
}

// ReleaseAbandoned drops viewers that stopped asking for video, so a closed
// browser or a sleeping phone does not hold a tuner.
func (h *Hub) ReleaseAbandoned(maxAge time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	for _, f := range h.feedsLocked() {
		for key, r := range f.renditions {
			if r.viewers == 0 || now.Sub(r.seen) < maxAge {
				continue
			}
			r.viewers = 0
			h.stopRenditionLocked(f, key)
		}
		h.dropIfUnusedLocked(f)
	}
}

// Release removes one viewer. An empty key releases from the busiest rendition,
// for clients that don't track which one they joined.
func (h *Hub) Release(channelID int64, key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	f := h.channels[channelID]
	if f == nil {
		return
	}
	if key == "" {
		best := 0
		for k, r := range f.renditions {
			if r.viewers > best {
				key, best = k, r.viewers
			}
		}
	}
	r := f.renditions[key]
	if r == nil || r.viewers == 0 {
		return
	}
	r.viewers--
	h.changed()
	if r.viewers == 0 {
		stopTimer(&r.idle)
		r.idle = time.AfterFunc(h.RenditionIdle, func() { h.idleStop(channelID, key) })
	}
}

func (h *Hub) idleStop(channelID int64, key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	f := h.channels[channelID]
	if f == nil {
		return
	}
	if r := f.renditions[key]; r != nil && r.viewers == 0 {
		h.stopRenditionLocked(f, key)
	}
	h.dropIfUnusedLocked(f)
}

func (h *Hub) Record(ctx context.Context, channelID int64, minutes int, title string) (store.Recording, error) {
	return h.RecordMeta(ctx, minutes, store.Recording{ChannelID: channelID, Title: title})
}

// RecordMeta records the original broadcast of a channel. It shares the tuned
// frequency with anyone watching and starts no transcode.
func (h *Hub) RecordMeta(ctx context.Context, minutes int, meta store.Recording) (store.Recording, error) {
	h.preemptScan()
	channelID := meta.ChannelID
	title := meta.Title
	if minutes <= 0 {
		minutes = 60
	}
	if err := h.ensureSpace(ctx); err != nil {
		return store.Recording{}, err
	}
	ch, err := h.Store.SourceChannel(ctx, channelID)
	if err != nil {
		return store.Recording{}, err
	}
	var res *http.Response
	if ch.TunerCount == 0 && ch.StreamURL != "" && !hlsStream(ch) {
		h.mu.Lock()
		_, tuned := h.channels[channelID]
		h.mu.Unlock()
		if !tuned {
			if res, err = openStream(ch.StreamURL, ch.UserAgent, ch.Referrer); err != nil {
				return store.Recording{}, err
			}
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	f, err := h.ensureFeedLocked(ctx, ch, res)
	if err != nil {
		return store.Recording{}, err
	}
	if f.recording != nil {
		return h.Store.Recording(ctx, f.recording.id)
	}
	dir := filepath.Join(h.Dir, "recordings")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		h.dropIfUnusedLocked(f)
		return store.Recording{}, err
	}
	name := fmt.Sprintf("%s_%s_%s.ts", time.Now().Format("20060102_150405"), f.channel.GuideNumber, sanitize(f.channel.DisplayName))
	path := uniquePath(filepath.Join(dir, name))
	ends := time.Now().Add(time.Duration(minutes) * time.Minute)
	if title == "" {
		title = f.channel.DisplayName
	}
	if meta.GameID == "" {
		meta.GameID = h.Store.AiringGame(ctx, channelID, title, time.Now())
	}
	id, err := h.Store.CreateRecording(ctx, store.Recording{
		ChannelID: channelID, GuideNumber: f.channel.GuideNumber, Title: title,
		Subtitle: meta.Subtitle, Description: meta.Description, Category: meta.Category, ProgramID: meta.ProgramID, GameID: meta.GameID,
		Path: path, Status: "recording", StartedAt: time.Now(), EndsAt: &ends,
	})
	if err != nil {
		h.dropIfUnusedLocked(f)
		return store.Recording{}, err
	}
	if key := store.EpisodeKey(meta.ProgramID, title, meta.Subtitle, channelID); key != "" {
		_ = h.Store.RememberSeen(ctx, key, false)
	}
	_ = h.Store.AddEvent(ctx, "recording", fmt.Sprintf("Started %s on %s", title, f.channel.GuideNumber))
	input := "pipe:0"
	if mux := muxOf(h, f); mux != nil && mux.input != "" {
		input = mux.input
	}
	cmd := exec.Command(h.FFmpeg, copyArgs(f.program, input, f.channel.UserAgent, f.channel.Referrer, path)...)
	var stdin io.WriteCloser
	if input == "pipe:0" {
		stdin, err = cmd.StdinPipe()
		if err != nil {
			h.abortRecordingLocked(ctx, f, id)
			return store.Recording{}, err
		}
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		h.abortRecordingLocked(ctx, f, id)
		return store.Recording{}, err
	}
	NotePID(h.Dir, cmd.Process.Pid)
	rec := &recording{id: id, cmd: cmd, stdin: stdin}
	if stdin != nil {
		rec.sub = h.attachPipeLocked(muxOf(h, f), stdin)
	}
	f.recording = rec
	rec.timer = time.AfterFunc(time.Duration(minutes)*time.Minute, func() { h.StopRecord(id) })
	h.changed()
	return h.Store.Recording(ctx, id)
}

// Shutdown stops every rendition, finishes recordings that are in progress,
// and releases tuners. Used on SIGTERM so a restart does not leave ffmpeg
// running or a tuner locked.
func (h *Hub) Shutdown() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, f := range h.feedsLocked() {
		if f.recording != nil {
			h.finishRecordingLocked(f, "complete", "")
		}
		h.stopFeedLocked(f)
	}
}

// uniquePath adds -2, -3, ... when a recording file already exists. Names are
// per second, and ffmpeg refuses to overwrite, so two recordings of one channel
// started in the same second would otherwise lose the second one.
func uniquePath(path string) string {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d%s", base, i, ext)
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
	}
}

func (h *Hub) StopRecord(id int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, f := range h.feedsLocked() {
		if f.recording != nil && f.recording.id == id {
			h.finishRecordingLocked(f, "complete", "")
			h.dropIfUnusedLocked(f)
			return
		}
	}
}

// ExtendRecording moves the end of an in-progress recording.
func (h *Hub) ExtendRecording(ctx context.Context, id int64, until time.Time) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, f := range h.feedsLocked() {
		if f.recording == nil || f.recording.id != id {
			continue
		}
		stopTimer(&f.recording.timer)
		f.recording.timer = time.AfterFunc(time.Until(until), func() { h.StopRecord(id) })
		return h.Store.SetRecordingEnd(ctx, id, until)
	}
	return fmt.Errorf("recording %d is not in progress", id)
}

// SetHold keeps that many tuners free for recordings that are about to start.
func (h *Hub) SetHold(n int) {
	if n < 0 {
		n = 0
	}
	h.mu.Lock()
	h.hold = n
	h.mu.Unlock()
}

func (h *Hub) Tuners(ctx context.Context) ([]Tuner, error) {
	h.mu.Lock()
	host := ""
	for _, m := range h.muxes {
		if m.tuner >= 0 {
			host = m.host
			break
		}
	}
	h.mu.Unlock()
	if h.Store != nil && !strings.Contains(host, "://") {
		devices, err := h.Store.Devices(ctx)
		if err != nil {
			return nil, err
		}
		for _, d := range devices {
			if d.TunerCount > 0 && (host == "" || hostOf(d.BaseURL) == host) {
				host = d.BaseURL
				break
			}
		}
	}
	if host == "" {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.readTuners(ctx, host)
}

func (h *Hub) attachPipeLocked(m *mux, w io.WriteCloser) *pipeSub {
	// A few seconds of the mux have to fit. The rendition does not read during
	// VAAPI startup, and a gap at the start leaves the deinterlacer with no
	// picture, so the playlist stays an empty file.
	sub := &pipeSub{w: w, ch: make(chan []byte, 4096), done: make(chan struct{})}
	if m == nil {
		sub.stop()
		_ = w.Close()
		return sub
	}
	m.pipeMu.Lock()
	m.pipes = append(m.pipes, sub)
	m.pipeMu.Unlock()
	go func() {
		defer w.Close()
		for {
			select {
			case <-sub.done:
				return
			case chunk, ok := <-sub.ch:
				if !ok {
					return
				}
				if _, err := w.Write(chunk); err != nil {
					return
				}
			}
		}
	}()
	return sub
}

// readLoop is the only reader of the tuner. A slow subscriber loses chunks
// rather than stalling the tuner for everyone else. When the tuner itself
// ends, the mux is released so the tuner does not stay busy.
func (h *Hub) readLoop(ctx context.Context, m *mux) {
	buf := make([]byte, 188*49)
	for {
		if ctx.Err() != nil {
			return
		}
		n, err := m.body.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			if g, ok := m.psip.Add(chunk); ok && h.OnPSIP != nil {
				freq, guide := m.freq, g
				go h.OnPSIP(freq, guide)
			}
			for _, sub := range m.snapshot() {
				select {
				case sub.ch <- chunk:
				default:
				}
			}
		}
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("mux %d ended: %v", m.freq, err)
			h.releaseMux(m)
			return
		}
	}
}

// releaseMux drops every channel on a mux whose tuner read has ended.
func (h *Hub) releaseMux(m *mux) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.muxes[m.freq] != m {
		return
	}
	feeds := make([]*feed, 0, len(m.feeds))
	for _, f := range m.feeds {
		feeds = append(feeds, f)
	}
	for _, f := range feeds {
		if f.recording != nil {
			h.finishRecordingLocked(f, "failed", "The tuner stopped.")
		}
		h.stopFeedLocked(f)
	}
}

func (h *Hub) abortRecordingLocked(ctx context.Context, f *feed, id int64) {
	_ = h.Store.FinishRecording(ctx, id, "failed", "Could not start the recording.")
	h.dropIfUnusedLocked(f)
}

func (h *Hub) sessionLocked(f *feed, r *rendition) Session {
	viewers := 0
	for _, other := range f.renditions {
		viewers += other.viewers
	}
	m := muxOf(h, f)
	shared := viewers > 1 || f.recording != nil || (m != nil && len(m.feeds) > 1)
	spec := r.spec
	key := spec.Key()
	info := StreamInfo{
		Rendition: key, Video: spec.Video, Audio: spec.Audio, Mode: spec.Mode,
		SourceVideo: f.source.VideoCodec, SourceAudio: f.source.AudioCodec,
	}
	if spec.Video != "copy" {
		info.Encoder = encoderOf(h.Encoder, spec)
		if r.fallback {
			info.Encoder = "libx264"
		}
	}
	legacyAudio := "stereo"
	if spec.Audio == "aac6" || spec.Audio == "copy" {
		legacyAudio = "surround"
	}
	videoMode := "transcode"
	if spec.Video == "copy" {
		videoMode = "copy"
	}
	return Session{
		ChannelID: f.channel.ID,
		Playlist:  fmt.Sprintf("/media/live/%d/%s/index.m3u8", f.channel.ID, key),
		Rendition: key,
		Stream:    info,
		Encoder:   h.Encoder,
		Shared:    shared,
		Viewers:   viewers,
		Frequency: f.channel.FrequencyHz,
		Program:   f.program,
		Profile:   renditionProfile(spec.Video),
		Audio:     legacyAudio,
		Picture:   spec.Mode,
		VideoMode: videoMode,
		Hints:     []string{},
		File:      filepath.Join(r.dir, "index.m3u8"),
	}
}

func (h *Hub) stopRenditionLocked(f *feed, key string) {
	r := f.renditions[key]
	if r == nil {
		return
	}
	stopTimer(&r.idle)
	muxOf(h, f).detach(r.sub)
	if r.cmd != nil && r.cmd.Process != nil {
		ForgetPID(h.Dir, r.cmd.Process.Pid)
		if r.stdin != nil {
			_ = r.stdin.Close()
		}
		_ = r.cmd.Process.Kill()
	}
	delete(f.renditions, key)
	_ = os.RemoveAll(r.dir)
	h.changed()
}

// dropIfUnusedLocked releases the feed, and the tuner with the last feed, once
// nobody is watching or recording it.
func (h *Hub) dropIfUnusedLocked(f *feed) {
	if len(f.renditions) > 0 || f.recording != nil || f.probing || f.exports > 0 {
		return
	}
	h.stopFeedLocked(f)
}

func (h *Hub) stopFeedLocked(f *feed) {
	for key := range f.renditions {
		h.stopRenditionLocked(f, key)
	}
	if h.channels[f.channel.ID] == f {
		delete(h.channels, f.channel.ID)
	}
	for id, other := range h.channels {
		if other == f {
			delete(h.channels, id)
		}
	}
	m := muxOf(h, f)
	if m == nil {
		return
	}
	delete(m.feeds, f.channel.GuideNumber)
	if len(m.feeds) == 0 {
		m.cancel()
		if m.body != nil {
			_ = m.body.Close()
		}
		for _, sub := range m.snapshot() {
			sub.stop()
		}
		if m.tuner >= 0 {
			_, _ = hdhr.Control{Addr: m.host}.Set(fmt.Sprintf("/tuner%d/channel", m.tuner), "none")
		}
		delete(h.muxes, m.freq)
	}
}

func (h *Hub) finishRecordingLocked(f *feed, status, errText string) {
	rec := f.recording
	if rec == nil {
		return
	}
	stopTimer(&rec.timer)
	muxOf(h, f).detach(rec.sub)
	if rec.stdin != nil {
		_ = rec.stdin.Close()
	}
	if rec.cmd.Process != nil {
		ForgetPID(h.Dir, rec.cmd.Process.Pid)
		done := make(chan struct{})
		go func() { _ = rec.cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = rec.cmd.Process.Kill()
		}
	}
	f.recording = nil
	h.changed()
	_ = h.Store.FinishRecording(context.Background(), rec.id, status, errText)
	if saved, err := h.Store.Recording(context.Background(), rec.id); err == nil {
		writeSidecar(saved)
		message := fmt.Sprintf("Saved %s on %s", saved.Title, saved.GuideNumber)
		if status != "complete" {
			message = fmt.Sprintf("Recording %s on %s ended %s", saved.Title, saved.GuideNumber, status)
		}
		_ = h.Store.AddEvent(context.Background(), "recording", message)
		go h.rememberDuration(saved)
		if h.OnSaved != nil && status == "complete" {
			go h.OnSaved(saved)
		}
	}
}

func (h *Hub) rememberDuration(rec store.Recording) {
	tool := FFProbePath(h.FFmpeg)
	if tool == "" || rec.Path == "" || h.Store == nil {
		return
	}
	seconds, err := ProbeDuration(tool, rec.Path)
	if err != nil || seconds <= 0 {
		_ = h.Store.SetDuration(context.Background(), rec.ID, -1)
		return
	}
	_ = h.Store.SetDuration(context.Background(), rec.ID, seconds)
}

func (h *Hub) ensureSpace(ctx context.Context) error {
	if h.Store == nil || h.Dir == "" {
		return nil
	}
	values, err := h.Store.Settings(ctx)
	if err != nil {
		return nil
	}
	reserve := disk.WatermarkBytes(values["watermarkGB"])
	if reserve == 0 {
		return nil
	}
	dir := filepath.Join(h.Dir, "recordings")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	space, err := disk.Stat(dir)
	if err != nil {
		return nil
	}
	if disk.BelowReserve(space.Free, reserve) {
		_ = h.Store.AddEvent(ctx, "disk", "Refused a recording because free space is under the reserve")
		return &disk.LowError{Free: space.Free, Need: reserve}
	}
	return nil
}

func (h *Hub) changed() {
	if h.OnChange != nil {
		go h.OnChange()
	}
}

func stopTimer(t **time.Timer) {
	if *t != nil {
		(*t).Stop()
		*t = nil
	}
}

// feedsLocked lists each feed once; several channel ids can point at one feed.
func (h *Hub) feedsLocked() []*feed {
	seen := map[*feed]bool{}
	var out []*feed
	for _, f := range h.channels {
		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].channel.ID < out[j].channel.ID })
	return out
}

func (h *Hub) usedTunersLocked() map[int]bool {
	used := map[int]bool{}
	for _, m := range h.muxes {
		if m.tuner >= 0 {
			used[m.tuner] = true
		}
	}
	return used
}

// StreamLimitMessage is what a viewer sees when a playlist has no free stream.
func StreamLimitMessage(limit int) string {
	return fmt.Sprintf("All %d streams from this playlist are in use. Stop one or raise the limit.", limit)
}

func streamBusy(ch store.SourceChannel, inUse int) bool {
	return ch.TunerCount == 0 && ch.StreamURL != "" && ch.StreamLimit > 0 && inUse >= ch.StreamLimit
}

// StreamsInUse counts channels from one source that are open right now.
func (h *Hub) StreamsInUse(deviceID string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.streamsForDeviceLocked(deviceID)
}

func (h *Hub) streamsForDeviceLocked(deviceID string) int {
	n := 0
	for _, f := range h.channels {
		if f.channel.DeviceID == deviceID {
			n++
		}
	}
	return n
}

func (h *Hub) viewersOnTunerLocked(tuner int) int {
	n := 0
	for _, m := range h.muxes {
		if m.tuner != tuner {
			continue
		}
		for _, f := range m.feeds {
			for _, r := range f.renditions {
				n += r.viewers
			}
		}
	}
	return n
}

func (h *Hub) readTuners(ctx context.Context, host string) ([]Tuner, error) {
	var raw []struct {
		Resource              string
		VctNumber             string
		VctName               string
		TargetIP              string
		SignalStrengthPercent int
		SignalQualityPercent  int
		SymbolQualityPercent  int
	}
	base := host
	if !strings.Contains(base, "://") {
		base = "http://" + base
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/status.json", nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	ours := h.usedTunersLocked()
	out := make([]Tuner, 0, len(raw))
	for i, row := range raw {
		out = append(out, Tuner{
			Index: i, Guide: row.VctNumber, Name: row.VctName, Target: row.TargetIP,
			Ours: ours[i], Strength: row.SignalStrengthPercent, Quality: row.SignalQualityPercent, Symbol: row.SymbolQualityPercent,
			Shared: h.viewersOnTunerLocked(i),
		})
	}
	return out, nil
}

func firstFree(tuners []Tuner, used, reserved map[int]bool) (int, bool) {
	for _, t := range tuners {
		if used[t.Index] || reserved[t.Index] || t.Target != "" || t.Guide != "" {
			continue
		}
		return t.Index, true
	}
	return 0, false
}

func openStream(u, userAgent, referrer string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	if referrer != "" {
		req.Header.Set("Referer", referrer)
	}
	res, err := (&http.Client{Timeout: 0}).Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, fmt.Errorf("stream returned %s", res.Status)
	}
	return res, nil
}

func probe(host string, tuner int, guide string) (int, []hdhr.Program, error) {
	c := hdhr.Control{Addr: controlAddr(host)}
	name := fmt.Sprintf("/tuner%d/vchannel", tuner)
	if _, err := c.Set(name, guide); err != nil {
		return 0, nil, err
	}
	var status string
	var err error
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		status, err = c.Get(fmt.Sprintf("/tuner%d/status", tuner))
		if err == nil && hdhr.FrequencyHz(status) > 0 && strings.Contains(status, "lock=8vsb") {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	freq := hdhr.FrequencyHz(status)
	if freq == 0 {
		_, _ = c.Set(fmt.Sprintf("/tuner%d/channel", tuner), "none")
		if err != nil {
			return 0, nil, err
		}
		return 0, nil, fmt.Errorf("tuner %d did not lock %s", tuner, guide)
	}
	info := ""
	infoDeadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(infoDeadline) {
		info, err = c.Get(fmt.Sprintf("/tuner%d/streaminfo", tuner))
		if err == nil && strings.Contains(info, guide) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err != nil {
		_, _ = c.Set(fmt.Sprintf("/tuner%d/channel", tuner), "none")
		return 0, nil, err
	}
	programs := hdhr.ParseStreamInfo(info)
	if programFor(programs, guide) == 0 {
		_, _ = c.Set(fmt.Sprintf("/tuner%d/channel", tuner), "none")
		return 0, nil, fmt.Errorf("tuner %d locked %s but did not list that channel", tuner, guide)
	}
	return freq, programs, nil
}

func controlAddr(host string) string {
	if p := os.Getenv("HDHR_CONTROL_PORT"); p != "" {
		return net.JoinHostPort(host, p)
	}
	return host
}

func streamRoot(ch store.SourceChannel) string {
	if u, err := url.Parse(ch.StreamURL); err == nil && u.Host != "" {
		return u.Scheme + "://" + u.Host
	}
	return "http://" + hostOf(ch.BaseURL) + ":5004"
}

func openMux(root string, tuner, freq int) (io.ReadCloser, error) {
	u := fmt.Sprintf("%s/tuner%d/ch%d", strings.TrimRight(root, "/"), tuner, freq)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	res, err := (&http.Client{Timeout: 0}).Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		res.Body.Close()
		return nil, fmt.Errorf("tuner returned %s: %s", res.Status, strings.TrimSpace(string(b)))
	}
	return res.Body, nil
}

func (m *mux) detach(sub *pipeSub) {
	if m == nil || sub == nil {
		return
	}
	m.pipeMu.Lock()
	for i, s := range m.pipes {
		if s == sub {
			m.pipes = append(m.pipes[:i], m.pipes[i+1:]...)
			break
		}
	}
	m.pipeMu.Unlock()
	sub.stop()
}

func (m *mux) snapshot() []*pipeSub {
	m.pipeMu.Lock()
	defer m.pipeMu.Unlock()
	return append([]*pipeSub(nil), m.pipes...)
}

func programFor(programs []hdhr.Program, guide string) int {
	for _, p := range programs {
		if p.GuideNumber == guide {
			return p.Number
		}
	}
	return 0
}

func hostOf(base string) string {
	u, err := url.Parse(base)
	if err != nil || u.Hostname() == "" {
		return strings.TrimPrefix(strings.TrimPrefix(base, "https://"), "http://")
	}
	return u.Hostname()
}

func sanitize(name string) string {
	out := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '-'
		}
		return r
	}, name)
	out = strings.TrimSpace(out)
	if out == "" {
		return "channel"
	}
	return out
}

func muxOf(h *Hub, f *feed) *mux {
	return h.muxes[f.channel.FrequencyHz]
}
