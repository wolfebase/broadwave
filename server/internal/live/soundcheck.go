package live

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"broadwave/internal/store"
)

// A sound check settles which of an ATSC 3.0 channel's two language tracks
// carries the station's main language. Its labels come from the station and
// have been the wrong way round, and a stored swap is wrong again the day the
// station fixes them. The station's regular (1.0) broadcast carries the same
// program with labels that have held, so the check listens to both and
// stores the swap the sound says (soundmatch.go). A check runs in the
// background while the channel is watched or recorded, when the last one is
// older than the program on now or soundRecheck, one at a time. The twin is
// read from its tune when one is up; otherwise it is tuned for soundListen
// under the gates the still sweep shares (backgroundOpen), and any watch or
// recording that needs the tuner stops it (StopSweep). With no twin, no free
// tuner, or no clear answer, the stored choice stays.

var (
	// soundLag is the most the two broadcasts are apart. The channel is
	// heard that long before and after the twin.
	soundLag = 30 * time.Second
	// soundListen is how long the twin is heard.
	soundListen = 20 * time.Second
	// soundEvery is how often the loop looks for a channel due.
	soundEvery = 30 * time.Second
	// soundTwinWait is how long a twin tune has to show its program map.
	soundTwinWait = 6 * time.Second
)

const (
	// soundRecheck is the oldest answer kept while a channel stays tuned.
	soundRecheck = 6 * time.Hour
	// soundRetry is the wait after a check that did not decide, doubling up
	// to soundRetryMax.
	soundRetry    = 10 * time.Minute
	soundRetryMax = 2 * time.Hour
)

// SoundCheckedByTwin and SoundCheckedByHand are how a channel's sound was settled.
const (
	SoundCheckedByTwin = "twin"
	SoundCheckedByHand = "hand"
)

// soundTry is a channel whose last check did not decide.
type soundTry struct {
	until time.Time
	wait  time.Duration
}

// soundJob is one channel due a check.
type soundJob struct {
	id    int64
	guide string
	twin  store.Channel
}

var errSoundStopped = errors.New("the twin's tuner was needed")

// soundGateError is a twin tune the background gates shut. It is tried
// again soon: tuners free up as other tunes end.
type soundGateError struct{ msg string }

func (e soundGateError) Error() string { return e.msg }

// soundGateRetry is the wait after a shut gate.
const soundGateRetry = time.Minute

// soundDue reports whether an answer from last (nil: none) is stale at now,
// with program the start of the program on now (zero when unknown). A new
// program is checked once it has settled past StillSettle.
func soundDue(last *time.Time, program, now time.Time) bool {
	if last == nil || now.Sub(*last) >= soundRecheck {
		return true
	}
	return !program.IsZero() && program.After(*last) && now.Sub(program) >= StillSettle
}

// soundPair is a program's main and second-language tracks as the station
// labels them, before any swap. ok is false unless both are there, on
// their own PIDs, in two named languages.
func soundPair(tracks []AudioTrack, swapped bool) (main, lang AudioTrack, ok bool) {
	if swapped {
		tracks = swapSounds(tracks)
	}
	main, okMain := PickTrack(tracks, "main")
	lang, okLang := PickTrack(tracks, "language")
	if !okMain || !okLang || main.Role != "main" || main.PID <= 0 || lang.PID <= 0 || main.PID == lang.PID {
		return AudioTrack{}, AudioTrack{}, false
	}
	if main.Language == "" || lang.Language == "" || main.Language == lang.Language {
		return AudioTrack{}, AudioTrack{}, false
	}
	return main, lang, true
}

// RunSoundChecks checks, one at a time, the watched or recorded ATSC 3.0
// channels whose sound is due, until ctx ends. BROADWAVE_SOUND_CHECK=0
// turns it off.
func (h *Hub) RunSoundChecks(ctx context.Context) {
	if os.Getenv("BROADWAVE_SOUND_CHECK") == "0" || h.Store == nil || h.FFmpeg == "" {
		return
	}
	for ctx.Err() == nil {
		if job, ok := h.soundJobDue(ctx, time.Now()); ok {
			h.runSoundCheck(ctx, job)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(soundEvery):
		}
	}
}

// soundJobDue is the first channel due a check: watched or recording, ATSC
// 3.0 with a regular twin, two languages, and not waiting out a retry. Every
// watched or recorded 3.0 channel's state is noted with its reason
// (noteSound), so a check that does not run says why.
func (h *Hub) soundJobDue(ctx context.Context, now time.Time) (soundJob, bool) {
	type candidate struct {
		id    int64
		guide string
		last  *time.Time
	}
	var list []candidate
	type skip struct {
		id           int64
		guide, state string
	}
	var skips []skip
	h.mu.Lock()
	for _, f := range h.feedsLocked() {
		if f.channel.PlaysAs != 0 || h.channels[f.channel.ID] != f {
			continue
		}
		if !f.recordingNow() && !f.watched() {
			continue
		}
		if !NeedFor(f.channel.VideoCodec, f.channel.AudioCodec, f.channel.ATSC3).ATSC3 {
			continue
		}
		id, guide := f.channel.ID, f.channel.GuideNumber
		main, _, ok := soundPair(f.soundTracks(), f.channel.SoundSwapped)
		switch {
		case !ok:
			skips = append(skips, skip{id, guide, "waiting: " + soundPairWhy(f.soundTracks(), f.channel.SoundSwapped)})
			continue
		case main.Codec == "ac4" && h.NoAC4:
			skips = append(skips, skip{id, guide, "waiting: this ffmpeg cannot decode AC-4"})
			continue
		}
		if try, waiting := h.soundTries[id]; waiting && now.Before(try.until) {
			skips = append(skips, skip{id, guide, "waiting: the last check had no answer; next try at " + try.until.Format("15:04")})
			continue
		}
		list = append(list, candidate{id, guide, f.channel.SoundCheckedAt})
	}
	h.mu.Unlock()
	for _, sk := range skips {
		h.noteSound(sk.id, sk.guide, sk.state)
	}
	if len(list) == 0 {
		return soundJob{}, false
	}
	channels, err := h.Store.Channels(ctx, false)
	if err != nil {
		return soundJob{}, false
	}
	byID := map[int64]store.Channel{}
	for _, ch := range channels {
		byID[ch.ID] = ch
	}
	onNow := map[int64]time.Time{}
	if airings, err := h.Store.Airings(ctx, now, now.Add(time.Second)); err == nil {
		for _, a := range airings {
			onNow[a.ChannelID] = a.Start
		}
	}
	for _, c := range list {
		ch, ok := byID[c.id]
		if !ok || ch.Standard != store.TwinATSC3 || ch.TwinID == 0 {
			h.noteSound(c.id, c.guide, "waiting: no regular broadcast of this station to compare with")
			continue
		}
		twin, ok := byID[ch.TwinID]
		if !ok || twin.Standard == store.TwinATSC3 || twin.Protected {
			h.noteSound(c.id, c.guide, "waiting: the station's regular broadcast cannot be read")
			continue
		}
		program := onNow[ch.ID]
		if program.IsZero() {
			program = onNow[twin.ID]
		}
		if soundDue(c.last, program, now) {
			return soundJob{id: c.id, guide: c.guide, twin: twin}, true
		}
		h.noteSound(c.id, c.guide, "checked: "+c.last.Local().Format("15:04")+"; next at the next program or "+c.last.Add(soundRecheck).Local().Format("15:04"))
	}
	return soundJob{}, false
}

// soundTracks is the program's audio as the channel plays it: this tune's
// PMT, or before it is read, the tracks the last tune stored.
func (f *feed) soundTracks() []AudioTrack {
	if len(f.tracks) > 0 {
		return f.tracks
	}
	return f.stored
}

// soundPairWhy says why soundPair found no pair.
func soundPairWhy(tracks []AudioTrack, swapped bool) string {
	if len(tracks) == 0 {
		return "the program's sound tracks are not read yet"
	}
	if swapped {
		tracks = swapSounds(tracks)
	}
	if _, ok := PickTrack(tracks, "language"); !ok {
		return "one sound language (" + trackLog(tracks) + ")"
	}
	return "the two tracks are not two named languages (" + trackLog(tracks) + ")"
}

// soundNoteEvery is how often an unchanged state is logged again.
const soundNoteEvery = 5 * time.Minute

// SoundCheck is one channel's sound check state for Diagnostics.
type SoundCheck struct {
	ChannelID   int64     `json:"channelId"`
	GuideNumber string    `json:"guideNumber"`
	State       string    `json:"state"`
	At          time.Time `json:"at"`
}

// noteSound keeps a channel's check state and logs it when it changes, or
// every soundNoteEvery while it stays.
func (h *Hub) noteSound(id int64, guide, state string) {
	now := time.Now()
	h.mu.Lock()
	if h.soundStates == nil {
		h.soundStates = map[int64]soundState{}
	}
	prev := h.soundStates[id]
	log := prev.state != state || now.Sub(prev.logged) >= soundNoteEvery
	next := soundState{state: state, at: now, logged: prev.logged, guide: guide}
	if log {
		next.logged = now
	}
	h.soundStates[id] = next
	h.mu.Unlock()
	if log {
		slog.Info(fmt.Sprintf("sound check for %s: %s", guide, state))
	}
}

type soundState struct {
	guide, state string
	at, logged   time.Time
}

// SoundChecks is each channel's last sound check state, newest first.
func (h *Hub) SoundChecks() []SoundCheck {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]SoundCheck, 0, len(h.soundStates))
	for id, st := range h.soundStates {
		out = append(out, SoundCheck{ChannelID: id, GuideNumber: st.guide, State: st.state, At: st.at})
	}
	slices.SortFunc(out, func(a, b SoundCheck) int { return b.At.Compare(a.At) })
	return out
}

// watched is a feed with a viewer on one of its encodes.
func (f *feed) watched() bool {
	for _, r := range f.renditions {
		if r.viewers > 0 {
			return true
		}
	}
	return false
}

// runSoundCheck runs one check and keeps its answer, or waits longer before
// the next try when it did not decide.
func (h *Hub) runSoundCheck(ctx context.Context, job soundJob) {
	h.noteSound(job.id, job.guide, "checking against "+job.twin.DisplayNumber)
	began := time.Now()
	v, err := h.checkSound(ctx, job)
	took := time.Since(began).Round(time.Second)
	scores := fmt.Sprintf("main %.2f, second %.2f, alike %.2f", v.main, v.lang, v.same)
	if err == nil && v.ok {
		h.mu.Lock()
		delete(h.soundTries, job.id)
		h.mu.Unlock()
		answer := "the track marked as the main language is it"
		if v.swapped {
			answer = "the track marked as the second language is the main language"
		}
		h.noteSound(job.id, job.guide, fmt.Sprintf("checked against %s in %s: %s (%s)", job.twin.DisplayNumber, took, answer, scores))
		h.settleSound(ctx, job.id, v.swapped, SoundCheckedByTwin, time.Now(), job.twin)
		return
	}
	why := v.why
	if err != nil {
		why = err.Error()
	}
	h.mu.Lock()
	if h.soundTries == nil {
		h.soundTries = map[int64]soundTry{}
	}
	try := h.soundTries[job.id]
	var shut soundGateError
	if errors.As(err, &shut) {
		// A shut gate is no failed check: it keeps the backoff it had.
		try.until = time.Now().Add(soundGateRetry)
		h.soundTries[job.id] = try
		h.mu.Unlock()
		h.noteSound(job.id, job.guide, "waiting: "+shut.msg+"; next try in "+soundGateRetry.String())
		return
	}
	try.wait = min(max(try.wait*2, soundRetry), soundRetryMax)
	try.until = time.Now().Add(try.wait)
	h.soundTries[job.id] = try
	h.mu.Unlock()
	h.noteSound(job.id, job.guide, fmt.Sprintf("no answer against %s after %s: %s (%s); next try in %s", job.twin.DisplayNumber, took, why, scores, try.wait))
}

// checkSound hears the channel's two tracks from soundLag before the twin to
// soundLag after it, and the twin's main for soundListen, and judges them.
func (h *Hub) checkSound(ctx context.Context, job soundJob) (soundVerdict, error) {
	src, err := h.Store.SourceChannel(ctx, job.twin.ID)
	if err != nil {
		return soundVerdict{}, err
	}
	h.mu.Lock()
	f := h.channels[job.id]
	var m *mux
	var main, lang AudioTrack
	ok := false
	if f != nil {
		m = muxOf(h, f)
		main, lang, ok = soundPair(f.soundTracks(), f.channel.SoundSwapped)
	}
	twinUp := h.channels[job.twin.ID] != nil
	h.mu.Unlock()
	if m == nil || !ok {
		return soundVerdict{}, errors.New("the channel is no longer tuned")
	}
	// A shut gate is found before the channel is heard for nothing.
	if !twinUp {
		if open, g := h.tunerGateOpen(ctx, src, tunerGate.backgroundOpen); !open {
			return soundVerdict{}, soundGateError{fmt.Sprintf("no tuner for %s: %s", src.GuideNumber, g.why())}
		}
	}
	dir, err := os.MkdirTemp(h.Dir, "sound-")
	if err != nil {
		return soundVerdict{}, err
	}
	defer os.RemoveAll(dir)
	near, err := h.startSoundCapture(m, []int{main.PID, lang.PID}, filepath.Join(dir, "near"))
	if err != nil {
		return soundVerdict{}, err
	}
	defer near.stop()
	if err := sleepCtx(ctx, soundLag); err != nil {
		return soundVerdict{}, err
	}
	twinCtx, tf, release, err := h.holdTwin(ctx, src)
	if err != nil {
		return soundVerdict{}, err
	}
	twinMain, err := h.twinMain(twinCtx, tf, main.Language)
	if err != nil {
		release()
		return soundVerdict{}, err
	}
	h.mu.Lock()
	tm := muxOf(h, tf)
	h.mu.Unlock()
	far, err := h.startSoundCapture(tm, []int{twinMain.PID}, filepath.Join(dir, "far"))
	if err != nil {
		release()
		return soundVerdict{}, err
	}
	err = sleepCtx(twinCtx, soundListen)
	far.stop()
	release()
	if err != nil {
		if ctx.Err() == nil {
			err = errSoundStopped
		}
		return soundVerdict{}, err
	}
	if err := sleepCtx(ctx, soundLag); err != nil {
		return soundVerdict{}, err
	}
	near.stop()
	return judgeFiles(far.file(twinMain.PID), near.file(main.PID), near.file(lang.PID))
}

// judgeFiles judges the PCM a check wrote.
func judgeFiles(twin, main, lang string) (soundVerdict, error) {
	var envs [3][]float64
	for i, path := range []string{twin, main, lang} {
		pcm, err := os.ReadFile(path)
		if err != nil {
			return soundVerdict{}, err
		}
		env, ok := soundEnvelope(pcm)
		if !ok {
			return soundVerdict{why: "a track was silent"}, nil
		}
		envs[i] = env
	}
	return judgeSound(envs[0], envs[1], envs[2]), nil
}

// twinMain waits for the twin's program map and returns its main track,
// which has to be in lang: that is the language the check looks for.
func (h *Hub) twinMain(ctx context.Context, tf *feed, lang string) (AudioTrack, error) {
	deadline := time.Now().Add(soundTwinWait)
	for {
		h.mu.Lock()
		m, program, tracks := muxOf(h, tf), tf.program, tf.tracks
		h.mu.Unlock()
		if len(tracks) == 0 && m != nil {
			tracks = AudioTracks(m.pictureBytes(), program)
		}
		if t, ok := PickTrack(tracks, "main"); ok && t.Role == "main" {
			if t.Language != lang {
				return AudioTrack{}, fmt.Errorf("the other broadcast's main sound is %q, not %q", t.Language, lang)
			}
			return t, nil
		}
		if time.Now().After(deadline) {
			return AudioTrack{}, errors.New("the other broadcast sent no program map")
		}
		if err := sleepCtx(ctx, 250*time.Millisecond); err != nil {
			return AudioTrack{}, err
		}
	}
}

// holdTwin keeps the twin's tune for the check: the one already up, or a
// tune of its own under the background gates, which a watch or recording
// that needs the tuner stops (its context ends). release lets it go.
func (h *Hub) holdTwin(ctx context.Context, src store.SourceChannel) (context.Context, *feed, func(), error) {
	h.mu.Lock()
	if tf := h.channels[src.ID]; tf != nil && muxOf(h, tf) != nil {
		tf.probes++
		h.mu.Unlock()
		return ctx, tf, func() {
			h.mu.Lock()
			tf.probes--
			h.dropIfUnusedLocked(tf)
			h.mu.Unlock()
		}, nil
	}
	h.mu.Unlock()
	if open, g := h.tunerGateOpen(ctx, src, tunerGate.backgroundOpen); !open {
		return nil, nil, nil, soundGateError{fmt.Sprintf("no tuner for %s: %s", src.GuideNumber, g.why())}
	}
	tctx, cancel := context.WithCancel(ctx)
	state := &sweepState{cancel: cancel}
	h.mu.Lock()
	if h.sweep != nil || h.soundTune != nil {
		h.mu.Unlock()
		cancel()
		return nil, nil, nil, errors.New("another background tune is on")
	}
	h.soundTune = state
	h.mu.Unlock()
	f, err := h.tuneFeed(tctx, src, nil)
	// tuneFeed returns holding h.mu.
	if err != nil || tctx.Err() != nil {
		if err == nil {
			h.dropIfUnusedLocked(f)
			err = errSoundStopped
		}
		if h.soundTune == state {
			h.soundTune = nil
		}
		h.mu.Unlock()
		cancel()
		return nil, nil, nil, err
	}
	f.probing = true
	state.feed = f
	h.mu.Unlock()
	return tctx, f, func() {
		cancel()
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.soundTune == state {
			h.soundTune = nil
		}
		if state.feed == f {
			state.feed = nil
			f.probing = false
			h.dropIfUnusedLocked(f)
		}
	}, nil
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// soundCapture is an ffmpeg decoding some of a tune's sound tracks to PCM.
type soundCapture struct {
	m    *mux
	sub  *pipeSub
	cmd  *exec.Cmd
	dir  string
	done chan struct{}
}

// soundArgs decode each PID of the MPEG-TS on stdin to mono PCM at soundRate,
// one file per PID in dir.
func soundArgs(pids []int, dir string) []string {
	args := []string{"-hide_banner", "-loglevel", "error", "-probesize", "8000000", "-analyzeduration", "3000000",
		"-f", "mpegts", "-i", "pipe:0"}
	for _, pid := range pids {
		args = append(args, "-map", fmt.Sprintf("0:i:%d", pid), "-ac", "1", "-ar", strconv.Itoa(soundRate),
			"-c:a", "pcm_s16le", "-f", "s16le", "-y", soundFile(dir, pid))
	}
	return args
}

func soundFile(dir string, pid int) string {
	return filepath.Join(dir, strconv.Itoa(pid)+".pcm")
}

func (h *Hub) startSoundCapture(m *mux, pids []int, dir string) (*soundCapture, error) {
	if m == nil {
		return nil, errors.New("the tune ended")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	cmd := exec.Command(h.FFmpeg, soundArgs(pids, dir)...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	NotePID(h.Dir, cmd.Process.Pid)
	c := &soundCapture{m: m, cmd: cmd, dir: dir, done: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(c.done)
	}()
	h.mu.Lock()
	c.sub = h.attachPipeLocked(m, stdin)
	h.mu.Unlock()
	if c.sub == nil {
		_ = stdin.Close()
	}
	return c, nil
}

// stop ends the input; ffmpeg writes what it decoded and exits.
func (c *soundCapture) stop() {
	if c.sub != nil {
		c.m.detach(c.sub)
		c.sub = nil
	}
	select {
	case <-c.done:
	case <-time.After(10 * time.Second):
		_ = c.cmd.Process.Kill()
		<-c.done
	}
}

func (c *soundCapture) file(pid int) string { return soundFile(c.dir, pid) }

// settleSound stores a settled choice of a channel's main sound and plays it
// on the tune that is up. A change is noted once in Activity.
func (h *Hub) settleSound(ctx context.Context, id int64, swapped bool, by string, at time.Time, twin store.Channel) {
	before, err := h.Store.Channel(ctx, id)
	if err != nil {
		return
	}
	if err := h.Store.SetChannelSound(ctx, id, swapped, at, by); err != nil {
		slog.Warn(fmt.Sprintf("store the sound of channel %d: %v", id, err))
		return
	}
	h.ApplySound(id, swapped, by, at)
	if before.SoundSwapped == swapped {
		return
	}
	name := before.DisplayNumber + " " + before.DisplayName
	msg := fmt.Sprintf("%s: its two sound tracks traded places, so the main language plays as the main sound. Checked against %s.", name, twin.DisplayNumber)
	h.mu.Lock()
	if f := h.channels[id]; f != nil {
		if m, l, ok := soundPair(f.tracks, f.channel.SoundSwapped); ok {
			main, lang := trackName(m), trackName(l)
			msg = fmt.Sprintf("%s: the track marked %s carries the %s sound, so it plays as %s. Checked against %s.", name, lang, main, main, twin.DisplayNumber)
			if !swapped {
				msg = fmt.Sprintf("%s: the track marked %s carries the %s sound again, so it plays as %s. Checked against %s.", name, main, main, main, twin.DisplayNumber)
			}
		}
	}
	h.mu.Unlock()
	_ = h.Store.AddEvent(ctx, "sound", msg)
}

func trackName(t AudioTrack) string {
	if name := languageName(t.Language); name != "" {
		return name
	}
	return t.Language
}

// ApplySound plays a channel's new sound choice on its tune, if one is up:
// the tracks trade places, encodes whose main changed restart once, and
// recordings in progress note the main they play. A choice that is already
// the tune's changes nothing.
func (h *Hub) ApplySound(id int64, swapped bool, by string, at time.Time) {
	h.mu.Lock()
	f := h.channels[id]
	if f == nil {
		h.mu.Unlock()
		return
	}
	f.channel.SoundCheckedAt, f.channel.SoundCheckedBy = &at, by
	if f.channel.SoundSwapped == swapped {
		h.mu.Unlock()
		return
	}
	f.channel.SoundSwapped = swapped
	f.tracks = swapSounds(f.tracks)
	f.stored = swapSounds(f.stored)
	stored := ""
	if len(f.stored) > 0 {
		stored = encodeTracks(f.stored)
	}
	h.rebuildRenditionsLocked(f)
	h.noteRecordingSoundLocked(f)
	h.mu.Unlock()
	h.changed()
	if stored != "" && h.Store != nil {
		// The swap cleared the stored tracks; these carry the new labels.
		if err := h.Store.SetChannelAudioTracks(context.Background(), id, stored); err != nil {
			slog.Warn(fmt.Sprintf("store audio tracks for channel %d: %v", id, err))
		}
	}
}

// noteRecordingSoundLocked stores, for each recording on f, the track f
// plays as its main sound, so playback plays the same one. A program with
// one sound track needs none.
func (h *Hub) noteRecordingSoundLocked(f *feed) {
	if len(f.tracks) < 2 || h.Store == nil {
		return
	}
	main, ok := PickTrack(f.tracks, "main")
	if !ok || main.PID <= 0 {
		return
	}
	for _, rec := range f.recordings {
		if rec.id == 0 || rec.soundPID == main.PID {
			continue
		}
		rec.soundPID = main.PID
		go h.storeRecordingSound(rec, main.PID, main.Language)
	}
}

// storeRecordingSound stores a recording's main sound unless a later choice
// replaced it first. The lock keeps two quick changes in order.
func (h *Hub) storeRecordingSound(rec *recording, pid int, lang string) {
	h.recSoundMu.Lock()
	defer h.recSoundMu.Unlock()
	h.mu.Lock()
	current := rec.soundPID == pid
	h.mu.Unlock()
	if !current {
		return
	}
	if err := h.Store.SetRecordingSound(context.Background(), rec.id, pid, lang); err != nil {
		slog.Warn(fmt.Sprintf("store the sound of recording %d: %v", rec.id, err))
	}
}
