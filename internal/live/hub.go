package live

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ota-viewer/internal/disk"
	"ota-viewer/internal/hdhr"
	"ota-viewer/internal/store"
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

type Session struct {
	ChannelID int64    `json:"channelId"`
	Playlist  string   `json:"playlist"`
	Profile   string   `json:"profile"`
	Audio     string   `json:"audio"`
	Encoder   string   `json:"encoder"`
	Picture   string   `json:"picture"`
	VideoMode string   `json:"videoMode"`
	Shared    bool     `json:"shared"`
	Viewers   int      `json:"viewers"`
	Frequency int      `json:"frequencyHz"`
	Program   int      `json:"program"`
	Hints     []string `json:"hints"`
	Tuners    []Tuner  `json:"tuners"`
}

type BusyError struct {
	Tuners []Tuner
}

func (e *BusyError) Error() string {
	return "both antenna tuners are busy"
}

type Hub struct {
	Store          *store.Store
	Dir            string
	FFmpeg         string
	Encoder        string
	DeintBroadcast string
	DeintSmooth    string
	Blend          bool
	OnSaved        func(store.Recording)
	mu             sync.Mutex
	muxes          map[int]*mux
	channels       map[int64]*feed
	reserved       map[int]bool
	next           int
	playMu         sync.Mutex
	plays          map[int64]struct{}
}

type mux struct {
	freq     int
	tuner    int
	host     string
	device   string
	body     io.ReadCloser
	cancel   context.CancelFunc
	feeds    map[string]*feed
	pipes    []*pipeSub
	programs []hdhr.Program
	pipeMu   sync.Mutex
}

type feed struct {
	channel   store.SourceChannel
	program   int
	viewers   int
	profile   string
	audio     string
	picture   string
	dir       string
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	recording *recording
	idle      *time.Timer
	seen      time.Time
}

type recording struct {
	id    int64
	cmd   *exec.Cmd
	stdin io.WriteCloser
	timer *time.Timer
}

type pipeSub struct {
	w    io.WriteCloser
	ch   chan []byte
	done chan struct{}
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
		Store: st, Dir: dir, FFmpeg: ffmpeg, Encoder: encoder,
		DeintBroadcast: broadcast, DeintSmooth: smooth, Blend: ProbeBlend(ffmpeg),
		muxes: map[int]*mux{}, channels: map[int64]*feed{}, reserved: map[int]bool{},
	}
}

func (h *Hub) deintFor(mode, codec string) string {
	if !InterlacedCodec(codec) || NormalizeMode(mode) == "film" {
		return ""
	}
	if NormalizeMode(mode) == "smooth" && h.DeintSmooth != "" {
		return h.DeintSmooth
	}
	return h.DeintBroadcast
}

func (h *Hub) liveArgs(program int, codec, profile, audio, mode string) []string {
	return PictureArgs(Graph{
		Program: program, VideoCodec: codec, Profile: profile, Audio: audio,
		Encoder: h.Encoder, Mode: mode, Deint: h.deintFor(mode, codec), Blend: h.Blend,
		Input: "pipe:0", Live: true,
	})
}

func (h *Hub) Watch(ctx context.Context, channelID int64, profile, audio, picture string) (Session, error) {
	if profile == "" {
		profile = "transparent"
	}
	if audio == "" {
		audio = "stereo"
	}
	picture = NormalizeMode(picture)
	ch, err := h.Store.SourceChannel(ctx, channelID)
	if err != nil {
		return Session{}, err
	}
	if ch.TunerCount == 0 && ch.StreamURL != "" {
		return h.openURL(ch, profile, audio, picture)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if feed := h.channels[channelID]; feed != nil {
		if feed.profile != profile || feed.audio != audio || feed.picture != picture {
			_ = h.restartFeedLocked(feed, profile, audio, picture)
		}
		feed.viewers++
		feed.seen = time.Now()
		h.stopIdle(feed)
		return h.sessionLocked(feed, false), nil
	}
	host := hostOf(ch.BaseURL)
	if ch.FrequencyHz > 0 {
		if m := h.muxes[ch.FrequencyHz]; m != nil {
			return h.addFeedLocked(ctx, m, ch, profile, audio, picture)
		}
	}
	tuners, err := h.readTuners(ctx, host)
	if err != nil {
		return Session{}, err
	}
	tuner, ok := firstFree(tuners, h.usedTunersLocked(), h.reserved)
	if !ok {
		return Session{}, &BusyError{Tuners: tuners}
	}
	h.reserved[tuner] = true
	defer delete(h.reserved, tuner)
	freq, programs, err := probe(host, tuner, ch.GuideNumber)
	if err != nil {
		delete(h.reserved, tuner)
		return h.openSingleLocked(host, ch, profile, audio, picture)
	}
	for _, p := range programs {
		_ = h.Store.RememberProgram(ctx, ch.DeviceID, p.GuideNumber, freq, p.Number)
		if p.GuideNumber == ch.GuideNumber {
			ch.ProgramNum = p.Number
			ch.FrequencyHz = freq
		}
	}
	if ch.ProgramNum == 0 {
		ch.ProgramNum = programFor(programs, ch.GuideNumber)
		ch.FrequencyHz = freq
	}
	body, err := openMux(host, tuner, freq)
	if err != nil {
		_, _ = hdhr.Control{Addr: host}.Set(fmt.Sprintf("/tuner%d/channel", tuner), "none")
		return Session{}, err
	}
	m := &mux{
		freq: freq, tuner: tuner, host: host, device: ch.DeviceID,
		body: body, feeds: map[string]*feed{}, programs: programs,
	}
	runCtx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	h.muxes[freq] = m
	go m.readLoop(runCtx)
	return h.addFeedLocked(ctx, m, ch, profile, audio, picture)
}

func (h *Hub) Touch(channelID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if feed := h.channels[channelID]; feed != nil {
		feed.seen = time.Now()
	}
}

// ReleaseAbandoned drops viewers that stopped asking for video, so a closed browser does not keep a tuner.
func (h *Hub) ReleaseAbandoned(maxAge time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	for _, feed := range h.channels {
		if feed.recording != nil || feed.viewers == 0 || feed.seen.IsZero() {
			continue
		}
		if now.Sub(feed.seen) < maxAge {
			continue
		}
		feed.viewers = 0
		h.stopFeedLocked(feed)
	}
}

func (h *Hub) Release(channelID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	feed := h.channels[channelID]
	if feed == nil || feed.viewers == 0 {
		return
	}
	feed.viewers--
	if feed.viewers == 0 && feed.recording == nil {
		feed.idle = time.AfterFunc(20*time.Second, func() { h.idleStop(channelID) })
	}
}

func (h *Hub) Record(ctx context.Context, channelID int64, minutes int, title string) (store.Recording, error) {
	return h.RecordMeta(ctx, minutes, store.Recording{ChannelID: channelID, Title: title})
}

func (h *Hub) RecordMeta(ctx context.Context, minutes int, meta store.Recording) (store.Recording, error) {
	channelID := meta.ChannelID
	title := meta.Title
	if minutes <= 0 {
		minutes = 60
	}
	if err := h.ensureSpace(ctx); err != nil {
		return store.Recording{}, err
	}
	if _, err := h.Watch(ctx, channelID, "transparent", "stereo", "broadcast"); err != nil {
		return store.Recording{}, err
	}
	// Watch counted a viewer so the mux stays up. Recording holds its own ref.
	h.Release(channelID)
	h.mu.Lock()
	defer h.mu.Unlock()
	feed := h.channels[channelID]
	if feed == nil {
		return store.Recording{}, fmt.Errorf("channel is not tuned")
	}
	if feed.recording != nil {
		rec, err := h.Store.Recording(ctx, feed.recording.id)
		return rec, err
	}
	dir := filepath.Join(h.Dir, "recordings")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return store.Recording{}, err
	}
	name := fmt.Sprintf("%s_%s_%s.ts", time.Now().Format("20060102_150405"), feed.channel.GuideNumber, sanitize(feed.channel.DisplayName))
	path := filepath.Join(dir, name)
	ends := time.Now().Add(time.Duration(minutes) * time.Minute)
	if title == "" {
		title = feed.channel.DisplayName
	}
	id, err := h.Store.CreateRecording(ctx, store.Recording{
		ChannelID: channelID, GuideNumber: feed.channel.GuideNumber, Title: title,
		Subtitle: meta.Subtitle, Description: meta.Description, Category: meta.Category, ProgramID: meta.ProgramID,
		Path: path, Status: "recording", StartedAt: time.Now(), EndsAt: &ends,
	})
	if err != nil {
		return store.Recording{}, err
	}
	if key := store.EpisodeKey(meta.ProgramID, title, meta.Subtitle, channelID); key != "" {
		_ = h.Store.RememberSeen(ctx, key, false)
	}
	_ = h.Store.AddEvent(ctx, "recording", fmt.Sprintf("Started %s on %s", title, feed.channel.GuideNumber))
	cmd := exec.Command(h.FFmpeg, copyArgs(feed.program, path)...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return store.Recording{}, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return store.Recording{}, err
	}
	sub := h.attachPipeLocked(muxOf(h, feed), stdin)
	rec := &recording{id: id, cmd: cmd, stdin: stdin}
	feed.recording = rec
	h.stopIdle(feed)
	rec.timer = time.AfterFunc(time.Duration(minutes)*time.Minute, func() { h.StopRecord(id) })
	_ = sub
	return h.Store.Recording(ctx, id)
}

func (h *Hub) StopRecord(id int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, feed := range h.channels {
		if feed.recording != nil && feed.recording.id == id {
			h.finishRecordingLocked(feed, "complete", "")
			if feed.viewers == 0 {
				feed.idle = time.AfterFunc(5*time.Second, func() { h.idleStop(feed.channel.ID) })
			}
			return
		}
	}
}

func (h *Hub) Tuners(ctx context.Context) ([]Tuner, error) {
	h.mu.Lock()
	host := ""
	for _, m := range h.muxes {
		host = m.host
		break
	}
	h.mu.Unlock()
	if host == "" && h.Store != nil {
		devices, err := h.Store.Devices(ctx)
		if err != nil {
			return nil, err
		}
		if len(devices) > 0 {
			host = hostOf(devices[0].BaseURL)
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

func (h *Hub) addFeedLocked(_ context.Context, m *mux, ch store.SourceChannel, profile, audio, picture string) (Session, error) {
	if ch.ProgramNum == 0 {
		ch.ProgramNum = programFor(m.programs, ch.GuideNumber)
	}
	if ch.FrequencyHz == 0 {
		ch.FrequencyHz = m.freq
	}
	if existing := m.feeds[ch.GuideNumber]; existing != nil {
		existing.viewers++
		h.stopIdle(existing)
		h.channels[ch.ID] = existing
		return h.sessionLocked(existing, true), nil
	}
	dir := filepath.Join(h.Dir, "live", fmt.Sprintf("%d", ch.ID))
	if err := os.RemoveAll(dir); err != nil {
		return Session{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Session{}, err
	}
	cmd := exec.Command(h.FFmpeg, h.liveArgs(ch.ProgramNum, ch.VideoCodec, profile, audio, picture)...)
	cmd.Dir = dir
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return Session{}, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return Session{}, err
	}
	h.attachPipeLocked(m, stdin)
	f := &feed{
		channel: ch, program: ch.ProgramNum, viewers: 1, profile: profile, audio: audio, picture: picture, seen: time.Now(),
		dir: dir, cmd: cmd, stdin: stdin,
	}
	m.feeds[ch.GuideNumber] = f
	h.channels[ch.ID] = f
	go func() { _ = cmd.Wait() }()
	return h.sessionLocked(f, len(m.feeds) > 1), nil
}

func (h *Hub) attachPipeLocked(m *mux, w io.WriteCloser) *pipeSub {
	sub := &pipeSub{w: w, ch: make(chan []byte, 800), done: make(chan struct{})}
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

func (m *mux) readLoop(ctx context.Context) {
	buf := make([]byte, 188*49)
	for {
		if ctx.Err() != nil {
			return
		}
		n, err := m.body.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			for _, sub := range m.snapshot() {
				select {
				case sub.ch <- chunk:
				default:
				}
			}
		}
		if err != nil {
			log.Printf("mux %d ended: %v", m.freq, err)
			return
		}
	}
}

func (h *Hub) sessionLocked(f *feed, shared bool) Session {
	mode := "transcode"
	hints := []string{}
	if strings.EqualFold(f.channel.VideoCodec, "MPEG2") {
		hints = append(hints, "This channel is MPEG-2, which a browser cannot play. The server is converting the picture. A recording stays the original broadcast.")
	} else if strings.EqualFold(f.channel.VideoCodec, "H264") {
		hints = append(hints, "This channel is already H.264. It is still resized for the browser, and the sound is converted from Dolby Digital.")
		mode = "transcode"
	}
	hints = append(hints, "The sound is Dolby Digital. This browser is hearing AAC. Surround is a choice in the player.")
	if shared || f.viewers > 1 {
		hints = append(hints, "Other screens on this broadcast are sharing the antenna tuner.")
	}
	hints = append(hints, "Live sits a few seconds behind the broadcast. The picture stays at normal speed.")
	if InterlacedCodec(f.channel.VideoCodec) && f.picture != "film" && f.profile != "saver" {
		hints = append(hints, "Interlaced channels are rebuilt at 60 frames a second.")
	}
	return Session{
		ChannelID: f.channel.ID,
		Playlist:  fmt.Sprintf("/media/live/%d/index.m3u8", f.channel.ID),
		Profile:   f.profile,
		Audio:     f.audio,
		Picture:   f.picture,
		Encoder:   h.Encoder,
		VideoMode: mode,
		Shared:    shared || f.viewers > 1,
		Viewers:   f.viewers,
		Frequency: f.channel.FrequencyHz,
		Program:   f.program,
		Hints:     hints,
	}
}

func (h *Hub) restartFeedLocked(f *feed, profile, audio, picture string) error {
	if f.cmd != nil && f.cmd.Process != nil {
		_ = f.stdin.Close()
		_ = f.cmd.Process.Kill()
	}
	_ = os.RemoveAll(f.dir)
	_ = os.MkdirAll(f.dir, 0o755)
	cmd := exec.Command(h.FFmpeg, h.liveArgs(f.program, f.channel.VideoCodec, profile, audio, picture)...)
	cmd.Dir = f.dir
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	f.cmd = cmd
	f.stdin = stdin
	f.profile = profile
	f.audio = audio
	f.picture = picture
	if m := h.muxes[f.channel.FrequencyHz]; m != nil {
		h.attachPipeLocked(m, stdin)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func (h *Hub) idleStop(channelID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	f := h.channels[channelID]
	if f == nil || f.viewers > 0 || f.recording != nil {
		return
	}
	h.stopFeedLocked(f)
}

func (h *Hub) stopFeedLocked(f *feed) {
	h.stopIdle(f)
	if f.cmd != nil && f.cmd.Process != nil {
		_ = f.stdin.Close()
		_ = f.cmd.Process.Kill()
	}
	delete(h.channels, f.channel.ID)
	m := h.muxes[f.channel.FrequencyHz]
	if m == nil {
		return
	}
	delete(m.feeds, f.channel.GuideNumber)
	if len(m.feeds) == 0 {
		m.cancel()
		_ = m.body.Close()
		for _, sub := range m.snapshot() {
			close(sub.done)
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
	if rec.timer != nil {
		rec.timer.Stop()
	}
	_ = rec.stdin.Close()
	if rec.cmd.Process != nil {
		_ = rec.cmd.Process.Kill()
	}
	f.recording = nil
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

func (h *Hub) stopIdle(f *feed) {
	if f.idle != nil {
		f.idle.Stop()
		f.idle = nil
	}
}

func (h *Hub) usedTunersLocked() map[int]bool {
	used := map[int]bool{}
	for _, m := range h.muxes {
		used[m.tuner] = true
	}
	return used
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+host+"/status.json", nil)
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

func (h *Hub) openURL(ch store.SourceChannel, profile, audio, picture string) (Session, error) {
	req, err := http.NewRequest(http.MethodGet, ch.StreamURL, nil)
	if err != nil {
		return Session{}, err
	}
	res, err := (&http.Client{Timeout: 0}).Do(req)
	if err != nil {
		return Session{}, err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return Session{}, fmt.Errorf("stream returned %s", res.Status)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if feed := h.channels[ch.ID]; feed != nil {
		res.Body.Close()
		if feed.profile != profile || feed.audio != audio || feed.picture != picture {
			_ = h.restartFeedLocked(feed, profile, audio, picture)
		}
		feed.viewers++
		feed.seen = time.Now()
		h.stopIdle(feed)
		return h.sessionLocked(feed, false), nil
	}
	h.next--
	key := h.next
	m := &mux{freq: key, tuner: -1, host: "stream", device: ch.DeviceID, body: res.Body, feeds: map[string]*feed{}}
	runCtx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	h.muxes[key] = m
	go m.readLoop(runCtx)
	ch.FrequencyHz = key
	ch.ProgramNum = 0
	session, err := h.addFeedLocked(context.Background(), m, ch, profile, audio, picture)
	if err != nil {
		return Session{}, err
	}
	session.Shared = false
	return session, nil
}

func (h *Hub) openSingleLocked(host string, ch store.SourceChannel, profile, audio, picture string) (Session, error) {
	u := fmt.Sprintf("http://%s:5004/auto/v%s", host, ch.GuideNumber)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u, nil)
	if err != nil {
		return Session{}, err
	}
	res, err := (&http.Client{Timeout: 0}).Do(req)
	if err != nil {
		return Session{}, err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return Session{}, fmt.Errorf("tuner returned %s", res.Status)
	}
	h.next--
	key := h.next
	m := &mux{freq: key, tuner: -1, host: host, device: ch.DeviceID, body: res.Body, feeds: map[string]*feed{}}
	runCtx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	h.muxes[key] = m
	go m.readLoop(runCtx)
	ch.FrequencyHz = key
	ch.ProgramNum = 0
	return h.addFeedLocked(context.Background(), m, ch, profile, audio, picture)
}

func probe(host string, tuner int, guide string) (int, []hdhr.Program, error) {
	c := hdhr.Control{Addr: host}
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

func openMux(host string, tuner, freq int) (io.ReadCloser, error) {
	u := fmt.Sprintf("http://%s:5004/tuner%d/ch%d", host, tuner, freq)
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
